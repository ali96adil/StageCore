import Foundation

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
    public var authenticatedSessionID: String?
    public var authenticatedUntil: Date?
    public var requiredMedia: [RequiredMedia]

    public init(
        companionID: String,
        machineRoleID: String? = nil,
        roleKey: String? = nil,
        appliedRuntimeSnapshotID: String? = nil,
        configHash: String = "",
        capabilities: Set<String> = [],
        readiness: CompanionReadiness = .unknown,
        authenticatedSessionID: String? = nil,
        authenticatedUntil: Date? = nil,
        requiredMedia: [RequiredMedia] = []
    ) {
        self.companionID = companionID
        self.machineRoleID = machineRoleID
        self.roleKey = roleKey
        self.appliedRuntimeSnapshotID = appliedRuntimeSnapshotID
        self.configHash = configHash
        self.capabilities = capabilities
        self.readiness = readiness
        self.authenticatedSessionID = authenticatedSessionID
        self.authenticatedUntil = authenticatedUntil
        self.requiredMedia = requiredMedia
    }

    public mutating func apply(_ ready: SessionReady) {
        machineRoleID = ready.machineRoleID
        roleKey = ready.roleKey
        appliedRuntimeSnapshotID = ready.runtimeSnapshotID
        configHash = ready.configHash
        requiredMedia = ready.requiredMedia
        readiness = ready.requiredMedia.contains(where: \.required) ? .syncing : .ready
    }

    public mutating func applyMediaSyncResult(_ result: MediaSyncResult) {
        switch result {
        case .ready:
            readiness = .ready
        case .mismatch:
            readiness = .mismatch
        case .failed:
            readiness = .blocked
        }
    }

    public mutating func authenticate(sessionID: String, expiresAt: Date) {
        authenticatedSessionID = sessionID
        authenticatedUntil = expiresAt
    }

    public mutating func clearAuthentication() {
        authenticatedSessionID = nil
        authenticatedUntil = nil
        readiness = .offline
    }

    public func isAuthenticated(at date: Date = Date()) -> Bool {
        guard authenticatedSessionID != nil, let authenticatedUntil else { return false }
        return date < authenticatedUntil
    }
}
