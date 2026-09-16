#if os(macOS)
import AppKit
@preconcurrency import AVFoundation
import Foundation

public struct LiveSourceRuntimeFailure: Error, Sendable, Equatable {
    public var code: String
    public var summary: String

    public init(code: String, summary: String) {
        self.code = code
        self.summary = summary
    }
}

public protocol LiveSourceRuntimeAdapter: Sendable {
    func open(_ descriptor: LiveSourceRuntimeDescriptor) async throws
    func close(sourceID: String) async throws
    func select(sourceID: String) async throws
    func route(_ route: LiveSourceRouteState) async throws
    func inspect(sourceID: String) async -> [String: JSONValue]
    func shutdown() async
}

public extension LiveSourceRuntimeAdapter {
    func inspect(sourceID: String) async -> [String: JSONValue] { [:] }
}

/// State-only adapter retained for contract tests. Production bootstrap wires
/// NativeLiveSourceRuntime when native Visual Engine support is enabled.
public actor StateOnlyLiveSourceRuntime: LiveSourceRuntimeAdapter {
    public init() {}
    public func open(_ descriptor: LiveSourceRuntimeDescriptor) async throws {}
    public func close(sourceID: String) async throws {}
    public func select(sourceID: String) async throws {}
    public func route(_ route: LiveSourceRouteState) async throws {}
    public func shutdown() async {}
}

/// Native capture/open runtime for F-007 LiveSource descriptors. Video frames
/// remain entirely on the Companion. The Hub sends only descriptor/routing
/// intent and never transports frame payloads.
public actor NativeLiveSourceRuntime: LiveSourceRuntimeAdapter {
    private struct CameraBacking {
        var session: AVCaptureSession
        var previewLayer: AVCaptureVideoPreviewLayer
    }

    private struct StreamBacking {
        var player: AVPlayer
        var playerLayer: AVPlayerLayer
    }

    private enum Backing {
        case camera(CameraBacking)
        case stream(StreamBacking)
    }

    public struct Snapshot: Sendable, Equatable {
        public var openSourceIDs: [String]
        public var selectedSourceID: String?
        public var routes: [LiveSourceRouteState]

        public init(openSourceIDs: [String], selectedSourceID: String?, routes: [LiveSourceRouteState]) {
            self.openSourceIDs = openSourceIDs
            self.selectedSourceID = selectedSourceID
            self.routes = routes
        }
    }

    private let renderSurface: (any NativeLiveSourceRenderSurface)?
    private var backings: [String: Backing] = [:]
    private var selectedSourceID: String?
    private var routes: [String: LiveSourceRouteState] = [:]

    public init(renderSurface: (any NativeLiveSourceRenderSurface)? = nil) {
        self.renderSurface = renderSurface
    }

    public func snapshot() -> Snapshot {
        Snapshot(
            openSourceIDs: backings.keys.sorted(),
            selectedSourceID: selectedSourceID,
            routes: routes.values.sorted {
                if $0.outputID != $1.outputID { return $0.outputID < $1.outputID }
                return $0.layerID < $1.layerID
            }
        )
    }

    public func open(_ descriptor: LiveSourceRuntimeDescriptor) async throws {
        if backings[descriptor.sourceID] != nil { return }

        switch descriptor.sourceClass {
        case .localCamera, .usbCapture:
            let device = try resolveVideoDevice(descriptor)
            let input: AVCaptureDeviceInput
            do {
                input = try AVCaptureDeviceInput(device: device)
            } catch {
                throw LiveSourceRuntimeFailure(
                    code: "LIVE_SOURCE_DEVICE_OPEN_FAILED",
                    summary: "video capture device could not be opened"
                )
            }
            let session = AVCaptureSession()
            session.beginConfiguration()
            guard session.canAddInput(input) else {
                session.commitConfiguration()
                throw LiveSourceRuntimeFailure(
                    code: "LIVE_SOURCE_DEVICE_UNSUPPORTED",
                    summary: "video capture input cannot be attached"
                )
            }
            session.addInput(input)
            session.commitConfiguration()
            let previewLayer = AVCaptureVideoPreviewLayer(session: session)
            previewLayer.videoGravity = .resizeAspect
            session.startRunning()
            backings[descriptor.sourceID] = .camera(
                CameraBacking(session: session, previewLayer: previewLayer)
            )

        case .networkStream:
            guard let url = URL(string: descriptor.endpointRef),
                  let scheme = url.scheme?.lowercased(),
                  ["http", "https"].contains(scheme) else {
                throw LiveSourceRuntimeFailure(
                    code: "LIVE_SOURCE_ENDPOINT_INVALID",
                    summary: "native network live source requires an HTTP or HTTPS endpoint"
                )
            }
            let player = AVPlayer(url: url)
            let playerLayer = AVPlayerLayer(player: player)
            playerLayer.videoGravity = .resizeAspect
            player.play()
            backings[descriptor.sourceID] = .stream(
                StreamBacking(player: player, playerLayer: playerLayer)
            )
        }
    }

    public func close(sourceID: String) async throws {
        guard let backing = backings.removeValue(forKey: sourceID) else {
            throw LiveSourceRuntimeFailure(
                code: "LIVE_SOURCE_NOT_OPEN",
                summary: "live source is not open"
            )
        }
        if let renderSurface {
            await renderSurface.detachLiveSourceLayers(sourceID: sourceID)
        }
        stop(backing)
        if selectedSourceID == sourceID { selectedSourceID = nil }
        routes = routes.filter { $0.value.sourceID != sourceID }
    }

    public func select(sourceID: String) async throws {
        guard backings[sourceID] != nil else {
            throw LiveSourceRuntimeFailure(
                code: "LIVE_SOURCE_NOT_OPEN",
                summary: "live source must be open before selection"
            )
        }
        selectedSourceID = sourceID
    }

    public func route(_ route: LiveSourceRouteState) async throws {
        guard let backing = backings[route.sourceID] else {
            throw LiveSourceRuntimeFailure(
                code: "LIVE_SOURCE_NOT_OPEN",
                summary: "live source must be open before routing"
            )
        }

        if let renderSurface {
            let presentation = presentationLayer(for: backing)
            do {
                try await renderSurface.attachLiveSourceLayer(
                    sourceID: route.sourceID,
                    layerID: route.layerID,
                    outputID: route.outputID,
                    presentation: presentation
                )
            } catch let failure as VisualRendererFailure {
                throw LiveSourceRuntimeFailure(code: failure.code, summary: failure.summary)
            } catch {
                throw LiveSourceRuntimeFailure(
                    code: "LIVE_SOURCE_RENDERER_FAILED",
                    summary: "live source could not be attached to the native renderer"
                )
            }
        }

        // Visual layer identity is global across named outputs. A successful
        // re-route moves the same layer identity rather than manufacturing a
        // second logical layer with the same ID.
        routes = routes.filter { $0.value.layerID != route.layerID }
        routes[route.outputID + "\u{0}" + route.layerID] = route
    }

    public func inspect(sourceID: String) async -> [String: JSONValue] {
        guard let backing = backings[sourceID] else {
            return [
                "native_runtime": .bool(true),
                "native_open": .bool(false),
                "native_renderer_surface": .bool(renderSurface != nil),
                "native_routes": .array([]),
            ]
        }

        let kind: String
        switch backing {
        case .camera: kind = "CAPTURE"
        case .stream: kind = "NETWORK_STREAM"
        }
        let sourceRoutes = routes.values
            .filter { $0.sourceID == sourceID }
            .sorted {
                if $0.outputID != $1.outputID { return $0.outputID < $1.outputID }
                return $0.layerID < $1.layerID
            }
        var routeDiagnostics: [JSONValue] = []
        for route in sourceRoutes {
            let attached: Bool
            if let renderSurface {
                attached = await renderSurface.inspectLiveSourceLayer(
                    sourceID: sourceID,
                    layerID: route.layerID
                ) != nil
            } else {
                attached = false
            }
            routeDiagnostics.append(.object([
                "layer_id": .string(route.layerID),
                "output_id": .string(route.outputID),
                "renderer_attached": .bool(attached),
            ]))
        }
        return [
            "native_runtime": .bool(true),
            "native_open": .bool(true),
            "native_kind": .string(kind),
            "native_renderer_surface": .bool(renderSurface != nil),
            "native_route_count": .int(sourceRoutes.count),
            "native_routes": .array(routeDiagnostics),
        ]
    }

    public func shutdown() async {
        if let renderSurface {
            for sourceID in backings.keys {
                await renderSurface.detachLiveSourceLayers(sourceID: sourceID)
            }
        }
        for backing in backings.values { stop(backing) }
        backings.removeAll()
        routes.removeAll()
        selectedSourceID = nil
    }

    private func presentationLayer(for backing: Backing) -> NativeLiveSourcePresentationLayer {
        switch backing {
        case .camera(let camera):
            let layer = AVCaptureVideoPreviewLayer(session: camera.session)
            layer.videoGravity = .resizeAspect
            return NativeLiveSourcePresentationLayer(layer: layer)
        case .stream(let stream):
            let layer = AVPlayerLayer(player: stream.player)
            layer.videoGravity = .resizeAspect
            return NativeLiveSourcePresentationLayer(layer: layer)
        }
    }

    private func resolveVideoDevice(_ descriptor: LiveSourceRuntimeDescriptor) throws -> AVCaptureDevice {
        let endpoint = descriptor.endpointRef.trimmingCharacters(in: .whitespacesAndNewlines)
        if !endpoint.isEmpty {
            if let exact = AVCaptureDevice.devices(for: .video).first(where: { $0.uniqueID == endpoint }) {
                return exact
            }
            throw LiveSourceRuntimeFailure(
                code: "LIVE_SOURCE_DEVICE_UNAVAILABLE",
                summary: "requested video capture device is unavailable"
            )
        }
        if descriptor.sourceClass == .usbCapture {
            throw LiveSourceRuntimeFailure(
                code: "LIVE_SOURCE_ENDPOINT_INVALID",
                summary: "USB capture requires an explicit video device endpoint"
            )
        }
        guard let fallback = AVCaptureDevice.default(for: .video) else {
            throw LiveSourceRuntimeFailure(
                code: "LIVE_SOURCE_DEVICE_UNAVAILABLE",
                summary: "no local video capture device is available"
            )
        }
        return fallback
    }

    private func stop(_ backing: Backing) {
        switch backing {
        case .camera(let camera):
            camera.session.stopRunning()
            camera.previewLayer.removeFromSuperlayer()
        case .stream(let stream):
            stream.player.pause()
            stream.player.replaceCurrentItem(with: nil)
            stream.playerLayer.removeFromSuperlayer()
        }
    }
}
#else
import Foundation

public struct LiveSourceRuntimeFailure: Error, Sendable, Equatable {
    public var code: String
    public var summary: String
    public init(code: String, summary: String) {
        self.code = code
        self.summary = summary
    }
}

public protocol LiveSourceRuntimeAdapter: Sendable {
    func open(_ descriptor: LiveSourceRuntimeDescriptor) async throws
    func close(sourceID: String) async throws
    func select(sourceID: String) async throws
    func route(_ route: LiveSourceRouteState) async throws
    func inspect(sourceID: String) async -> [String: JSONValue]
    func shutdown() async
}

public extension LiveSourceRuntimeAdapter {
    func inspect(sourceID: String) async -> [String: JSONValue] { [:] }
}

public actor StateOnlyLiveSourceRuntime: LiveSourceRuntimeAdapter {
    public init() {}
    public func open(_ descriptor: LiveSourceRuntimeDescriptor) async throws {}
    public func close(sourceID: String) async throws {}
    public func select(sourceID: String) async throws {}
    public func route(_ route: LiveSourceRouteState) async throws {}
    public func shutdown() async {}
}
#endif
