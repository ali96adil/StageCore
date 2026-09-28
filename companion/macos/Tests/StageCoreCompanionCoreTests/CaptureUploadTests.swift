import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class CaptureUploadTests: XCTestCase {
    func testUploaderUsesScopedSessionAndSeparateHTTPSBody() async throws {
        let ticketID = "11111111-1111-4111-8111-111111111111"
        let sessionID = "22222222-2222-4222-8222-222222222222"
        let contentHash = String(repeating: "a", count: 64)
        let credential = CompanionRuntimeCredential(
            sessionID: sessionID,
            token: "runtime-token",
            expiresAt: Date().addingTimeInterval(300)
        )
        let responseData = try JSONSerialization.data(withJSONObject: [
            "ticket_id": ticketID,
            "status": "COMPLETED",
            "content_hash": contentHash,
            "size_bytes": 123456,
        ])
        let transport = RecordingCaptureUploadTransport(
            response: CompanionCaptureUploadHTTPResponse(
                statusCode: 201,
                body: responseData
            )
        )
        let executor = try CompanionCaptureUploadExecutor(
            apiBaseURL: URL(string: "https://stagecore.local/")!,
            securityPolicy: .production,
            authenticator: FixedCaptureAuthenticator(credential: credential),
            source: FixedCaptureObjectSource(
                url: URL(fileURLWithPath: "/tmp/stagecore-capture-object")
            ),
            transport: transport
        )

        let outcome = await executor.execute(parameters: captureUploadParameters(
            ticketID: ticketID,
            sessionID: sessionID,
            contentHash: contentHash,
            sizeBytes: 123456
        ))

        XCTAssertEqual(outcome.status, .completed)
        XCTAssertEqual(outcome.ackLevel, .accepted)
        XCTAssertNil(outcome.errorCode)
        XCTAssertEqual(outcome.output["ticket_id"], .string(ticketID))
        XCTAssertEqual(outcome.output["content_hash"], .string(contentHash))
        XCTAssertEqual(outcome.output["size_bytes"], .int(123456))

        let request = await transport.lastRequest()
        XCTAssertEqual(request?.httpMethod, "PUT")
        XCTAssertEqual(
            request?.url?.path,
            "/api/v1/companion/capture-uploads/\(ticketID)"
        )
        XCTAssertEqual(
            request?.value(forHTTPHeaderField: "Authorization"),
            "StageCoreSession runtime-token"
        )
        XCTAssertEqual(
            request?.value(forHTTPHeaderField: "X-StageCore-Upload-Credential"),
            "upload-secret"
        )
        XCTAssertEqual(
            request?.value(forHTTPHeaderField: "Content-Length"),
            "123456"
        )
        XCTAssertEqual(await transport.callCount(), 1)
    }

    func testUploaderRejectsRuntimeSessionRotationBeforeHTTP() async throws {
        let transport = RecordingCaptureUploadTransport(
            response: CompanionCaptureUploadHTTPResponse(
                statusCode: 500,
                body: Data()
            )
        )
        let executor = try CompanionCaptureUploadExecutor(
            apiBaseURL: URL(string: "https://stagecore.local/")!,
            securityPolicy: .production,
            authenticator: FixedCaptureAuthenticator(
                credential: CompanionRuntimeCredential(
                    sessionID: "33333333-3333-4333-8333-333333333333",
                    token: "rotated-token",
                    expiresAt: Date().addingTimeInterval(300)
                )
            ),
            source: FixedCaptureObjectSource(
                url: URL(fileURLWithPath: "/tmp/unused-capture-object")
            ),
            transport: transport
        )

        let outcome = await executor.execute(parameters: captureUploadParameters(
            ticketID: "11111111-1111-4111-8111-111111111111",
            sessionID: "22222222-2222-4222-8222-222222222222",
            contentHash: String(repeating: "b", count: 64),
            sizeBytes: 42
        ))

        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.ackLevel, .none)
        XCTAssertEqual(outcome.errorCode, "CAPTURE_UPLOAD_SESSION_MISMATCH")
        XCTAssertEqual(await transport.callCount(), 0)
    }

    func testDirectorySourceRequiresExactHashNamedRegularFileAndSize() async throws {
        let root = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }

        let source = try DirectoryCompanionCaptureObjectSource(rootURL: root)
        let payload = Data("capture-bytes".utf8)
        let contentHash = StageCoreSHA256.hexDigest(payload)
        let objectURL = try source.objectURL(contentHash: contentHash)
        try payload.write(to: objectURL, options: .atomic)

        let resolved = try await source.fileURL(
            contentHash: contentHash,
            sizeBytes: Int64(payload.count)
        )
        XCTAssertEqual(resolved.standardizedFileURL, objectURL.standardizedFileURL)

        do {
            _ = try await source.fileURL(
                contentHash: contentHash,
                sizeBytes: Int64(payload.count + 1)
            )
            XCTFail("expected exact-size rejection")
        } catch let error as CompanionCaptureUploadError {
            XCTAssertEqual(error, .sourceUnavailable)
        }
    }

    private func captureUploadParameters(
        ticketID: String,
        sessionID: String,
        contentHash: String,
        sizeBytes: Int
    ) -> [String: JSONValue] {
        [
            "ticket_id": .string(ticketID),
            "upload_credential": .string("upload-secret"),
            "runtime_session_id": .string(sessionID),
            "purpose": .string("EXECUTION_ENVIRONMENT_CAPTURE"),
            "content_hash": .string(contentHash),
            "size_bytes": .int(sizeBytes),
        ]
    }
}

private struct FixedCaptureAuthenticator: CompanionRuntimeAuthenticator {
    let credential: CompanionRuntimeCredential

    func authenticate() async throws -> CompanionRuntimeCredential {
        credential
    }
}

private struct FixedCaptureObjectSource: CompanionCaptureObjectSource {
    let url: URL

    func fileURL(contentHash: String, sizeBytes: Int64) async throws -> URL {
        url
    }
}

private actor RecordingCaptureUploadTransport: CompanionCaptureUploadTransport {
    private let response: CompanionCaptureUploadHTTPResponse
    private var requests: [URLRequest] = []

    init(response: CompanionCaptureUploadHTTPResponse) {
        self.response = response
    }

    func upload(
        request: URLRequest,
        fromFile fileURL: URL
    ) async throws -> CompanionCaptureUploadHTTPResponse {
        requests.append(request)
        return response
    }

    func lastRequest() -> URLRequest? {
        requests.last
    }

    func callCount() -> Int {
        requests.count
    }
}
