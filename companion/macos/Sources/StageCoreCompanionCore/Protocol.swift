import Foundation

public enum CompanionMessageType: String, Codable, Sendable {
    case hello = "companion.hello"
    case sessionReady = "session.ready"
    case executionRequest = "execution.request"
    case executionResult = "execution.result"
}

public enum JSONValue: Codable, Sendable, Equatable {
    case string(String)
    case int(Int)
    case double(Double)
    case bool(Bool)
    case object([String: JSONValue])
    case array([JSONValue])
    case null

    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if container.decodeNil() {
            self = .null
        } else if let value = try? container.decode(Bool.self) {
            self = .bool(value)
        } else if let value = try? container.decode(Int.self) {
            self = .int(value)
        } else if let value = try? container.decode(Double.self) {
            self = .double(value)
        } else if let value = try? container.decode(String.self) {
            self = .string(value)
        } else if let value = try? container.decode([String: JSONValue].self) {
            self = .object(value)
        } else if let value = try? container.decode([JSONValue].self) {
            self = .array(value)
        } else {
            throw DecodingError.typeMismatch(
                JSONValue.self,
                .init(codingPath: decoder.codingPath, debugDescription: "unsupported JSON value")
            )
        }
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .string(let value): try container.encode(value)
        case .int(let value): try container.encode(value)
        case .double(let value): try container.encode(value)
        case .bool(let value): try container.encode(value)
        case .object(let value): try container.encode(value)
        case .array(let value): try container.encode(value)
        case .null: try container.encodeNil()
        }
    }
}

public struct CompanionHello: Codable, Sendable, Equatable {
    public let type: CompanionMessageType
    public let schemaVersion: Int
    public let messageID: String
    public let companionID: String
    public let displayName: String
    public let hostname: String
    public let agentVersion: String
    public let platform: String
    public let architecture: String
    public let capabilities: [String]
    public let machineRoleID: String?
    public let roleKey: String?
    public let appliedRuntimeSnapshotID: String?
    public let configHash: String
    public let readiness: String

    enum CodingKeys: String, CodingKey {
        case type
        case schemaVersion = "schema_version"
        case messageID = "message_id"
        case companionID = "companion_id"
        case displayName = "display_name"
        case hostname
        case agentVersion = "agent_version"
        case platform
        case architecture
        case capabilities
        case machineRoleID = "machine_role_id"
        case roleKey = "role_key"
        case appliedRuntimeSnapshotID = "applied_runtime_snapshot_id"
        case configHash = "config_hash"
        case readiness
    }

    public init(
        schemaVersion: Int = 1,
        messageID: String = UUID().uuidString.lowercased(),
        companionID: String,
        displayName: String = "",
        hostname: String = "",
        agentVersion: String,
        platform: String,
        architecture: String,
        capabilities: [String],
        machineRoleID: String? = nil,
        roleKey: String? = nil,
        appliedRuntimeSnapshotID: String?,
        configHash: String,
        readiness: String
    ) {
        self.type = .hello
        self.schemaVersion = schemaVersion
        self.messageID = messageID
        self.companionID = companionID
        self.displayName = displayName
        self.hostname = hostname
        self.agentVersion = agentVersion
        self.platform = platform
        self.architecture = architecture
        self.capabilities = capabilities
        self.machineRoleID = machineRoleID
        self.roleKey = roleKey
        self.appliedRuntimeSnapshotID = appliedRuntimeSnapshotID
        self.configHash = configHash
        self.readiness = readiness
    }
}

public struct RequiredMedia: Codable, Sendable, Equatable, Hashable {
    public let mediaAssetID: String
    public let contentVersionID: String
    public let checksumAlgorithm: String
    public let contentHash: String
    public let sizeBytes: Int64
    public let required: Bool

    enum CodingKeys: String, CodingKey {
        case mediaAssetID = "media_asset_id"
        case contentVersionID = "content_version_id"
        case checksumAlgorithm = "checksum_algorithm"
        case contentHash = "content_hash"
        case sizeBytes = "size_bytes"
        case required
    }

    public init(
        mediaAssetID: String,
        contentVersionID: String,
        checksumAlgorithm: String = "SHA256",
        contentHash: String,
        sizeBytes: Int64,
        required: Bool = true
    ) {
        self.mediaAssetID = mediaAssetID
        self.contentVersionID = contentVersionID
        self.checksumAlgorithm = checksumAlgorithm
        self.contentHash = contentHash
        self.sizeBytes = sizeBytes
        self.required = required
    }
}

public struct SessionReady: Codable, Sendable, Equatable {
    public let type: CompanionMessageType
    public let schemaVersion: Int
    public let messageID: String
    public let machineRoleID: String
    public let roleKey: String
    public let runtimeSnapshotID: String
    public let configHash: String
    public let requiredMedia: [RequiredMedia]

    enum CodingKeys: String, CodingKey {
        case type
        case schemaVersion = "schema_version"
        case messageID = "message_id"
        case machineRoleID = "machine_role_id"
        case roleKey = "role_key"
        case runtimeSnapshotID = "runtime_snapshot_id"
        case configHash = "config_hash"
        case requiredMedia = "required_media"
    }

    public init(
        schemaVersion: Int = 1,
        messageID: String = UUID().uuidString.lowercased(),
        machineRoleID: String,
        roleKey: String,
        runtimeSnapshotID: String,
        configHash: String,
        requiredMedia: [RequiredMedia] = []
    ) {
        self.type = .sessionReady
        self.schemaVersion = schemaVersion
        self.messageID = messageID
        self.machineRoleID = machineRoleID
        self.roleKey = roleKey
        self.runtimeSnapshotID = runtimeSnapshotID
        self.configHash = configHash
        self.requiredMedia = requiredMedia
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        type = try container.decode(CompanionMessageType.self, forKey: .type)
        schemaVersion = try container.decode(Int.self, forKey: .schemaVersion)
        messageID = try container.decode(String.self, forKey: .messageID)
        machineRoleID = try container.decode(String.self, forKey: .machineRoleID)
        roleKey = try container.decode(String.self, forKey: .roleKey)
        runtimeSnapshotID = try container.decode(String.self, forKey: .runtimeSnapshotID)
        configHash = try container.decode(String.self, forKey: .configHash)
        requiredMedia = try container.decodeIfPresent([RequiredMedia].self, forKey: .requiredMedia) ?? []
    }
}

public struct CompanionExecutionRequest: Codable, Sendable, Equatable {
    public let type: CompanionMessageType
    public let schemaVersion: Int
    public let messageID: String
    public let executionID: String
    public let correlationID: String?
    public let machineRoleID: String
    public let runtimeSnapshotID: String
    public let capability: String
    public let parameters: [String: JSONValue]
    public let timeoutMS: Int64

    enum CodingKeys: String, CodingKey {
        case type
        case schemaVersion = "schema_version"
        case messageID = "message_id"
        case executionID = "execution_id"
        case correlationID = "correlation_id"
        case machineRoleID = "machine_role_id"
        case runtimeSnapshotID = "runtime_snapshot_id"
        case capability
        case parameters
        case timeoutMS = "timeout_ms"
    }

    public init(
        schemaVersion: Int = 1,
        messageID: String = UUID().uuidString.lowercased(),
        executionID: String,
        correlationID: String?,
        machineRoleID: String,
        runtimeSnapshotID: String,
        capability: String,
        parameters: [String: JSONValue],
        timeoutMS: Int64
    ) {
        self.type = .executionRequest
        self.schemaVersion = schemaVersion
        self.messageID = messageID
        self.executionID = executionID
        self.correlationID = correlationID
        self.machineRoleID = machineRoleID
        self.runtimeSnapshotID = runtimeSnapshotID
        self.capability = capability
        self.parameters = parameters
        self.timeoutMS = timeoutMS
    }
}

public enum CompanionExecutionStatus: String, Codable, Sendable {
    case completed = "COMPLETED"
    case failed = "FAILED"
    case timedOut = "TIMED_OUT"
    case cancelled = "CANCELLED"
    case rejected = "REJECTED"
}

public enum CompanionAckLevel: String, Codable, Sendable {
    case none = "NONE"
    case transportOnly = "TRANSPORT_ONLY"
    case accepted = "ACCEPTED"
    case deviceAck = "DEVICE_ACK"
    case verifiedState = "VERIFIED_STATE"
}

public struct CompanionExecutionResult: Codable, Sendable, Equatable {
    public let type: CompanionMessageType
    public let schemaVersion: Int
    public let messageID: String
    public let executionID: String
    public let status: CompanionExecutionStatus
    public let ackLevel: CompanionAckLevel
    public let errorCode: String?
    public let responseSummary: String
    public let output: [String: JSONValue]

    enum CodingKeys: String, CodingKey {
        case type
        case schemaVersion = "schema_version"
        case messageID = "message_id"
        case executionID = "execution_id"
        case status
        case ackLevel = "ack_level"
        case errorCode = "error_code"
        case responseSummary = "response_summary"
        case output
    }

    public init(
        schemaVersion: Int = 1,
        messageID: String = UUID().uuidString.lowercased(),
        executionID: String,
        status: CompanionExecutionStatus,
        ackLevel: CompanionAckLevel,
        errorCode: String?,
        responseSummary: String,
        output: [String: JSONValue] = [:]
    ) {
        self.type = .executionResult
        self.schemaVersion = schemaVersion
        self.messageID = messageID
        self.executionID = executionID
        self.status = status
        self.ackLevel = ackLevel
        self.errorCode = errorCode
        self.responseSummary = responseSummary
        self.output = output
    }
}
