import XCTest
@testable import StageCoreCompanionCore

final class IdentityAndExecutionGuardTests: XCTestCase {
    func testGeneratedCompanionIDMatchesHubUUIDPersistenceShape() {
        let id = CompanionIdentity.generateID()
        XCTAssertEqual(id.count, 36)
        XCTAssertTrue(CompanionIdentity.isCanonicalID(id))
        XCTAssertFalse(id.hasPrefix("cmp_"))
    }

    func testGuardRejectsDuplicateSnapshotRoleAndCapabilityMismatch() {
        var state = CompanionRuntimeState(
            companionID: "11111111-1111-4111-8111-111111111111",
            machineRoleID: "role-video-main",
            roleKey: "VIDEO-MAIN",
            appliedRuntimeSnapshotID: "snap-1",
            capabilities: ["osc.send"],
            readiness: .ready
        )
        var guardState = ExecutionGuard()

        let valid = request(id: "exec-1", role: "role-video-main", snapshot: "snap-1", capability: "osc.send")
        XCTAssertEqual(guardState.decision(for: valid, state: state), .execute)
        guardState.markTerminal(valid.executionID)
        XCTAssertEqual(guardState.decision(for: valid, state: state), .rejectDuplicate)

        XCTAssertEqual(
            guardState.decision(for: request(id: "exec-2", role: "role-video-main", snapshot: "snap-old", capability: "osc.send"), state: state),
            .rejectSnapshotMismatch
        )
        XCTAssertEqual(
            guardState.decision(for: request(id: "exec-3", role: "role-audio", snapshot: "snap-1", capability: "osc.send"), state: state),
            .rejectRoleMismatch
        )
        XCTAssertEqual(
            guardState.decision(for: request(id: "exec-4", role: "role-video-main", snapshot: "snap-1", capability: "midi.send"), state: state),
            .rejectUnsupportedCapability
        )

        state.capabilities.insert("midi.send")
        XCTAssertEqual(
            guardState.decision(for: request(id: "exec-4", role: "role-video-main", snapshot: "snap-1", capability: "midi.send"), state: state),
            .execute
        )
    }

    func testGuardEvictsOldestTerminalIDAtBoundedCapacity() {
        let state = CompanionRuntimeState(
            companionID: "11111111-1111-4111-8111-111111111111",
            machineRoleID: "role-video-main",
            appliedRuntimeSnapshotID: "snap-1",
            capabilities: ["osc.send"],
            readiness: .ready
        )
        var guardState = ExecutionGuard(capacity: 2)
        guardState.markTerminal("exec-1")
        guardState.markTerminal("exec-2")
        guardState.markTerminal("exec-3")

        XCTAssertFalse(guardState.remembers("exec-1"))
        XCTAssertTrue(guardState.remembers("exec-2"))
        XCTAssertTrue(guardState.remembers("exec-3"))
        XCTAssertEqual(
            guardState.decision(for: request(id: "exec-1", role: "role-video-main", snapshot: "snap-1", capability: "osc.send"), state: state),
            .execute
        )
    }

    private func request(id: String, role: String, snapshot: String, capability: String) -> CompanionExecutionRequest {
        CompanionExecutionRequest(
            executionID: id,
            correlationID: nil,
            machineRoleID: role,
            runtimeSnapshotID: snapshot,
            capability: capability,
            parameters: [:],
            timeoutMS: 500
        )
    }
}
