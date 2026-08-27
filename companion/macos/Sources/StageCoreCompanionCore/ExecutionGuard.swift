public enum ExecutionDecision: Equatable, Sendable {
    case execute
    case rejectDuplicate
    case rejectSnapshotMismatch
    case rejectUnsupportedCapability
    case rejectRoleMismatch
}

public struct ExecutionGuard: Sendable {
    private var terminalIDs: Set<String> = []
    private var terminalOrder: [String] = []
    private let capacity: Int

    public init(capacity: Int = 512) {
        self.capacity = max(1, capacity)
    }

    public mutating func decision(
        for request: CompanionExecutionRequest,
        state: CompanionRuntimeState
    ) -> ExecutionDecision {
        if terminalIDs.contains(request.executionID) {
            return .rejectDuplicate
        }
        guard request.runtimeSnapshotID == state.appliedRuntimeSnapshotID else {
            return .rejectSnapshotMismatch
        }
        guard request.machineRoleID == state.machineRoleID else {
            return .rejectRoleMismatch
        }
        guard state.capabilities.contains(request.capability) else {
            return .rejectUnsupportedCapability
        }
        return .execute
    }

    public mutating func markTerminal(_ executionID: String) {
        guard !terminalIDs.contains(executionID) else {
            return
        }
        terminalIDs.insert(executionID)
        terminalOrder.append(executionID)
        while terminalOrder.count > capacity {
            let evicted = terminalOrder.removeFirst()
            terminalIDs.remove(evicted)
        }
    }

    public func remembers(_ executionID: String) -> Bool {
        terminalIDs.contains(executionID)
    }
}
