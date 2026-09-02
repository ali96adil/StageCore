import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class InspectionTests: XCTestCase {
    func testRegisteredProviderReturnsTruthfulObservation() async throws {
        let router = try CompanionInspectionRouter(providers: [FixedInspectionProvider()])
        let request = CompanionInspectionRequest(
            inspectionID: "inspect-1",
            adapterKey: "test.readonly",
            manifest: [
                "schema_version": .int(1),
                "environment_key": .string("video-main"),
                "adapter_key": .string("test.readonly")
            ]
        )

        let response = try await router.handleIfInspection(
            JSONEncoder().encode(request),
            authenticated: true
        )
        let result = try JSONDecoder().decode(
            CompanionInspectionResult.self,
            from: try XCTUnwrap(response)
        )

        XCTAssertEqual(result.inspectionID, "inspect-1")
        XCTAssertEqual(result.adapterKey, "test.readonly")
        XCTAssertEqual(result.status, .completed)
        XCTAssertNil(result.errorCode)
        XCTAssertEqual(result.observation?.os, "macos")
        XCTAssertEqual(result.observation?.architecture, "arm64")
        XCTAssertEqual(result.observation?.application.present, true)
        XCTAssertEqual(result.observation?.application.observedVersion, "1.2.3")
        XCTAssertEqual(result.observation?.assets.first?.key, "show-file")
    }

    func testUnknownAdapterIsUnsupportedWithoutFabricatedObservation() async throws {
        let router = try CompanionInspectionRouter()
        let request = CompanionInspectionRequest(
            inspectionID: "inspect-unsupported",
            adapterKey: "vdmx.future",
            manifest: ["schema_version": .int(1)]
        )

        let response = try await router.handleIfInspection(
            JSONEncoder().encode(request),
            authenticated: true
        )
        let result = try JSONDecoder().decode(
            CompanionInspectionResult.self,
            from: try XCTUnwrap(response)
        )

        XCTAssertEqual(result.status, .unsupported)
        XCTAssertEqual(result.errorCode, "INSPECTION_ADAPTER_UNSUPPORTED")
        XCTAssertNil(result.observation)
    }

    func testUnauthenticatedInspectionDoesNotCallProvider() async throws {
        let counter = InspectionCounter()
        let router = try CompanionInspectionRouter(
            providers: [CountingInspectionProvider(counter: counter)]
        )
        let request = CompanionInspectionRequest(
            inspectionID: "inspect-blocked",
            adapterKey: "test.counting",
            manifest: ["schema_version": .int(1)]
        )

        let response = try await router.handleIfInspection(
            JSONEncoder().encode(request),
            authenticated: false
        )
        let result = try JSONDecoder().decode(
            CompanionInspectionResult.self,
            from: try XCTUnwrap(response)
        )

        XCTAssertEqual(result.status, .failed)
        XCTAssertEqual(result.errorCode, "SESSION_UNAUTHENTICATED")
        XCTAssertNil(result.observation)
        let count = await counter.value()
        XCTAssertEqual(count, 0)
    }

    func testNonInspectionMessageFallsThrough() async throws {
        let router = try CompanionInspectionRouter(providers: [FixedInspectionProvider()])
        let ready = SessionReady(
            machineRoleID: "role-1",
            roleKey: "VIDEO-MAIN",
            runtimeSnapshotID: "snapshot-1",
            configHash: "cfg"
        )

        let response = try await router.handleIfInspection(
            JSONEncoder().encode(ready),
            authenticated: true
        )
        XCTAssertNil(response)
    }

    func testDuplicateAdapterRegistrationIsRejected() throws {
        XCTAssertThrowsError(
            try CompanionInspectionRouter(
                providers: [FixedInspectionProvider(), FixedInspectionProvider()]
            )
        ) { error in
            XCTAssertEqual(
                error as? CompanionInspectionRouterError,
                .duplicateAdapterKey("test.readonly")
            )
        }
    }
}

private struct FixedInspectionProvider: CompanionInspectionProvider {
    let adapterKey = "test.readonly"

    func inspect(manifest: [String: JSONValue]) async -> CompanionInspectionOutcome {
        CompanionInspectionOutcome(
            status: .completed,
            responseSummary: "declared requirements inspected",
            observation: CompanionInspectionObservation(
                os: "macos",
                architecture: "arm64",
                application: CompanionInspectionApplicationObservation(
                    present: true,
                    observedVersion: "1.2.3",
                    versionConstraintSatisfied: true
                ),
                assets: [
                    CompanionInspectionAssetObservation(
                        key: "show-file",
                        present: true,
                        inspectable: true,
                        contentHash: String(repeating: "a", count: 64),
                        sizeBytes: 1024
                    )
                ]
            )
        )
    }
}

private actor InspectionCounter {
    private var count = 0

    func increment() {
        count += 1
    }

    func value() -> Int {
        count
    }
}

private struct CountingInspectionProvider: CompanionInspectionProvider {
    let adapterKey = "test.counting"
    let counter: InspectionCounter

    func inspect(manifest: [String: JSONValue]) async -> CompanionInspectionOutcome {
        await counter.increment()
        return CompanionInspectionOutcome(
            status: .failed,
            errorCode: "SHOULD_NOT_RUN",
            responseSummary: "provider should not run without authentication"
        )
    }
}
