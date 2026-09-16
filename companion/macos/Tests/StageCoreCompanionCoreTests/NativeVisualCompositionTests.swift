#if os(macOS)
import AppKit
import XCTest
@testable import StageCoreCompanionCore

final class NativeVisualCompositionTests: XCTestCase {
    func testNativeCropMaskAndEffectState() async throws {
        let directory = try temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let imageURL = directory.appendingPathComponent("frame.png")
        try writePNG(to: imageURL)

        let renderer = try NativeVisualRenderer(surfaceWidth: 640, surfaceHeight: 360)
        try await renderer.preload(
            VisualRenderLayer(
                layerID: "hero",
                mediaURL: imageURL,
                contentMode: .fit,
                opacity: 1,
                transform: VisualTransformState()
            )
        )
        let state = VisualLayerCompositionState(
            layerID: "hero",
            crop: VisualNormalizedRect(x: 0.1, y: 0.15, width: 0.7, height: 0.65),
            mask: .ellipse,
            effect: VisualEffectState(brightness: 0.15, contrast: 1.25, saturation: 0.7)
        )
        try await renderer.applyComposition(layerID: "hero", state: state)

        let snapshot = await renderer.snapshot()
        let layer = try XCTUnwrap(snapshot.layers.first)
        XCTAssertEqual(layer.crop, state.crop)
        XCTAssertEqual(layer.mask, .ellipse)
        XCTAssertEqual(layer.effect, state.effect)
    }

    func testNativeCutAndCrossfadeCommitFinalModelOpacity() async throws {
        let directory = try temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let imageURL = directory.appendingPathComponent("frame.png")
        try writePNG(to: imageURL)

        let renderer = try NativeVisualRenderer(surfaceWidth: 640, surfaceHeight: 360)
        try await renderer.preload(
            VisualRenderLayer(
                layerID: "a",
                mediaURL: imageURL,
                contentMode: .fit,
                opacity: 0.8,
                transform: VisualTransformState(),
                zIndex: 0
            )
        )
        try await renderer.preload(
            VisualRenderLayer(
                layerID: "b",
                mediaURL: imageURL,
                contentMode: .fit,
                opacity: 0,
                transform: VisualTransformState(),
                zIndex: 1
            )
        )

        try await renderer.transition(
            VisualTransitionRenderState(
                kind: .crossfade,
                fromLayerID: "a",
                toLayerID: "b",
                durationMS: 250,
                fromOpacity: 0.8,
                targetOpacity: 0.9
            )
        )
        var snapshot = await renderer.snapshot()
        let afterCrossfadeA = try XCTUnwrap(snapshot.layers.first { $0.layerID == "a" })
        let afterCrossfadeB = try XCTUnwrap(snapshot.layers.first { $0.layerID == "b" })
        XCTAssertEqual(afterCrossfadeA.opacity, 0)
        XCTAssertEqual(afterCrossfadeB.opacity, 0.9, accuracy: 0.001)

        try await renderer.transition(
            VisualTransitionRenderState(
                kind: .cut,
                fromLayerID: "b",
                toLayerID: "a",
                durationMS: 0,
                fromOpacity: 0.9,
                targetOpacity: 0.6
            )
        )
        snapshot = await renderer.snapshot()
        let afterCutA = try XCTUnwrap(snapshot.layers.first { $0.layerID == "a" })
        let afterCutB = try XCTUnwrap(snapshot.layers.first { $0.layerID == "b" })
        XCTAssertEqual(afterCutB.opacity, 0)
        XCTAssertEqual(afterCutA.opacity, 0.6, accuracy: 0.001)
    }

    private func temporaryDirectory() throws -> URL {
        let url = FileManager.default.temporaryDirectory
            .appendingPathComponent("stagecore-native-composition-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        return url
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
            throw NSError(domain: "NativeVisualCompositionTests", code: 1)
        }
        let byteCount = bitmap.bytesPerRow * bitmap.pixelsHigh
        for index in stride(from: 0, to: byteCount, by: 4) {
            bytes[index] = 0x40
            bytes[index + 1] = 0x90
            bytes[index + 2] = 0xD0
            bytes[index + 3] = 0xFF
        }
        guard let data = bitmap.representation(using: .png, properties: [:]) else {
            throw NSError(domain: "NativeVisualCompositionTests", code: 2)
        }
        try data.write(to: url, options: .atomic)
    }
}
#endif
