import Foundation
import Testing
@testable import StageCoreCompanionCore

@Test func identityPersistsAcrossLoads() throws {
    let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    defer { try? FileManager.default.removeItem(at: root) }
    let store = FileIdentityStore(url: root.appendingPathComponent("id"))
    let first = try store.loadOrCreate()
    let second = try store.loadOrCreate()
    #expect(first == second)
    #expect(first.hasPrefix("cmp_"))
}

@Test func executionGuardRejectsDuplicateAndSnapshotMismatch() {
    var guardState = ExecutionGuard()
    #expect(guardState.decision(executionID: "exec-1", requestSnapshotID: "snap-1", appliedSnapshotID: "snap-1", capability: "local.echo") == .execute)
    guardState.markCompleted("exec-1")
    #expect(guardState.decision(executionID: "exec-1", requestSnapshotID: "snap-1", appliedSnapshotID: "snap-1", capability: "local.echo") == .rejectDuplicate)
    #expect(guardState.decision(executionID: "exec-2", requestSnapshotID: "snap-old", appliedSnapshotID: "snap-1", capability: "local.echo") == .rejectSnapshotMismatch)
}
