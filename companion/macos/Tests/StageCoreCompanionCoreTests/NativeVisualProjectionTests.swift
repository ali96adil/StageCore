#if os(macOS)
import AppKit
import XCTest
@testable import StageCoreCompanionCore

final class NativeVisualProjectionTests: XCTestCase {
    func testNativeRendererSupportsNamedOutputsOrderingAndReassignment() async throws {
        let directory = FileManager.default.temporaryDirectory
            .appendingPathComponent("stagecore-projection-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let imageURL = directory.appendingPathComponent("frame.png")
        try writePNG(to: imageURL)

        let renderer = try NativeVisualRenderer(surfaceWidth: 640, surfaceHeight: 360)
        let mapping = VisualQuadState(
            topLeft: .init(x: 0.02, y: 0.03),
            topRight: .init(x: 0.98, y: 0.01),
            bottomRight: .init(x: 0.96, y: 0.97),
            bottomLeft: .init(x: 0.04, y: 0.99)
        )
        try await renderer.configureOutput(
            VisualOutputState(outputID: "projector-a", width: 800, height: 600, mapping: mapping)
        )
        try await renderer.preload(
            VisualRenderLayer(
                layerID: "mapped",
                mediaURL: imageURL,
                contentMode: .fit,
                opacity: 0.8,
                transform: .init(x: 4, y: 5, scaleX: 1, scaleY: 1, rotationDegrees: 0),
                outputID: "projector-a",
                zIndex: 10
            )
        )
        try await renderer.preload(
            VisualRenderLayer(
                layerID: "main-layer",
                mediaURL: imageURL,
                contentMode: .crop,
                opacity: 1,
                transform: .init(),
                outputID: "main",
                zIndex: -5
            )
        )

        var snapshot = await renderer.snapshot()
        XCTAssertEqual(snapshot.outputs.map(\.outputID), ["main", "projector-a"])
        let projector = try XCTUnwrap(snapshot.outputs.first { $0.outputID == "projector-a" })
        XCTAssertEqual(projector.width, 800)
        XCTAssertEqual(projector.height, 600)
        XCTAssertEqual(projector.mapping, mapping)
        let mapped = try XCTUnwrap(snapshot.layers.first { $0.layerID == "mapped" })
        XCTAssertEqual(mapped.outputID, "projector-a")
        XCTAssertEqual(mapped.zIndex, 10)

        try await renderer.setLayerOrder(layerID: "mapped", zIndex: -20)
        try await renderer.setLayerOutput(layerID: "mapped", outputID: "main")
        snapshot = await renderer.snapshot()
        let moved = try XCTUnwrap(snapshot.layers.first { $0.layerID == "mapped" })
        XCTAssertEqual(moved.outputID, "main")
        XCTAssertEqual(moved.zIndex, -20)

        try await renderer.setBlackout(true)
        let blackoutSnapshot = await renderer.snapshot()
        XCTAssertTrue(blackoutSnapshot.blackout)
        await renderer.shutdown()
        let reset = await renderer.snapshot()
        XCTAssertEqual(reset.outputs.map(\.outputID), ["main"])
        XCTAssertTrue(reset.layers.isEmpty)
        XCTAssertFalse(reset.blackout)
    }

    func testPerspectiveTransformRejectsDegenerateMapping() throws {
        let invalid = VisualQuadState(
            topLeft: .init(x: 0, y: 0),
            topRight: .init(x: 1, y: 0),
            bottomRight: .init(x: 1, y: 0),
            bottomLeft: .init(x: 0, y: 0)
        )
        XCTAssertThrowsError(
            try makeNativeVisualPerspectiveTransform(width: 1920, height: 1080, mapping: invalid)
        ) { error in
            guard let failure = error as? VisualRendererFailure else {
                return XCTFail("unexpected error \(error)")
            }
            XCTAssertEqual(failure.code, "VISUAL_RENDERER_MAPPING_INVALID")
        }
    }

    func testRendererRejectsUnknownOutputBeforeLayerInsertion() async throws {
        let directory = FileManager.default.temporaryDirectory
            .appendingPathComponent("stagecore-projection-missing-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let imageURL = directory.appendingPathComponent("frame.png")
        try writePNG(to: imageURL)
        let renderer = try NativeVisualRenderer()

        do {
            try await renderer.preload(
                VisualRenderLayer(
                    layerID: "orphan",
                    mediaURL: imageURL,
                    contentMode: .fit,
                    opacity: 1,
                    transform: .init(),
                    outputID: "missing",
                    zIndex: 0
                )
            )
            XCTFail("unknown output must fail")
        } catch let failure as VisualRendererFailure {
            XCTAssertEqual(failure.code, "VISUAL_RENDERER_OUTPUT_NOT_CONFIGURED")
        }
        let snapshot = await renderer.snapshot()
        XCTAssertTrue(snapshot.layers.isEmpty)
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
            throw NSError(domain: "NativeVisualProjectionTests", code: 1)
        }
        let byteCount = bitmap.bytesPerRow * bitmap.pixelsHigh
        for index in stride(from: 0, to: byteCount, by: 4) {
            bytes[index] = 0x80
            bytes[index + 1] = 0x30
            bytes[index + 2] = 0xD0
            bytes[index + 3] = 0xFF
        }
        guard let data = bitmap.representation(using: .png, properties: [:]) else {
            throw NSError(domain: "NativeVisualProjectionTests", code: 2)
        }
        try data.write(to: url, options: .atomic)
    }
}
#endif
