import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class ExecutionEnvironmentOperationTests: XCTestCase {
    func testSnapshotProviderReturnsTypedIdentityAndSnapshot() async throws {
        let executor = try ExecutionEnvironmentOperationExecutor(
            providers: [FixedEnvironmentOperationProvider()]
        )
        let sourceHash = String(repeating: "a", count: 64)
        let outcome = await executor.execute(parameters: [
            "operation_kind": .string("CAPTURE_SNAPSHOT"),
            "adapter_key": .string("test.environment"),
            "source_manifest_sha256": .string(sourceHash),
            "manifest": .object([
                "schema_version": .int(1),
                "environment_key": .string("video-main"),
                "adapter_key": .string("test.environment"),
            ]),
        ])

        XCTAssertEqual(outcome.status, .completed)
        XCTAssertEqual(outcome.ackLevel, .accepted)
        XCTAssertNil(outcome.errorCode)
        XCTAssertEqual(outcome.output["operation_kind"], .string("CAPTURE_SNAPSHOT"))
        XCTAssertEqual(outcome.output["adapter_key"], .string("test.environment"))
        XCTAssertEqual(outcome.output["source_manifest_sha256"], .string(sourceHash))
        guard case .object(let snapshot)? = outcome.output["snapshot"] else {
            return XCTFail("expected typed snapshot output")
        }
        XCTAssertEqual(snapshot["environment_key"], .string("video-main"))
        XCTAssertEqual(snapshot["capture_status"], .string("PARTIAL"))
    }

    func testRestoreRequiresHubSelectedSnapshotAndReturnsNoSnapshotOutput() async throws {
        let executor = try ExecutionEnvironmentOperationExecutor(
            providers: [RestoreEnvironmentOperationProvider()]
        )
        var parameters = validParameters(kind: "RESTORE_OBSERVABLE_STATE")
        parameters["adapter_key"] = .string("test.restore")
        parameters["snapshot"] = .object([
            "schema_version": .int(1),
            "environment_key": .string("video-main"),
            "adapter_key": .string("test.restore"),
            "source_manifest_sha256": .string(String(repeating: "a", count: 64)),
            "capture_status": .string("PARTIAL"),
        ])

        let outcome = await executor.execute(parameters: parameters)

        XCTAssertEqual(outcome.status, .completed)
        XCTAssertEqual(outcome.ackLevel, .accepted)
        XCTAssertEqual(outcome.output["operation_kind"], .string("RESTORE_OBSERVABLE_STATE"))
        XCTAssertNil(outcome.output["snapshot"])
    }

    func testRestoreWithoutSnapshotFailsBeforeProvider() async throws {
        let executor = try ExecutionEnvironmentOperationExecutor(
            providers: [RestoreEnvironmentOperationProvider()]
        )
        var parameters = validParameters(kind: "RESTORE_OBSERVABLE_STATE")
        parameters["adapter_key"] = .string("test.restore")

        let outcome = await executor.execute(parameters: parameters)

        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.errorCode, "ENVIRONMENT_RESTORE_SNAPSHOT_REQUIRED")
    }

    func testUnknownAdapterFailsTruthfully() async throws {
        let executor = try ExecutionEnvironmentOperationExecutor()
        let outcome = await executor.execute(parameters: validParameters(kind: "OPEN"))
        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.ackLevel, .none)
        XCTAssertEqual(outcome.errorCode, "ENVIRONMENT_ADAPTER_UNSUPPORTED")
        XCTAssertTrue(outcome.output.isEmpty)
    }

    func testProviderUnsupportedOperationDoesNotFallBack() async throws {
        let executor = try ExecutionEnvironmentOperationExecutor(
            providers: [FixedEnvironmentOperationProvider()]
        )
        let outcome = await executor.execute(parameters: validParameters(kind: "OPEN"))
        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.errorCode, "ENVIRONMENT_OPERATION_UNSUPPORTED")
        XCTAssertTrue(outcome.output.isEmpty)
    }

    func testDuplicateAdapterKeysAreRejected() {
        XCTAssertThrowsError(
            try ExecutionEnvironmentOperationExecutor(providers: [
                FixedEnvironmentOperationProvider(),
                FixedEnvironmentOperationProvider(),
            ])
        ) { error in
            XCTAssertEqual(
                error as? ExecutionEnvironmentOperationExecutorError,
                .duplicateAdapterKey("test.environment")
            )
        }
    }

    private func validParameters(kind: String) -> [String: JSONValue] {
        [
            "operation_kind": .string(kind),
            "adapter_key": .string("test.environment"),
            "source_manifest_sha256": .string(String(repeating: "a", count: 64)),
            "manifest": .object([
                "schema_version": .int(1),
                "environment_key": .string("video-main"),
                "adapter_key": .string("test.environment"),
            ]),
        ]
    }
}

private struct RestoreEnvironmentOperationProvider: ExecutionEnvironmentOperationProvider {
    let adapterKey = "test.restore"
    let supportedOperations: Set<ExecutionEnvironmentOperationKind> = [.restoreObservableState]

    func perform(
        kind: ExecutionEnvironmentOperationKind,
        manifest: [String: JSONValue],
        sourceManifestSHA256: String,
        snapshot: [String: JSONValue]?
    ) async -> ExecutionEnvironmentProviderOutcome {
        guard kind == .restoreObservableState,
              snapshot != nil
        else {
            return .init(
                status: .failed,
                errorCode: "TEST_RESTORE_INVALID",
                responseSummary: "restore input missing"
            )
        }
        return .init(
            status: .completed,
            responseSummary: "test restore completed"
        )
    }
}

private struct FixedEnvironmentOperationProvider: ExecutionEnvironmentOperationProvider {
    let adapterKey = "test.environment"
    let supportedOperations: Set<ExecutionEnvironmentOperationKind> = [.captureSnapshot]

    func perform(
        kind: ExecutionEnvironmentOperationKind,
        manifest: [String: JSONValue],
        sourceManifestSHA256: String,
        snapshot: [String: JSONValue]?
    ) async -> ExecutionEnvironmentProviderOutcome {
        guard kind == .captureSnapshot else {
            return .init(
                status: .unsupported,
                errorCode: "TEST_UNSUPPORTED",
                responseSummary: "test provider does not support this operation"
            )
        }
        return .init(
            status: .completed,
            responseSummary: "test snapshot captured",
            snapshot: [
                "schema_version": .int(1),
                "environment_key": manifest["environment_key"] ?? .string("unknown"),
                "adapter_key": .string(adapterKey),
                "source_manifest_sha256": .string(sourceManifestSHA256),
                "capture_status": .string("PARTIAL"),
                "notes": .string("test snapshot"),
            ]
        )
    }
}
