import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class CompanionSessionTests: XCTestCase {
    func testSessionHelloReadyExecuteAndDuplicateGuard() async throws {
        let companionID = "11111111-1111-4111-8111-111111111111"
        let session = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: companionID,
                agentVersion: "0.1.0",
                platform: "macos",
                architecture: "arm64"
            ),
            executors: [LocalEchoExecutor()]
        )

        let hello = try JSONDecoder().decode(CompanionHello.self, from: await session.helloData())
        XCTAssertEqual(hello.companionID, companionID)
        XCTAssertEqual(hello.capabilities, ["local.echo"])
        XCTAssertNil(hello.appliedRuntimeSnapshotID)

        await authenticate(session)

        let ready = SessionReady(
            machineRoleID: "role-video-main",
            roleKey: "VIDEO-MAIN",
            runtimeSnapshotID: "snap-1",
            configHash: "cfg-1"
        )
        let readyResponse = try await session.handle(JSONEncoder().encode(ready))
        let appliedHello = try JSONDecoder().decode(
            CompanionHello.self,
            from: try XCTUnwrap(readyResponse)
        )
        XCTAssertEqual(appliedHello.machineRoleID, "role-video-main")
        XCTAssertEqual(appliedHello.appliedRuntimeSnapshotID, "snap-1")
        XCTAssertEqual(appliedHello.readiness, "READY")

        let request = CompanionExecutionRequest(
            executionID: "exec-1",
            correlationID: "corr-1",
            machineRoleID: "role-video-main",
            runtimeSnapshotID: "snap-1",
            capability: "local.echo",
            parameters: ["message": .string("GO")],
            timeoutMS: 250
        )
        let firstResponse = try await session.handle(JSONEncoder().encode(request))
        let firstData = try XCTUnwrap(firstResponse)
        let first = try JSONDecoder().decode(CompanionExecutionResult.self, from: firstData)
        XCTAssertEqual(first.status, .completed)
        XCTAssertEqual(first.ackLevel, .accepted)
        XCTAssertEqual(first.output["echo"], .string("GO"))

        let duplicateResponse = try await session.handle(JSONEncoder().encode(request))
        let duplicateData = try XCTUnwrap(duplicateResponse)
        let duplicate = try JSONDecoder().decode(CompanionExecutionResult.self, from: duplicateData)
        XCTAssertEqual(duplicate.status, .rejected)
        XCTAssertEqual(duplicate.errorCode, "DUPLICATE_EXECUTION")
    }

    func testRequiredMediaMustVerifyBeforeReady() async throws {
        let media = RequiredMedia(
            mediaAssetID: "asset-1",
            contentVersionID: "content-1",
            contentHash: String(repeating: "a", count: 64),
            sizeBytes: 1024
        )
        let session = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: "11111111-1111-4111-8111-111111111111",
                agentVersion: "0.1.0",
                platform: "macos",
                architecture: "arm64"
            ),
            executors: [LocalEchoExecutor()],
            mediaSynchronizer: FixedMediaSynchronizer(result: .ready)
        )
        await authenticate(session)
        let ready = SessionReady(
            machineRoleID: "role-video-main",
            roleKey: "VIDEO-MAIN",
            runtimeSnapshotID: "snap-media",
            configHash: "cfg",
            requiredMedia: [media]
        )
        let response = try await session.handle(JSONEncoder().encode(ready))
        let hello = try JSONDecoder().decode(CompanionHello.self, from: try XCTUnwrap(response))
        XCTAssertEqual(hello.readiness, "READY")
        let runtimeState = await session.runtimeState()
        XCTAssertEqual(runtimeState.requiredMedia, [media])
    }

    func testMissingMediaSynchronizerBlocksReadyAndExecution() async throws {
        let media = RequiredMedia(
            mediaAssetID: "asset-1",
            contentVersionID: "content-1",
            contentHash: String(repeating: "b", count: 64),
            sizeBytes: 2048
        )
        let session = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: "11111111-1111-4111-8111-111111111111",
                agentVersion: "0.1.0",
                platform: "macos",
                architecture: "arm64"
            ),
            executors: [LocalEchoExecutor()]
        )
        await authenticate(session)
        let ready = SessionReady(
            machineRoleID: "role-video-main",
            roleKey: "VIDEO-MAIN",
            runtimeSnapshotID: "snap-media",
            configHash: "cfg",
            requiredMedia: [media]
        )
        let response = try await session.handle(JSONEncoder().encode(ready))
        let hello = try JSONDecoder().decode(CompanionHello.self, from: try XCTUnwrap(response))
        XCTAssertEqual(hello.readiness, "BLOCKED")

        let request = CompanionExecutionRequest(
            executionID: "exec-blocked",
            correlationID: nil,
            machineRoleID: "role-video-main",
            runtimeSnapshotID: "snap-media",
            capability: "local.echo",
            parameters: [:],
            timeoutMS: 100
        )
        let blocked = try await result(session, request: request)
        XCTAssertEqual(blocked.status, .rejected)
        XCTAssertEqual(blocked.errorCode, "COMPANION_NOT_READY")
    }

    func testMediaChecksumMismatchReportsMismatch() async throws {
        let media = RequiredMedia(
            mediaAssetID: "asset-1",
            contentVersionID: "content-1",
            contentHash: String(repeating: "c", count: 64),
            sizeBytes: 12
        )
        let session = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: "11111111-1111-4111-8111-111111111111",
                agentVersion: "0.1.0",
                platform: "macos",
                architecture: "arm64"
            ),
            executors: [LocalEchoExecutor()],
            mediaSynchronizer: FixedMediaSynchronizer(result: .mismatch(media.contentHash))
        )
        await authenticate(session)
        let ready = SessionReady(
            machineRoleID: "role-video-main",
            roleKey: "VIDEO-MAIN",
            runtimeSnapshotID: "snap-media",
            configHash: "cfg",
            requiredMedia: [media]
        )
        let response = try await session.handle(JSONEncoder().encode(ready))
        let hello = try JSONDecoder().decode(CompanionHello.self, from: try XCTUnwrap(response))
        XCTAssertEqual(hello.readiness, "MISMATCH")
    }

    func testRejectedStaleExecutionIDCannotBecomeExecutableAfterSync() async throws {
        let session = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: "11111111-1111-4111-8111-111111111111",
                agentVersion: "0.1.0",
                platform: "macos",
                architecture: "arm64"
            ),
            executors: [LocalEchoExecutor()]
        )
        try await applyReady(session, snapshot: "snap-1")

        let stale = CompanionExecutionRequest(
            executionID: "exec-stale",
            correlationID: nil,
            machineRoleID: "role-video-main",
            runtimeSnapshotID: "snap-2",
            capability: "local.echo",
            parameters: [:],
            timeoutMS: 100
        )
        let staleResult = try await result(session, request: stale)
        XCTAssertEqual(staleResult.status, .rejected)
        XCTAssertEqual(staleResult.errorCode, "SNAPSHOT_MISMATCH")

        try await applyReady(session, snapshot: "snap-2")
        let repeated = try await result(session, request: stale)
        XCTAssertEqual(repeated.status, .rejected)
        XCTAssertEqual(repeated.errorCode, "DUPLICATE_EXECUTION")

        let fresh = CompanionExecutionRequest(
            executionID: "exec-fresh",
            correlationID: nil,
            machineRoleID: stale.machineRoleID,
            runtimeSnapshotID: stale.runtimeSnapshotID,
            capability: stale.capability,
            parameters: stale.parameters,
            timeoutMS: stale.timeoutMS
        )
        let freshResult = try await result(session, request: fresh)
        XCTAssertEqual(freshResult.status, .completed)
    }

    func testSessionProducesExplicitTimeout() async throws {
        let session = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: "11111111-1111-4111-8111-111111111111",
                agentVersion: "0.1.0",
                platform: "macos",
                architecture: "arm64"
            ),
            executors: [SlowExecutor()]
        )
        try await applyReady(session, snapshot: "snap-1", role: "role-video-main")
        let request = CompanionExecutionRequest(
            executionID: "exec-timeout",
            correlationID: nil,
            machineRoleID: "role-video-main",
            runtimeSnapshotID: "snap-1",
            capability: "local.slow",
            parameters: [:],
            timeoutMS: 5
        )
        let timedOut = try await result(session, request: request)
        XCTAssertEqual(timedOut.status, .timedOut)
        XCTAssertEqual(timedOut.ackLevel, .none)
        XCTAssertEqual(timedOut.errorCode, "COMPANION_EXECUTION_TIMEOUT")

        let duplicate = try await result(session, request: request)
        XCTAssertEqual(duplicate.errorCode, "DUPLICATE_EXECUTION")
    }

    private func applyReady(
        _ session: CompanionSession,
        snapshot: String,
        role: String = "role-video-main"
    ) async throws {
        await authenticate(session)
        let ready = SessionReady(
            machineRoleID: role,
            roleKey: "VIDEO-MAIN",
            runtimeSnapshotID: snapshot,
            configHash: ""
        )
        _ = try await session.handle(JSONEncoder().encode(ready))
    }

    private func authenticate(_ session: CompanionSession) async {
        await session.establishAuthenticatedSession(
            CompanionRuntimeCredential(
                sessionID: "session-test",
                token: "not-persisted",
                expiresAt: Date().addingTimeInterval(300)
            )
        )
    }

    private func result(
        _ session: CompanionSession,
        request: CompanionExecutionRequest
    ) async throws -> CompanionExecutionResult {
        let response = try await session.handle(JSONEncoder().encode(request))
        let data = try XCTUnwrap(response)
        return try JSONDecoder().decode(CompanionExecutionResult.self, from: data)
    }
}

private struct FixedMediaSynchronizer: CompanionMediaSynchronizer {
    let result: MediaSyncResult

    func synchronize(requiredMedia: [RequiredMedia], sessionToken: String) async -> MediaSyncResult {
        result
    }
}

private struct SlowExecutor: CompanionCapabilityExecutor {
    let capabilityKey = "local.slow"

    func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        do {
            try await Task.sleep(for: .seconds(1))
        } catch {
            return CompanionCapabilityOutcome(
                status: .cancelled,
                ackLevel: .none,
                errorCode: "LOCAL_EXECUTION_CANCELLED",
                responseSummary: "cancelled"
            )
        }
        return CompanionCapabilityOutcome(
            status: .completed,
            ackLevel: .accepted,
            responseSummary: "slow execution completed"
        )
    }
}
