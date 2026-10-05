import Foundation
import XCTest
@testable import StageCoreCompanionCore

final class MIDISendExecutorTests: XCTestCase {
    func testExecutorSendsValidatedChannelMessageAndReturnsTransportAck() async {
        let sender = RecordingMIDISender()
        let executor = MIDISendExecutor(sender: sender)

        let outcome = await executor.execute(parameters: [
            "destination_index": .int(2),
            "bytes": .array([.int(0x90), .int(60), .int(127)]),
        ])

        XCTAssertEqual(outcome.status, .completed)
        XCTAssertEqual(outcome.ackLevel, .transportOnly)
        XCTAssertNil(outcome.errorCode)
        XCTAssertEqual(outcome.output["bytes_sent"], .int(3))
        XCTAssertEqual(sender.destination, MIDIDestination(index: 2))
        XCTAssertEqual(sender.bytes, [0x90, 60, 127])
        XCTAssertEqual(sender.sendCount, 1)
    }

    func testExecutorAcceptsStableDestinationName() async {
        let sender = RecordingMIDISender()
        let executor = MIDISendExecutor(sender: sender)

        let outcome = await executor.execute(parameters: [
            "destination_name": .string("IAC Driver Bus 1"),
            "bytes": .array([.int(0xB0), .int(7), .int(100)]),
        ])

        XCTAssertEqual(outcome.status, .completed)
        XCTAssertEqual(outcome.ackLevel, .transportOnly)
        XCTAssertEqual(sender.destination, MIDIDestination(name: "IAC Driver Bus 1"))
        XCTAssertEqual(sender.bytes, [0xB0, 7, 100])
    }

    func testDestinationRejectsNameAndIndexTogether() async {
        let sender = RecordingMIDISender()
        let executor = MIDISendExecutor(sender: sender)

        let outcome = await executor.execute(parameters: [
            "destination_name": .string("IAC Driver Bus 1"),
            "destination_index": .int(0),
            "bytes": .array([.int(0x90), .int(60), .int(100)]),
        ])

        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.errorCode, "MIDI_INVALID_PARAMETERS")
        XCTAssertEqual(sender.sendCount, 0)
    }

    func testInvalidMIDIParametersFailWithoutSending() async {
        let sender = RecordingMIDISender()
        let executor = MIDISendExecutor(sender: sender)

        let outcome = await executor.execute(parameters: [
            "destination_index": .int(0),
            "bytes": .array([.int(0x90), .int(60), .int(255)]),
        ])

        XCTAssertEqual(outcome.status, .failed)
        XCTAssertEqual(outcome.ackLevel, .none)
        XCTAssertEqual(outcome.errorCode, "MIDI_INVALID_PARAMETERS")
        XCTAssertEqual(sender.sendCount, 0)
    }

    func testCompanionHelloRefreshesMIDIDestinationInventoryWithoutRestart() async throws {
        let inventory = MutableMIDIDestinationInventory()
        let session = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: "11111111-1111-4111-8111-111111111111",
                agentVersion: "0.1.0",
                platform: "macos",
                architecture: "arm64"
            ),
            executors: [MIDISendExecutor(sender: RecordingMIDISender())],
            midiDestinationInventory: { inventory.current() }
        )

        var hello = try JSONDecoder().decode(
            CompanionHello.self,
            from: await session.helloData()
        )
        XCTAssertEqual(hello.midiDestinations, [])

        inventory.set([
            "IAC Driver Bus 1",
            "IAC Driver StageCore Ableton",
            "IAC Driver xdmxx",
        ])

        hello = try JSONDecoder().decode(
            CompanionHello.self,
            from: await session.helloData()
        )
        XCTAssertEqual(
            hello.midiDestinations,
            [
                "IAC Driver Bus 1",
                "IAC Driver StageCore Ableton",
                "IAC Driver xdmxx",
            ]
        )
    }

    func testCompanionSessionExecutesMIDIOnceAndRejectsDuplicateExecution() async throws {
        let sender = RecordingMIDISender()
        let executor = MIDISendExecutor(sender: sender)
        let session = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: "11111111-1111-4111-8111-111111111111",
                agentVersion: "0.1.0",
                platform: "macos",
                architecture: "arm64"
            ),
            executors: [executor]
        )

        let hello = try JSONDecoder().decode(CompanionHello.self, from: await session.helloData())
        XCTAssertEqual(hello.capabilities, ["midi.send"])

        await session.establishAuthenticatedSession(
            CompanionRuntimeCredential(
                sessionID: "session-midi",
                token: "not-persisted",
                expiresAt: Date().addingTimeInterval(300)
            )
        )
        _ = try await session.handle(JSONEncoder().encode(SessionReady(
            machineRoleID: "role-midi-main",
            roleKey: "MIDI-MAIN",
            runtimeSnapshotID: "snapshot-midi",
            configHash: ""
        )))

        let request = CompanionExecutionRequest(
            executionID: "execution-midi",
            correlationID: "correlation-midi",
            machineRoleID: "role-midi-main",
            runtimeSnapshotID: "snapshot-midi",
            capability: "midi.send",
            parameters: [
                "destination_index": .int(0),
                "bytes": .array([.int(0xB0), .int(7), .int(100)]),
            ],
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

private final class RecordingMIDISender: MIDISending, @unchecked Sendable {
    private let lock = NSLock()
    private var storedBytes: [UInt8] = []
    private var storedDestination: MIDIDestination?
    private var storedSendCount = 0

    var bytes: [UInt8] {
        lock.withLock { storedBytes }
    }

    var destination: MIDIDestination? {
        lock.withLock { storedDestination }
    }

    var sendCount: Int {
        lock.withLock { storedSendCount }
    }

    func send(_ bytes: [UInt8], to destination: MIDIDestination) throws -> Int {
        lock.withLock {
            storedBytes = bytes
            storedDestination = destination
            storedSendCount += 1
        }
        return bytes.count
    }
}


private final class MutableMIDIDestinationInventory: @unchecked Sendable {
    private let lock = NSLock()
    private var names: [String] = []

    func current() -> [String] {
        lock.withLock { names }
    }

    func set(_ value: [String]) {
        lock.withLock {
            names = value
        }
    }
}
