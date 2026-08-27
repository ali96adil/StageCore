import Foundation
#if os(macOS)
import CoreMIDI
#endif

public struct MIDIDestination: Codable, Sendable, Equatable {
    public var index: Int

    public init(index: Int) {
        self.index = index
    }

    public var isValid: Bool {
        index >= 0
    }
}

public enum MIDIExecutorError: Error, Equatable {
    case invalidDestination
    case platformUnavailable
    case destinationUnavailable
    case sendFailed
}

protocol MIDISending: Sendable {
    func send(_ bytes: [UInt8], to destination: MIDIDestination) throws -> Int
}

#if os(macOS)
struct CoreMIDISender: MIDISending {
    func send(_ bytes: [UInt8], to destination: MIDIDestination) throws -> Int {
        guard destination.isValid else { throw MIDIExecutorError.invalidDestination }
        guard destination.index < MIDIGetNumberOfDestinations() else {
            throw MIDIExecutorError.destinationUnavailable
        }
        let endpoint = MIDIGetDestination(destination.index)
        guard endpoint != 0 else { throw MIDIExecutorError.destinationUnavailable }

        var client = MIDIClientRef()
        guard MIDIClientCreate("StageCore Companion" as CFString, nil, nil, &client) == 0 else {
            throw MIDIExecutorError.sendFailed
        }
        defer { _ = MIDIClientDispose(client) }

        var port = MIDIPortRef()
        guard MIDIOutputPortCreate(client, "StageCore MIDI Out" as CFString, &port) == 0 else {
            throw MIDIExecutorError.sendFailed
        }
        defer { _ = MIDIPortDispose(port) }

        var packetList = MIDIPacketList()
        let firstPacket = MIDIPacketListInit(&packetList)
        let added = bytes.withUnsafeBufferPointer { buffer -> UnsafeMutablePointer<MIDIPacket>? in
            guard let base = buffer.baseAddress else { return nil }
            return MIDIPacketListAdd(
                &packetList,
                MemoryLayout<MIDIPacketList>.size,
                firstPacket,
                0,
                buffer.count,
                base
            )
        }
        guard added != nil else { throw MIDIExecutorError.sendFailed }
        guard MIDISend(port, endpoint, &packetList) == 0 else {
            throw MIDIExecutorError.sendFailed
        }
        return bytes.count
    }
}
#endif

public struct MIDISendExecutor: CompanionCapabilityExecutor {
    public let capabilityKey = "midi.send"

    private let destination: MIDIDestination
    private let sender: any MIDISending

    public init(destination: MIDIDestination) throws {
        guard destination.isValid else { throw MIDIExecutorError.invalidDestination }
        self.destination = destination
        #if os(macOS)
        self.sender = CoreMIDISender()
        #else
        throw MIDIExecutorError.platformUnavailable
        #endif
    }

    init(destination: MIDIDestination, sender: any MIDISending) throws {
        guard destination.isValid else { throw MIDIExecutorError.invalidDestination }
        self.destination = destination
        self.sender = sender
    }

    public func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        let bytes: [UInt8]
        do {
            bytes = try MIDIMessageEncoder.encode(parameters: parameters)
        } catch {
            return CompanionCapabilityOutcome(
                status: .failed,
                ackLevel: .none,
                errorCode: "MIDI_INVALID_PARAMETERS",
                responseSummary: "MIDI parameters are invalid"
            )
        }

        do {
            let sent = try sender.send(bytes, to: destination)
            guard sent == bytes.count else {
                return CompanionCapabilityOutcome(
                    status: .failed,
                    ackLevel: .none,
                    errorCode: "MIDI_SEND_FAILED",
                    responseSummary: "MIDI message was not fully accepted by local transport"
                )
            }
            return CompanionCapabilityOutcome(
                status: .completed,
                ackLevel: .transportOnly,
                responseSummary: "MIDI message accepted by CoreMIDI local transport",
                output: ["bytes_sent": .int(sent)]
            )
        } catch MIDIExecutorError.destinationUnavailable {
            return CompanionCapabilityOutcome(
                status: .failed,
                ackLevel: .none,
                errorCode: "MIDI_DESTINATION_UNAVAILABLE",
                responseSummary: "configured MIDI destination is unavailable"
            )
        } catch {
            return CompanionCapabilityOutcome(
                status: .failed,
                ackLevel: .none,
                errorCode: "MIDI_SEND_FAILED",
                responseSummary: "MIDI send failed"
            )
        }
    }
}

enum MIDIMessageEncoder {
    static func encode(parameters: [String: JSONValue]) throws -> [UInt8] {
        guard case .array(let rawBytes)? = parameters["bytes"] else {
            throw MIDIExecutorError.sendFailed
        }
        let values = try rawBytes.map { value -> UInt8 in
            guard case .int(let integer) = value, (0...255).contains(integer) else {
                throw MIDIExecutorError.sendFailed
            }
            return UInt8(integer)
        }
        guard let status = values.first, (0x80...0xEF).contains(status) else {
            throw MIDIExecutorError.sendFailed
        }
        let expectedLength: Int
        switch status & 0xF0 {
        case 0xC0, 0xD0:
            expectedLength = 2
        default:
            expectedLength = 3
        }
        guard values.count == expectedLength else {
            throw MIDIExecutorError.sendFailed
        }
        for dataByte in values.dropFirst() where dataByte > 0x7F {
            throw MIDIExecutorError.sendFailed
        }
        return values
    }
}
