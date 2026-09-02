#if os(macOS)
import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class VDMXInspectionTests: XCTestCase {
    func testVDMX6PlusInspectsDeclaredContentAndReferenceAssets() async throws {
        let root = try makeTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: root) }

        let app = root.appendingPathComponent("VDMX6 Plus.app", isDirectory: true)
        try makeApplicationBundle(at: app, version: "1.3.4")

        let workspace = root.appendingPathComponent("Show.vdmx6")
        let workspaceData = Data("stagecore-vdmx-workspace".utf8)
        try workspaceData.write(to: workspace)
        let reference = root.appendingPathComponent("LicensedState", isDirectory: true)
        try FileManager.default.createDirectory(at: reference, withIntermediateDirectories: true)

        let provider = VDMXInspectionProvider(
            applicationCandidates: [app],
            architecture: "arm64"
        )
        let outcome = await provider.inspect(manifest: manifest(
            versionConstraint: "6.x-tested",
            assets: [
                contentBoundAsset(
                    key: "workspace",
                    locator: workspace.path,
                    hash: StageCoreSHA256.hexDigest(workspaceData),
                    size: workspaceData.count
                ),
                referenceOnlyAsset(key: "licensed-state", locator: reference.path),
            ]
        ))

        XCTAssertEqual(outcome.status, .completed)
        XCTAssertNil(outcome.errorCode)
        let observation = try XCTUnwrap(outcome.observation)
        XCTAssertEqual(observation.os, "darwin")
        XCTAssertEqual(observation.architecture, "arm64")
        XCTAssertTrue(observation.application.present)
        XCTAssertEqual(observation.application.observedVersion, "1.3.4")
        XCTAssertEqual(observation.application.versionConstraintSatisfied, true)
        XCTAssertEqual(observation.assets.count, 2)

        let content = try XCTUnwrap(observation.assets.first { $0.key == "workspace" })
        XCTAssertTrue(content.present)
        XCTAssertTrue(content.inspectable)
        XCTAssertEqual(content.contentHash, StageCoreSHA256.hexDigest(workspaceData))
        XCTAssertEqual(content.sizeBytes, Int64(workspaceData.count))

        let referenceObservation = try XCTUnwrap(observation.assets.first { $0.key == "licensed-state" })
        XCTAssertTrue(referenceObservation.present)
        XCTAssertFalse(referenceObservation.inspectable)
        XCTAssertEqual(referenceObservation.contentHash, "")
        XCTAssertNil(referenceObservation.sizeBytes)
    }

    func testVDMXNumericVersionConstraintsAreAdapterOwnedAndDeterministic() async throws {
        let root = try makeTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: root) }
        let app = root.appendingPathComponent("VDMX6.app", isDirectory: true)
        try makeApplicationBundle(at: app, version: "1.3.4")
        let provider = VDMXInspectionProvider(applicationCandidates: [app])

        let minimum = await provider.inspect(manifest: manifest(versionConstraint: ">=1.3.0"))
        XCTAssertEqual(minimum.observation?.application.versionConstraintSatisfied, true)

        let tooNew = await provider.inspect(manifest: manifest(versionConstraint: ">=2.0.0"))
        XCTAssertEqual(tooNew.observation?.application.versionConstraintSatisfied, false)

        let exact = await provider.inspect(manifest: manifest(versionConstraint: "1.3.4"))
        XCTAssertEqual(exact.observation?.application.versionConstraintSatisfied, true)

        let unknownGrammar = await provider.inspect(manifest: manifest(versionConstraint: "latest-stable"))
        XCTAssertNil(unknownGrammar.observation?.application.versionConstraintSatisfied)
    }

    func testMissingVDMXIsReportedWithoutFabricatingVersionCompatibility() async {
        let provider = VDMXInspectionProvider(applicationCandidates: [])
        let outcome = await provider.inspect(manifest: manifest(versionConstraint: "6.x-tested"))
        XCTAssertEqual(outcome.status, .completed)
        XCTAssertEqual(outcome.observation?.application.present, false)
        XCTAssertEqual(outcome.observation?.application.observedVersion, "")
        XCTAssertNil(outcome.observation?.application.versionConstraintSatisfied)
    }

    func testDeclaredSymlinkAssetIsNotFollowed() async throws {
        let root = try makeTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: root) }
        let app = root.appendingPathComponent("VDMX6.app", isDirectory: true)
        try makeApplicationBundle(at: app, version: "1.3.4")

        let outside = root.appendingPathComponent("outside.mov")
        try Data("outside".utf8).write(to: outside)
        let link = root.appendingPathComponent("declared.mov")
        try FileManager.default.createSymbolicLink(at: link, withDestinationURL: outside)

        let provider = VDMXInspectionProvider(applicationCandidates: [app])
        let outcome = await provider.inspect(manifest: manifest(
            versionConstraint: "6.x-tested",
            assets: [contentBoundAsset(
                key: "media",
                locator: link.path,
                hash: StageCoreSHA256.hexDigest(Data("outside".utf8)),
                size: 7
            )]
        ))

        let asset = outcome.observation?.assets.first
        XCTAssertEqual(outcome.status, .completed)
        XCTAssertEqual(asset?.present, false)
        XCTAssertEqual(asset?.inspectable, false)
        XCTAssertEqual(asset?.contentHash, "")
    }

    func testUnsupportedVDMXExtensionOrBindingScopeFailsExplicitly() async throws {
        let root = try makeTemporaryDirectory()
        defer { try? FileManager.default.removeItem(at: root) }
        let app = root.appendingPathComponent("VDMX6.app", isDirectory: true)
        try makeApplicationBundle(at: app, version: "1.3.4")
        let provider = VDMXInspectionProvider(applicationCandidates: [app])

        var withExtension = manifest(versionConstraint: "6.x-tested")
        withExtension["external_extensions"] = .array([.object([
            "key": .string("isf-pack"),
            "name": .string("ISF Pack"),
            "version_constraint": .string(">=2.0"),
            "required": .bool(true),
        ])])
        let extensionOutcome = await provider.inspect(manifest: withExtension)
        XCTAssertEqual(extensionOutcome.status, .failed)
        XCTAssertEqual(extensionOutcome.errorCode, "VDMX_INSPECTION_SCOPE_UNSUPPORTED")
        XCTAssertNil(extensionOutcome.observation)

        var withBinding = manifest(versionConstraint: "6.x-tested")
        withBinding["bindings"] = .array([.object([
            "key": .string("main-output"),
            "kind": .string("DISPLAY"),
            "name": .string("Main output"),
            "external_ref": .string("display:main"),
            "required": .bool(true),
        ])])
        let bindingOutcome = await provider.inspect(manifest: withBinding)
        XCTAssertEqual(bindingOutcome.status, .failed)
        XCTAssertEqual(bindingOutcome.errorCode, "VDMX_INSPECTION_SCOPE_UNSUPPORTED")
        XCTAssertNil(bindingOutcome.observation)
    }

    private func makeTemporaryDirectory() throws -> URL {
        let url = FileManager.default.temporaryDirectory
            .appendingPathComponent("stagecore-vdmx-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        return url
    }

    private func makeApplicationBundle(at url: URL, version: String) throws {
        let contents = url.appendingPathComponent("Contents", isDirectory: true)
        try FileManager.default.createDirectory(at: contents, withIntermediateDirectories: true)
        let plist: [String: Any] = [
            "CFBundleIdentifier": "com.vidvox.test.vdmx6",
            "CFBundleName": url.deletingPathExtension().lastPathComponent,
            "CFBundleShortVersionString": version,
            "CFBundleVersion": version,
        ]
        let data = try PropertyListSerialization.data(
            fromPropertyList: plist,
            format: .xml,
            options: 0
        )
        try data.write(to: contents.appendingPathComponent("Info.plist"))
    }

    private func manifest(
        versionConstraint: String,
        assets: [JSONValue] = []
    ) -> [String: JSONValue] {
        [
            "schema_version": .int(1),
            "environment_key": .string("video-main"),
            "name": .string("Main video workstation"),
            "adapter_key": .string("stagecore.adapter.vdmx"),
            "application": .object([
                "key": .string("vdmx"),
                "name": .string("VDMX"),
                "vendor": .string("VIDVOX"),
                "version_constraint": .string(versionConstraint),
                "hosts": .array([.object([
                    "os": .string("darwin"),
                    "architecture": .string("arm64"),
                ])]),
            ]),
            "assets": .array(assets),
        ]
    }

    private func contentBoundAsset(
        key: String,
        locator: String,
        hash: String,
        size: Int
    ) -> JSONValue {
        .object([
            "key": .string(key),
            "kind": .string("PROJECT_FILE"),
            "name": .string(key),
            "capture_policy": .string("CONTENT_BOUND"),
            "content_hash": .string(hash),
            "size_bytes": .int(size),
            "locator": .string(locator),
        ])
    }

    private func referenceOnlyAsset(key: String, locator: String) -> JSONValue {
        .object([
            "key": .string(key),
            "kind": .string("RESOURCE"),
            "name": .string(key),
            "capture_policy": .string("REFERENCE_ONLY"),
            "locator": .string(locator),
        ])
    }
}
#endif
