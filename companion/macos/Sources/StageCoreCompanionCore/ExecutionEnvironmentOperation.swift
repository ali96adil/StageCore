import Foundation

public enum ExecutionEnvironmentOperationKind: String, Codable, Sendable, Hashable {
    case open = "OPEN"
    case reconnect = "RECONNECT"
    case captureSnapshot = "CAPTURE_SNAPSHOT"
    case restoreObservableState = "RESTORE_OBSERVABLE_STATE"
}

public enum ExecutionEnvironmentProviderStatus: Sendable, Equatable {
    case completed
    case unsupported
    case failed
}

public struct ExecutionEnvironmentCaptureObjectDescriptor: Sendable, Equatable {
    public var purpose: String
    public var contentHash: String
    public var sizeBytes: Int64

    public init(
        purpose: String = "EXECUTION_ENVIRONMENT_CAPTURE",
        contentHash: String,
        sizeBytes: Int64
    ) {
        self.purpose = purpose
        self.contentHash = contentHash
        self.sizeBytes = sizeBytes
    }
}

public struct ExecutionEnvironmentProviderOutcome: Sendable, Equatable {
    public var status: ExecutionEnvironmentProviderStatus
    public var errorCode: String?
    public var responseSummary: String
    public var snapshot: [String: JSONValue]?
    public var captureObject: ExecutionEnvironmentCaptureObjectDescriptor?

    public init(
        status: ExecutionEnvironmentProviderStatus,
        errorCode: String? = nil,
        responseSummary: String,
        snapshot: [String: JSONValue]? = nil,
        captureObject: ExecutionEnvironmentCaptureObjectDescriptor? = nil
    ) {
        self.status = status
        self.errorCode = errorCode
        self.responseSummary = responseSummary
        self.snapshot = snapshot
        self.captureObject = captureObject
    }
}

public protocol ExecutionEnvironmentOperationProvider: Sendable {
    var adapterKey: String { get }
    var supportedOperations: Set<ExecutionEnvironmentOperationKind> { get }
    func perform(
        kind: ExecutionEnvironmentOperationKind,
        manifest: [String: JSONValue],
        sourceManifestSHA256: String,
        snapshot: [String: JSONValue]?
    ) async -> ExecutionEnvironmentProviderOutcome
}

public extension ExecutionEnvironmentOperationProvider {
    func perform(
        kind: ExecutionEnvironmentOperationKind,
        manifest: [String: JSONValue],
        sourceManifestSHA256: String
    ) async -> ExecutionEnvironmentProviderOutcome {
        await perform(
            kind: kind,
            manifest: manifest,
            sourceManifestSHA256: sourceManifestSHA256,
            snapshot: nil
        )
    }
}

public enum ExecutionEnvironmentOperationExecutorError: Error, Equatable {
    case invalidAdapterKey
    case duplicateAdapterKey(String)
}

public struct ExecutionEnvironmentOperationExecutor: CompanionCapabilityExecutor {
    public let capabilityKey = "execution.environment.operation"
    private let router: ExecutionEnvironmentOperationRouter

    public init(providers: [any ExecutionEnvironmentOperationProvider] = []) throws {
        self.router = try ExecutionEnvironmentOperationRouter(providers: providers)
    }

    public func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        await router.execute(parameters: parameters)
    }
}

public actor ExecutionEnvironmentOperationRouter {
    private let providers: [String: any ExecutionEnvironmentOperationProvider]

    public init(providers: [any ExecutionEnvironmentOperationProvider] = []) throws {
        var registry: [String: any ExecutionEnvironmentOperationProvider] = [:]
        for provider in providers {
            let key = provider.adapterKey.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !key.isEmpty else { throw ExecutionEnvironmentOperationExecutorError.invalidAdapterKey }
            guard registry[key] == nil else { throw ExecutionEnvironmentOperationExecutorError.duplicateAdapterKey(key) }
            registry[key] = provider
        }
        self.providers = registry
    }

    public func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        guard case .string(let rawKind) = parameters["operation_kind"],
              let kind = ExecutionEnvironmentOperationKind(rawValue: rawKind),
              case .string(let adapterKey) = parameters["adapter_key"],
              !adapterKey.isEmpty,
              case .string(let sourceManifestSHA256) = parameters["source_manifest_sha256"],
              sourceManifestSHA256.count == 64,
              case .object(let manifest) = parameters["manifest"]
        else {
            return .init(status: .failed, ackLevel: .none, errorCode: "ENVIRONMENT_OPERATION_INVALID", responseSummary: "typed execution-environment operation parameters are invalid")
        }
        let snapshot: [String: JSONValue]?
        switch parameters["snapshot"] {
        case .none:
            snapshot = nil
        case .some(.object(let value)):
            snapshot = value
        default:
            return .init(
                status: .failed,
                ackLevel: .none,
                errorCode: "ENVIRONMENT_OPERATION_INVALID",
                responseSummary: "execution-environment snapshot parameters are invalid"
            )
        }
        if kind == .restoreObservableState && snapshot == nil {
            return .init(
                status: .failed,
                ackLevel: .none,
                errorCode: "ENVIRONMENT_RESTORE_SNAPSHOT_REQUIRED",
                responseSummary: "observable-state restore requires a Hub-selected execution-environment snapshot"
            )
        }
        if kind != .restoreObservableState && snapshot != nil {
            return .init(
                status: .failed,
                ackLevel: .none,
                errorCode: "ENVIRONMENT_OPERATION_INVALID",
                responseSummary: "snapshot input is only valid for observable-state restore"
            )
        }

        guard let provider = providers[adapterKey] else {
            return .init(status: .failed, ackLevel: .none, errorCode: "ENVIRONMENT_ADAPTER_UNSUPPORTED", responseSummary: "no execution-environment operation provider is registered for adapter_key")
        }
        guard provider.supportedOperations.contains(kind) else {
            return .init(status: .failed, ackLevel: .none, errorCode: "ENVIRONMENT_OPERATION_UNSUPPORTED", responseSummary: "adapter does not support the requested execution-environment operation")
        }

        let outcome = await provider.perform(
            kind: kind,
            manifest: manifest,
            sourceManifestSHA256: sourceManifestSHA256,
            snapshot: snapshot
        )
        switch outcome.status {
        case .unsupported:
            return .init(status: .failed, ackLevel: .none, errorCode: outcome.errorCode ?? "ENVIRONMENT_OPERATION_UNSUPPORTED", responseSummary: outcome.responseSummary)
        case .failed:
            return .init(status: .failed, ackLevel: .none, errorCode: outcome.errorCode ?? "ENVIRONMENT_OPERATION_FAILED", responseSummary: outcome.responseSummary)
        case .completed:
            if kind == .captureSnapshot && outcome.snapshot == nil {
                return .init(status: .failed, ackLevel: .none, errorCode: "ENVIRONMENT_SNAPSHOT_RESULT_MISSING", responseSummary: "snapshot provider completed without snapshot metadata")
            }
            if kind != .captureSnapshot && outcome.snapshot != nil {
                return .init(status: .failed, ackLevel: .none, errorCode: "ENVIRONMENT_OPERATION_RESULT_INVALID", responseSummary: "non-capture provider returned an unexpected snapshot payload")
            }
            if kind != .captureSnapshot && outcome.captureObject != nil {
                return .init(status: .failed, ackLevel: .none, errorCode: "ENVIRONMENT_OPERATION_RESULT_INVALID", responseSummary: "capture_object is only valid for CAPTURE_SNAPSHOT")
            }
            if let captureObject = outcome.captureObject {
                let normalizedHash = captureObject.contentHash.lowercased()
                let validHash = normalizedHash.utf8.count == 64 && normalizedHash.utf8.allSatisfy { byte in
                    (48...57).contains(byte) || (97...102).contains(byte)
                }
                guard captureObject.purpose == "EXECUTION_ENVIRONMENT_CAPTURE",
                      validHash,
                      captureObject.sizeBytes >= 0
                else {
                    return .init(
                        status: .failed,
                        ackLevel: .none,
                        errorCode: "ENVIRONMENT_CAPTURE_OBJECT_INVALID",
                        responseSummary: "capture object descriptor is invalid"
                    )
                }
            }
            var output: [String: JSONValue] = [
                "operation_kind": .string(kind.rawValue),
                "adapter_key": .string(adapterKey),
                "source_manifest_sha256": .string(sourceManifestSHA256),
            ]
            if let snapshot = outcome.snapshot { output["snapshot"] = .object(snapshot) }
            if let captureObject = outcome.captureObject {
                output["capture_object"] = .object([
                    "purpose": .string(captureObject.purpose),
                    "content_hash": .string(captureObject.contentHash.lowercased()),
                    "size_bytes": .int(Int(captureObject.sizeBytes)),
                ])
            }
            return .init(status: .completed, ackLevel: .accepted, responseSummary: outcome.responseSummary, output: output)
        }
    }
}
