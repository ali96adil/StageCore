import Foundation
import XCTest
@testable import StageCoreCompanionCore

private struct CompositionMediaResolver: VisualMediaResolver {
    let hash: String
    func verifiedMediaURL(contentHash: String) async -> URL? {
        guard contentHash == hash else { return nil }
        return URL(fileURLWithPath: "/verified/\(contentHash).mov")
    }
}

final class VisualCompositionTests: XCTestCase {
    private let contentHash = String(repeating: "c", count: 64)

    private func makeEngine() -> VisualEngine {
        VisualEngine(mediaResolver: CompositionMediaResolver(hash: contentHash))
    }

    private func preload(_ layerID: String, opacity: Double = 1, engine: VisualEngine) async throws {
        let result = await engine.execute(
            capability: VisualCapability.preload,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string(layerID),
                "content_version_id": .string("version-\(layerID)"),
                "content_hash": .string(contentHash),
                "opacity": .double(opacity),
            ]
        )
        XCTAssertEqual(result.status, .completed)
    }

    func testCropMaskEffectCommitIntoSingleVisualSnapshot() async throws {
        let engine = makeEngine()
        try await preload("hero", engine: engine)

        let crop = await engine.execute(
            capability: VisualCapability.layerCrop,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("hero"),
                "rect": .object([
                    "x": .double(0.1), "y": .double(0.2),
                    "width": .double(0.7), "height": .double(0.6),
                ]),
            ]
        )
        XCTAssertEqual(crop.status, .completed)

        let mask = await engine.execute(
            capability: VisualCapability.layerMask,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("hero"),
                "kind": .string("ELLIPSE"),
            ]
        )
        XCTAssertEqual(mask.status, .completed)

        let effect = await engine.execute(
            capability: VisualCapability.layerEffect,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("hero"),
                "brightness": .double(0.2),
                "contrast": .double(1.4),
                "saturation": .double(0.65),
            ]
        )
        XCTAssertEqual(effect.status, .completed)

        let snapshot = await engine.snapshot()
        let state = try XCTUnwrap(snapshot.composition.first)
        XCTAssertEqual(state.layerID, "hero")
        XCTAssertEqual(state.crop, VisualNormalizedRect(x: 0.1, y: 0.2, width: 0.7, height: 0.6))
        XCTAssertEqual(state.mask, .ellipse)
        XCTAssertEqual(state.effect, VisualEffectState(brightness: 0.2, contrast: 1.4, saturation: 0.65))
    }

    func testTransitionCommitsDeterministicFinalOpacity() async throws {
        let engine = makeEngine()
        try await preload("from", opacity: 0.8, engine: engine)
        try await preload("to", opacity: 0, engine: engine)

        let result = await engine.execute(
            capability: VisualCapability.transition,
            parameters: [
                "contract_version": .int(1),
                "kind": .string("CROSSFADE"),
                "from_layer_id": .string("from"),
                "to_layer_id": .string("to"),
                "duration_ms": .int(500),
                "target_opacity": .double(0.9),
            ]
        )
        XCTAssertEqual(result.status, .completed)

        let snapshot = await engine.snapshot()
        let from = try XCTUnwrap(snapshot.layers.first { $0.layerID == "from" })
        let to = try XCTUnwrap(snapshot.layers.first { $0.layerID == "to" })
        XCTAssertEqual(from.opacity, 0)
        XCTAssertEqual(to.opacity, 0.9)
    }

    func testCompositionValidationFailsBeforeStateMutation() async throws {
        let engine = makeEngine()
        try await preload("hero", engine: engine)

        let invalidCrop = await engine.execute(
            capability: VisualCapability.layerCrop,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("hero"),
                "rect": .object([
                    "x": .double(0.8), "y": .double(0),
                    "width": .double(0.3), "height": .double(1),
                ]),
            ]
        )
        XCTAssertEqual(invalidCrop.status, .failed)
        XCTAssertEqual(invalidCrop.errorCode, "VISUAL_PARAMETERS_INVALID")

        let invalidTransition = await engine.execute(
            capability: VisualCapability.transition,
            parameters: [
                "contract_version": .int(1),
                "kind": .string("CUT"),
                "from_layer_id": .string("hero"),
                "to_layer_id": .string("hero"),
                "duration_ms": .int(0),
                "target_opacity": .double(1),
            ]
        )
        XCTAssertEqual(invalidTransition.status, .failed)
        XCTAssertEqual(invalidTransition.errorCode, "VISUAL_PARAMETERS_INVALID")

        let snapshot = await engine.snapshot()
        XCTAssertEqual(snapshot.composition.first?.crop, .full)
        XCTAssertEqual(snapshot.layers.first?.opacity, 1)
    }

    func testPreloadResetsCompositionStateForReplacedLayer() async throws {
        let engine = makeEngine()
        try await preload("hero", engine: engine)
        _ = await engine.execute(
            capability: VisualCapability.layerMask,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("hero"),
                "kind": .string("ELLIPSE"),
            ]
        )
        try await preload("hero", opacity: 0.5, engine: engine)

        let snapshot = await engine.snapshot()
        XCTAssertEqual(snapshot.composition.first, VisualLayerCompositionState(layerID: "hero"))
        XCTAssertEqual(snapshot.layers.first?.opacity, 0.5)
    }
}
