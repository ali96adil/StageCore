import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif
#if canImport(Security)
import Security
#endif

public struct CompanionReportIdentity: Sendable, Equatable {
    public var displayName: String
    public var hostname: String
    public var platform: String
    public var architecture: String
    public var version: String
    public var capabilities: [String]

    public init(
        displayName: String,
        hostname: String,
        platform: String,
        architecture: String,
        version: String,
        capabilities: [String]
    ) {
        self.displayName = displayName
        self.hostname = hostname
        self.platform = platform
        self.architecture = architecture
        self.version = version
        self.capabilities = capabilities
    }
}

public struct CompanionPairingReceipt: Sendable, Equatable {
    public let requestID: String
    public let pairingCode: String
    public let expiresAt: Date
}

public enum CompanionPairingState: String, Sendable, Equatable {
    case pending = "PENDING"
    case approved = "APPROVED"
    case rejected = "REJECTED"
    case expired = "EXPIRED"
}

public struct CompanionRuntimeCredential: Sendable, Equatable {
    public let sessionID: String
    public let token: String
    public let expiresAt: Date
}

public protocol CompanionRuntimeAuthenticator: Sendable {
    func authenticate() async throws -> CompanionRuntimeCredential
}

public enum HubSecurityClientError: Error, Equatable {
    case secureTransportRequired
    case invalidResponse
    case hubRejected(String)
}

public actor HubSecurityClient: CompanionRuntimeAuthenticator {
    private let apiBaseURL: URL
    private let securityPolicy: CompanionTransportSecurityPolicy
    private let identityStore: any SecureDeviceIdentityStore
    private let report: CompanionReportIdentity
    private let session: URLSession
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()
    private var cachedCredential: CompanionRuntimeCredential?

    public init(
        apiBaseURL: URL,
        securityPolicy: CompanionTransportSecurityPolicy = .production,
        identityStore: any SecureDeviceIdentityStore,
        report: CompanionReportIdentity,
        session: URLSession = URLSession(configuration: .ephemeral)
    ) throws {
        try Self.validateAPIURL(apiBaseURL, policy: securityPolicy)
        self.apiBaseURL = apiBaseURL
        self.securityPolicy = securityPolicy
        self.identityStore = identityStore
        self.report = report
        self.session = session
    }

    public func requestPairing() async throws -> CompanionPairingReceipt {
        try Self.validateAPIURL(apiBaseURL, policy: securityPolicy)
        let identity = try identityStore.loadOrCreateIdentity()
        var nonce = Data(count: 32)
        let status = nonce.withUnsafeMutableBytes { (bytes: UnsafeMutableRawBufferPointer) in
            SecRandomCopyBytesCompat(bytes: bytes)
        }
        guard status else { throw HubSecurityClientError.invalidResponse }
        let response: PairingRequestResponse = try await post(
            path: "api/v1/companion/pairing/requests",
            body: PairingRequestBody(
                companionID: identity.companionID,
                displayName: report.displayName,
                hostname: report.hostname,
                platform: report.platform,
                architecture: report.architecture,
                version: report.version,
                capabilities: report.capabilities,
                publicKeyAlgorithm: identity.publicKeyAlgorithm,
                publicKeyBase64: identity.publicKeyBase64,
                clientNonceBase64: nonce.base64EncodedString()
            )
        )
        return CompanionPairingReceipt(
            requestID: response.requestID,
            pairingCode: response.pairingCode,
            expiresAt: try parseDate(response.expiresAt)
        )
    }

    public func pairingStatus(receipt: CompanionPairingReceipt) async throws -> CompanionPairingState {
        let response: PairingStatusResponse = try await post(
            path: "api/v1/companion/pairing/status",
            body: PairingStatusBody(requestID: receipt.requestID, pairingCode: receipt.pairingCode)
        )
        guard let state = CompanionPairingState(rawValue: response.status) else {
            throw HubSecurityClientError.invalidResponse
        }
        return state
    }

    public func authenticate() async throws -> CompanionRuntimeCredential {
        try Self.validateAPIURL(apiBaseURL, policy: securityPolicy)
        if let cachedCredential, cachedCredential.expiresAt > Date().addingTimeInterval(5) {
            return cachedCredential
        }
        let identity = try identityStore.loadOrCreateIdentity()
        let challenge: AuthChallengeResponse = try await post(
            path: "api/v1/companion/auth/challenges",
            body: AuthChallengeBody(companionID: identity.companionID)
        )
        let message = Self.authenticationMessage(
            companionID: identity.companionID,
            challengeID: challenge.challengeID,
            nonceBase64: challenge.nonceBase64
        )
        let signature = try identityStore.signAuthenticationChallenge(message)
        let response: AuthSessionResponse = try await post(
            path: "api/v1/companion/auth/sessions",
            body: AuthSessionBody(
                companionID: identity.companionID,
                challengeID: challenge.challengeID,
                signatureBase64: signature.base64EncodedString()
            )
        )
        let credential = CompanionRuntimeCredential(
            sessionID: response.sessionID,
            token: response.sessionToken,
            expiresAt: try parseDate(response.expiresAt)
        )
        cachedCredential = credential
        return credential
    }

    public static func authenticationMessage(
        companionID: String,
        challengeID: String,
        nonceBase64: String
    ) -> Data {
        Data("StageCore Companion Authentication v1\n\(companionID)\n\(challengeID)\n\(nonceBase64)".utf8)
    }

    private func post<Body: Encodable, Response: Decodable>(path: String, body: Body) async throws -> Response {
        let url = apiBaseURL.appendingPathComponent(path)
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try encoder.encode(body)
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw HubSecurityClientError.invalidResponse
        }
        guard (200..<300).contains(http.statusCode) else {
            let failure = try? decoder.decode(ErrorResponse.self, from: data)
            throw HubSecurityClientError.hubRejected(failure?.errorCode ?? "HUB_REQUEST_REJECTED")
        }
        return try decoder.decode(Response.self, from: data)
    }

    private static func validateAPIURL(_ url: URL, policy: CompanionTransportSecurityPolicy) throws {
        guard let scheme = url.scheme?.lowercased() else {
            throw HubSecurityClientError.secureTransportRequired
        }
        if scheme == "https" { return }
        guard scheme == "http", case .allowInsecureLoopbackForTesting = policy,
              let host = url.host?.lowercased(), ["localhost", "127.0.0.1", "::1"].contains(host) else {
            throw HubSecurityClientError.secureTransportRequired
        }
    }

    private func parseDate(_ value: String) throws -> Date {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = formatter.date(from: value) { return date }
        formatter.formatOptions = [.withInternetDateTime]
        guard let date = formatter.date(from: value) else {
            throw HubSecurityClientError.invalidResponse
        }
        return date
    }
}

private struct PairingRequestBody: Encodable {
    let companionID: String
    let displayName: String
    let hostname: String
    let platform: String
    let architecture: String
    let version: String
    let capabilities: [String]
    let publicKeyAlgorithm: String
    let publicKeyBase64: String
    let clientNonceBase64: String

    enum CodingKeys: String, CodingKey {
        case companionID = "companion_id"
        case displayName = "display_name"
        case hostname, platform, architecture, version, capabilities
        case publicKeyAlgorithm = "public_key_algorithm"
        case publicKeyBase64 = "public_key_base64"
        case clientNonceBase64 = "client_nonce_base64"
    }
}

private struct PairingRequestResponse: Decodable {
    let requestID: String
    let pairingCode: String
    let expiresAt: String

    enum CodingKeys: String, CodingKey {
        case requestID = "request_id"
        case pairingCode = "pairing_code"
        case expiresAt = "expires_at"
    }
}

private struct PairingStatusBody: Encodable {
    let requestID: String
    let pairingCode: String

    enum CodingKeys: String, CodingKey {
        case requestID = "request_id"
        case pairingCode = "pairing_code"
    }
}

private struct PairingStatusResponse: Decodable { let status: String }

private struct AuthChallengeBody: Encodable {
    let companionID: String
    enum CodingKeys: String, CodingKey { case companionID = "companion_id" }
}

private struct AuthChallengeResponse: Decodable {
    let challengeID: String
    let nonceBase64: String
    let expiresAt: String

    enum CodingKeys: String, CodingKey {
        case challengeID = "challenge_id"
        case nonceBase64 = "nonce_base64"
        case expiresAt = "expires_at"
    }
}

private struct AuthSessionBody: Encodable {
    let companionID: String
    let challengeID: String
    let signatureBase64: String

    enum CodingKeys: String, CodingKey {
        case companionID = "companion_id"
        case challengeID = "challenge_id"
        case signatureBase64 = "signature_base64"
    }
}

private struct AuthSessionResponse: Decodable {
    let sessionID: String
    let sessionToken: String
    let expiresAt: String

    enum CodingKeys: String, CodingKey {
        case sessionID = "session_id"
        case sessionToken = "session_token"
        case expiresAt = "expires_at"
    }
}

private struct ErrorResponse: Decodable {
    let errorCode: String
    enum CodingKeys: String, CodingKey { case errorCode = "error_code" }
}

private func SecRandomCopyBytesCompat(bytes: UnsafeMutableRawBufferPointer) -> Bool {
    #if canImport(Security)
    return SecRandomCopyBytes(kSecRandomDefault, bytes.count, bytes.baseAddress!) == errSecSuccess
    #else
    guard let baseAddress = bytes.baseAddress else { return false }
    let buffer = baseAddress.assumingMemoryBound(to: UInt8.self)
    for index in 0..<bytes.count {
        buffer[index] = UInt8.random(in: .min ... .max)
    }
    return true
    #endif
}
