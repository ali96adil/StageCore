import Foundation

public enum LiveSourceCapability {
    public static let contractVersion = 1
    public static let open = "video.source.open"
    public static let close = "video.source.close"
    public static let select = "video.source.select"
    public static let route = "video.source.route"
    public static let inspect = "video.source.inspect"

    public static let all = [open, close, select, route, inspect]
}

public enum LiveSourceClass: String, Sendable, Equatable, CaseIterable {
    case localCamera = "LOCAL_CAMERA"
    case usbCapture = "USB_CAPTURE"
    case networkStream = "NETWORK_STREAM"
}

public struct LiveSourceRuntimeDescriptor: Sendable, Equatable {
    public var sourceID: String
    public var sourceClass: LiveSourceClass
    public var endpointRef: String
    public var config: [String: JSONValue]

    public init(
        sourceID: String,
        sourceClass: LiveSourceClass,
        endpointRef: String = "",
        config: [String: JSONValue] = [:]
    ) {
        self.sourceID = sourceID
        self.sourceClass = sourceClass
        self.endpointRef = endpointRef
        self.config = config
    }
}

public struct LiveSourceRouteState: Sendable, Equatable {
    public var sourceID: String
    public var layerID: String
    public var outputID: String

    public init(sourceID: String, layerID: String, outputID: String = "main") {
        self.sourceID = sourceID
        self.layerID = layerID
        self.outputID = outputID
    }
}

public struct LiveSourceRuntimeState: Sendable, Equatable {
    public var descriptor: LiveSourceRuntimeDescriptor
    public var open: Bool

    public init(descriptor: LiveSourceRuntimeDescriptor, open: Bool) {
        self.descriptor = descriptor
        self.open = open
    }
}

public struct LiveSourceEngineSnapshot: Sendable, Equatable {
    public var contractVersion: Int
    public var selectedSourceID: String?
    public var sources: [LiveSourceRuntimeState]
    public var routes: [LiveSourceRouteState]

    public init(
        contractVersion: Int,
        selectedSourceID: String?,
        sources: [LiveSourceRuntimeState],
        routes: [LiveSourceRouteState]
    ) {
        self.contractVersion = contractVersion
        self.selectedSourceID = selectedSourceID
        self.sources = sources
        self.routes = routes
    }
}

/// Deterministic F-007 control state for a render Companion. This actor carries
/// descriptors and routing intent only; it never transports video frames.
/// Native capture/render adapters are attached in a later Slice D runtime layer.
public actor LiveSourceEngine {
    private var sources: [String: LiveSourceRuntimeState] = [:]
    private var selectedSourceID: String?
    private var routes: [String: LiveSourceRouteState] = [:]

    public init() {}

    public func snapshot() -> LiveSourceEngineSnapshot {
        LiveSourceEngineSnapshot(
            contractVersion: LiveSourceCapability.contractVersion,
            selectedSourceID: selectedSourceID,
            sources: sources.values.sorted { $0.descriptor.sourceID < $1.descriptor.sourceID },
            routes: routes.values.sorted {
                if $0.outputID != $1.outputID { return $0.outputID < $1.outputID }
                return $0.layerID < $1.layerID
            }
        )
    }

    public func execute(
        capability: String,
        parameters: [String: JSONValue]
    ) -> CompanionCapabilityOutcome {
        guard LiveSourceCapability.all.contains(capability) else {
            return failure("LIVE_SOURCE_CAPABILITY_UNSUPPORTED", "unsupported live-source capability")
        }
        guard liveInt(parameters["contract_version"]) == LiveSourceCapability.contractVersion else {
            return failure("LIVE_SOURCE_CONTRACT_UNSUPPORTED", "contract_version must be 1")
        }
        guard let sourceID = liveIdentifier(parameters["source_id"], maxLength: 128) else {
            return failure("LIVE_SOURCE_INVALID", "source_id is required")
        }

        switch capability {
        case LiveSourceCapability.open:
            guard let rawClass = liveString(parameters["source_class"], maxLength: 32),
                  let sourceClass = LiveSourceClass(rawValue: rawClass) else {
                return failure("LIVE_SOURCE_CLASS_UNSUPPORTED", "source_class is unsupported")
            }
            let endpointRef = liveOptionalString(parameters["endpoint_ref"], maxLength: 512) ?? ""
            let config: [String: JSONValue]
            if let value = parameters["config"] {
                guard case .object(let object) = value else {
                    return failure("LIVE_SOURCE_INVALID", "config must be an object")
                }
                config = object
            } else {
                config = [:]
            }
            let allowed: Set<String> = ["contract_version", "source_id", "source_class", "endpoint_ref", "config"]
            guard Set(parameters.keys).isSubset(of: allowed) else {
                return failure("LIVE_SOURCE_INVALID", "open contains unknown fields")
            }
            let descriptor = LiveSourceRuntimeDescriptor(
                sourceID: sourceID,
                sourceClass: sourceClass,
                endpointRef: endpointRef,
                config: config
            )
            sources[sourceID] = LiveSourceRuntimeState(descriptor: descriptor, open: true)
            return success("live source opened", output: sourceOutput(sources[sourceID]!))

        case LiveSourceCapability.close:
            guard liveOnly(parameters, allowed: ["contract_version", "source_id"]) else {
                return failure("LIVE_SOURCE_INVALID", "close contains unknown fields")
            }
            guard var state = sources[sourceID], state.open else {
                return failure("LIVE_SOURCE_NOT_OPEN", "live source is not open")
            }
            state.open = false
            sources[sourceID] = state
            if selectedSourceID == sourceID { selectedSourceID = nil }
            routes = routes.filter { $0.value.sourceID != sourceID }
            return success("live source closed", output: sourceOutput(state))

        case LiveSourceCapability.select:
            guard liveOnly(parameters, allowed: ["contract_version", "source_id"]) else {
                return failure("LIVE_SOURCE_INVALID", "select contains unknown fields")
            }
            guard let state = sources[sourceID], state.open else {
                return failure("LIVE_SOURCE_NOT_OPEN", "live source must be open before selection")
            }
            selectedSourceID = sourceID
            return success("live source selected", output: sourceOutput(state))

        case LiveSourceCapability.route:
            guard liveOnly(parameters, allowed: ["contract_version", "source_id", "layer_id", "output_id"]) else {
                return failure("LIVE_SOURCE_INVALID", "route contains unknown fields")
            }
            guard let state = sources[sourceID], state.open else {
                return failure("LIVE_SOURCE_NOT_OPEN", "live source must be open before routing")
            }
            guard let layerID = liveIdentifier(parameters["layer_id"], maxLength: 64) else {
                return failure("LIVE_SOURCE_INVALID", "layer_id is required")
            }
            let outputID = liveOptionalString(parameters["output_id"], maxLength: 64) ?? "main"
            guard !outputID.isEmpty else {
                return failure("LIVE_SOURCE_INVALID", "output_id cannot be empty")
            }
            let route = LiveSourceRouteState(sourceID: sourceID, layerID: layerID, outputID: outputID)
            routes[outputID + "\u{0}" + layerID] = route
            return success("live source routed", output: [
                "source_id": .string(sourceID),
                "layer_id": .string(layerID),
                "output_id": .string(outputID),
            ])

        case LiveSourceCapability.inspect:
            guard liveOnly(parameters, allowed: ["contract_version", "source_id"]) else {
                return failure("LIVE_SOURCE_INVALID", "inspect contains unknown fields")
            }
            guard let state = sources[sourceID] else {
                return failure("LIVE_SOURCE_UNKNOWN", "live source is unknown on this Companion")
            }
            var output = sourceOutput(state)
            output["selected"] = .bool(selectedSourceID == sourceID)
            output["routes"] = .array(
                routes.values
                    .filter { $0.sourceID == sourceID }
                    .sorted { $0.layerID < $1.layerID }
                    .map { route in
                        .object([
                            "layer_id": .string(route.layerID),
                            "output_id": .string(route.outputID),
                        ])
                    }
            )
            return success("live source state inspected", output: output)

        default:
            return failure("LIVE_SOURCE_CAPABILITY_UNSUPPORTED", "unsupported live-source capability")
        }
    }

    private func sourceOutput(_ state: LiveSourceRuntimeState) -> [String: JSONValue] {
        [
            "source_id": .string(state.descriptor.sourceID),
            "source_class": .string(state.descriptor.sourceClass.rawValue),
            "open": .bool(state.open),
        ]
    }

    private func success(
        _ summary: String,
        output: [String: JSONValue] = [:]
    ) -> CompanionCapabilityOutcome {
        CompanionCapabilityOutcome(
            status: .completed,
            ackLevel: .accepted,
            responseSummary: summary,
            output: output
        )
    }

    private func failure(_ code: String, _ summary: String) -> CompanionCapabilityOutcome {
        CompanionCapabilityOutcome(
            status: .failed,
            ackLevel: .none,
            errorCode: code,
            responseSummary: summary
        )
    }
}

public struct LiveSourceCapabilityExecutor: CompanionCapabilityExecutor {
    public let capabilityKey: String
    private let engine: LiveSourceEngine

    public init(capabilityKey: String, engine: LiveSourceEngine) throws {
        guard LiveSourceCapability.all.contains(capabilityKey) else {
            throw LiveSourceCapabilityExecutorError.unsupportedCapability(capabilityKey)
        }
        self.capabilityKey = capabilityKey
        self.engine = engine
    }

    public func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        await engine.execute(capability: capabilityKey, parameters: parameters)
    }
}

public enum LiveSourceCapabilityExecutorError: Error, Equatable {
    case unsupportedCapability(String)
}

public func makeLiveSourceCapabilityExecutors(
    engine: LiveSourceEngine
) throws -> [any CompanionCapabilityExecutor] {
    try LiveSourceCapability.all.map { try LiveSourceCapabilityExecutor(capabilityKey: $0, engine: engine) }
}

private func liveInt(_ value: JSONValue?) -> Int? {
    guard case .int(let number)? = value else { return nil }
    return number
}

private func liveString(_ value: JSONValue?, maxLength: Int) -> String? {
    guard case .string(let raw)? = value else { return nil }
    let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty, trimmed == raw, trimmed.count <= maxLength else { return nil }
    return trimmed
}

private func liveIdentifier(_ value: JSONValue?, maxLength: Int) -> String? {
    liveString(value, maxLength: maxLength)
}

private func liveOptionalString(_ value: JSONValue?, maxLength: Int) -> String? {
    guard let value else { return nil }
    return liveString(value, maxLength: maxLength)
}

private func liveOnly(_ values: [String: JSONValue], allowed: Set<String>) -> Bool {
    Set(values.keys).isSubset(of: allowed)
}
