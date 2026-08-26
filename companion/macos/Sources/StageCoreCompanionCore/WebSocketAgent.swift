import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public enum CompanionTransportSecurityPolicy: Sendable {
    case production
    case allowInsecureLoopbackForTesting
}

public enum CompanionTransportError: Error, Equatable {
    case unsupportedScheme
    case insecureTransportNotAllowed
    case invalidLoopbackHost
    case disconnected
}

public enum CompanionTransportPolicy {
    public static func validate(
        url: URL,
        policy: CompanionTransportSecurityPolicy
    ) throws {
        guard let scheme = url.scheme?.lowercased() else {
            throw CompanionTransportError.unsupportedScheme
        }
        if scheme == "wss" {
            return
        }
        guard scheme == "ws" else {
            throw CompanionTransportError.unsupportedScheme
        }
        guard case .allowInsecureLoopbackForTesting = policy else {
            throw CompanionTransportError.insecureTransportNotAllowed
        }
        guard isLoopbackHost(url.host) else {
            throw CompanionTransportError.invalidLoopbackHost
        }
    }

    private static func isLoopbackHost(_ host: String?) -> Bool {
        guard let host = host?.lowercased() else { return false }
        return host == "localhost" || host == "127.0.0.1" || host == "::1"
    }
}

public actor WebSocketCompanionAgent {
    private let url: URL
    private let securityPolicy: CompanionTransportSecurityPolicy
    private let session: CompanionSession
    private let reconnectDelay: Duration
    private let maxReconnects: Int

    public init(
        url: URL,
        securityPolicy: CompanionTransportSecurityPolicy = .production,
        session: CompanionSession,
        reconnectDelay: Duration = .milliseconds(250),
        maxReconnects: Int = 8
    ) throws {
        try CompanionTransportPolicy.validate(url: url, policy: securityPolicy)
        self.url = url
        self.securityPolicy = securityPolicy
        self.session = session
        self.reconnectDelay = reconnectDelay
        self.maxReconnects = max(0, maxReconnects)
    }

    public func run() async throws {
        // Revalidate at execution time as defense in depth if URL policy is
        // later made configurable by the app shell.
        try CompanionTransportPolicy.validate(url: url, policy: securityPolicy)

        var failures = 0
        while true {
            do {
                try await runOneConnection()
                return
            } catch is CancellationError {
                throw CancellationError()
            } catch {
                failures += 1
                guard failures <= maxReconnects else {
                    throw error
                }
                try await Task.sleep(for: reconnectDelay)
            }
        }
    }

    private func runOneConnection() async throws {
        let urlSession = URLSession(configuration: .ephemeral)
        let socket = urlSession.webSocketTask(with: url)
        socket.resume()
        defer {
            socket.cancel(with: .normalClosure, reason: nil)
            urlSession.invalidateAndCancel()
        }

        try await send(try await session.helloData(), socket: socket)
        while !Task.isCancelled {
            let data = try await receive(socket: socket)
            if let response = try await session.handle(data) {
                try await send(response, socket: socket)
            }
        }
        throw CancellationError()
    }

    private func send(_ data: Data, socket: URLSessionWebSocketTask) async throws {
        guard let text = String(data: data, encoding: .utf8) else {
            throw CompanionTransportError.disconnected
        }
        try await socket.send(.string(text))
    }

    private func receive(socket: URLSessionWebSocketTask) async throws -> Data {
        switch try await socket.receive() {
        case .string(let text):
            return Data(text.utf8)
        case .data(let data):
            return data
        @unknown default:
            throw CompanionTransportError.disconnected
        }
    }
}
