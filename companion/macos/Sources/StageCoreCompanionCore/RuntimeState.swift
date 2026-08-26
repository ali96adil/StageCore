public enum CompanionReadiness: String, Codable, Sendable {
    case unknown = "UNKNOWN"
    case syncing = "SYNCING"
    case ready = "READY"
    case degraded = "DEGRADED"
    case offline = "OFFLINE"
    case mismatch = "MISMATCH"
    case blocked = "BLOCKED"
}

public struct CompanionRuntimeState: Sendable, Equatable {
    public var companionID: String
    public var machineRoleID: String?
    public var roleKey: String?
    public var appliedRuntimeSnapshotID: String?
    public var configHash: String
    public var capabilities: Set<String>
    public var readiness: CompanionReadiness

    public init(
        companionID: String,
        machineRoleID: String? = nil,
        roleKey: String? = nil,
        appliedRuntimeSnapshotID: String? = nil,
        configHash: String = "",
        capabilities: Set<String> = [],
        readiness: CompanionReadiness = .unknown
    ) {
        self.companionID = companionID
        self.machineRoleID = machineRoleID
        self.roleKey = roleKey
        self.appliedRuntimeSnapshotID = appliedRuntimeSnapshotID
        self.configHash = configHash
        self.capabilities = capabilities
        self.readiness = readiness
    }

    public mutating func apply(_ ready: SessionReady) {
        machineRoleID = ready.machineRoleID
        roleKey = ready.roleKey
        appliedRuntimeSnapshotID = ready.runtimeSnapshotID
        configHash = ready.configHash
        readiness = .ready
    }
}
