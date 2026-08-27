import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class OSCSendExecutorTests: XCTestCase {
    func testExecutorEncodesCanonicalOSCParametersAndReturnsTransportAck() async throws {
        let sender = RecordingOSCSender()
        let endpoint = OSCEndpoint(host: "127.0.0.1", port: 9000)
        let executor = try OSCSendExecutor(endpoint: endpoint, sender: sender)

        let outcome = await executor.execute(parameters: [
            "address": .string("/go"),
            "arguments": .array([
                .object(["type": .string("int32"), "value": .int(7)]),
                .object(["type": .string("float32"), "value": .double(1.5)]),
                .object(["type": .string("string"), "value": .string("hi")]),
                .object(["type": .string("bool"), "value": .bool(true)]),
            ]),
        ])

        XCTAssertEqual(outcome.status, .completed)
        XCTAssertEqual(outcome.ackLevel, .transportOnly)
        XCTAssertNil(outcome.errorCode)
        XCTAssertEqual(outcome.output["bytes_sent"], .int(24))
        XCTAssertEqual(sender.endpoint, endpoint)
        XCTAssertEqual(
            sender.packet,
            Data([
                0x2f, 0x67, 0x6f, 0x00,
                0x2c, 0x69, 0x66, 0x73, 0x54, 0x00, 0x00, 0x00,
                0x00, 0x00, 0x00, 0x07,
                0x3f, 0xc0, 0x00, 0x00,
                0x68, 0x69, 0x00, 0x00,
            ])
        )
    }

    func testInvalidOSCParametersFailWithoutSending() async throws {
        let sender = RecordingOSCSender()
        let executor = try OSCSendExecutor(
            endpoint: OSCEndpoint(host: "127.0.0.1", port: 9000),
            sender: sender
        )

        let outcome = await executor.execute(parameters: [
            "address": .string("missing-leading-slash"),
        ])

        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.ackLevel, .none)
        XCTAssertEqual(outcome.errorCode, "OSC_INVALID_PARAMETERS")
        XCTAssertEqual(sender.sendCount, 0)
    }

    func testCompanionSessionAdvertisesAndExecutesOSCThroughExistingGuard() async throws {
        let sender = RecordingOSCSender()
        let executor = try OSCSendExecutor(
            endpoint: OSCEndpoint(host: "127.0.0.1", port: 9000),
            sender: sender
        )
        let session = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: "11111111-1111-4111-8111-111111111111",
                agentVersion: "0.1.0",
                platform: "macos",
                architecture: "arm64"
            ),
            executors: [executor]
        )

        let initialHello = try JSONDecoder().decode(CompanionHello.self, from: await session.helloData())
        XCTAssertEqual(initialHello.capabilities, ["osc.send"])

        await session.establishAuthenticatedSession(
            CompanionRuntimeCredential(
                sessionID: "session-osc",
                token: "not-persisted",
                expiresAt: Date().addingTimeInterval(300)
            )
        )
        _ = try await session.handle(JSONEncoder().encode(SessionReady(
            machineRoleID: "role-video-main",
            roleKey: "VIDEO-MAIN",
            runtimeSnapshotID: "snapshot-osc",
            configHash: ""
        )))

        let request = CompanionExecutionRequest(
            executionID: "execution-osc",
            correlationID: "correlation-osc",
            machineRoleID: "role-video-main",
            runtimeSnapshotID: "snapshot-osc",
            capability: "osc.send",
            parameters: ["address": .string("/stage/go")],
            timeoutMS: 250
        )
        let response = try await session.handle(JSONEncoder().encode(request))
        let result = try JSONDecoder().decode(
            CompanionExecutionResult.self,
            from: try XCTUnwrap(response)
        )

        XCTAssertEqual(result.status, .completed)
        XCTAssertEqual(result.ackLevel, .transportOnly)
        XCTAssertEqual(sender.sendCount, 1)

        let duplicateResponse = try await session.handle(JSONEncoder().encode(request))
        let duplicate = try JSONDecoder().decode(
            CompanionExecutionResult.self,
            from: try XCTUnwrap(duplicateResponse)
        )
        XCTAssertEqual(duplicate.status, .rejected)
        XCTAssertEqual(duplicate.errorCode, "DUPLICATE_EXECUTION")
        XCTAssertEqual(sender.sendCount, 1)
    }
}

private final class RecordingOSCSender: OSCDatagramSending, @unchecked Sendable {
    private let lock = NSLock()
    private var storedPacket: Data?
    private var storedEndpoint: OSCEndpoint?
    private var storedSendCount = 0

    var packet: Data? {
        lock.withLock { storedPacket }
    }

    var endpoint: OSCEndpoint? {
        lock.withLock { storedEndpoint }
    }

    var sendCount: Int {
        lock.withLock { storedSendCount }
    }

    func send(_ packet: Data, to endpoint: OSCEndpoint) throws -> Int {
        lock.withLock {
            storedPacket = packet
            storedEndpoint = endpoint
            storedSendCount += 1
        }
        return packet.count
    }
}
