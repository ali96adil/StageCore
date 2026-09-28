import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public protocol CompanionCaptureObjectSource: Sendable {
    func fileURL(contentHash: String, sizeBytes: Int64) async throws -> URL
}

public enum CompanionCaptureUploadError: Error, Equatable {
    case insecureTransport
    case invalidSourceRoot
    case invalidContentHash
    case sourceUnavailable
}

public struct DirectoryCompanionCaptureObjectSource: CompanionCaptureObjectSource, Sendable {
    public let rootURL: URL

    public init(rootURL: URL) throws {
        guard rootURL.isFileURL else {
            throw CompanionCaptureUploadError.invalidSourceRoot
        }
        let standardized = rootURL.standardizedFileURL
        do {
            try FileManager.default.createDirectory(
                at: standardized,
                withIntermediateDirectories: true
            )
        } catch {
            throw CompanionCaptureUploadError.invalidSourceRoot
        }
        self.rootURL = standardized.resolvingSymlinksInPath().standardizedFileURL
    }

    public func objectURL(contentHash: String) throws -> URL {
        guard let normalized = normalizedCaptureSHA256(contentHash) else {
            throw CompanionCaptureUploadError.invalidContentHash
        }
        return rootURL.appendingPathComponent(normalized, isDirectory: false).standardizedFileURL
    }

    public func fileURL(contentHash: String, sizeBytes: Int64) async throws -> URL {
        guard sizeBytes >= 0 else {
            throw CompanionCaptureUploadError.sourceUnavailable
        }
        let candidate = try objectURL(contentHash: contentHash)
        let resolved = candidate.resolvingSymlinksInPath().standardizedFileURL
        guard resolved.path == candidate.path else {
            throw CompanionCaptureUploadError.sourceUnavailable
        }
        do {
            let values = try candidate.resourceValues(forKeys: [
                .isRegularFileKey,
                .fileSizeKey,
            ])
            guard values.isRegularFile == true,
                  let fileSize = values.fileSize,
                  Int64(fileSize) == sizeBytes
            else {
                throw CompanionCaptureUploadError.sourceUnavailable
            }
        } catch let error as CompanionCaptureUploadError {
            throw error
        } catch {
            throw CompanionCaptureUploadError.sourceUnavailable
        }
        return candidate
    }
}

struct CompanionCaptureUploadHTTPResponse: Sendable {
    let statusCode: Int
    let body: Data
}

protocol CompanionCaptureUploadTransport: Sendable {
    func upload(
        request: URLRequest,
        fromFile fileURL: URL
    ) async throws -> CompanionCaptureUploadHTTPResponse
}

struct URLSessionCompanionCaptureUploadTransport: CompanionCaptureUploadTransport {
    let session: URLSession

    func upload(
        request: URLRequest,
        fromFile fileURL: URL
    ) async throws -> CompanionCaptureUploadHTTPResponse {
        let (data, response) = try await session.upload(for: request, fromFile: fileURL)
        guard let http = response as? HTTPURLResponse else {
            throw URLError(.badServerResponse)
        }
        return CompanionCaptureUploadHTTPResponse(statusCode: http.statusCode, body: data)
    }
}

public struct CompanionCaptureUploadExecutor: CompanionCapabilityExecutor {
    public static let capability = "execution.environment.capture.upload"
    public let capabilityKey = CompanionCaptureUploadExecutor.capability

    private static let expectedPurpose = "EXECUTION_ENVIRONMENT_CAPTURE"
    private static let uploadCredentialHeader = "X-StageCore-Upload-Credential"

    private let apiBaseURL: URL
    private let authenticator: any CompanionRuntimeAuthenticator
    private let source: any CompanionCaptureObjectSource
    private let transport: any CompanionCaptureUploadTransport
    private let decoder = JSONDecoder()

    public init(
        apiBaseURL: URL,
        securityPolicy: CompanionTransportSecurityPolicy = .production,
        authenticator: any CompanionRuntimeAuthenticator,
        source: any CompanionCaptureObjectSource,
        session: URLSession = URLSession(configuration: .ephemeral)
    ) throws {
        try self.init(
            apiBaseURL: apiBaseURL,
            securityPolicy: securityPolicy,
            authenticator: authenticator,
            source: source,
            transport: URLSessionCompanionCaptureUploadTransport(session: session)
        )
    }

    init(
        apiBaseURL: URL,
        securityPolicy: CompanionTransportSecurityPolicy,
        authenticator: any CompanionRuntimeAuthenticator,
        source: any CompanionCaptureObjectSource,
        transport: any CompanionCaptureUploadTransport
    ) throws {
        try Self.validateAPIURL(apiBaseURL, policy: securityPolicy)
        self.apiBaseURL = apiBaseURL
        self.authenticator = authenticator
        self.source = source
        self.transport = transport
    }

    public func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        guard case .string(let ticketID)? = parameters["ticket_id"],
              UUID(uuidString: ticketID) != nil,
              case .string(let uploadCredential)? = parameters["upload_credential"],
              !uploadCredential.isEmpty,
              uploadCredential.utf8.count <= 1024,
              case .string(let runtimeSessionID)? = parameters["runtime_session_id"],
              UUID(uuidString: runtimeSessionID) != nil,
              case .string(let purpose)? = parameters["purpose"],
              purpose == Self.expectedPurpose,
              case .string(let rawContentHash)? = parameters["content_hash"],
              let contentHash = normalizedCaptureSHA256(rawContentHash),
              case .int(let rawSizeBytes)? = parameters["size_bytes"],
              rawSizeBytes >= 0
        else {
            return failure(
                code: "CAPTURE_UPLOAD_COMMAND_INVALID",
                summary: "scoped capture-upload command parameters are invalid"
            )
        }
        let sizeBytes = Int64(rawSizeBytes)

        let credential: CompanionRuntimeCredential
        do {
            credential = try await authenticator.authenticate()
        } catch {
            return failure(
                code: "CAPTURE_UPLOAD_SESSION_UNAVAILABLE",
                summary: "authenticated runtime session is unavailable for capture upload"
            )
        }
        guard credential.sessionID == runtimeSessionID else {
            return failure(
                code: "CAPTURE_UPLOAD_SESSION_MISMATCH",
                summary: "capture-upload ticket belongs to a different runtime session"
            )
        }

        let fileURL: URL
        do {
            fileURL = try await source.fileURL(
                contentHash: contentHash,
                sizeBytes: sizeBytes
            )
        } catch {
            return failure(
                code: "CAPTURE_UPLOAD_SOURCE_UNAVAILABLE",
                summary: "declared capture object is not available from the local content-addressed source"
            )
        }

        let uploadURL = apiBaseURL
            .appendingPathComponent("api", isDirectory: true)
            .appendingPathComponent("v1", isDirectory: true)
            .appendingPathComponent("companion", isDirectory: true)
            .appendingPathComponent("capture-uploads", isDirectory: true)
            .appendingPathComponent(ticketID, isDirectory: false)
        var request = URLRequest(url: uploadURL)
        request.httpMethod = "PUT"
        request.timeoutInterval = 120
        request.setValue(
            "StageCoreSession \(credential.token)",
            forHTTPHeaderField: "Authorization"
        )
        request.setValue(
            uploadCredential,
            forHTTPHeaderField: Self.uploadCredentialHeader
        )
        request.setValue(
            String(sizeBytes),
            forHTTPHeaderField: "Content-Length"
        )
        request.setValue(
            "application/octet-stream",
            forHTTPHeaderField: "Content-Type"
        )

        let response: CompanionCaptureUploadHTTPResponse
        do {
            response = try await transport.upload(request: request, fromFile: fileURL)
        } catch is CancellationError {
            return failure(
                code: "CAPTURE_UPLOAD_CANCELLED",
                summary: "capture upload was cancelled before verified completion"
            )
        } catch {
            return failure(
                code: "CAPTURE_UPLOAD_TRANSPORT_FAILED",
                summary: "capture upload transport failed before verified completion"
            )
        }

        guard response.statusCode == 201 else {
            let failureResponse = try? decoder.decode(
                CaptureUploadFailureResponse.self,
                from: response.body
            )
            return failure(
                code: failureResponse?.errorCode ?? "CAPTURE_UPLOAD_HTTP_REJECTED",
                summary: "Hub rejected the scoped capture upload"
            )
        }

        guard let receipt = try? decoder.decode(
            CaptureUploadSuccessResponse.self,
            from: response.body
        ),
        receipt.ticketID == ticketID,
        receipt.status == "COMPLETED",
        receipt.contentHash.lowercased() == contentHash,
        receipt.sizeBytes == sizeBytes
        else {
            return failure(
                code: "CAPTURE_UPLOAD_RESPONSE_INVALID",
                summary: "Hub capture-upload receipt did not match the scoped command"
            )
        }

        return CompanionCapabilityOutcome(
            status: .completed,
            ackLevel: .accepted,
            responseSummary: "capture object uploaded and verified by Hub Vault",
            output: [
                "ticket_id": .string(ticketID),
                "status": .string(receipt.status),
                "content_hash": .string(contentHash),
                "size_bytes": .int(rawSizeBytes),
            ]
        )
    }

    private func failure(code: String, summary: String) -> CompanionCapabilityOutcome {
        CompanionCapabilityOutcome(
            status: .failed,
            ackLevel: .none,
            errorCode: code,
            responseSummary: summary
        )
    }

    private static func validateAPIURL(
        _ url: URL,
        policy: CompanionTransportSecurityPolicy
    ) throws {
        guard url.user == nil,
              url.password == nil,
              url.query == nil,
              url.fragment == nil,
              let scheme = url.scheme?.lowercased()
        else {
            throw CompanionCaptureUploadError.insecureTransport
        }
        if scheme == "https" {
            return
        }
        guard scheme == "http",
              case .allowInsecureLoopbackForTesting = policy,
              let host = url.host?.lowercased(),
              ["localhost", "127.0.0.1", "::1"].contains(host)
        else {
            throw CompanionCaptureUploadError.insecureTransport
        }
    }
}

private struct CaptureUploadSuccessResponse: Decodable {
    let ticketID: String
    let status: String
    let contentHash: String
    let sizeBytes: Int64

    enum CodingKeys: String, CodingKey {
        case ticketID = "ticket_id"
        case status
        case contentHash = "content_hash"
        case sizeBytes = "size_bytes"
    }
}

private struct CaptureUploadFailureResponse: Decodable {
    let errorCode: String

    enum CodingKeys: String, CodingKey {
        case errorCode = "error_code"
    }
}

private func normalizedCaptureSHA256(_ value: String) -> String? {
    let normalized = value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    guard normalized.utf8.count == 64,
          normalized.utf8.allSatisfy({ byte in
              (48...57).contains(byte) || (97...102).contains(byte)
          })
    else {
        return nil
    }
    return normalized
}
