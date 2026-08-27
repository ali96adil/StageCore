import Foundation
#if os(Linux)
import Glibc
#else
import Darwin
#endif

public struct OSCEndpoint: Codable, Sendable, Equatable {
    public var host: String
    public var port: Int

    public init(host: String, port: Int) {
        self.host = host
        self.port = port
    }

    public var isValid: Bool {
        !host.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && (1...65535).contains(port)
    }
}

public enum OSCExecutorError: Error, Equatable {
    case invalidEndpoint
    case invalidParameters
    case sendFailed
}

protocol OSCDatagramSending: Sendable {
    func send(_ packet: Data, to endpoint: OSCEndpoint) throws -> Int
}

struct POSIXOSCDatagramSender: OSCDatagramSending {
    func send(_ packet: Data, to endpoint: OSCEndpoint) throws -> Int {
        guard endpoint.isValid, !packet.isEmpty else {
            throw OSCExecutorError.invalidEndpoint
        }

        var hints = addrinfo()
        hints.ai_family = AF_UNSPEC
        #if os(Linux)
        hints.ai_socktype = Int32(SOCK_DGRAM.rawValue)
        #else
        hints.ai_socktype = SOCK_DGRAM
        #endif

        var resolved: UnsafeMutablePointer<addrinfo>?
        let resolution = getaddrinfo(endpoint.host, String(endpoint.port), &hints, &resolved)
        guard resolution == 0, let first = resolved else {
            throw OSCExecutorError.sendFailed
        }
        defer { freeaddrinfo(first) }

        var cursor: UnsafeMutablePointer<addrinfo>? = first
        while let current = cursor {
            let info = current.pointee
            let descriptor = socket(info.ai_family, info.ai_socktype, info.ai_protocol)
            if descriptor >= 0 {
                var timeout = timeval(tv_sec: 0, tv_usec: 500_000)
                _ = setsockopt(
                    descriptor,
                    SOL_SOCKET,
                    SO_SNDTIMEO,
                    &timeout,
                    socklen_t(MemoryLayout<timeval>.size)
                )

                let sent: Int = packet.withUnsafeBytes { buffer in
                    guard let baseAddress = buffer.baseAddress else { return -1 }
                    return sendto(
                        descriptor,
                        baseAddress,
                        buffer.count,
                        0,
                        info.ai_addr,
                        info.ai_addrlen
                    )
                }
                _ = close(descriptor)
                if sent == packet.count {
                    return sent
                }
            }
            cursor = info.ai_next
        }

        throw OSCExecutorError.sendFailed
    }
}

public struct OSCSendExecutor: CompanionCapabilityExecutor {
    public let capabilityKey = "osc.send"

    private let endpoint: OSCEndpoint
    private let sender: any OSCDatagramSending

    public init(endpoint: OSCEndpoint) throws {
        guard endpoint.isValid else { throw OSCExecutorError.invalidEndpoint }
        self.endpoint = endpoint
        self.sender = POSIXOSCDatagramSender()
    }

    init(endpoint: OSCEndpoint, sender: any OSCDatagramSending) throws {
        guard endpoint.isValid else { throw OSCExecutorError.invalidEndpoint }
        self.endpoint = endpoint
        self.sender = sender
    }

    public func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        let packet: Data
        do {
            packet = try OSCPacketEncoder.encode(parameters: parameters)
        } catch {
            return CompanionCapabilityOutcome(
                status: .failed,
                ackLevel: .none,
                errorCode: "OSC_INVALID_PARAMETERS",
                responseSummary: "OSC parameters are invalid"
            )
        }

        do {
            let bytesSent = try sender.send(packet, to: endpoint)
            guard bytesSent == packet.count else {
                return CompanionCapabilityOutcome(
                    status: .failed,
                    ackLevel: .none,
                    errorCode: "OSC_SEND_FAILED",
                    responseSummary: "OSC UDP datagram was not fully sent"
                )
            }
            return CompanionCapabilityOutcome(
                status: .completed,
                ackLevel: .transportOnly,
                responseSummary: "OSC UDP datagram accepted by local transport",
                output: ["bytes_sent": .int(bytesSent)]
            )
        } catch {
            return CompanionCapabilityOutcome(
                status: .failed,
                ackLevel: .none,
                errorCode: "OSC_SEND_FAILED",
                responseSummary: "OSC UDP send failed"
            )
        }
    }
}

enum OSCPacketEncoder {
    static func encode(parameters: [String: JSONValue]) throws -> Data {
        guard case .string(let address)? = parameters["address"] else {
            throw OSCExecutorError.invalidParameters
        }
        try validateAddress(address)

        let rawArguments: [JSONValue]
        switch parameters["arguments"] {
        case .none:
            rawArguments = []
        case .some(.array(let values)):
            rawArguments = values
        default:
            throw OSCExecutorError.invalidParameters
        }

        let arguments = try rawArguments.map(parseArgument)
        var packet = Data()
        appendPaddedString(address, to: &packet)
        appendPaddedString("," + arguments.map(\.tag).joined(), to: &packet)
        for argument in arguments {
            try appendPayload(argument, to: &packet)
        }
        return packet
    }

    private enum Argument {
        case int32(Int32)
        case float32(Float)
        case string(String)
        case bool(Bool)

        var tag: String {
            switch self {
            case .int32: return "i"
            case .float32: return "f"
            case .string: return "s"
            case .bool(let value): return value ? "T" : "F"
            }
        }
    }

    private static func parseArgument(_ raw: JSONValue) throws -> Argument {
        guard case .object(let object) = raw,
              case .string(let type)? = object["type"] else {
            throw OSCExecutorError.invalidParameters
        }

        switch type {
        case "int32":
            guard case .int(let value)? = object["value"],
                  value >= Int(Int32.min), value <= Int(Int32.max) else {
                throw OSCExecutorError.invalidParameters
            }
            return .int32(Int32(value))

        case "float32":
            switch object["value"] {
            case .some(.double(let value)):
                guard value.isFinite else { throw OSCExecutorError.invalidParameters }
                return .float32(Float(value))
            case .some(.int(let value)):
                return .float32(Float(value))
            default:
                throw OSCExecutorError.invalidParameters
            }

        case "string":
            guard case .string(let value)? = object["value"] else {
                throw OSCExecutorError.invalidParameters
            }
            return .string(value)

        case "bool":
            guard case .bool(let value)? = object["value"] else {
                throw OSCExecutorError.invalidParameters
            }
            return .bool(value)

        default:
            throw OSCExecutorError.invalidParameters
        }
    }

    private static func validateAddress(_ address: String) throws {
        guard address.hasPrefix("/"),
              !address.unicodeScalars.contains(where: { $0.value == 0 || CharacterSet.whitespacesAndNewlines.contains($0) }) else {
            throw OSCExecutorError.invalidParameters
        }
    }

    private static func appendPaddedString(_ value: String, to packet: inout Data) {
        packet.append(contentsOf: value.utf8)
        packet.append(0)
        while packet.count % 4 != 0 {
            packet.append(0)
        }
    }

    private static func appendPayload(_ argument: Argument, to packet: inout Data) throws {
        switch argument {
        case .int32(let value):
            var bigEndian = value.bigEndian
            withUnsafeBytes(of: &bigEndian) { packet.append(contentsOf: $0) }

        case .float32(let value):
            var bigEndian = value.bitPattern.bigEndian
            withUnsafeBytes(of: &bigEndian) { packet.append(contentsOf: $0) }

        case .string(let value):
            appendPaddedString(value, to: &packet)

        case .bool:
            break
        }
    }
}
