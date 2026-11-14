import Foundation
import XCTest
@testable import StageCoreCompanionCore

private actor FakeSignedBridge: SignedApplicationBridge {
    private(set) var calls = 0

    func execute(_ request: CompanionExecutionRequest) async throws -> [Data] {
        calls += 1
        return [Data([1]), Data([2])]
    }
}

private struct MustNotExecuteOSC: CompanionCapabilityExecutor {
    let capabilityKey = "osc.send"

    func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        XCTFail("A signed execution reached the ordinary OSC executor")
        return CompanionCapabilityOutcome(status: .failed, ackLevel: .none, responseSummary: "unexpected dispatch")
    }
}

final class SignedApplicationBridgeTests: XCTestCase {
    func testSignedCueUsesOnlyBridgeAndRejectsReplayAndMismatchedSnapshot() async throws {
        let bridge = FakeSignedBridge()
        let session = CompanionSession(
            configuration: .init(companionID: "companion", agentVersion: "test", platform: "macos", architecture: "arm64"),
            executors: [MustNotExecuteOSC()], signedApplicationBridge: bridge
        )
        let hello = try JSONDecoder().decode(CompanionHello.self, from: await session.helloData())
        XCTAssertTrue(hello.capabilities.contains("receiver.signed_ack.v1"))
        await session.establishAuthenticatedSession(.init(sessionID: "session", token: "token", expiresAt: Date().addingTimeInterval(60)))
        let ready = SessionReady(machineRoleID: "role", roleKey: "VIDEO-MAIN", runtimeSnapshotID: "snap", configHash: "")
        _ = try await session.handle(JSONEncoder().encode(ready))

        func run(_ id: String, scopeSnapshot: String) async throws -> CompanionExecutionResult {
            let challenge = String(repeating: "a", count: 64)
            let wire = """
            {"type":"execution.request","schema_version":1,"message_id":"message","execution_id":"\(id)",
            "correlation_id":"correlation","machine_role_id":"role","runtime_snapshot_id":"snap","capability":"osc.send",
            "parameters":{},"timeout_ms":1000,"application_ack":{"scope":{"project_id":"project",
            "runtime_snapshot_id":"\(scopeSnapshot)","machine_role_id":"role","companion_id":"companion",
            "receiver_instance_id":"receiver","kind":"VDMX","connection_generation":1},
            "execution_id":"\(id)","cue_id":"cue","challenge":"\(challenge)","expires_at":"2030-01-01T00:00:00Z"}}
            """
            let reply = try await session.handle(Data(wire.utf8))
            return try JSONDecoder().decode(CompanionExecutionResult.self, from: XCTUnwrap(reply))
        }
        let completed = try await run("first", scopeSnapshot: "snap")
        XCTAssertEqual(completed.ackLevel, .transportOnly)
        XCTAssertEqual(completed.signedApplicationReceipts, [Data([1]), Data([2])])
        let duplicate = try await run("first", scopeSnapshot: "snap")
        XCTAssertEqual(duplicate.errorCode, "DUPLICATE_EXECUTION")
        let wrongScope = try await run("second", scopeSnapshot: "wrong")
        XCTAssertEqual(wrongScope.errorCode, "APPLICATION_ACK_UNAVAILABLE")
        let calls = await bridge.calls
        XCTAssertEqual(calls, 1)
    }

    #if os(macOS)
    func testBridgeEndpointRequiresLiteralLoopbackAndPrivateToken() throws {
        for invalid in ["http://localhost:9000/stagecore/execute", "http://127.0.0.1:9000/stagecore/execute?redirect=1",
                        "http://user@127.0.0.1:9000/stagecore/execute"] {
            XCTAssertThrowsError(try LoopbackSignedApplicationBridge(endpoint: XCTUnwrap(URL(string: invalid)),
                token: String(repeating: "a", count: 32)))
        }
        XCTAssertNoThrow(try LoopbackSignedApplicationBridge(endpoint: XCTUnwrap(URL(string: "http://127.0.0.1:9011/stagecore/execute")),
            token: String(repeating: "b", count: 32)))
    }
    #endif
}
