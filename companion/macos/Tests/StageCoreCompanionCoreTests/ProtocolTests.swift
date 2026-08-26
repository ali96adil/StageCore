import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class ProtocolTests: XCTestCase {
    func testExecutionRequestRoundTripsNestedJSON() throws {
        let request = CompanionExecutionRequest(
            messageID: "00000000-0000-4000-8000-000000000001",
            executionID: "exec-1",
            correlationID: "corr-1",
            machineRoleID: "role-1",
            runtimeSnapshotID: "snap-1",
            capability: "osc.send",
            parameters: [
                "address": .string("/scene/go"),
                "arguments": .array([
                    .object(["type": .string("int32"), "value": .int(4)]),
                    .object(["type": .string("bool"), "value": .bool(true)]),
                ]),
            ],
            timeoutMS: 500
        )

        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let data = try encoder.encode(request)
        let decoded = try JSONDecoder().decode(CompanionExecutionRequest.self, from: data)

        XCTAssertEqual(decoded, request)
        let json = try XCTUnwrap(String(data: data, encoding: .utf8))
        XCTAssertTrue(json.contains("\"schema_version\":1"))
        XCTAssertTrue(json.contains("\"runtime_snapshot_id\":\"snap-1\""))
        XCTAssertTrue(json.contains("\"type\":\"execution.request\""))
    }

    func testExecutionResultRoundTripsTruthfulFailure() throws {
        let result = CompanionExecutionResult(
            messageID: "00000000-0000-4000-8000-000000000002",
            executionID: "exec-2",
            status: .failed,
            ackLevel: .none,
            errorCode: "COMPANION_EXECUTION_INTERRUPTED",
            responseSummary: "completion is unknown"
        )
        let data = try JSONEncoder().encode(result)
        XCTAssertEqual(try JSONDecoder().decode(CompanionExecutionResult.self, from: data), result)
    }
}
