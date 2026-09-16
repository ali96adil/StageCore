#if os(macOS)
import QuartzCore
import XCTest
@testable import StageCoreCompanionCore

final class NativeLiveSourceRendererIntegrationTests: XCTestCase {
    func testRendererOwnsGlobalLiveLayerIdentityAndNamedOutputAttachment() async throws {
        let renderer = try NativeVisualRenderer(surfaceWidth: 640, surfaceHeight: 360)
        try await renderer.configureOutput(
            VisualOutputState(outputID: "projector-a", width: 800, height: 600)
        )

        try await renderer.attachLiveSourceLayer(
            sourceID: "camera-a",
            layerID: "live-hero",
            outputID: "projector-a",
            presentation: NativeLiveSourcePresentationLayer(layer: CALayer())
        )
        var attachment = await renderer.inspectLiveSourceLayer(
            sourceID: "camera-a",
            layerID: "live-hero"
        )
        XCTAssertEqual(attachment?.outputID, "projector-a")

        do {
            try await renderer.attachLiveSourceLayer(
                sourceID: "camera-b",
                layerID: "live-hero",
                outputID: "main",
                presentation: NativeLiveSourcePresentationLayer(layer: CALayer())
            )
            XCTFail("another source must not steal an existing live layer ID")
        } catch let failure as VisualRendererFailure {
            XCTAssertEqual(failure.code, "VISUAL_RENDERER_LAYER_CONFLICT")
        }

        try await renderer.attachLiveSourceLayer(
            sourceID: "camera-a",
            layerID: "live-hero",
            outputID: "main",
            presentation: NativeLiveSourcePresentationLayer(layer: CALayer())
        )
        attachment = await renderer.inspectLiveSourceLayer(
            sourceID: "camera-a",
            layerID: "live-hero"
        )
        XCTAssertEqual(attachment?.outputID, "main")

        await renderer.detachLiveSourceLayers(sourceID: "camera-a")
        attachment = await renderer.inspectLiveSourceLayer(
            sourceID: "camera-a",
            layerID: "live-hero"
        )
        XCTAssertNil(attachment)
    }

    func testNativeNetworkSourceRouteAttachesAndInspectReportsRendererTruth() async throws {
        let renderer = try NativeVisualRenderer(surfaceWidth: 640, surfaceHeight: 360)
        try await renderer.configureOutput(
            VisualOutputState(outputID: "projector-a", width: 800, height: 600)
        )
        let runtime = NativeLiveSourceRuntime(renderSurface: renderer)
        let engine = LiveSourceEngine(runtime: runtime)

        var result = await engine.execute(
            capability: LiveSourceCapability.open,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("stream-a"),
                "source_class": .string("NETWORK_STREAM"),
                "endpoint_ref": .string("https://example.invalid/stagecore-live.m3u8"),
            ]
        )
        XCTAssertEqual(result.status, .completed)

        result = await engine.execute(
            capability: LiveSourceCapability.route,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("stream-a"),
                "layer_id": .string("live-hero"),
                "output_id": .string("projector-a"),
            ]
        )
        XCTAssertEqual(result.status, .completed)

        result = await engine.execute(
            capability: LiveSourceCapability.inspect,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("stream-a"),
            ]
        )
        XCTAssertEqual(result.status, .completed)
        XCTAssertEqual(result.output["native_runtime"], .bool(true))
        XCTAssertEqual(result.output["native_renderer_surface"], .bool(true))
        XCTAssertEqual(result.output["native_route_count"], .int(1))
        XCTAssertEqual(
            result.output["native_routes"],
            .array([
                .object([
                    "layer_id": .string("live-hero"),
                    "output_id": .string("projector-a"),
                    "renderer_attached": .bool(true),
                ]),
            ])
        )

        let routeBeforeFailure = await engine.snapshot().routes
        result = await engine.execute(
            capability: LiveSourceCapability.route,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("stream-a"),
                "layer_id": .string("live-hero"),
                "output_id": .string("missing-output"),
            ]
        )
        XCTAssertEqual(result.status, .failed)
        XCTAssertEqual(result.errorCode, "VISUAL_RENDERER_OUTPUT_NOT_CONFIGURED")
        XCTAssertEqual(await engine.snapshot().routes, routeBeforeFailure)

        result = await engine.execute(
            capability: LiveSourceCapability.route,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("stream-a"),
                "layer_id": .string("live-hero"),
                "output_id": .string("main"),
            ]
        )
        XCTAssertEqual(result.status, .completed)
        XCTAssertEqual(await engine.snapshot().routes.count, 1)
        XCTAssertEqual(await engine.snapshot().routes.first?.outputID, "main")

        result = await engine.execute(
            capability: LiveSourceCapability.close,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("stream-a"),
            ]
        )
        XCTAssertEqual(result.status, .completed)
        XCTAssertNil(
            await renderer.inspectLiveSourceLayer(
                sourceID: "stream-a",
                layerID: "live-hero"
            )
        )
        await renderer.shutdown()
    }
}
#endif
