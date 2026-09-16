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
    public var outputID: String
    public var zIndex: Int
}

public struct NativeVisualOutputSnapshot: Sendable, Equatable {
    public var outputID: String
    public var width: Double
    public var height: Double
    public var mapping: VisualQuadState
}

public struct NativeVisualRendererSnapshot: Sendable, Equatable {
    public var blackout: Bool
    public var surfaceWidth: Double
    public var surfaceHeight: Double
    public var outputs: [NativeVisualOutputSnapshot]
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
        var outputID: String
        var zIndex: Int
        var transformState: VisualTransformState
    }

    private struct OutputBacking {
        var renderLayer: CALayer
        var width: Double
        var height: Double
        var mapping: VisualQuadState
    }

    private let surfaceWidth: Double
    private let surfaceHeight: Double
    private let rootLayer: CALayer
    private var blackout = false
    private var layers: [String: LayerBacking] = [:]
    private var outputs: [String: OutputBacking] = [:]

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
        self.rootLayer = root

        let main = CALayer()
        main.bounds = bounds
        main.anchorPoint = CGPoint(x: 0, y: 0)
        main.position = .zero
        main.masksToBounds = true
        main.isHidden = false
        main.transform = try makeNativeVisualPerspectiveTransform(
            width: surfaceWidth,
            height: surfaceHeight,
            mapping: .identity
        )
        root.addSublayer(main)
        self.outputs["main"] = OutputBacking(
            renderLayer: main,
            width: surfaceWidth,
            height: surfaceHeight,
            mapping: .identity
        )
    }

    public func preload(_ layer: VisualRenderLayer) async throws {
        guard layer.mediaURL.isFileURL,
              FileManager.default.fileExists(atPath: layer.mediaURL.path) else {
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_MEDIA_UNAVAILABLE",
                summary: "verified media object is unavailable to the native renderer"
            )
        }
        guard let output = outputs[layer.outputID] else {
            throw missingOutput()
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
            applyAppearance(layer, to: renderLayer, output: output)
            output.renderLayer.addSublayer(renderLayer)
            layers[layer.layerID] = LayerBacking(
                renderLayer: renderLayer,
                mediaKind: .image,
                contentMode: layer.contentMode,
                player: nil,
                loopFlag: nil,
                endObserver: nil,
                outputID: layer.outputID,
                zIndex: layer.zIndex,
                transformState: layer.transform
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
        applyAppearance(layer, to: playerLayer, output: output)
        output.renderLayer.addSublayer(playerLayer)

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
            endObserver: observer,
            outputID: layer.outputID,
            zIndex: layer.zIndex,
            transformState: layer.transform
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
        for output in outputs.values {
            output.renderLayer.isHidden = enabled
        }
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
        guard var layer = layers[layerID] else { throw missingLayer() }
        guard let output = outputs[layer.outputID] else { throw missingOutput() }
        applyTransform(transform, to: layer.renderLayer, output: output)
        layer.transformState = transform
        layers[layerID] = layer
    }

    public func configureOutput(_ output: VisualOutputState) async throws {
        guard output.width.isFinite, output.height.isFinite,
              output.width >= 1, output.width <= 16_384,
              output.height >= 1, output.height <= 16_384,
              output.mapping.isValidProjection else {
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_OUTPUT_INVALID",
                summary: "native output configuration is invalid"
            )
        }
        let transform = try makeNativeVisualPerspectiveTransform(
            width: output.width,
            height: output.height,
            mapping: output.mapping
        )
        let bounds = CGRect(x: 0, y: 0, width: output.width, height: output.height)

        if var existing = outputs[output.outputID] {
            existing.width = output.width
            existing.height = output.height
            existing.mapping = output.mapping
            existing.renderLayer.bounds = bounds
            existing.renderLayer.anchorPoint = CGPoint(x: 0, y: 0)
            existing.renderLayer.position = .zero
            existing.renderLayer.transform = transform
            existing.renderLayer.isHidden = blackout
            outputs[output.outputID] = existing
            refreshLayers(on: output.outputID)
            return
        }

        let renderLayer = CALayer()
        renderLayer.bounds = bounds
        renderLayer.anchorPoint = CGPoint(x: 0, y: 0)
        renderLayer.position = .zero
        renderLayer.masksToBounds = true
        renderLayer.isHidden = blackout
        renderLayer.transform = transform
        rootLayer.addSublayer(renderLayer)
        outputs[output.outputID] = OutputBacking(
            renderLayer: renderLayer,
            width: output.width,
            height: output.height,
            mapping: output.mapping
        )
    }

    public func setLayerOutput(layerID: String, outputID: String) async throws {
        guard var layer = layers[layerID] else { throw missingLayer() }
        guard let output = outputs[outputID] else { throw missingOutput() }
        if layer.outputID == outputID { return }
        output.renderLayer.addSublayer(layer.renderLayer)
        layer.outputID = outputID
        layer.renderLayer.bounds = output.renderLayer.bounds
        applyTransform(layer.transformState, to: layer.renderLayer, output: output)
        layers[layerID] = layer
    }

    public func setLayerOrder(layerID: String, zIndex: Int) async throws {
        guard var layer = layers[layerID] else { throw missingLayer() }
        guard (-4096...4096).contains(zIndex) else {
            throw VisualRendererFailure(
                code: "VISUAL_RENDERER_ORDER_INVALID",
                summary: "native layer order is outside supported range"
            )
        }
        layer.zIndex = zIndex
        layer.renderLayer.zPosition = CGFloat(zIndex)
        layers[layerID] = layer
    }

    public func setOutputMapping(outputID: String, mapping: VisualQuadState) async throws {
        guard var output = outputs[outputID] else { throw missingOutput() }
        output.renderLayer.transform = try makeNativeVisualPerspectiveTransform(
            width: output.width,
            height: output.height,
            mapping: mapping
        )
        output.mapping = mapping
        outputs[outputID] = output
    }

    public func snapshot() -> NativeVisualRendererSnapshot {
        let layerSnapshots = layers.map { layerID, layer in
            NativeVisualLayerSnapshot(
                layerID: layerID,
                mediaKind: layer.mediaKind,
                contentMode: layer.contentMode,
                loopEnabled: layer.loopFlag?.value() ?? false,
                opacity: Double(layer.renderLayer.opacity),
                isPlaying: (layer.player?.rate ?? 0) != 0,
                outputID: layer.outputID,
                zIndex: layer.zIndex
            )
        }.sorted {
            if $0.outputID != $1.outputID { return $0.outputID < $1.outputID }
            if $0.zIndex != $1.zIndex { return $0.zIndex < $1.zIndex }
            return $0.layerID < $1.layerID
        }
        let outputSnapshots = outputs.map { outputID, output in
            NativeVisualOutputSnapshot(
                outputID: outputID,
                width: output.width,
                height: output.height,
                mapping: output.mapping
            )
        }.sorted { $0.outputID < $1.outputID }
        return NativeVisualRendererSnapshot(
            blackout: blackout,
            surfaceWidth: surfaceWidth,
            surfaceHeight: surfaceHeight,
            outputs: outputSnapshots,
            layers: layerSnapshots
        )
    }

    public func shutdown() async {
        for layer in layers.values {
            cleanup(layer)
        }
        layers.removeAll()
        for output in outputs.values {
            output.renderLayer.removeFromSuperlayer()
        }
        outputs.removeAll()

        let bounds = CGRect(x: 0, y: 0, width: surfaceWidth, height: surfaceHeight)
        let main = CALayer()
        main.bounds = bounds
        main.anchorPoint = CGPoint(x: 0, y: 0)
        main.position = .zero
        main.masksToBounds = true
        main.isHidden = false
        main.transform = (try? makeNativeVisualPerspectiveTransform(
            width: surfaceWidth,
            height: surfaceHeight,
            mapping: .identity
        )) ?? CATransform3DIdentity
        rootLayer.addSublayer(main)
        outputs["main"] = OutputBacking(
            renderLayer: main,
            width: surfaceWidth,
            height: surfaceHeight,
            mapping: .identity
        )
        blackout = false
    }

    private func refreshLayers(on outputID: String) {
        guard let output = outputs[outputID] else { return }
        let affected = layers.keys.filter { layers[$0]?.outputID == outputID }
        for layerID in affected {
            guard var layer = layers[layerID] else { continue }
            layer.renderLayer.bounds = output.renderLayer.bounds
            applyTransform(layer.transformState, to: layer.renderLayer, output: output)
            layer.renderLayer.zPosition = CGFloat(layer.zIndex)
            layers[layerID] = layer
        }
    }

    private func applyAppearance(_ source: VisualRenderLayer, to layer: CALayer, output: OutputBacking) {
        layer.bounds = output.renderLayer.bounds
        layer.opacity = Float(source.opacity)
        layer.zPosition = CGFloat(source.zIndex)
        applyTransform(source.transform, to: layer, output: output)
    }

    private func applyTransform(
        _ transform: VisualTransformState,
        to layer: CALayer,
        output: OutputBacking
    ) {
        layer.position = CGPoint(
            x: output.renderLayer.bounds.midX + transform.x,
            y: output.renderLayer.bounds.midY + transform.y
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

    private func missingOutput() -> VisualRendererFailure {
        VisualRendererFailure(
            code: "VISUAL_RENDERER_OUTPUT_NOT_CONFIGURED",
            summary: "native renderer output is not configured"
        )
    }
}
#endif
