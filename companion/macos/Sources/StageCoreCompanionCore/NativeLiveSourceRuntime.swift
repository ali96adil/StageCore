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
    func shutdown() async
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

    private var backings: [String: Backing] = [:]
    private var selectedSourceID: String?
    private var routes: [String: LiveSourceRouteState] = [:]

    public init() {}

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
        guard backings[route.sourceID] != nil else {
            throw LiveSourceRuntimeFailure(
                code: "LIVE_SOURCE_NOT_OPEN",
                summary: "live source must be open before routing"
            )
        }
        // D2a deliberately retains output/layer routing intent locally while
        // native capture is live. D2b attaches these native layers to the
        // Visual Engine named-output/layer renderer without moving frames
        // through the Hub.
        routes[route.outputID + "\u{0}" + route.layerID] = route
    }

    public func shutdown() async {
        for backing in backings.values { stop(backing) }
        backings.removeAll()
        routes.removeAll()
        selectedSourceID = nil
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
    func shutdown() async
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
