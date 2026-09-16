#if os(macOS)
import QuartzCore

public final class NativeLiveSourcePresentationLayer: @unchecked Sendable {
    let layer: CALayer

    public init(layer: CALayer) {
        self.layer = layer
    }
}

public protocol NativeLiveSourceRenderSurface: Sendable {
    func attachLiveSourceLayer(
        sourceID: String,
        layerID: String,
        outputID: String,
        presentation: NativeLiveSourcePresentationLayer
    ) async throws

    func detachLiveSourceLayer(sourceID: String, layerID: String) async
    func detachLiveSourceLayers(sourceID: String) async
    func inspectLiveSourceLayer(sourceID: String, layerID: String) async -> NativeLiveSourceLayerAttachment?
}

public struct NativeLiveSourceLayerAttachment: Sendable, Equatable {
    public var sourceID: String
    public var layerID: String
    public var outputID: String

    public init(sourceID: String, layerID: String, outputID: String) {
        self.sourceID = sourceID
        self.layerID = layerID
        self.outputID = outputID
    }
}
#endif
