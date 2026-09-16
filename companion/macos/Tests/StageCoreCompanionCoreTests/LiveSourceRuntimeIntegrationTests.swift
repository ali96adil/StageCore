import XCTest
@testable import StageCoreCompanionCore

private actor RecordingLiveSourceRuntime: LiveSourceRuntimeAdapter {
    enum InjectedFailure: Error {
        case open
        case route
    }

    var failOpen = false
    var failRoute = false
    var opened: [String] = []
    var closed: [String] = []
    var selected: [String] = []
    var routed: [LiveSourceRouteState] = []

    func setFailOpen(_ value: Bool) { failOpen = value }
    func setFailRoute(_ value: Bool) { failRoute = value }

    func open(_ descriptor: LiveSourceRuntimeDescriptor) async throws {
        if failOpen { throw InjectedFailure.open }
        opened.append(descriptor.sourceID)
    }

    func close(sourceID: String) async throws {
        closed.append(sourceID)
    }

    func select(sourceID: String) async throws {
        selected.append(sourceID)
    }

    func route(_ route: LiveSourceRouteState) async throws {
        if failRoute { throw InjectedFailure.route }
        routed.append(route)
    }

    func shutdown() async {}
}

final class LiveSourceRuntimeIntegrationTests: XCTestCase {
    func testRuntimeOpenFailureDoesNotCommitSourceState() async {
        let runtime = RecordingLiveSourceRuntime()
        await runtime.setFailOpen(true)
        let engine = LiveSourceEngine(runtime: runtime)

        let result = await engine.execute(
            capability: LiveSourceCapability.open,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("camera-a"),
                "source_class": .string("LOCAL_CAMERA"),
            ]
        )

        XCTAssertEqual(result.status, .failed)
        XCTAssertEqual(result.errorCode, "LIVE_SOURCE_RUNTIME_FAILED")
        let snapshot = await engine.snapshot()
        XCTAssertTrue(snapshot.sources.isEmpty)
        XCTAssertTrue(snapshot.routes.isEmpty)
    }

    func testRuntimeRouteFailureDoesNotCommitRouteState() async {
        let runtime = RecordingLiveSourceRuntime()
        let engine = LiveSourceEngine(runtime: runtime)
        _ = await engine.execute(
            capability: LiveSourceCapability.open,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("network-a"),
                "source_class": .string("NETWORK_STREAM"),
                "endpoint_ref": .string("https://example.invalid/live.m3u8"),
            ]
        )
        await runtime.setFailRoute(true)

        let result = await engine.execute(
            capability: LiveSourceCapability.route,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("network-a"),
                "layer_id": .string("live-a"),
            ]
        )

        XCTAssertEqual(result.status, .failed)
        XCTAssertEqual(result.errorCode, "LIVE_SOURCE_RUNTIME_FAILED")
        let snapshot = await engine.snapshot()
        XCTAssertTrue(snapshot.routes.isEmpty)
    }

    func testFreshEngineAfterReconnectDoesNotReplayOpenSelectOrRoute() async {
        let oldRuntime = RecordingLiveSourceRuntime()
        let oldEngine = LiveSourceEngine(runtime: oldRuntime)
        _ = await oldEngine.execute(
            capability: LiveSourceCapability.open,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("capture-a"),
                "source_class": .string("USB_CAPTURE"),
                "endpoint_ref": .string("device-a"),
            ]
        )
        _ = await oldEngine.execute(
            capability: LiveSourceCapability.select,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("capture-a"),
            ]
        )
        _ = await oldEngine.execute(
            capability: LiveSourceCapability.route,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("capture-a"),
                "layer_id": .string("hero"),
                "output_id": .string("main"),
            ]
        )

        let freshRuntime = RecordingLiveSourceRuntime()
        let freshEngine = LiveSourceEngine(runtime: freshRuntime)
        let fresh = await freshEngine.snapshot()

        XCTAssertTrue(fresh.sources.isEmpty)
        XCTAssertNil(fresh.selectedSourceID)
        XCTAssertTrue(fresh.routes.isEmpty)
        XCTAssertTrue(await freshRuntime.opened.isEmpty)
        XCTAssertTrue(await freshRuntime.selected.isEmpty)
        XCTAssertTrue(await freshRuntime.routed.isEmpty)

        let routeWithoutExplicitReopen = await freshEngine.execute(
            capability: LiveSourceCapability.route,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("capture-a"),
                "layer_id": .string("hero"),
            ]
        )
        XCTAssertEqual(routeWithoutExplicitReopen.errorCode, "LIVE_SOURCE_NOT_OPEN")
    }

    func testShutdownClearsAuthoritativeLiveSourceState() async {
        let runtime = RecordingLiveSourceRuntime()
        let engine = LiveSourceEngine(runtime: runtime)
        _ = await engine.execute(
            capability: LiveSourceCapability.open,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("network-a"),
                "source_class": .string("NETWORK_STREAM"),
                "endpoint_ref": .string("https://example.invalid/live.m3u8"),
            ]
        )

        await engine.shutdown()
        let snapshot = await engine.snapshot()
        XCTAssertTrue(snapshot.sources.isEmpty)
        XCTAssertTrue(snapshot.routes.isEmpty)
        XCTAssertNil(snapshot.selectedSourceID)
    }
}
