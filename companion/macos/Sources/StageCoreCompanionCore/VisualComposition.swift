import Foundation

public enum VisualTransitionKind: String, Sendable, Equatable, CaseIterable {
    case cut = "CUT"
    case fade = "FADE"
    case crossfade = "CROSSFADE"
}

public enum VisualMaskKind: String, Sendable, Equatable, CaseIterable {
    case none = "NONE"
    case rect = "RECT"
    case ellipse = "ELLIPSE"
}

public struct VisualNormalizedRect: Sendable, Equatable {
    public var x: Double
    public var y: Double
    public var width: Double
    public var height: Double

    public init(x: Double = 0, y: Double = 0, width: Double = 1, height: Double = 1) {
        self.x = x
        self.y = y
        self.width = width
        self.height = height
    }

    public static let full = VisualNormalizedRect()

    public var isValid: Bool {
        x.isFinite && y.isFinite && width.isFinite && height.isFinite &&
        x >= 0 && y >= 0 && x <= 1 && y <= 1 &&
        width > 0 && height > 0 && width <= 1 && height <= 1 &&
        x + width <= 1.000000001 && y + height <= 1.000000001
    }
}

public struct VisualEffectState: Sendable, Equatable {
    public var brightness: Double
    public var contrast: Double
    public var saturation: Double

    public init(brightness: Double = 0, contrast: Double = 1, saturation: Double = 1) {
        self.brightness = brightness
        self.contrast = contrast
        self.saturation = saturation
    }

    public static let neutral = VisualEffectState()

    public var isValid: Bool {
        brightness.isFinite && contrast.isFinite && saturation.isFinite &&
        brightness >= -1 && brightness <= 1 &&
        contrast >= 0 && contrast <= 4 &&
        saturation >= 0 && saturation <= 2
    }
}

public struct VisualLayerCompositionState: Sendable, Equatable {
    public var layerID: String
    public var crop: VisualNormalizedRect
    public var mask: VisualMaskKind
    public var effect: VisualEffectState

    public init(
        layerID: String,
        crop: VisualNormalizedRect = .full,
        mask: VisualMaskKind = .none,
        effect: VisualEffectState = .neutral
    ) {
        self.layerID = layerID
        self.crop = crop
        self.mask = mask
        self.effect = effect
    }
}

public struct VisualTransitionRenderState: Sendable, Equatable {
    public var kind: VisualTransitionKind
    public var fromLayerID: String
    public var toLayerID: String
    public var durationMS: Int
    public var fromOpacity: Double
    public var targetOpacity: Double

    public init(
        kind: VisualTransitionKind,
        fromLayerID: String,
        toLayerID: String,
        durationMS: Int,
        fromOpacity: Double,
        targetOpacity: Double
    ) {
        self.kind = kind
        self.fromLayerID = fromLayerID
        self.toLayerID = toLayerID
        self.durationMS = durationMS
        self.fromOpacity = fromOpacity
        self.targetOpacity = targetOpacity
    }
}

public protocol VisualCompositionRenderer: VisualRenderer {
    func applyComposition(layerID: String, state: VisualLayerCompositionState) async throws
    func transition(_ state: VisualTransitionRenderState) async throws
}

extension StateOnlyVisualRenderer: VisualCompositionRenderer {
    public func applyComposition(layerID: String, state: VisualLayerCompositionState) async throws {}
    public func transition(_ state: VisualTransitionRenderState) async throws {}
}

extension VisualEngine {
    func setCropComposition(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        guard compositionHasOnly(parameters, allowed: ["contract_version", "layer_id", "rect"]),
              let layerID = compositionTrimmedString(parameters["layer_id"], maxLength: 64),
              case .object(let object)? = parameters["rect"],
              Set(object.keys) == Set(["x", "y", "width", "height"]),
              let x = compositionFiniteDouble(object["x"]),
              let y = compositionFiniteDouble(object["y"]),
              let width = compositionFiniteDouble(object["width"]),
              let height = compositionFiniteDouble(object["height"]) else {
            return failure("VISUAL_PARAMETERS_INVALID", "crop requires a normalized x/y/width/height rectangle")
        }
        let rect = VisualNormalizedRect(x: x, y: y, width: width, height: height)
        guard rect.isValid else {
            return failure("VISUAL_PARAMETERS_INVALID", "crop must remain inside normalized layer bounds")
        }
        guard layers[layerID] != nil else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded before crop changes")
        }
        guard let compositionRenderer = renderer as? any VisualCompositionRenderer else {
            return failure("VISUAL_RENDERER_OPERATION_UNSUPPORTED", "renderer does not support layer composition")
        }
        var state = compositionStates[layerID] ?? VisualLayerCompositionState(layerID: layerID)
        state.crop = rect
        if let rendererFailure = await compositionRendererFailure({
            try await compositionRenderer.applyComposition(layerID: layerID, state: state)
        }) {
            return rendererFailure
        }
        compositionStates[layerID] = state
        return success("visual crop updated", output: ["state": snapshotJSON()])
    }

    func setMaskComposition(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        guard compositionHasOnly(parameters, allowed: ["contract_version", "layer_id", "kind"]),
              let layerID = compositionTrimmedString(parameters["layer_id"], maxLength: 64),
              case .string(let rawKind)? = parameters["kind"],
              let kind = VisualMaskKind(rawValue: rawKind) else {
            return failure("VISUAL_PARAMETERS_INVALID", "mask kind must be NONE, RECT or ELLIPSE")
        }
        guard layers[layerID] != nil else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded before mask changes")
        }
        guard let compositionRenderer = renderer as? any VisualCompositionRenderer else {
            return failure("VISUAL_RENDERER_OPERATION_UNSUPPORTED", "renderer does not support layer composition")
        }
        var state = compositionStates[layerID] ?? VisualLayerCompositionState(layerID: layerID)
        state.mask = kind
        if let rendererFailure = await compositionRendererFailure({
            try await compositionRenderer.applyComposition(layerID: layerID, state: state)
        }) {
            return rendererFailure
        }
        compositionStates[layerID] = state
        return success("visual mask updated", output: ["state": snapshotJSON()])
    }

    func setEffectComposition(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        let allowed = Set(["contract_version", "layer_id", "brightness", "contrast", "saturation"])
        guard compositionHasOnly(parameters, allowed: allowed),
              let layerID = compositionTrimmedString(parameters["layer_id"], maxLength: 64),
              parameters["brightness"] != nil || parameters["contrast"] != nil || parameters["saturation"] != nil else {
            return failure("VISUAL_PARAMETERS_INVALID", "layer effect requires at least one bounded control")
        }
        guard layers[layerID] != nil else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "layer must be preloaded before effect changes")
        }
        var state = compositionStates[layerID] ?? VisualLayerCompositionState(layerID: layerID)
        if let raw = parameters["brightness"] {
            guard let value = compositionFiniteDouble(raw), value >= -1, value <= 1 else {
                return failure("VISUAL_PARAMETERS_INVALID", "brightness must be between -1 and 1")
            }
            state.effect.brightness = value
        }
        if let raw = parameters["contrast"] {
            guard let value = compositionFiniteDouble(raw), value >= 0, value <= 4 else {
                return failure("VISUAL_PARAMETERS_INVALID", "contrast must be between 0 and 4")
            }
            state.effect.contrast = value
        }
        if let raw = parameters["saturation"] {
            guard let value = compositionFiniteDouble(raw), value >= 0, value <= 2 else {
                return failure("VISUAL_PARAMETERS_INVALID", "saturation must be between 0 and 2")
            }
            state.effect.saturation = value
        }
        guard state.effect.isValid,
              let compositionRenderer = renderer as? any VisualCompositionRenderer else {
            return failure("VISUAL_RENDERER_OPERATION_UNSUPPORTED", "renderer does not support layer composition")
        }
        if let rendererFailure = await compositionRendererFailure({
            try await compositionRenderer.applyComposition(layerID: layerID, state: state)
        }) {
            return rendererFailure
        }
        compositionStates[layerID] = state
        return success("visual effect updated", output: ["state": snapshotJSON()])
    }

    func performTransition(_ parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        let allowed = Set([
            "contract_version", "kind", "from_layer_id", "to_layer_id", "duration_ms", "target_opacity",
        ])
        guard compositionHasOnly(parameters, allowed: allowed),
              case .string(let rawKind)? = parameters["kind"],
              let kind = VisualTransitionKind(rawValue: rawKind),
              let fromLayerID = compositionTrimmedString(parameters["from_layer_id"], maxLength: 64),
              let toLayerID = compositionTrimmedString(parameters["to_layer_id"], maxLength: 64),
              fromLayerID != toLayerID,
              case .int(let durationMS)? = parameters["duration_ms"],
              let targetOpacity = compositionFiniteDouble(parameters["target_opacity"]),
              targetOpacity >= 0, targetOpacity <= 1 else {
            return failure("VISUAL_PARAMETERS_INVALID", "transition parameters are invalid")
        }
        switch kind {
        case .cut:
            guard durationMS == 0 else {
                return failure("VISUAL_PARAMETERS_INVALID", "CUT transition requires duration_ms 0")
            }
        case .fade, .crossfade:
            guard (1...30_000).contains(durationMS) else {
                return failure("VISUAL_PARAMETERS_INVALID", "FADE and CROSSFADE duration_ms must be in 1...30000")
            }
        }
        guard var fromLayer = layers[fromLayerID], var toLayer = layers[toLayerID] else {
            return failure("VISUAL_LAYER_NOT_PRELOADED", "both transition layers must be preloaded")
        }
        guard fromLayer.outputID == toLayer.outputID else {
            return failure("VISUAL_STATE_CONFLICT", "transition layers must target the same visual output")
        }
        guard let compositionRenderer = renderer as? any VisualCompositionRenderer else {
            return failure("VISUAL_RENDERER_OPERATION_UNSUPPORTED", "renderer does not support transitions")
        }
        let transition = VisualTransitionRenderState(
            kind: kind,
            fromLayerID: fromLayerID,
            toLayerID: toLayerID,
            durationMS: durationMS,
            fromOpacity: fromLayer.opacity,
            targetOpacity: targetOpacity
        )
        if let rendererFailure = await compositionRendererFailure({
            try await compositionRenderer.transition(transition)
        }) {
            return rendererFailure
        }
        fromLayer.opacity = 0
        toLayer.opacity = targetOpacity
        layers[fromLayerID] = fromLayer
        layers[toLayerID] = toLayer
        return success("visual transition applied", output: ["state": snapshotJSON()])
    }

    private func compositionRendererFailure(
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
}

func visualCompositionJSON(_ state: VisualLayerCompositionState) -> JSONValue {
    .object([
        "layer_id": .string(state.layerID),
        "crop": .object([
            "x": .double(state.crop.x),
            "y": .double(state.crop.y),
            "width": .double(state.crop.width),
            "height": .double(state.crop.height),
        ]),
        "mask": .string(state.mask.rawValue),
        "effect": .object([
            "brightness": .double(state.effect.brightness),
            "contrast": .double(state.effect.contrast),
            "saturation": .double(state.effect.saturation),
        ]),
    ])
}

private func compositionTrimmedString(_ value: JSONValue?, maxLength: Int) -> String? {
    guard case .string(let raw)? = value else { return nil }
    let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty, trimmed == raw, trimmed.count <= maxLength else { return nil }
    return trimmed
}

private func compositionFiniteDouble(_ value: JSONValue?) -> Double? {
    let result: Double
    switch value {
    case .double(let number): result = number
    case .int(let number): result = Double(number)
    default: return nil
    }
    return result.isFinite ? result : nil
}

private func compositionHasOnly(_ values: [String: JSONValue], allowed: Set<String>) -> Bool {
    Set(values.keys).isSubset(of: allowed)
}
