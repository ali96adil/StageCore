import Foundation
#if os(Linux)
import Glibc
#else
import Darwin
#endif

public enum LocalOSCControlError: Error, Equatable {
    case invalidPort
    case socketFailed
    case bindFailed
    case receiveFailed
    case unsupportedPlatform
}

struct LocalOSCControlParser {
    static func isGo(_ packet: Data) -> Bool {
        let bytes = [UInt8](packet)
        var offset = 0
        guard let address = readPaddedString(bytes, offset: &offset),
              address == "/stagecore/go",
              let typeTags = readPaddedString(bytes, offset: &offset)
        else {
            return false
        }

        switch typeTags {
        case ",":
            return trailingBytesAreZero(bytes, from: offset)
        case ",i":
            guard offset + 4 <= bytes.count else { return false }
            let raw =
                UInt32(bytes[offset]) << 24 |
                UInt32(bytes[offset + 1]) << 16 |
                UInt32(bytes[offset + 2]) << 8 |
                UInt32(bytes[offset + 3])
            offset += 4
            return Int32(bitPattern: raw) == 1 && trailingBytesAreZero(bytes, from: offset)
        default:
            return false
        }
    }

    private static func readPaddedString(_ bytes: [UInt8], offset: inout Int) -> String? {
        guard offset < bytes.count else { return nil }
        var end = offset
        while end < bytes.count && bytes[end] != 0 {
            end += 1
        }
        guard end < bytes.count,
              let value = String(bytes: bytes[offset..<end], encoding: .utf8)
        else {
            return nil
        }
        let next = ((end + 1 + 3) / 4) * 4
        guard next <= bytes.count else { return nil }
        offset = next
        return value
    }

    private static func trailingBytesAreZero(_ bytes: [UInt8], from offset: Int) -> Bool {
        guard offset <= bytes.count else { return false }
        return bytes[offset...].allSatisfy { $0 == 0 }
    }
}

public struct LocalOSCControlListener: Sendable {
    public let port: Int

    public init(port: Int) throws {
        guard (1...65535).contains(port) else {
            throw LocalOSCControlError.invalidPort
        }
        self.port = port
    }

    public func run(
        handler: @escaping @Sendable (CompanionControlSurfaceEvent) async throws -> Void
    ) async throws {
        #if os(macOS)
        let descriptor = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP)
        guard descriptor >= 0 else {
            throw LocalOSCControlError.socketFailed
        }
        defer { _ = close(descriptor) }

        var timeout = timeval(tv_sec: 0, tv_usec: 250_000)
        _ = setsockopt(
            descriptor,
            SOL_SOCKET,
            SO_RCVTIMEO,
            &timeout,
            socklen_t(MemoryLayout<timeval>.size)
        )

        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = in_port_t(port).bigEndian
        address.sin_addr = in_addr(s_addr: inet_addr("127.0.0.1"))

        let bindResult = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                bind(descriptor, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        guard bindResult == 0 else {
            throw LocalOSCControlError.bindFailed
        }

        var buffer = [UInt8](repeating: 0, count: 2048)
        var lastAccepted = Date.distantPast

        while !Task.isCancelled {
            let count = buffer.withUnsafeMutableBytes { storage -> Int in
                guard let base = storage.baseAddress else { return -1 }
                return recv(descriptor, base, storage.count, 0)
            }
            if count < 0 {
                if errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR {
                    await Task.yield()
                    continue
                }
                throw LocalOSCControlError.receiveFailed
            }
            guard count > 0 else {
                await Task.yield()
                continue
            }

            let packet = Data(buffer.prefix(count))
            guard LocalOSCControlParser.isGo(packet) else {
                continue
            }

            let now = Date()
            guard now.timeIntervalSince(lastAccepted) >= 0.25 else {
                continue
            }
            lastAccepted = now
            try await handler(CompanionControlSurfaceEvent(action: "GO"))
        }
        throw CancellationError()
        #else
        throw LocalOSCControlError.unsupportedPlatform
        #endif
    }
}
