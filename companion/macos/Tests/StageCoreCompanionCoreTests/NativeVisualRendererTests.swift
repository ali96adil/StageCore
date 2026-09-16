#if os(macOS)
import AppKit
@preconcurrency import AVFoundation
import CoreVideo
import XCTest
@testable import StageCoreCompanionCore

final class NativeVisualRendererTests: XCTestCase {
    func testImagePreloadContentModeOpacityAndBlackout() async throws {
        let directory = try temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let imageURL = directory.appendingPathComponent("frame.png")
        try writePNG(to: imageURL)

        let renderer = try NativeVisualRenderer(surfaceWidth: 640, surfaceHeight: 360)
        try await renderer.preload(
            VisualRenderLayer(
                layerID: "image",
                mediaURL: imageURL,
                contentMode: .crop,
                opacity: 0.75,
                transform: VisualTransformState(x: 12, y: -8, scaleX: 1.2, scaleY: 0.9, rotationDegrees: 4)
            )
        )

        var snapshot = await renderer.snapshot()
        XCTAssertFalse(snapshot.blackout)
        XCTAssertEqual(snapshot.surfaceWidth, 640)
        XCTAssertEqual(snapshot.surfaceHeight, 360)
        XCTAssertEqual(snapshot.layers.count, 1)
        XCTAssertEqual(snapshot.layers[0].layerID, "image")
        XCTAssertEqual(snapshot.layers[0].mediaKind, .image)
        XCTAssertEqual(snapshot.layers[0].contentMode, .crop)
        XCTAssertEqual(snapshot.layers[0].opacity, 0.75, accuracy: 0.001)

        try await renderer.setOpacity(layerID: "image", opacity: 0.4)
        try await renderer.setBlackout(true)
        snapshot = await renderer.snapshot()
        XCTAssertTrue(snapshot.blackout)
        XCTAssertEqual(snapshot.layers[0].opacity, 0.4, accuracy: 0.001)

        do {
            try await renderer.seek(layerID: "image", positionMS: 100)
            XCTFail("static image non-zero seek must fail")
        } catch let failure as VisualRendererFailure {
            XCTAssertEqual(failure.code, "VISUAL_RENDERER_OPERATION_UNSUPPORTED")
        }

        await renderer.shutdown()
        snapshot = await renderer.snapshot()
        XCTAssertFalse(snapshot.blackout)
        XCTAssertTrue(snapshot.layers.isEmpty)
    }

    func testVideoPreloadPlaybackSeekLoopAndStop() async throws {
        let directory = try temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let videoURL = directory.appendingPathComponent("clip.mov")
        try await writeTinyVideo(to: videoURL)

        let renderer = try NativeVisualRenderer(surfaceWidth: 640, surfaceHeight: 360)
        try await renderer.preload(
            VisualRenderLayer(
                layerID: "video",
                mediaURL: videoURL,
                contentMode: .fit,
                opacity: 1,
                transform: VisualTransformState()
            )
        )
        var snapshot = await renderer.snapshot()
        XCTAssertEqual(snapshot.layers.count, 1)
        XCTAssertEqual(snapshot.layers[0].mediaKind, .video)
        XCTAssertEqual(snapshot.layers[0].contentMode, .fit)

        try await renderer.setLoop(layerID: "video", enabled: true)
        try await renderer.seek(layerID: "video", positionMS: 50)
        try await renderer.play(layerID: "video")
        try await Task.sleep(for: .milliseconds(20))
        try await renderer.pause(layerID: "video")
        try await renderer.stop(layerID: "video")

        snapshot = await renderer.snapshot()
        XCTAssertTrue(snapshot.layers[0].loopEnabled)
        XCTAssertFalse(snapshot.layers[0].isPlaying)
        await renderer.shutdown()
    }

    func testMissingNativeMediaFailsClosed() async throws {
        let renderer = try NativeVisualRenderer()
        do {
            try await renderer.preload(
                VisualRenderLayer(
                    layerID: "missing",
                    mediaURL: URL(fileURLWithPath: "/definitely/not/stagecore/media.mov"),
                    contentMode: .fit,
                    opacity: 1,
                    transform: VisualTransformState()
                )
            )
            XCTFail("missing native media must fail")
        } catch let failure as VisualRendererFailure {
            XCTAssertEqual(failure.code, "VISUAL_RENDERER_MEDIA_UNAVAILABLE")
        }
        let snapshot = await renderer.snapshot()
        XCTAssertTrue(snapshot.layers.isEmpty)
    }

    private func temporaryDirectory() throws -> URL {
        let url = FileManager.default.temporaryDirectory
            .appendingPathComponent("stagecore-native-visual-\(UUID().uuidString)", isDirectory: true)
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
            throw NSError(domain: "NativeVisualRendererTests", code: 1)
        }
        let byteCount = bitmap.bytesPerRow * bitmap.pixelsHigh
        for index in stride(from: 0, to: byteCount, by: 4) {
            bytes[index] = 0x20
            bytes[index + 1] = 0x80
            bytes[index + 2] = 0xE0
            bytes[index + 3] = 0xFF
        }
        guard let data = bitmap.representation(using: .png, properties: [:]) else {
            throw NSError(domain: "NativeVisualRendererTests", code: 2)
        }
        try data.write(to: url, options: .atomic)
    }

    private func writeTinyVideo(to url: URL) async throws {
        let writer = try AVAssetWriter(outputURL: url, fileType: .mov)
        let settings: [String: Any] = [
            AVVideoCodecKey: AVVideoCodecType.h264,
            AVVideoWidthKey: 32,
            AVVideoHeightKey: 32,
        ]
        let input = AVAssetWriterInput(mediaType: .video, outputSettings: settings)
        input.expectsMediaDataInRealTime = false
        let adaptor = AVAssetWriterInputPixelBufferAdaptor(
            assetWriterInput: input,
            sourcePixelBufferAttributes: [
                kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_32BGRA,
                kCVPixelBufferWidthKey as String: 32,
                kCVPixelBufferHeightKey as String: 32,
            ]
        )
        guard writer.canAdd(input) else {
            throw NSError(domain: "NativeVisualRendererTests", code: 3)
        }
        writer.add(input)
        guard writer.startWriting() else {
            throw writer.error ?? NSError(domain: "NativeVisualRendererTests", code: 4)
        }
        writer.startSession(atSourceTime: .zero)

        for frame in 0..<3 {
            while !input.isReadyForMoreMediaData {
                try await Task.sleep(for: .milliseconds(1))
            }
            guard let pool = adaptor.pixelBufferPool else {
                throw NSError(domain: "NativeVisualRendererTests", code: 5)
            }
            var optionalBuffer: CVPixelBuffer?
            guard CVPixelBufferPoolCreatePixelBuffer(nil, pool, &optionalBuffer) == kCVReturnSuccess,
                  let buffer = optionalBuffer else {
                throw NSError(domain: "NativeVisualRendererTests", code: 6)
            }
            CVPixelBufferLockBaseAddress(buffer, [])
            if let base = CVPixelBufferGetBaseAddress(buffer) {
                let fillByte = Int32(frame * 60)
                memset(base, fillByte, CVPixelBufferGetDataSize(buffer))
            }
            CVPixelBufferUnlockBaseAddress(buffer, [])
            guard adaptor.append(buffer, withPresentationTime: CMTime(value: CMTimeValue(frame), timescale: 10)) else {
                throw writer.error ?? NSError(domain: "NativeVisualRendererTests", code: 7)
            }
        }
        input.markAsFinished()
        await withCheckedContinuation { continuation in
            writer.finishWriting {
                continuation.resume()
            }
        }
        guard writer.status == .completed else {
            throw writer.error ?? NSError(domain: "NativeVisualRendererTests", code: 8)
        }
    }
}
#endif
