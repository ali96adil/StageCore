import XCTest
@testable import StageCoreCompanionCore

final class LiveSourceEngineTests: XCTestCase {
    func testOpenSelectRouteInspectAndCloseAreDeterministic() async throws {
        let engine = LiveSourceEngine()

        let opened = await engine.execute(
            capability: LiveSourceCapability.open,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("camera-main"),
                "source_class": .string("LOCAL_CAMERA"),
                "endpoint_ref": .string("avfoundation:camera-main"),
                "config": .object(["preferred_width": .int(1920)]),
            ]
        )
        XCTAssertEqual(opened.status, .completed)
        XCTAssertEqual(opened.errorCode, nil)

        let selected = await engine.execute(
            capability: LiveSourceCapability.select,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("camera-main"),
            ]
        )
        XCTAssertEqual(selected.status, .completed)

        let routed = await engine.execute(
            capability: LiveSourceCapability.route,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("camera-main"),
                "layer_id": .string("live-hero"),
                "output_id": .string("projector-a"),
            ]
        )
        XCTAssertEqual(routed.status, .completed)

        let inspected = await engine.execute(
            capability: LiveSourceCapability.inspect,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("camera-main"),
            ]
        )
        XCTAssertEqual(inspected.status, .completed)
        XCTAssertEqual(inspected.output["open"], .bool(true))
        XCTAssertEqual(inspected.output["selected"], .bool(true))
        XCTAssertEqual(
            inspected.output["routes"],
            .array([.object(["layer_id": .string("live-hero"), "output_id": .string("projector-a")])])
        )

        let closed = await engine.execute(
            capability: LiveSourceCapability.close,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("camera-main"),
            ]
        )
        XCTAssertEqual(closed.status, .completed)

        let snapshot = await engine.snapshot()
        XCTAssertNil(snapshot.selectedSourceID)
        XCTAssertTrue(snapshot.routes.isEmpty)
        XCTAssertEqual(snapshot.sources.count, 1)
        XCTAssertEqual(snapshot.sources[0].descriptor.sourceID, "camera-main")
        XCTAssertFalse(snapshot.sources[0].open)
    }

    func testRouteFailsClosedUntilSourceIsExplicitlyOpened() async {
        let engine = LiveSourceEngine()
        let result = await engine.execute(
            capability: LiveSourceCapability.route,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("capture-a"),
                "layer_id": .string("live"),
            ]
        )
        XCTAssertEqual(result.status, .failed)
        XCTAssertEqual(result.errorCode, "LIVE_SOURCE_NOT_OPEN")
        let snapshot = await engine.snapshot()
        XCTAssertTrue(snapshot.sources.isEmpty)
        XCTAssertTrue(snapshot.routes.isEmpty)
    }

    func testContractRejectsUnknownClassVersionAndFields() async {
        let engine = LiveSourceEngine()
        var result = await engine.execute(
            capability: LiveSourceCapability.open,
            parameters: [
                "contract_version": .int(2),
                "source_id": .string("camera"),
                "source_class": .string("LOCAL_CAMERA"),
            ]
        )
        XCTAssertEqual(result.errorCode, "LIVE_SOURCE_CONTRACT_UNSUPPORTED")

        result = await engine.execute(
            capability: LiveSourceCapability.open,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("camera"),
                "source_class": .string("MAGIC_INPUT"),
            ]
        )
        XCTAssertEqual(result.errorCode, "LIVE_SOURCE_CLASS_UNSUPPORTED")

        result = await engine.execute(
            capability: LiveSourceCapability.open,
            parameters: [
                "contract_version": .int(1),
                "source_id": .string("camera"),
                "source_class": .string("LOCAL_CAMERA"),
                "frames": .array([]),
            ]
        )
        XCTAssertEqual(result.errorCode, "LIVE_SOURCE_INVALID")
        XCTAssertTrue((await engine.snapshot()).sources.isEmpty)
    }

    func testExecutorFactoryUsesExistingF007CapabilityVocabulary() throws {
        let engine = LiveSourceEngine()
        let executors = try makeLiveSourceCapabilityExecutors(engine: engine)
        XCTAssertEqual(
            executors.map(\.capabilityKey).sorted(),
            [
                "video.source.close",
                "video.source.inspect",
                "video.source.open",
                "video.source.route",
                "video.source.select",
            ]
        )
    }
}
