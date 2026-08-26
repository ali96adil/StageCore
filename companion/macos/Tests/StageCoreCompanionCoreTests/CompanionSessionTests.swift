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

        let ready = SessionReady(
            machineRoleID: "role-video-main",
            roleKey: "VIDEO-MAIN",
            runtimeSnapshotID: "snap-1",
            configHash: "cfg-1"
        )
        let readyResponse = try await session.handle(JSONEncoder().encode(ready))
        XCTAssertNil(readyResponse)

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

        var fresh = stale
        fresh = CompanionExecutionRequest(
            executionID: "exec-fresh",
            correlationID: nil,
            machineRoleID: fresh.machineRoleID,
            runtimeSnapshotID: fresh.runtimeSnapshotID,
            capability: fresh.capability,
            parameters: fresh.parameters,
            timeoutMS: fresh.timeoutMS
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
        let ready = SessionReady(
            machineRoleID: role,
            roleKey: "VIDEO-MAIN",
            runtimeSnapshotID: snapshot,
            configHash: ""
        )
        _ = try await session.handle(JSONEncoder().encode(ready))
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
