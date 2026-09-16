import Foundation
import XCTest
@testable import StageCoreCompanionCore

private struct ProjectionContractMediaResolver: VisualMediaResolver {
    let url: URL
    func verifiedMediaURL(contentHash: String) async -> URL? { url }
}

private actor ProjectionRecordingRenderer: VisualRenderer {
    enum Operation: Sendable, Equatable {
        case preload(String, String, Int)
        case configure(String, Double, Double)
        case assign(String, String)
        case order(String, Int)
        case mapping(String)
    }

    private(set) var operations: [Operation] = []
    var failConfigure = false

    func preload(_ layer: VisualRenderLayer) async throws {
        operations.append(.preload(layer.layerID, layer.outputID, layer.zIndex))
    }
    func play(layerID: String) async throws {}
    func pause(layerID: String) async throws {}
    func stop(layerID: String) async throws {}
    func seek(layerID: String, positionMS: Int64) async throws {}
    func setLoop(layerID: String, enabled: Bool) async throws {}
    func setBlackout(_ enabled: Bool) async throws {}
    func setOpacity(layerID: String, opacity: Double) async throws {}
    func setTransform(layerID: String, transform: VisualTransformState) async throws {}
    func configureOutput(_ output: VisualOutputState) async throws {
        if failConfigure {
            throw VisualRendererFailure(code: "VISUAL_RENDERER_OUTPUT_INVALID", summary: "rejected")
        }
        operations.append(.configure(output.outputID, output.width, output.height))
    }
    func setLayerOutput(layerID: String, outputID: String) async throws {
        operations.append(.assign(layerID, outputID))
    }
    func setLayerOrder(layerID: String, zIndex: Int) async throws {
        operations.append(.order(layerID, zIndex))
    }
    func setOutputMapping(outputID: String, mapping: VisualQuadState) async throws {
        operations.append(.mapping(outputID))
    }
    func shutdown() async {}

    func recorded() -> [Operation] { operations }
    func setFailConfigure(_ enabled: Bool) { failConfigure = enabled }
}

final class VisualProjectionContractTests: XCTestCase {
    private let contentHash = String(repeating: "c", count: 64)

    func testNamedOutputLayerOrderingAndMappingCommitAfterRendererSuccess() async throws {
        let renderer = ProjectionRecordingRenderer()
        let engine = VisualEngine(
            mediaResolver: ProjectionContractMediaResolver(url: URL(fileURLWithPath: "/verified/frame.png")),
            renderer: renderer
        )

        let configure = await engine.execute(
            capability: VisualCapability.outputConfigure,
            parameters: [
                "contract_version": .int(1),
                "output_id": .string("projector-a"),
                "width": .double(1280),
                "height": .double(800),
            ]
        )
        XCTAssertEqual(configure.status, .completed)

        let mapping = await engine.execute(
            capability: VisualCapability.outputMapping,
            parameters: [
                "contract_version": .int(1),
                "output_id": .string("projector-a"),
                "mapping": .object([
                    "top_left": .object(["x": .double(0.03), "y": .double(0.04)]),
                    "top_right": .object(["x": .double(0.98), "y": .double(0.01)]),
                    "bottom_right": .object(["x": .double(0.94), "y": .double(0.97)]),
                    "bottom_left": .object(["x": .double(0.05), "y": .double(0.99)]),
                ]),
            ]
        )
        XCTAssertEqual(mapping.status, .completed)

        for (layerID, zIndex) in [("back", -10), ("front", 20)] {
            let result = await engine.execute(
                capability: VisualCapability.preload,
                parameters: [
                    "contract_version": .int(1),
                    "layer_id": .string(layerID),
                    "content_version_id": .string("version-\(layerID)"),
                    "content_hash": .string(contentHash),
                    "output_id": .string("projector-a"),
                    "z_index": .int(zIndex),
                ]
            )
            XCTAssertEqual(result.status, .completed)
        }

        let state = await engine.snapshot()
        let projector = try XCTUnwrap(state.outputs.first { $0.outputID == "projector-a" })
        XCTAssertEqual(projector.width, 1280)
        XCTAssertEqual(projector.height, 800)
        XCTAssertEqual(projector.mapping.topLeft.x, 0.03, accuracy: 0.0001)
        let projectorLayers = state.layers.filter { $0.outputID == "projector-a" }
        XCTAssertEqual(projectorLayers.map(\.layerID), ["back", "front"])
        XCTAssertEqual(projectorLayers.map(\.zIndex), [-10, 20])

        let reorder = await engine.execute(
            capability: VisualCapability.layerOrder,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("front"),
                "z_index": .int(-20),
            ]
        )
        XCTAssertEqual(reorder.status, .completed)
        let reassign = await engine.execute(
            capability: VisualCapability.layerOutput,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("front"),
                "output_id": .string("main"),
            ]
        )
        XCTAssertEqual(reassign.status, .completed)

        let finalState = await engine.snapshot()
        let front = try XCTUnwrap(finalState.layers.first { $0.layerID == "front" })
        XCTAssertEqual(front.outputID, "main")
        XCTAssertEqual(front.zIndex, -20)

        let operations = await renderer.recorded()
        XCTAssertTrue(operations.contains(.configure("projector-a", 1280, 800)))
        XCTAssertTrue(operations.contains(.mapping("projector-a")))
        XCTAssertTrue(operations.contains(.preload("back", "projector-a", -10)))
        XCTAssertTrue(operations.contains(.preload("front", "projector-a", 20)))
        XCTAssertTrue(operations.contains(.order("front", -20)))
        XCTAssertTrue(operations.contains(.assign("front", "main")))
    }

    func testUnknownOutputFailsBeforeRendererAndDoesNotCommitLayer() async {
        let renderer = ProjectionRecordingRenderer()
        let engine = VisualEngine(
            mediaResolver: ProjectionContractMediaResolver(url: URL(fileURLWithPath: "/verified/frame.png")),
            renderer: renderer
        )

        let result = await engine.execute(
            capability: VisualCapability.preload,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("orphan"),
                "content_version_id": .string("version-1"),
                "content_hash": .string(contentHash),
                "output_id": .string("missing"),
            ]
        )
        XCTAssertEqual(result.status, .failed)
        XCTAssertEqual(result.errorCode, "VISUAL_OUTPUT_NOT_CONFIGURED")
        let snapshot = await engine.snapshot()
        let operations = await renderer.recorded()
        XCTAssertTrue(snapshot.layers.isEmpty)
        XCTAssertTrue(operations.isEmpty)
    }

    func testRendererFailureDoesNotCommitOutputConfiguration() async {
        let renderer = ProjectionRecordingRenderer()
        await renderer.setFailConfigure(true)
        let engine = VisualEngine(
            mediaResolver: ProjectionContractMediaResolver(url: URL(fileURLWithPath: "/verified/frame.png")),
            renderer: renderer
        )

        let result = await engine.execute(
            capability: VisualCapability.outputConfigure,
            parameters: [
                "contract_version": .int(1),
                "output_id": .string("projector-a"),
                "width": .double(1920),
                "height": .double(1080),
            ]
        )
        XCTAssertEqual(result.status, .failed)
        XCTAssertEqual(result.errorCode, "VISUAL_RENDERER_OUTPUT_INVALID")
        let state = await engine.snapshot()
        XCTAssertEqual(state.outputs.map(\.outputID), ["main"])
    }

    func testDegenerateMappingFailsWithoutRendererMutation() async {
        let renderer = ProjectionRecordingRenderer()
        let engine = VisualEngine(
            mediaResolver: ProjectionContractMediaResolver(url: URL(fileURLWithPath: "/verified/frame.png")),
            renderer: renderer
        )
        _ = await engine.execute(
            capability: VisualCapability.outputConfigure,
            parameters: [
                "contract_version": .int(1),
                "output_id": .string("projector-a"),
                "width": .double(1920),
                "height": .double(1080),
            ]
        )
        let before = await renderer.recorded()

        let result = await engine.execute(
            capability: VisualCapability.outputMapping,
            parameters: [
                "contract_version": .int(1),
                "output_id": .string("projector-a"),
                "mapping": .object([
                    "top_left": .object(["x": .double(0), "y": .double(0)]),
                    "top_right": .object(["x": .double(1), "y": .double(0)]),
                    "bottom_right": .object(["x": .double(1), "y": .double(0)]),
                    "bottom_left": .object(["x": .double(0), "y": .double(0)]),
                ]),
            ]
        )
        XCTAssertEqual(result.status, .failed)
        XCTAssertEqual(result.errorCode, "VISUAL_PARAMETERS_INVALID")
        let after = await renderer.recorded()
        XCTAssertEqual(after, before)
    }
}
