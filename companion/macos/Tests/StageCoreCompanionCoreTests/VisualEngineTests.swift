import Foundation
import XCTest
@testable import StageCoreCompanionCore

private struct FixedVisualMediaResolver: VisualMediaResolver {
    let hashes: Set<String>

    func verifiedMediaURL(contentHash: String) async -> URL? {
        guard hashes.contains(contentHash) else { return nil }
        return URL(fileURLWithPath: "/verified/\(contentHash)")
    }
}

final class VisualEngineTests: XCTestCase {
    private let hash = String(repeating: "a", count: 64)

    func testDeterministicLayerLifecycleAndStateInspection() async throws {
        let engine = VisualEngine(mediaResolver: FixedVisualMediaResolver(hashes: [hash]))

        let preload = await engine.execute(
            capability: VisualCapability.preload,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("main"),
                "content_version_id": .string("version-1"),
                "content_hash": .string(hash),
                "opacity": .double(0.75),
                "transform": .object(["scale_x": .double(1.25)]),
            ]
        )
        XCTAssertEqual(preload.status, .completed)
        XCTAssertEqual(preload.ackLevel, .accepted)

        let play = await engine.execute(
            capability: VisualCapability.play,
            parameters: ["contract_version": .int(1), "layer_id": .string("main")]
        )
        XCTAssertEqual(play.status, .completed)

        let seek = await engine.execute(
            capability: VisualCapability.seek,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("main"),
                "position_ms": .int(1250),
            ]
        )
        XCTAssertEqual(seek.status, .completed)

        let loop = await engine.execute(
            capability: VisualCapability.loop,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("main"),
                "enabled": .bool(true),
            ]
        )
        XCTAssertEqual(loop.status, .completed)

        let blackout = await engine.execute(
            capability: VisualCapability.blackout,
            parameters: ["contract_version": .int(1), "enabled": .bool(true)]
        )
        XCTAssertEqual(blackout.status, .completed)

        let pause = await engine.execute(
            capability: VisualCapability.pause,
            parameters: ["contract_version": .int(1), "layer_id": .string("main")]
        )
        XCTAssertEqual(pause.status, .completed)

        let snapshot = await engine.snapshot()
        XCTAssertTrue(snapshot.blackout)
        XCTAssertEqual(snapshot.layers.count, 1)
        let layer = try XCTUnwrap(snapshot.layers.first)
        XCTAssertEqual(layer.layerID, "main")
        XCTAssertEqual(layer.contentVersionID, "version-1")
        XCTAssertEqual(layer.contentHash, hash)
        XCTAssertEqual(layer.playback, .paused)
        XCTAssertEqual(layer.positionMS, 1250)
        XCTAssertTrue(layer.loopEnabled)
        XCTAssertEqual(layer.opacity, 0.75)
        XCTAssertEqual(layer.transform.scaleX, 1.25)

        let inspect = await engine.execute(
            capability: VisualCapability.stateInspect,
            parameters: ["contract_version": .int(1)]
        )
        XCTAssertEqual(inspect.status, .completed)
        XCTAssertNotNil(inspect.output["state"])
    }

    func testMissingMediaFailsClosedWithoutCreatingLayer() async {
        let engine = VisualEngine(mediaResolver: FixedVisualMediaResolver(hashes: []))
        let result = await engine.execute(
            capability: VisualCapability.preload,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("main"),
                "content_version_id": .string("version-1"),
                "content_hash": .string(hash),
            ]
        )
        XCTAssertEqual(result.status, .failed)
        XCTAssertEqual(result.errorCode, "VISUAL_MEDIA_UNAVAILABLE")
        let snapshot = await engine.snapshot()
        XCTAssertTrue(snapshot.layers.isEmpty)
    }

    func testStrictParametersAndStateConflictsFailClosed() async {
        let engine = VisualEngine(mediaResolver: FixedVisualMediaResolver(hashes: [hash]))

        let unknownField = await engine.execute(
            capability: VisualCapability.preload,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("main"),
                "content_version_id": .string("version-1"),
                "content_hash": .string(hash),
                "path": .string("/tmp/movie.mov"),
            ]
        )
        XCTAssertEqual(unknownField.status, .failed)
        XCTAssertEqual(unknownField.errorCode, "VISUAL_PARAMETERS_INVALID")

        let futureVersion = await engine.execute(
            capability: VisualCapability.blackout,
            parameters: ["contract_version": .int(2), "enabled": .bool(true)]
        )
        XCTAssertEqual(futureVersion.status, .failed)
        XCTAssertEqual(futureVersion.errorCode, "VISUAL_CONTRACT_VERSION_UNSUPPORTED")

        let pauseMissing = await engine.execute(
            capability: VisualCapability.pause,
            parameters: ["contract_version": .int(1), "layer_id": .string("missing")]
        )
        XCTAssertEqual(pauseMissing.errorCode, "VISUAL_LAYER_NOT_PRELOADED")
    }

    func testCompanionSessionProvidesSnapshotAndDuplicateGuardsForVisualCommands() async throws {
        let engine = VisualEngine(mediaResolver: FixedVisualMediaResolver(hashes: [hash]))
        let executors = try makeVisualCapabilityExecutors(engine: engine)
        let session = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: "11111111-1111-4111-8111-111111111111",
                agentVersion: "0.1.0",
                platform: "macos",
                architecture: "arm64"
            ),
            executors: executors
        )
        await session.establishAuthenticatedSession(
            CompanionRuntimeCredential(
                sessionID: "auth-session",
                token: "token",
                expiresAt: Date().addingTimeInterval(60)
            )
        )
        let ready = SessionReady(
            machineRoleID: "role-visual-main",
            roleKey: "VISUAL-MAIN",
            runtimeSnapshotID: "snap-visual",
            configHash: "cfg"
        )
        _ = try await session.handle(JSONEncoder().encode(ready))

        let request = CompanionExecutionRequest(
            executionID: "exec-visual-1",
            correlationID: "corr-visual-1",
            machineRoleID: "role-visual-main",
            runtimeSnapshotID: "snap-visual",
            capability: VisualCapability.preload,
            parameters: [
                "contract_version": .int(1),
                "layer_id": .string("main"),
                "content_version_id": .string("version-1"),
                "content_hash": .string(hash),
            ],
            timeoutMS: 500
        )
        let firstData = try XCTUnwrap(try await session.handle(JSONEncoder().encode(request)))
        let first = try JSONDecoder().decode(CompanionExecutionResult.self, from: firstData)
        XCTAssertEqual(first.status, .completed)
        XCTAssertEqual(first.ackLevel, .accepted)

        let duplicateData = try XCTUnwrap(try await session.handle(JSONEncoder().encode(request)))
        let duplicate = try JSONDecoder().decode(CompanionExecutionResult.self, from: duplicateData)
        XCTAssertEqual(duplicate.status, .rejected)
        XCTAssertEqual(duplicate.errorCode, "DUPLICATE_EXECUTION")

        let mismatch = CompanionExecutionRequest(
            executionID: "exec-visual-2",
            correlationID: nil,
            machineRoleID: "role-visual-main",
            runtimeSnapshotID: "wrong-snapshot",
            capability: VisualCapability.play,
            parameters: ["contract_version": .int(1), "layer_id": .string("main")],
            timeoutMS: 500
        )
        let mismatchData = try XCTUnwrap(try await session.handle(JSONEncoder().encode(mismatch)))
        let mismatchResult = try JSONDecoder().decode(CompanionExecutionResult.self, from: mismatchData)
        XCTAssertEqual(mismatchResult.status, .rejected)
        XCTAssertEqual(mismatchResult.errorCode, "SNAPSHOT_MISMATCH")
    }
}
