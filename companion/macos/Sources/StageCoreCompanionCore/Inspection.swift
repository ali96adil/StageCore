import Foundation

public enum CompanionInspectionStatus: String, Codable, Sendable {
    case completed = "COMPLETED"
    case unsupported = "UNSUPPORTED"
    case failed = "FAILED"
}

public struct CompanionInspectionApplicationObservation: Codable, Sendable, Equatable {
    public var present: Bool
    public var observedVersion: String
    public var versionConstraintSatisfied: Bool?

    enum CodingKeys: String, CodingKey {
        case present
        case observedVersion = "observed_version"
        case versionConstraintSatisfied = "version_constraint_satisfied"
    }

    public init(
        present: Bool,
        observedVersion: String = "",
        versionConstraintSatisfied: Bool? = nil
    ) {
        self.present = present
        self.observedVersion = observedVersion
        self.versionConstraintSatisfied = versionConstraintSatisfied
    }
}

public struct CompanionInspectionAssetObservation: Codable, Sendable, Equatable {
    public var key: String
    public var present: Bool
    public var inspectable: Bool
    public var contentHash: String
    public var sizeBytes: Int64?

    enum CodingKeys: String, CodingKey {
        case key
        case present
        case inspectable
        case contentHash = "content_hash"
        case sizeBytes = "size_bytes"
    }

    public init(
        key: String,
        present: Bool,
        inspectable: Bool,
        contentHash: String = "",
        sizeBytes: Int64? = nil
    ) {
        self.key = key
        self.present = present
        self.inspectable = inspectable
        self.contentHash = contentHash
        self.sizeBytes = sizeBytes
    }
}

public struct CompanionInspectionExtensionObservation: Codable, Sendable, Equatable {
    public var key: String
    public var present: Bool
    public var observedVersion: String
    public var versionConstraintSatisfied: Bool?

    enum CodingKeys: String, CodingKey {
        case key
        case present
        case observedVersion = "observed_version"
        case versionConstraintSatisfied = "version_constraint_satisfied"
    }

    public init(
        key: String,
        present: Bool,
        observedVersion: String = "",
        versionConstraintSatisfied: Bool? = nil
    ) {
        self.key = key
        self.present = present
        self.observedVersion = observedVersion
        self.versionConstraintSatisfied = versionConstraintSatisfied
    }
}

public struct CompanionInspectionBindingObservation: Codable, Sendable, Equatable {
    public var key: String
    public var present: Bool

    public init(key: String, present: Bool) {
        self.key = key
        self.present = present
    }
}

public struct CompanionInspectionObservation: Codable, Sendable, Equatable {
    public var os: String
    public var architecture: String
    public var application: CompanionInspectionApplicationObservation
    public var assets: [CompanionInspectionAssetObservation]
    public var extensions: [CompanionInspectionExtensionObservation]
    public var bindings: [CompanionInspectionBindingObservation]

    enum CodingKeys: String, CodingKey {
        case os
        case architecture
        case application
        case assets
        case extensions = "external_extensions"
        case bindings
    }

    public init(
        os: String,
        architecture: String,
        application: CompanionInspectionApplicationObservation,
        assets: [CompanionInspectionAssetObservation] = [],
        extensions: [CompanionInspectionExtensionObservation] = [],
        bindings: [CompanionInspectionBindingObservation] = []
    ) {
        self.os = os
        self.architecture = architecture
        self.application = application
        self.assets = assets
        self.extensions = extensions
        self.bindings = bindings
    }
}

public struct CompanionInspectionRequest: Codable, Sendable, Equatable {
    public let type: String
    public let schemaVersion: Int
    public let messageID: String
    public let inspectionID: String
    public let adapterKey: String
    public let manifest: [String: JSONValue]

    enum CodingKeys: String, CodingKey {
        case type
        case schemaVersion = "schema_version"
        case messageID = "message_id"
        case inspectionID = "inspection_id"
        case adapterKey = "adapter_key"
        case manifest
    }

    public init(
        schemaVersion: Int = 1,
        messageID: String = UUID().uuidString.lowercased(),
        inspectionID: String,
        adapterKey: String,
        manifest: [String: JSONValue]
    ) {
        self.type = "inspection.request"
        self.schemaVersion = schemaVersion
        self.messageID = messageID
        self.inspectionID = inspectionID
        self.adapterKey = adapterKey
        self.manifest = manifest
    }
}

public struct CompanionInspectionResult: Codable, Sendable, Equatable {
    public let type: String
    public let schemaVersion: Int
    public let messageID: String
    public let inspectionID: String
    public let adapterKey: String
    public let status: CompanionInspectionStatus
    public let errorCode: String?
    public let responseSummary: String
    public let observation: CompanionInspectionObservation?

    enum CodingKeys: String, CodingKey {
        case type
        case schemaVersion = "schema_version"
        case messageID = "message_id"
        case inspectionID = "inspection_id"
        case adapterKey = "adapter_key"
        case status
        case errorCode = "error_code"
        case responseSummary = "response_summary"
        case observation
    }

    public init(
        schemaVersion: Int = 1,
        messageID: String = UUID().uuidString.lowercased(),
        inspectionID: String,
        adapterKey: String,
        status: CompanionInspectionStatus,
        errorCode: String? = nil,
        responseSummary: String,
        observation: CompanionInspectionObservation? = nil
    ) {
        self.type = "inspection.result"
        self.schemaVersion = schemaVersion
        self.messageID = messageID
        self.inspectionID = inspectionID
        self.adapterKey = adapterKey
        self.status = status
        self.errorCode = errorCode
        self.responseSummary = responseSummary
        self.observation = observation
    }
}

public struct CompanionInspectionOutcome: Sendable, Equatable {
    public var status: CompanionInspectionStatus
    public var errorCode: String?
    public var responseSummary: String
    public var observation: CompanionInspectionObservation?

    public init(
        status: CompanionInspectionStatus,
        errorCode: String? = nil,
        responseSummary: String,
        observation: CompanionInspectionObservation? = nil
    ) {
        self.status = status
        self.errorCode = errorCode
        self.responseSummary = responseSummary
        self.observation = observation
    }
}

/// A read-only, adapter-specific inspector. The only input supplied by StageCore
/// is the declared, bounded execution-environment manifest. Implementations must
/// not launch applications, mutate files, install extensions, or broaden the
/// request into an ambient workstation scan.
public protocol CompanionInspectionProvider: Sendable {
    var adapterKey: String { get }
    func inspect(manifest: [String: JSONValue]) async -> CompanionInspectionOutcome
}

public enum CompanionInspectionRouterError: Error, Equatable {
    case invalidAdapterKey
    case duplicateAdapterKey(String)
    case unsupportedSchemaVersion(Int)
}

public actor CompanionInspectionRouter {
    private let providers: [String: any CompanionInspectionProvider]
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    public init(providers: [any CompanionInspectionProvider] = []) throws {
        var registry: [String: any CompanionInspectionProvider] = [:]
        for provider in providers {
            let key = provider.adapterKey.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !key.isEmpty else {
                throw CompanionInspectionRouterError.invalidAdapterKey
            }
            guard registry[key] == nil else {
                throw CompanionInspectionRouterError.duplicateAdapterKey(key)
            }
            registry[key] = provider
        }
        self.providers = registry
    }

    /// Returns nil for non-inspection messages so the existing CompanionSession
    /// can handle session.ready / execution.request without sharing execution
    /// authority with the inspection registry.
    public func handleIfInspection(_ data: Data, authenticated: Bool) async throws -> Data? {
        struct Header: Decodable {
            let type: String
            let schemaVersion: Int

            enum CodingKeys: String, CodingKey {
                case type
                case schemaVersion = "schema_version"
            }
        }

        let header = try decoder.decode(Header.self, from: data)
        guard header.type == "inspection.request" else {
            return nil
        }
        guard header.schemaVersion == 1 else {
            throw CompanionInspectionRouterError.unsupportedSchemaVersion(header.schemaVersion)
        }

        let request = try decoder.decode(CompanionInspectionRequest.self, from: data)
        guard authenticated else {
            return try encoder.encode(
                CompanionInspectionResult(
                    inspectionID: request.inspectionID,
                    adapterKey: request.adapterKey,
                    status: .failed,
                    errorCode: "SESSION_UNAUTHENTICATED",
                    responseSummary: "authenticated runtime session is required"
                )
            )
        }
        guard let provider = providers[request.adapterKey] else {
            return try encoder.encode(
                CompanionInspectionResult(
                    inspectionID: request.inspectionID,
                    adapterKey: request.adapterKey,
                    status: .unsupported,
                    errorCode: "INSPECTION_ADAPTER_UNSUPPORTED",
                    responseSummary: "no read-only inspection provider is registered for adapter_key"
                )
            )
        }

        let outcome = await provider.inspect(manifest: request.manifest)
        return try encoder.encode(
            CompanionInspectionResult(
                inspectionID: request.inspectionID,
                adapterKey: request.adapterKey,
                status: outcome.status,
                errorCode: outcome.errorCode,
                responseSummary: outcome.responseSummary,
                observation: outcome.observation
            )
        )
    }
}
