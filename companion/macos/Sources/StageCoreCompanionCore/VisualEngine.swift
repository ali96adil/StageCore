import Foundation

public enum VisualCapability {
    public static let contractVersion = 1
    public static let preload = "visual.preload"
    public static let play = "visual.play"
    public static let pause = "visual.pause"
    public static let stop = "visual.stop"
    public static let seek = "visual.seek"
    public static let loop = "visual.loop"
    public static let blackout = "visual.blackout"
    public static let layerOpacity = "visual.layer.opacity"
    public static let layerTransform = "visual.layer.transform"
    public static let stateInspect = "visual.state.inspect"

    public static let all: [String] = [
        preload, play, pause, stop, seek, loop,
        blackout, layerOpacity, layerTransform, stateInspect,
    ]
}

public protocol VisualMediaResolver: Sendable {
    func verifiedMediaURL(contentHash: String) async -> URL?
}

public enum VisualPlaybackState: String, Sendable, Equatable {
    case preloaded = "PRELOADED"
    case playing = "PLAYING"
    case paused = "PAUSED"
    case stopped = "STOPPED"
}

public struct VisualTransformState: Sendable, Equatable {
    public var x: Double
    public var y: Double
    public var scaleX: Double
    public var scaleY: Double
    public var rotationDegrees: Double

    public init(
        x: Double = 0,
        y: Double = 0,
        scaleX: Double = 1,
        scaleY: Double = 1,
        rotationDegrees: Double = 0
    ) {
        self.x = x
        self.y = y
        self.scaleX = scaleX
        self.scaleY = scaleY
        self.rotationDegrees = rotationDegrees
    }
}

public struct VisualLayerState: Sendable, Equatable {
    public var layerID: String
    public var contentVersionID: String
    public var contentHash: String
    public var mediaURL: URL
    public var playback: VisualPlaybackState
    public var positionMS: Int64
    public var loopEnabled: Bool
    public var opacity: Double
    public var transform: VisualTransformState
}

public struct VisualEngineSnapshot: Sendable, Equatable {
    public var contractVersion: Int
    public var blackout: Bool
    public var layers: [VisualLayerState]
}

public actor VisualEngine {
    private let mediaResolver: any VisualMediaResolver
    private var blackout = false
    private var layers: [String: VisualLayerState] = [:]

    public init(mediaResolver: any VisualMediaResolver) {
        self.mediaResolver = mediaResolver
    }

    public func snapshot() -> VisualEngineSnapshot {
        VisualEngineSnapshot(
            contractVersion: VisualCapability.contractVersion,
            blackout: blackout,
            layers: layers.values.sorted { $0.layerID < $1.layerID }
        )
    }

    public func execute(
        capability: String,
        parameters: [String: JSONValue]
    ) async -> CompanionCapabilityOutcome {
        guard VisualCapability.all.contains(capability) else {
            return failure("VISUAL_CAPABILITY_UNSUPPORTED", "unsupported Visual Engine capability")
        }
        guard case .int(let version)? = parameters["contract_version"],
              version == VisualCapability.contractVersion else {
            return failure("VISUAL_CONTRACT_VERSION_UNSUPPORTED", "contract_version 1 is required")
        }

        switch capability {
        case VisualCapability.preload:
            return await preload(parameters)
        case VisualCapability.play:
            return mutateLayer(parameters, allowed: ["contract_version", "layer_id"]) { layer in
                guard layer.playback != .playing else { return nil }
                layer.playback = .playing
                return nil
            }
        case VisualCapability.pause:
            return mutateLayer(parameters, allowed: ["contract_version", "layer_id"]) { layer in
                guard layer.playback == .playing else {
                    return ("VISUAL_STATE_CONFLICT", "pause requires a playing layer")
                }
                layer.playback = .paused
                return nil
            }
        case VisualCapability.stop:
            return mutateLayer(parameters, allowed: ["contract_version", "layer_id"]) { layer in
                layer.playback = .stopped
                layer.positionMS = 0
                return nil
            }
        case VisualCapability.seek:
            return seek(parameters)
        case VisualCapability.loop:
            return setLoop(parameters)
        case VisualCapability.blackout:
            return setBlackout(parameters)
        case VisualCapability.layerOpacity:
            return setOpacity(parameters)
        case VisualCapability.layerTransform:
            return setTransform(parameters)
        case VisualCapability.stateInspect:
            guard hasOnly(parameters, allowed: ["contract_version"]) else {
                return failure("VISUAL_PARAMETERS_INVALID", "state inspection contains unsupported parameters")
            }
            return success("visual state inspected", output: ["state": snapshotJSON()])
        default:
            return failure("VISUAL_CAPABILITY_UNSUPPORTED", "unsupported Visual Engine capability")
        }
    }

    private func preload(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        let allowed = Set(["contract_version", "layer_id", "content_version_id", "content_hash", "opacity", "transform"])
        guard hasOnly(parameters, allowed: allowed),
              let layerID = trimmedString(parameters["layer_id"], maxLength: 64),
              let contentVersionID = trimmedString(parameters["content_version_id"], maxLength: 256),
              let contentHash = canonicalSHA256(parameters["content_hash"]) else {
            return failure("VISUAL_PARAMETERS_INVALID", "preload requires canonical layer and managed media identity")
        }

        var opacity = 1.0
        if let raw = parameters["opacity"] {
            guard let parsed = finiteDouble(raw), parsed >= 0, parsed <= 1 else {
                return failure("VISUAL_PARAMETERS_INVALID", "opacity must be between 0 and 1")
            }
            opacity = parsed
        }

        var transform = VisualTransformState()
        if let raw = parameters["transform"] {
            guard case .object(let object) = raw,
                  let parsed = parseTransform(object, base: transform) else {
                return failure("VISUAL_PARAMETERS_INVALID", "transform is invalid")
            }
            transform = parsed
        }

        guard let mediaURL = await mediaResolver.verifiedMediaURL(contentHash: contentHash) else {
            return failure("VISUAL_MEDIA_UNAVAILABLE", "managed media is not verified in the render-node cache")
        }

        layers[layerID] = VisualLayerState(
            layerID: layerID,
            contentVersionID: contentVersionID,
            contentHash: contentHash,
            mediaURL: mediaURL,
            playback: .preloaded,
            positionMS: 0,
            loopEnabled: false,
            opacity: opacity,
            transform: transform
        )
        return success("visual layer preloaded into deterministic state", output: ["state": snapshotJSON()])
    }

    private func seek(_ parameters: [String: JSONValue]) -> CompanionCapabilityOutcome {
        let allowed = Set(["contract_version", "layer_id", "position_ms"])
        guard hasOnly(parameters, allowed: allowed),
              let layerID = trimmedString(parameters["layer_id"], maxLength: 64),
              case .int(let position)? = parameters["position_ms"],
              position >= 0 else {
            return failure("VISUAL_PARAMETERS_INVALID", "seek requires layer_id and non-negative position_ms")
        }
        guard var layer = layers[layerID] else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded before seek")
        }
        layer.positionMS = Int64(position)
        layers[layerID] = layer
        return success("visual seek state updated", output: ["state": snapshotJSON()])
    }

    private func setLoop(_ parameters: [String: JSONValue]) -> CompanionCapabilityOutcome {
        let allowed = Set(["contract_version", "layer_id", "enabled"])
        guard hasOnly(parameters, allowed: allowed),
              let layerID = trimmedString(parameters["layer_id"], maxLength: 64),
              case .bool(let enabled)? = parameters["enabled"] else {
            return failure("VISUAL_PARAMETERS_INVALID", "loop requires layer_id and enabled")
        }
        guard var layer = layers[layerID] else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded before loop changes")
        }
        layer.loopEnabled = enabled
        layers[layerID] = layer
        return success("visual loop state updated", output: ["state": snapshotJSON()])
    }

    private func setBlackout(_ parameters: [String: JSONValue]) -> CompanionCapabilityOutcome {
        guard hasOnly(parameters, allowed: ["contract_version", "enabled"]),
              case .bool(let enabled)? = parameters["enabled"] else {
            return failure("VISUAL_PARAMETERS_INVALID", "blackout requires enabled")
        }
        blackout = enabled
        return success("visual blackout state updated", output: ["state": snapshotJSON()])
    }

    private func setOpacity(_ parameters: [String: JSONValue]) -> CompanionCapabilityOutcome {
        let allowed = Set(["contract_version", "layer_id", "opacity"])
        guard hasOnly(parameters, allowed: allowed),
              let layerID = trimmedString(parameters["layer_id"], maxLength: 64),
              let opacity = finiteDouble(parameters["opacity"]),
              opacity >= 0, opacity <= 1 else {
            return failure("VISUAL_PARAMETERS_INVALID", "layer opacity must be between 0 and 1")
        }
        guard var layer = layers[layerID] else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded before opacity changes")
        }
        layer.opacity = opacity
        layers[layerID] = layer
        return success("visual layer opacity updated", output: ["state": snapshotJSON()])
    }

    private func setTransform(_ parameters: [String: JSONValue]) -> CompanionCapabilityOutcome {
        let allowed = Set(["contract_version", "layer_id", "transform"])
        guard hasOnly(parameters, allowed: allowed),
              let layerID = trimmedString(parameters["layer_id"], maxLength: 64),
              case .object(let object)? = parameters["transform"],
              var layer = layers[layerID],
              let transform = parseTransform(object, base: layer.transform) else {
            if let layerID = trimmedString(parameters["layer_id"], maxLength: 64), layers[layerID] == nil {
                return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded before transform changes")
            }
            return failure("VISUAL_PARAMETERS_INVALID", "visual transform is invalid")
        }
        layer.transform = transform
        layers[layerID] = layer
        return success("visual layer transform updated", output: ["state": snapshotJSON()])
    }

    private func mutateLayer(
        _ parameters: [String: JSONValue],
        allowed: Set<String>,
        mutation: (inout VisualLayerState) -> (String, String)?
    ) -> CompanionCapabilityOutcome {
        guard hasOnly(parameters, allowed: allowed),
              let layerID = trimmedString(parameters["layer_id"], maxLength: 64) else {
            return failure("VISUAL_PARAMETERS_INVALID", "layer_id is required")
        }
        guard var layer = layers[layerID] else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded first")
        }
        if let problem = mutation(&layer) {
            return failure(problem.0, problem.1)
        }
        layers[layerID] = layer
        return success("visual layer state updated", output: ["state": snapshotJSON()])
    }

    private func parseTransform(
        _ object: [String: JSONValue],
        base: VisualTransformState
    ) -> VisualTransformState? {
        let allowed = Set(["x", "y", "scale_x", "scale_y", "rotation_degrees"])
        guard !object.isEmpty, hasOnly(object, allowed: allowed) else { return nil }
        var result = base
        if let raw = object["x"] {
            guard let value = finiteDouble(raw) else { return nil }
            result.x = value
        }
        if let raw = object["y"] {
            guard let value = finiteDouble(raw) else { return nil }
            result.y = value
        }
        if let raw = object["scale_x"] {
            guard let value = finiteDouble(raw), value > 0 else { return nil }
            result.scaleX = value
        }
        if let raw = object["scale_y"] {
            guard let value = finiteDouble(raw), value > 0 else { return nil }
            result.scaleY = value
        }
        if let raw = object["rotation_degrees"] {
            guard let value = finiteDouble(raw) else { return nil }
            result.rotationDegrees = value
        }
        return result
    }

    private func snapshotJSON() -> JSONValue {
        let sorted = layers.values.sorted { $0.layerID < $1.layerID }
        return .object([
            "contract_version": .int(VisualCapability.contractVersion),
            "blackout": .bool(blackout),
            "layers": .array(sorted.map(layerJSON)),
        ])
    }

    private func layerJSON(_ layer: VisualLayerState) -> JSONValue {
        .object([
            "layer_id": .string(layer.layerID),
            "content_version_id": .string(layer.contentVersionID),
            "content_hash": .string(layer.contentHash),
            "playback": .string(layer.playback.rawValue),
            "position_ms": .int(Int(layer.positionMS)),
            "loop": .bool(layer.loopEnabled),
            "opacity": .double(layer.opacity),
            "transform": .object([
                "x": .double(layer.transform.x),
                "y": .double(layer.transform.y),
                "scale_x": .double(layer.transform.scaleX),
                "scale_y": .double(layer.transform.scaleY),
                "rotation_degrees": .double(layer.transform.rotationDegrees),
            ]),
        ])
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

public struct VisualCapabilityExecutor: CompanionCapabilityExecutor {
    public let capabilityKey: String
    private let engine: VisualEngine

    public init(capabilityKey: String, engine: VisualEngine) throws {
        guard VisualCapability.all.contains(capabilityKey) else {
            throw VisualCapabilityExecutorError.unsupportedCapability(capabilityKey)
        }
        self.capabilityKey = capabilityKey
        self.engine = engine
    }

    public func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        await engine.execute(capability: capabilityKey, parameters: parameters)
    }
}

public enum VisualCapabilityExecutorError: Error, Equatable {
    case unsupportedCapability(String)
}

public func makeVisualCapabilityExecutors(
    engine: VisualEngine
) throws -> [any CompanionCapabilityExecutor] {
    try VisualCapability.all.map { try VisualCapabilityExecutor(capabilityKey: $0, engine: engine) }
}

private func trimmedString(_ value: JSONValue?, maxLength: Int) -> String? {
    guard case .string(let raw)? = value else { return nil }
    let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty, trimmed == raw, trimmed.count <= maxLength else { return nil }
    return trimmed
}

private func canonicalSHA256(_ value: JSONValue?) -> String? {
    guard let raw = trimmedString(value, maxLength: 64), raw.count == 64, raw == raw.lowercased() else {
        return nil
    }
    let allowed = CharacterSet(charactersIn: "0123456789abcdef")
    guard raw.unicodeScalars.allSatisfy({ allowed.contains($0) }) else { return nil }
    return raw
}

private func finiteDouble(_ value: JSONValue?) -> Double? {
    let result: Double
    switch value {
    case .double(let number): result = number
    case .int(let number): result = Double(number)
    default: return nil
    }
    return result.isFinite ? result : nil
}

private func hasOnly(_ values: [String: JSONValue], allowed: Set<String>) -> Bool {
    Set(values.keys).isSubset(of: allowed)
}
