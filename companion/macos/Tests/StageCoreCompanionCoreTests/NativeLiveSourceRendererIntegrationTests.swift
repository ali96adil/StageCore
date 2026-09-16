#if os(macOS)
import AppKit
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

    func testManagedAndLiveLayersCannotClaimTheSameLayerID() async throws {
        let directory = FileManager.default.temporaryDirectory
            .appendingPathComponent("stagecore-live-layer-conflict-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let imageURL = directory.appendingPathComponent("managed.png")
        try writePNG(to: imageURL)

        let managedFirst = try NativeVisualRenderer(surfaceWidth: 320, surfaceHeight: 180)
        try await managedFirst.preload(
            VisualRenderLayer(
                layerID: "shared-layer",
                mediaURL: imageURL,
                contentMode: .fit,
                opacity: 1,
                transform: VisualTransformState()
            )
        )
        do {
            try await managedFirst.attachLiveSourceLayer(
                sourceID: "camera-a",
                layerID: "shared-layer",
                outputID: "main",
                presentation: NativeLiveSourcePresentationLayer(layer: CALayer())
            )
            XCTFail("live source must not claim a managed-media layer ID")
        } catch let failure as VisualRendererFailure {
            XCTAssertEqual(failure.code, "VISUAL_RENDERER_LAYER_CONFLICT")
        }

        let liveFirst = try NativeVisualRenderer(surfaceWidth: 320, surfaceHeight: 180)
        try await liveFirst.attachLiveSourceLayer(
            sourceID: "camera-a",
            layerID: "shared-layer",
            outputID: "main",
            presentation: NativeLiveSourcePresentationLayer(layer: CALayer())
        )
        do {
            try await liveFirst.preload(
                VisualRenderLayer(
                    layerID: "shared-layer",
                    mediaURL: imageURL,
                    contentMode: .fit,
                    opacity: 1,
                    transform: VisualTransformState()
                )
            )
            XCTFail("managed media must not claim a live-source layer ID")
        } catch let failure as VisualRendererFailure {
            XCTAssertEqual(failure.code, "VISUAL_RENDERER_LAYER_CONFLICT")
        }
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
        let routeAfterFailure = await engine.snapshot().routes
        XCTAssertEqual(routeAfterFailure, routeBeforeFailure)

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
        let rerouted = await engine.snapshot()
        XCTAssertEqual(rerouted.routes.count, 1)
        XCTAssertEqual(rerouted.routes.first?.outputID, "main")

        result = await engine.execute(
            capability: LiveSourceCapability.close,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("stream-a"),
            ]
        )
        XCTAssertEqual(result.status, .completed)
        let attachmentAfterClose = await renderer.inspectLiveSourceLayer(
            sourceID: "stream-a",
            layerID: "live-hero"
        )
        XCTAssertNil(attachmentAfterClose)
        await renderer.shutdown()
    }

    private func writePNG(to url: URL) throws {
        guard let bitmap = NSBitmapImageRep(
            bitmapDataPlanes: nil,
            pixelsWide: 8,
            pixelsHigh: 8,
            bitsPerSample: 8,
            samplesPerPixel: 4,
            hasAlpha: true,
            isPlanar: false,
            colorSpaceName: .deviceRGB,
            bytesPerRow: 0,
            bitsPerPixel: 0
        ), let bytes = bitmap.bitmapData else {
            throw NSError(domain: "NativeLiveSourceRendererIntegrationTests", code: 1)
        }
        let byteCount = bitmap.bytesPerRow * bitmap.pixelsHigh
        for index in stride(from: 0, to: byteCount, by: 4) {
            bytes[index] = 0x20
            bytes[index + 1] = 0x80
            bytes[index + 2] = 0xE0
            bytes[index + 3] = 0xFF
        }
        guard let data = bitmap.representation(using: .png, properties: [:]) else {
            throw NSError(domain: "NativeLiveSourceRendererIntegrationTests", code: 2)
        }
        try data.write(to: url, options: .atomic)
    }
}
#endif
