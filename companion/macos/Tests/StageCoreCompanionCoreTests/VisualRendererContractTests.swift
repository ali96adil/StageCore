import Foundation
import XCTest
@testable import StageCoreCompanionCore

private struct RendererContractMediaResolver: VisualMediaResolver {
    let url: URL
    func verifiedMediaURL(contentHash: String) async -> URL? { url }
}

private actor RecordingVisualRenderer: VisualRenderer {
    enum Operation: Sendable, Equatable {
        case preload(String, VisualContentMode)
        case play(String)
        case pause(String)
        case stop(String)
        case seek(String, Int64)
        case loop(String, Bool)
        case blackout(Bool)
        case opacity(String, Double)
        case transform(String)
        case shutdown
    }

    private(set) var operations: [Operation] = []
    var failure: VisualRendererFailure?

    init(failure: VisualRendererFailure? = nil) {
        self.failure = failure
    }

    private func check() throws {
        if let failure { throw failure }
    }

    func preload(_ layer: VisualRenderLayer) async throws {
        try check()
        operations.append(.preload(layer.layerID, layer.contentMode))
    }
    func play(layerID: String) async throws { try check(); operations.append(.play(layerID)) }
    func pause(layerID: String) async throws { try check(); operations.append(.pause(layerID)) }
    func stop(layerID: String) async throws { try check(); operations.append(.stop(layerID)) }
    func seek(layerID: String, positionMS: Int64) async throws { try check(); operations.append(.seek(layerID, positionMS)) }
    func setLoop(layerID: String, enabled: Bool) async throws { try check(); operations.append(.loop(layerID, enabled)) }
    func setBlackout(_ enabled: Bool) async throws { try check(); operations.append(.blackout(enabled)) }
    func setOpacity(layerID: String, opacity: Double) async throws { try check(); operations.append(.opacity(layerID, opacity)) }
    func setTransform(layerID: String, transform: VisualTransformState) async throws { try check(); operations.append(.transform(layerID)) }
    func shutdown() async { operations.append(.shutdown) }

    func recorded() -> [Operation] { operations }
}

final class VisualRendererContractTests: XCTestCase {
    private let hash = String(repeating: "b", count: 64)

    func testRendererSuccessCommitsStateAfterNativeOperations() async throws {
        let renderer = RecordingVisualRenderer()
        let engine = VisualEngine(
            mediaResolver: RendererContractMediaResolver(url: URL(fileURLWithPath: "/verified/video.mov")),
            renderer: renderer
        )

        let preload = await engine.execute(
            capability: VisualCapability.preload,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("main"),
                "content_version_id": .string("version-1"),
                "content_hash": .string(hash),
                "content_mode": .string("CROP"),
            ]
        )
        XCTAssertEqual(preload.status, .completed)
        let play = await engine.execute(
            capability: VisualCapability.play,
            parameters: ["contract_version": .int(1), "layer_id": .string("main")]
        )
        XCTAssertEqual(play.status, .completed)
        let opacity = await engine.execute(
            capability: VisualCapability.layerOpacity,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("main"),
                "opacity": .double(0.4),
            ]
        )
        XCTAssertEqual(opacity.status, .completed)

        let state = await engine.snapshot()
        let layer = try XCTUnwrap(state.layers.first)
        XCTAssertEqual(layer.contentMode, .crop)
        XCTAssertEqual(layer.playback, .playing)
        XCTAssertEqual(layer.opacity, 0.4)
        let recorded = await renderer.recorded()
        XCTAssertEqual(
            recorded,
            [.preload("main", .crop), .play("main"), .opacity("main", 0.4)]
        )
    }

    func testRendererFailureDoesNotCommitPreloadState() async {
        let renderer = RecordingVisualRenderer(
            failure: VisualRendererFailure(
                code: "VISUAL_RENDERER_MEDIA_UNSUPPORTED",
                summary: "unsupported"
            )
        )
        let engine = VisualEngine(
            mediaResolver: RendererContractMediaResolver(url: URL(fileURLWithPath: "/verified/bad.bin")),
            renderer: renderer
        )

        let result = await engine.execute(
            capability: VisualCapability.preload,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("main"),
                "content_version_id": .string("version-1"),
                "content_hash": .string(hash),
            ]
        )
        XCTAssertEqual(result.status, .failed)
        XCTAssertEqual(result.errorCode, "VISUAL_RENDERER_MEDIA_UNSUPPORTED")
        let state = await engine.snapshot()
        XCTAssertTrue(state.layers.isEmpty)
    }

    func testInvalidContentModeFailsBeforeRenderer() async {
        let renderer = RecordingVisualRenderer()
        let engine = VisualEngine(
            mediaResolver: RendererContractMediaResolver(url: URL(fileURLWithPath: "/verified/video.mov")),
            renderer: renderer
        )
        let result = await engine.execute(
            capability: VisualCapability.preload,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("main"),
                "content_version_id": .string("version-1"),
                "content_hash": .string(hash),
                "content_mode": .string("STRETCHISH"),
            ]
        )
        XCTAssertEqual(result.status, .failed)
        XCTAssertEqual(result.errorCode, "VISUAL_PARAMETERS_INVALID")
        let recorded = await renderer.recorded()
        XCTAssertTrue(recorded.isEmpty)
    }
}
