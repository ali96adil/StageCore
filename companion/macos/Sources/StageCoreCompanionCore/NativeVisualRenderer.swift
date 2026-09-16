#if os(macOS)
import AppKit
@preconcurrency import AVFoundation
import Foundation
import QuartzCore

public enum NativeVisualMediaKind: String, Sendable, Equatable {
    case image = "IMAGE"
    case video = "VIDEO"
}

public struct NativeVisualLayerSnapshot: Sendable, Equatable {
    public var layerID: String
    public var mediaKind: NativeVisualMediaKind
    public var contentMode: VisualContentMode
    public var loopEnabled: Bool
    public var opacity: Double
    public var isPlaying: Bool
}

public struct NativeVisualRendererSnapshot: Sendable, Equatable {
    public var blackout: Bool
    public var surfaceWidth: Double
    public var surfaceHeight: Double
    public var layers: [NativeVisualLayerSnapshot]
}

private final class NativeVisualLoopFlag: @unchecked Sendable {
    private let lock = NSLock()
    private var enabled = false

    func set(_ value: Bool) {
        lock.lock()
        enabled = value
        lock.unlock()
    }

    func value() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return enabled
    }
}

public actor NativeVisualRenderer: VisualRenderer {
    private struct LayerBacking {
        var renderLayer: CALayer
        var mediaKind: NativeVisualMediaKind
        var contentMode: VisualContentMode
        var player: AVPlayer?
        var loopFlag: NativeVisualLoopFlag?
        var endObserver: NSObjectProtocol?
    }

    private let surfaceWidth: Double
    private let surfaceHeight: Double
    private let rootLayer: CALayer
    private let contentRoot: CALayer
    private var blackout = false
    private var layers: [String: LayerBacking] = [:]

    public init(surfaceWidth: Double = 1920, surfaceHeight: Double = 1080) throws {
        guard surfaceWidth.isFinite, surfaceHeight.isFinite,
              surfaceWidth > 0, surfaceHeight > 0 else {
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_CONFIGURATION_INVALID",
                summary: "native render surface dimensions must be positive and finite"
            )
        }
        self.surfaceWidth = surfaceWidth
        self.surfaceHeight = surfaceHeight
        let bounds = CGRect(x: 0, y: 0, width: surfaceWidth, height: surfaceHeight)
        let root = CALayer()
        root.bounds = bounds
        root.position = CGPoint(x: bounds.midX, y: bounds.midY)
        root.backgroundColor = NSColor.black.cgColor
        root.masksToBounds = true
        let content = CALayer()
        content.frame = bounds
        content.masksToBounds = true
        root.addSublayer(content)
        self.rootLayer = root
        self.contentRoot = content
    }

    public func preload(_ layer: VisualRenderLayer) async throws {
        guard layer.mediaURL.isFileURL,
              FileManager.default.fileExists(atPath: layer.mediaURL.path) else {
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_MEDIA_UNAVAILABLE",
                summary: "verified media object is unavailable to the native renderer"
            )
        }

        if let previous = layers.removeValue(forKey: layer.layerID) {
            cleanup(previous)
        }

        if let image = NSImage(contentsOf: layer.mediaURL),
           let cgImage = image.cgImage(forProposedRect: nil, context: nil, hints: nil) {
            let renderLayer = CALayer()
            renderLayer.contents = cgImage
            renderLayer.contentsScale = 1
            applyContentMode(layer.contentMode, to: renderLayer)
            applyAppearance(layer, to: renderLayer)
            contentRoot.addSublayer(renderLayer)
            layers[layer.layerID] = LayerBacking(
                renderLayer: renderLayer,
                mediaKind: .image,
                contentMode: layer.contentMode,
                player: nil,
                loopFlag: nil,
                endObserver: nil
            )
            return
        }

        let asset = AVURLAsset(url: layer.mediaURL)
        let tracks: [AVAssetTrack]
        do {
            tracks = try await asset.loadTracks(withMediaType: .video)
        } catch {
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_MEDIA_UNSUPPORTED",
                summary: "managed media could not be loaded as image or video"
            )
        }
        guard !tracks.isEmpty else {
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_MEDIA_UNSUPPORTED",
                summary: "managed media contains no native image or video track"
            )
        }

        let item = AVPlayerItem(asset: asset)
        let player = AVPlayer(playerItem: item)
        player.actionAtItemEnd = .pause
        let playerLayer = AVPlayerLayer(player: player)
        applyContentMode(layer.contentMode, to: playerLayer)
        applyAppearance(layer, to: playerLayer)
        contentRoot.addSublayer(playerLayer)

        let loopFlag = NativeVisualLoopFlag()
        let observer = NotificationCenter.default.addObserver(
            forName: .AVPlayerItemDidPlayToEndTime,
            object: item,
            queue: nil
        ) { [weak player] _ in
            guard loopFlag.value(), let player else { return }
            player.seek(to: .zero, toleranceBefore: .zero, toleranceAfter: .zero)
            player.play()
        }
        layers[layer.layerID] = LayerBacking(
            renderLayer: playerLayer,
            mediaKind: .video,
            contentMode: layer.contentMode,
            player: player,
            loopFlag: loopFlag,
            endObserver: observer
        )
    }

    public func play(layerID: String) async throws {
        guard let layer = layers[layerID] else { throw missingLayer() }
        layer.player?.play()
    }

    public func pause(layerID: String) async throws {
        guard let layer = layers[layerID] else { throw missingLayer() }
        layer.player?.pause()
    }

    public func stop(layerID: String) async throws {
        guard let layer = layers[layerID] else { throw missingLayer() }
        if let player = layer.player {
            player.pause()
            await player.seek(to: .zero, toleranceBefore: .zero, toleranceAfter: .zero)
        }
    }

    public func seek(layerID: String, positionMS: Int64) async throws {
        guard let layer = layers[layerID] else { throw missingLayer() }
        guard positionMS >= 0 else {
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_SEEK_INVALID",
                summary: "native renderer seek position cannot be negative"
            )
        }
        guard let player = layer.player else {
            if positionMS == 0 { return }
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_OPERATION_UNSUPPORTED",
                summary: "static image layers do not support non-zero seek"
            )
        }
        let time = CMTime(value: positionMS, timescale: 1000)
        await player.seek(to: time, toleranceBefore: .zero, toleranceAfter: .zero)
    }

    public func setLoop(layerID: String, enabled: Bool) async throws {
        guard let layer = layers[layerID] else { throw missingLayer() }
        guard let flag = layer.loopFlag else {
            if !enabled { return }
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_OPERATION_UNSUPPORTED",
                summary: "static image layers do not require playback looping"
            )
        }
        flag.set(enabled)
    }

    public func setBlackout(_ enabled: Bool) async throws {
        blackout = enabled
        contentRoot.isHidden = enabled
    }

    public func setOpacity(layerID: String, opacity: Double) async throws {
        guard let layer = layers[layerID] else { throw missingLayer() }
        guard opacity.isFinite, opacity >= 0, opacity <= 1 else {
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_OPACITY_INVALID",
                summary: "native renderer opacity must be between zero and one"
            )
        }
        layer.renderLayer.opacity = Float(opacity)
    }

    public func setTransform(layerID: String, transform: VisualTransformState) async throws {
        guard let layer = layers[layerID] else { throw missingLayer() }
        applyTransform(transform, to: layer.renderLayer)
    }

    public func snapshot() -> NativeVisualRendererSnapshot {
        let result = layers.map { layerID, layer in
            NativeVisualLayerSnapshot(
                layerID: layerID,
                mediaKind: layer.mediaKind,
                contentMode: layer.contentMode,
                loopEnabled: layer.loopFlag?.value() ?? false,
                opacity: Double(layer.renderLayer.opacity),
                isPlaying: (layer.player?.rate ?? 0) != 0
            )
        }.sorted { $0.layerID < $1.layerID }
        return NativeVisualRendererSnapshot(
            blackout: blackout,
            surfaceWidth: surfaceWidth,
            surfaceHeight: surfaceHeight,
            layers: result
        )
    }

    public func shutdown() async {
        for layer in layers.values {
            cleanup(layer)
        }
        layers.removeAll()
        contentRoot.sublayers?.forEach { $0.removeFromSuperlayer() }
        contentRoot.isHidden = false
        blackout = false
    }

    private func applyAppearance(_ source: VisualRenderLayer, to layer: CALayer) {
        layer.bounds = contentRoot.bounds
        layer.position = CGPoint(x: contentRoot.bounds.midX, y: contentRoot.bounds.midY)
        layer.opacity = Float(source.opacity)
        applyTransform(source.transform, to: layer)
    }

    private func applyTransform(_ transform: VisualTransformState, to layer: CALayer) {
        layer.position = CGPoint(
            x: contentRoot.bounds.midX + transform.x,
            y: contentRoot.bounds.midY + transform.y
        )
        var affine = CGAffineTransform.identity
        affine = affine.scaledBy(x: transform.scaleX, y: transform.scaleY)
        affine = affine.rotated(by: transform.rotationDegrees * .pi / 180)
        layer.setAffineTransform(affine)
    }

    private func applyContentMode(_ mode: VisualContentMode, to layer: CALayer) {
        switch mode {
        case .fit: layer.contentsGravity = .resizeAspect
        case .fill: layer.contentsGravity = .resize
        case .crop: layer.contentsGravity = .resizeAspectFill
        }
    }

    private func applyContentMode(_ mode: VisualContentMode, to layer: AVPlayerLayer) {
        switch mode {
        case .fit: layer.videoGravity = .resizeAspect
        case .fill: layer.videoGravity = .resize
        case .crop: layer.videoGravity = .resizeAspectFill
        }
    }

    private func cleanup(_ layer: LayerBacking) {
        layer.player?.pause()
        if let observer = layer.endObserver {
            NotificationCenter.default.removeObserver(observer)
        }
        layer.renderLayer.removeFromSuperlayer()
    }

    private func missingLayer() -> VisualRendererFailure {
        VisualRendererFailure(
            code: "VISUAL_RENDERER_LAYER_NOT_PRELOADED",
            summary: "native renderer layer is not preloaded"
        )
    }
}
#endif
