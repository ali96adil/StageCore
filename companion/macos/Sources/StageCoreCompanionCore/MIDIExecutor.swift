import Foundation
#if os(macOS)
import CoreMIDI
#endif

public struct MIDIDestination: Codable, Sendable, Equatable {
    public var index: Int?
    public var name: String?

    public init(index: Int) {
        self.index = index
        self.name = nil
    }

    public init(name: String) {
        self.index = nil
        self.name = name.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    public var isValid: Bool {
        if let index {
            return index >= 0
        }
        return !(name ?? "").isEmpty
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
    private func endpointName(_ endpoint: MIDIEndpointRef) -> String {
        var value: Unmanaged<CFString>?
        if MIDIObjectGetStringProperty(endpoint, kMIDIPropertyDisplayName, &value) == noErr,
           let value {
            return (value.takeRetainedValue() as String).trimmingCharacters(in: .whitespacesAndNewlines)
        }
        value = nil
        if MIDIObjectGetStringProperty(endpoint, kMIDIPropertyName, &value) == noErr,
           let value {
            return (value.takeRetainedValue() as String).trimmingCharacters(in: .whitespacesAndNewlines)
        }
        return ""
    }

    private func resolve(_ destination: MIDIDestination) throws -> MIDIEndpointRef {
        guard destination.isValid else { throw MIDIExecutorError.invalidDestination }
        if let name = destination.name {
            let count = MIDIGetNumberOfDestinations()
            var match: MIDIEndpointRef = 0
            var matches = 0
            for index in 0..<count {
                let endpoint = MIDIGetDestination(index)
                if endpoint != 0 && endpointName(endpoint) == name {
                    match = endpoint
                    matches += 1
                }
            }
            guard matches == 1 && match != 0 else {
                throw MIDIExecutorError.destinationUnavailable
            }
            return match
        }
        guard let index = destination.index, index >= 0, index < MIDIGetNumberOfDestinations() else {
            throw MIDIExecutorError.destinationUnavailable
        }
        let endpoint = MIDIGetDestination(index)
        guard endpoint != 0 else { throw MIDIExecutorError.destinationUnavailable }
        return endpoint
    }

    func send(_ bytes: [UInt8], to destination: MIDIDestination) throws -> Int {
        let endpoint = try resolve(destination)

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

    private let sender: any MIDISending

    public init() throws {
        #if os(macOS)
        self.sender = CoreMIDISender()
        #else
        throw MIDIExecutorError.platformUnavailable
        #endif
    }

    init(sender: any MIDISending) {
        self.sender = sender
    }

    public func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        let destination: MIDIDestination
        let bytes: [UInt8]
        do {
            destination = try MIDIMessageEncoder.destination(parameters: parameters)
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
    static func destination(parameters: [String: JSONValue]) throws -> MIDIDestination {
        let hasName = parameters["destination_name"] != nil
        let hasIndex = parameters["destination_index"] != nil
        guard hasName != hasIndex else {
            throw MIDIExecutorError.invalidDestination
        }
        let destination: MIDIDestination
        if case .string(let rawName)? = parameters["destination_name"] {
            destination = MIDIDestination(name: rawName)
        } else if case .int(let index)? = parameters["destination_index"] {
            destination = MIDIDestination(index: index)
        } else {
            throw MIDIExecutorError.invalidDestination
        }
        guard destination.isValid else { throw MIDIExecutorError.invalidDestination }
        return destination
    }

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
