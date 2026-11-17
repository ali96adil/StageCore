import Foundation

public struct SignedApplicationDispatch: Codable, Sendable, Equatable {
    public struct Scope: Codable, Sendable, Equatable {
        public let projectID: String
        public let runtimeSnapshotID: String
        public let machineRoleID: String
        public let companionID: String
        public let receiverInstanceID: String
        public let kind: String
        public let connectionGeneration: Int64

        enum CodingKeys: String, CodingKey {
            case projectID = "project_id"
            case runtimeSnapshotID = "runtime_snapshot_id"
            case machineRoleID = "machine_role_id"
            case companionID = "companion_id"
            case receiverInstanceID = "receiver_instance_id"
            case kind
            case connectionGeneration = "connection_generation"
        }
    }

    public let scope: Scope
    public let executionID: String
    public let cueID: String
    public let challenge: String
    public let expiresAt: String

    enum CodingKeys: String, CodingKey {
        case scope
        case executionID = "execution_id"
        case cueID = "cue_id"
        case challenge
        case expiresAt = "expires_at"
    }

    func matches(_ request: CompanionExecutionRequest, companionID: String) -> Bool {
        let identifiers = [scope.projectID, scope.receiverInstanceID, cueID, executionID]
        return executionID == request.executionID && scope.companionID == companionID &&
            scope.machineRoleID == request.machineRoleID && scope.runtimeSnapshotID == request.runtimeSnapshotID &&
            scope.connectionGeneration > 0 && ["ABLETON", "VDMX"].contains(scope.kind) &&
            ["osc.send", "midi.send"].contains(request.capability) &&
            identifiers.allSatisfy { !$0.isEmpty && $0.utf8.count <= 128 && $0.rangeOfCharacter(from: .controlCharacters) == nil } &&
            challenge.utf8.count == 64 && challenge.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) }
    }
}

// The bridge owns dispatch of a signed Action. The standard Companion must
// never also dispatch the OSC/MIDI Action, including on failure or timeout.
public protocol SignedApplicationBridge: Sendable {
    func execute(_ request: CompanionExecutionRequest) async throws -> [Data]
}

public enum SignedApplicationBridgeError: Error {
    case invalidConfiguration
    case invalidResponse
}

#if os(macOS)
public struct LoopbackSignedApplicationBridge: SignedApplicationBridge {
    private let endpoint: URL
    private let token: String

    public init(endpoint: URL, token: String) throws {
        guard let parts = URLComponents(url: endpoint, resolvingAgainstBaseURL: false),
              parts.scheme == "http", parts.host == "127.0.0.1",
              let port = parts.port, (1024...65535).contains(port),
              parts.percentEncodedPath == "/stagecore/execute",
              parts.user == nil, parts.password == nil, parts.query == nil, parts.fragment == nil,
              (32...256).contains(token.utf8.count),
              token.utf8.allSatisfy({ (48...57).contains($0) || (65...90).contains($0) ||
                  (97...122).contains($0) || $0 == 45 || $0 == 95 }) else {
            throw SignedApplicationBridgeError.invalidConfiguration
        }
        self.endpoint = endpoint
        self.token = token
    }

    public func execute(_ request: CompanionExecutionRequest) async throws -> [Data] {
        let body = try JSONEncoder().encode(request)
        guard body.count <= 48 * 1024 else { throw SignedApplicationBridgeError.invalidResponse }
        let timeout = request.timeoutMS > 0 ? min(12, Double(request.timeoutMS) / 1000) : 12
        let config = URLSessionConfiguration.ephemeral
        config.connectionProxyDictionary = [:]
        config.httpShouldSetCookies = false
        config.timeoutIntervalForRequest = timeout
        config.timeoutIntervalForResource = timeout
        let session = URLSession(configuration: config, delegate: RefuseBridgeRedirect(), delegateQueue: nil)
        defer { session.invalidateAndCancel() }
        var outgoing = URLRequest(url: endpoint, timeoutInterval: timeout)
        outgoing.httpMethod = "POST"
        outgoing.httpBody = body
        outgoing.setValue("application/json", forHTTPHeaderField: "Content-Type")
        outgoing.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        let (bytes, response) = try await session.bytes(for: outgoing)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200,
              http.url == endpoint, response.expectedContentLength <= 12 * 1024 else {
            throw SignedApplicationBridgeError.invalidResponse
        }
        var data = Data()
        for try await byte in bytes {
            guard data.count < 12 * 1024 else { throw SignedApplicationBridgeError.invalidResponse }
            data.append(byte)
        }
        struct Reply: Decodable {
            let execution_id: String
            let correlation_id: String?
            let signed_application_receipts: [Data]
        }
        let reply = try JSONDecoder().decode(Reply.self, from: data)
        guard reply.execution_id == request.executionID, reply.correlation_id == request.correlationID,
              reply.signed_application_receipts.count == 2,
              reply.signed_application_receipts.allSatisfy({ (1...4096).contains($0.count) }) else {
            throw SignedApplicationBridgeError.invalidResponse
        }
        return reply.signed_application_receipts
    }
}

private final class RefuseBridgeRedirect: NSObject, URLSessionTaskDelegate, @unchecked Sendable {
    func urlSession(_ session: URLSession, task: URLSessionTask,
                    willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest,
                    completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(nil)
    }
}
#endif
