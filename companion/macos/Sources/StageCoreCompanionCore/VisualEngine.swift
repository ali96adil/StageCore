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

public enum VisualContentMode: String, Sendable, Equatable, CaseIterable {
    case fit = "FIT"
    case fill = "FILL"
    case crop = "CROP"
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

public struct VisualRenderLayer: Sendable, Equatable {
    public var layerID: String
    public var mediaURL: URL
    public var contentMode: VisualContentMode
    public var opacity: Double
    public var transform: VisualTransformState

    public init(
        layerID: String,
        mediaURL: URL,
        contentMode: VisualContentMode,
        opacity: Double,
        transform: VisualTransformState
    ) {
        self.layerID = layerID
        self.mediaURL = mediaURL
        self.contentMode = contentMode
        self.opacity = opacity
        self.transform = transform
    }
}

public struct VisualRendererFailure: Error, Sendable, Equatable {
    public var code: String
    public var summary: String

    public init(code: String, summary: String) {
        self.code = code
        self.summary = summary
    }
}

public protocol VisualRenderer: Sendable {
    func preload(_ layer: VisualRenderLayer) async throws
    func play(layerID: String) async throws
    func pause(layerID: String) async throws
    func stop(layerID: String) async throws
    func seek(layerID: String, positionMS: Int64) async throws
    func setLoop(layerID: String, enabled: Bool) async throws
    func setBlackout(_ enabled: Bool) async throws
    func setOpacity(layerID: String, opacity: Double) async throws
    func setTransform(layerID: String, transform: VisualTransformState) async throws
    func shutdown() async
}

/// State-only renderer used by contract/unit tests. Production bootstrap must
/// only advertise native visual capabilities when a real renderer is wired.
public struct StateOnlyVisualRenderer: VisualRenderer {
    public init() {}
    public func preload(_ layer: VisualRenderLayer) async throws {}
    public func play(layerID: String) async throws {}
    public func pause(layerID: String) async throws {}
    public func stop(layerID: String) async throws {}
    public func seek(layerID: String, positionMS: Int64) async throws {}
    public func setLoop(layerID: String, enabled: Bool) async throws {}
    public func setBlackout(_ enabled: Bool) async throws {}
    public func setOpacity(layerID: String, opacity: Double) async throws {}
    public func setTransform(layerID: String, transform: VisualTransformState) async throws {}
    public func shutdown() async {}
}

public struct VisualLayerState: Sendable, Equatable {
    public var layerID: String
    public var contentVersionID: String
    public var contentHash: String
    public var mediaURL: URL
    public var contentMode: VisualContentMode
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
    private let renderer: any VisualRenderer
    private var blackout = false
    private var layers: [String: VisualLayerState] = [:]

    public init(
        mediaResolver: any VisualMediaResolver,
        renderer: any VisualRenderer = StateOnlyVisualRenderer()
    ) {
        self.mediaResolver = mediaResolver
        self.renderer = renderer
    }

    public func snapshot() -> VisualEngineSnapshot {
        VisualEngineSnapshot(
            contractVersion: VisualCapability.contractVersion,
            blackout: blackout,
            layers: layers.values.sorted { $0.layerID < $1.layerID }
        )
    }

    public func shutdown() async {
        await renderer.shutdown()
        layers.removeAll()
        blackout = false
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
        case VisualCapability.preload: return await preload(parameters)
        case VisualCapability.play: return await play(parameters)
        case VisualCapability.pause: return await pause(parameters)
        case VisualCapability.stop: return await stop(parameters)
        case VisualCapability.seek: return await seek(parameters)
        case VisualCapability.loop: return await setLoop(parameters)
        case VisualCapability.blackout: return await setBlackout(parameters)
        case VisualCapability.layerOpacity: return await setOpacity(parameters)
        case VisualCapability.layerTransform: return await setTransform(parameters)
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
        let allowed = Set([
            "contract_version", "layer_id", "content_version_id", "content_hash",
            "content_mode", "opacity", "transform",
        ])
        guard hasOnly(parameters, allowed: allowed),
              let layerID = trimmedString(parameters["layer_id"], maxLength: 64),
              let contentVersionID = trimmedString(parameters["content_version_id"], maxLength: 256),
              let contentHash = canonicalSHA256(parameters["content_hash"]) else {
            return failure("VISUAL_PARAMETERS_INVALID", "preload requires canonical layer and managed media identity")
        }

        let contentMode: VisualContentMode
        if let raw = parameters["content_mode"] {
            guard case .string(let value) = raw,
                  let parsed = VisualContentMode(rawValue: value) else {
                return failure("VISUAL_PARAMETERS_INVALID", "content_mode must be FIT, FILL or CROP")
            }
            contentMode = parsed
        } else {
            contentMode = .fit
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

        let renderLayer = VisualRenderLayer(
            layerID: layerID,
            mediaURL: mediaURL,
            contentMode: contentMode,
            opacity: opacity,
            transform: transform
        )
        if let rendererFailure = await rendererFailure({ try await renderer.preload(renderLayer) }) {
            return rendererFailure
        }

        layers[layerID] = VisualLayerState(
            layerID: layerID,
            contentVersionID: contentVersionID,
            contentHash: contentHash,
            mediaURL: mediaURL,
            contentMode: contentMode,
            playback: .preloaded,
            positionMS: 0,
            loopEnabled: false,
            opacity: opacity,
            transform: transform
        )
        return success("visual layer preloaded", output: ["state": snapshotJSON()])
    }

    private func play(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        guard let layerID = validatedLayerID(parameters) else {
            return failure("VISUAL_PARAMETERS_INVALID", "layer_id is required")
        }
        guard var layer = layers[layerID] else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded first")
        }
        if layer.playback == .playing {
            return success("visual layer already playing", output: ["state": snapshotJSON()])
        }
        if let rendererFailure = await rendererFailure({ try await renderer.play(layerID: layerID) }) {
            return rendererFailure
        }
        layer.playback = .playing
        layers[layerID] = layer
        return success("visual layer playing", output: ["state": snapshotJSON()])
    }

    private func pause(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        guard let layerID = validatedLayerID(parameters) else {
            return failure("VISUAL_PARAMETERS_INVALID", "layer_id is required")
        }
        guard var layer = layers[layerID] else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded first")
        }
        guard layer.playback == .playing else {
            return failure("VISUAL_STATE_CONFLICT", "pause requires a playing layer")
        }
        if let rendererFailure = await rendererFailure({ try await renderer.pause(layerID: layerID) }) {
            return rendererFailure
        }
        layer.playback = .paused
        layers[layerID] = layer
        return success("visual layer paused", output: ["state": snapshotJSON()])
    }

    private func stop(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        guard let layerID = validatedLayerID(parameters) else {
            return failure("VISUAL_PARAMETERS_INVALID", "layer_id is required")
        }
        guard var layer = layers[layerID] else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded first")
        }
        if let rendererFailure = await rendererFailure({ try await renderer.stop(layerID: layerID) }) {
            return rendererFailure
        }
        layer.playback = .stopped
        layer.positionMS = 0
        layers[layerID] = layer
        return success("visual layer stopped", output: ["state": snapshotJSON()])
    }

    private func seek(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
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
        if let rendererFailure = await rendererFailure({ try await renderer.seek(layerID: layerID, positionMS: Int64(position)) }) {
            return rendererFailure
        }
        layer.positionMS = Int64(position)
        layers[layerID] = layer
        return success("visual seek applied", output: ["state": snapshotJSON()])
    }

    private func setLoop(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        let allowed = Set(["contract_version", "layer_id", "enabled"])
        guard hasOnly(parameters, allowed: allowed),
              let layerID = trimmedString(parameters["layer_id"], maxLength: 64),
              case .bool(let enabled)? = parameters["enabled"] else {
            return failure("VISUAL_PARAMETERS_INVALID", "loop requires layer_id and enabled")
        }
        guard var layer = layers[layerID] else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded before loop changes")
        }
        if let rendererFailure = await rendererFailure({ try await renderer.setLoop(layerID: layerID, enabled: enabled) }) {
            return rendererFailure
        }
        layer.loopEnabled = enabled
        layers[layerID] = layer
        return success("visual loop updated", output: ["state": snapshotJSON()])
    }

    private func setBlackout(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        guard hasOnly(parameters, allowed: ["contract_version", "enabled"]),
              case .bool(let enabled)? = parameters["enabled"] else {
            return failure("VISUAL_PARAMETERS_INVALID", "blackout requires enabled")
        }
        if blackout == enabled {
            return success("visual blackout unchanged", output: ["state": snapshotJSON()])
        }
        if let rendererFailure = await rendererFailure({ try await renderer.setBlackout(enabled) }) {
            return rendererFailure
        }
        blackout = enabled
        return success("visual blackout updated", output: ["state": snapshotJSON()])
    }

    private func setOpacity(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
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
        if let rendererFailure = await rendererFailure({ try await renderer.setOpacity(layerID: layerID, opacity: opacity) }) {
            return rendererFailure
        }
        layer.opacity = opacity
        layers[layerID] = layer
        return success("visual opacity updated", output: ["state": snapshotJSON()])
    }

    private func setTransform(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        let allowed = Set(["contract_version", "layer_id", "transform"])
        guard hasOnly(parameters, allowed: allowed),
              let layerID = trimmedString(parameters["layer_id"], maxLength: 64),
              case .object(let object)? = parameters["transform"] else {
            return failure("VISUAL_PARAMETERS_INVALID", "visual transform is invalid")
        }
        guard var layer = layers[layerID] else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded before transform changes")
        }
        guard let transform = parseTransform(object, base: layer.transform) else {
            return failure("VISUAL_PARAMETERS_INVALID", "visual transform is invalid")
        }
        if let rendererFailure = await rendererFailure({ try await renderer.setTransform(layerID: layerID, transform: transform) }) {
            return rendererFailure
        }
        layer.transform = transform
        layers[layerID] = layer
        return success("visual transform updated", output: ["state": snapshotJSON()])
    }

    private func validatedLayerID(_ parameters: [String: JSONValue]) -> String? {
        guard hasOnly(parameters, allowed: ["contract_version", "layer_id"]) else { return nil }
        return trimmedString(parameters["layer_id"], maxLength: 64)
    }

    private func rendererFailure(
        _ operation: () async throws -> Void
    ) async -> CompanionCapabilityOutcome? {
        do {
            try await operation()
            return nil
        } catch let failure as VisualRendererFailure {
            return self.failure(failure.code, failure.summary)
        } catch {
            return failure("VISUAL_RENDERER_FAILED", "visual renderer failed closed")
        }
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
            "content_mode": .string(layer.contentMode.rawValue),
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
