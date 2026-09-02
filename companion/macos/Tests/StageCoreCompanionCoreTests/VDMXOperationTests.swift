import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class VDMXOperationTests: XCTestCase {
    func testOpenAssetLaunchUsesDeclaredVDMXApplicationAndTarget() async throws {
        let root = try temporaryDirectory()
        let application = root.appendingPathComponent("VDMX6 Plus.app", isDirectory: true)
        let workspace = root.appendingPathComponent("Show.vdmx6", isDirectory: false)
        try FileManager.default.createDirectory(at: application, withIntermediateDirectories: true)
        try Data("workspace".utf8).write(to: workspace)
        let recorder = VDMXOpenRecorder(result: true)
        let provider = VDMXOperationProvider(
            applicationCandidates: [application],
            opener: { target, app in recorder.record(target: target, application: app) }
        )

        let outcome = await provider.perform(
            kind: .open,
            manifest: manifest(launch: .asset(workspace.path)),
            sourceManifestSHA256: hash
        )

        XCTAssertEqual(outcome.status, .completed)
        XCTAssertNil(outcome.errorCode)
        XCTAssertEqual(recorder.count, 1)
        XCTAssertEqual(recorder.target?.standardizedFileURL.path, workspace.standardizedFileURL.path)
        XCTAssertEqual(recorder.application?.standardizedFileURL.path, application.standardizedFileURL.path)
    }

    func testOpenLocatorLaunchUsesDeclaredLocator() async throws {
        let root = try temporaryDirectory()
        let application = root.appendingPathComponent("VDMX6.app", isDirectory: true)
        let workspace = root.appendingPathComponent("Locator.vdmx6", isDirectory: false)
        try FileManager.default.createDirectory(at: application, withIntermediateDirectories: true)
        try Data("workspace".utf8).write(to: workspace)
        let recorder = VDMXOpenRecorder(result: true)
        let provider = VDMXOperationProvider(
            applicationCandidates: [application],
            opener: { target, app in recorder.record(target: target, application: app) }
        )

        let outcome = await provider.perform(
            kind: .open,
            manifest: manifest(launch: .locator(workspace.absoluteString)),
            sourceManifestSHA256: hash
        )

        XCTAssertEqual(outcome.status, .completed)
        XCTAssertEqual(recorder.count, 1)
        XCTAssertEqual(recorder.target?.standardizedFileURL.path, workspace.standardizedFileURL.path)
    }

    func testOpenFailsTruthfullyWhenApplicationOrTargetIsUnavailable() async throws {
        let root = try temporaryDirectory()
        let workspace = root.appendingPathComponent("Show.vdmx6", isDirectory: false)
        try Data("workspace".utf8).write(to: workspace)
        let recorder = VDMXOpenRecorder(result: true)
        let missingApp = root.appendingPathComponent("Missing VDMX.app", isDirectory: true)
        let provider = VDMXOperationProvider(
            applicationCandidates: [missingApp],
            opener: { target, app in recorder.record(target: target, application: app) }
        )

        var outcome = await provider.perform(kind: .open, manifest: manifest(launch: .asset(workspace.path)), sourceManifestSHA256: hash)
        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.errorCode, "VDMX_APPLICATION_NOT_FOUND")
        XCTAssertEqual(recorder.count, 0)

        let application = root.appendingPathComponent("VDMX6.app", isDirectory: true)
        try FileManager.default.createDirectory(at: application, withIntermediateDirectories: true)
        let missingTargetProvider = VDMXOperationProvider(
            applicationCandidates: [application],
            opener: { target, app in recorder.record(target: target, application: app) }
        )
        outcome = await missingTargetProvider.perform(
            kind: .open,
            manifest: manifest(launch: .asset(root.appendingPathComponent("Missing.vdmx6").path)),
            sourceManifestSHA256: hash
        )
        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.errorCode, "VDMX_LAUNCH_TARGET_UNAVAILABLE")
        XCTAssertEqual(recorder.count, 0)
    }

    func testOpenRejectsSymlinkTargetAndReportsOpenFailure() async throws {
        let root = try temporaryDirectory()
        let application = root.appendingPathComponent("VDMX6.app", isDirectory: true)
        let workspace = root.appendingPathComponent("Show.vdmx6", isDirectory: false)
        let symlink = root.appendingPathComponent("Alias.vdmx6", isDirectory: false)
        try FileManager.default.createDirectory(at: application, withIntermediateDirectories: true)
        try Data("workspace".utf8).write(to: workspace)
        try FileManager.default.createSymbolicLink(at: symlink, withDestinationURL: workspace)
        let recorder = VDMXOpenRecorder(result: false)
        let provider = VDMXOperationProvider(
            applicationCandidates: [application],
            opener: { target, app in recorder.record(target: target, application: app) }
        )

        var outcome = await provider.perform(kind: .open, manifest: manifest(launch: .asset(symlink.path)), sourceManifestSHA256: hash)
        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.errorCode, "VDMX_LAUNCH_TARGET_UNAVAILABLE")
        XCTAssertEqual(recorder.count, 0)

        outcome = await provider.perform(kind: .open, manifest: manifest(launch: .asset(workspace.path)), sourceManifestSHA256: hash)
        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.errorCode, "VDMX_OPEN_FAILED")
        XCTAssertEqual(recorder.count, 1)
    }

    func testCaptureSnapshotIsPartialAndBoundToManifestIdentity() async throws {
        let root = try temporaryDirectory()
        let application = root.appendingPathComponent("VDMX6 Plus.app", isDirectory: true)
        let workspace = root.appendingPathComponent("Show.vdmx6", isDirectory: false)
        try FileManager.default.createDirectory(at: application, withIntermediateDirectories: true)
        try Data("workspace".utf8).write(to: workspace)
        let provider = VDMXOperationProvider(applicationCandidates: [application], opener: { _, _ in false })

        let outcome = await provider.perform(
            kind: .captureSnapshot,
            manifest: manifest(launch: .asset(workspace.path)),
            sourceManifestSHA256: hash.uppercased()
        )

        XCTAssertEqual(outcome.status, .completed)
        let snapshot = try XCTUnwrap(outcome.snapshot)
        XCTAssertEqual(snapshot["schema_version"], .int(1))
        XCTAssertEqual(snapshot["environment_key"], .string("video-main"))
        XCTAssertEqual(snapshot["adapter_key"], .string("stagecore.adapter.vdmx"))
        XCTAssertEqual(snapshot["source_manifest_sha256"], .string(hash))
        XCTAssertEqual(snapshot["capture_status"], .string("PARTIAL"))
        guard case .array(let items) = snapshot["items"] else {
            return XCTFail("snapshot items missing")
        }
        XCTAssertEqual(items.count, 3)
        XCTAssertTrue(items.contains { item in
            guard case .object(let value) = item else { return false }
            return value["key"] == .string("declared-launch-target")
                && value["capture_status"] == .string("OBSERVED")
                && value["portability"] == .string("REFERENCE_ONLY")
        })
        XCTAssertTrue(items.contains { item in
            guard case .object(let value) = item else { return false }
            return value["key"] == .string("vdmx-internal-state")
                && value["capture_status"] == .string("UNSUPPORTED")
        })
    }

    func testReconnectRemainsUnsupportedAndInvalidManifestFailsClosed() async {
        let provider = VDMXOperationProvider(applicationCandidates: [], opener: { _, _ in true })
        XCTAssertFalse(provider.supportedOperations.contains(.reconnect))

        var outcome = await provider.perform(kind: .reconnect, manifest: manifest(launch: .locator("/tmp/show.vdmx6")), sourceManifestSHA256: hash)
        XCTAssertEqual(outcome.status, .unsupported)
        XCTAssertEqual(outcome.errorCode, "ENVIRONMENT_OPERATION_UNSUPPORTED")

        var invalid = manifest(launch: .locator("/tmp/show.vdmx6"))
        invalid["adapter_key"] = .string("stagecore.adapter.qlab")
        outcome = await provider.perform(kind: .open, manifest: invalid, sourceManifestSHA256: hash)
        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.errorCode, "VDMX_MANIFEST_INVALID")
    }

    private let hash = String(repeating: "a", count: 64)

    private enum LaunchFixture {
        case asset(String)
        case locator(String)
    }

    private func manifest(launch: LaunchFixture) -> [String: JSONValue] {
        var assets: [JSONValue] = []
        let launchObject: [String: JSONValue]
        switch launch {
        case .asset(let locator):
            assets = [.object([
                "key": .string("workspace"),
                "locator": .string(locator),
            ])]
            launchObject = ["kind": .string("ASSET"), "asset_key": .string("workspace")]
        case .locator(let locator):
            launchObject = ["kind": .string("LOCATOR"), "locator": .string(locator)]
        }
        return [
            "schema_version": .int(1),
            "environment_key": .string("video-main"),
            "adapter_key": .string("stagecore.adapter.vdmx"),
            "application": .object(["key": .string("vdmx")]),
            "assets": .array(assets),
            "launch": .object(launchObject),
        ]
    }

    private func temporaryDirectory() throws -> URL {
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        addTeardownBlock { try? FileManager.default.removeItem(at: url) }
        return url
    }
}

private final class VDMXOpenRecorder: @unchecked Sendable {
    private let lock = NSLock()
    private let result: Bool
    private var recordedCount = 0
    private var recordedTarget: URL?
    private var recordedApplication: URL?

    init(result: Bool) {
        self.result = result
    }

    var count: Int { lock.withLock { recordedCount } }
    var target: URL? { lock.withLock { recordedTarget } }
    var application: URL? { lock.withLock { recordedApplication } }

    func record(target: URL, application: URL) -> Bool {
        lock.withLock {
            recordedCount += 1
            recordedTarget = target
            recordedApplication = application
        }
        return result
    }
}
