import Foundation

public struct Envelope: Codable, Sendable {
    public var type: String
    public var version: Int
    public var messageID: String?
    public var companionID: String?
    public var agentVersion: String?
    public var platform: String?
    public var role: String?
    public var runtimeSnapshotID: String?
    public var executionID: String?
    public var capability: String?
    public var status: String?
    public var ackLevel: String?
    public var errorCode: String?
    public var parameters: [String: JSONValue]?
    public var output: [String: JSONValue]?
    public var capabilities: [String]?

    enum CodingKeys: String, CodingKey {
        case type, version
        case messageID = "message_id"
        case companionID = "companion_id"
        case agentVersion = "agent_version"
        case platform
        case role
        case runtimeSnapshotID = "runtime_snapshot_id"
        case executionID = "execution_id"
        case capability, status
        case ackLevel = "ack_level"
        case errorCode = "error_code"
        case parameters, output, capabilities
    }

    public init(
        type: String,
        version: Int = 1,
        messageID: String? = nil,
        companionID: String? = nil,
        agentVersion: String? = nil,
        platform: String? = nil,
        role: String? = nil,
        runtimeSnapshotID: String? = nil,
        executionID: String? = nil,
        capability: String? = nil,
        status: String? = nil,
        ackLevel: String? = nil,
        errorCode: String? = nil,
        parameters: [String: JSONValue]? = nil,
        output: [String: JSONValue]? = nil,
        capabilities: [String]? = nil
    ) {
        self.type = type
        self.version = version
        self.messageID = messageID
        self.companionID = companionID
        self.agentVersion = agentVersion
        self.platform = platform
        self.role = role
        self.runtimeSnapshotID = runtimeSnapshotID
        self.executionID = executionID
        self.capability = capability
        self.status = status
        self.ackLevel = ackLevel
        self.errorCode = errorCode
        self.parameters = parameters
        self.output = output
        self.capabilities = capabilities
    }
}

public enum JSONValue: Codable, Sendable, Equatable {
    case string(String)
    case int(Int)
    case double(Double)
    case bool(Bool)
    case null

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null; return }
        if let v = try? c.decode(Bool.self) { self = .bool(v); return }
        if let v = try? c.decode(Int.self) { self = .int(v); return }
        if let v = try? c.decode(Double.self) { self = .double(v); return }
        if let v = try? c.decode(String.self) { self = .string(v); return }
        throw DecodingError.typeMismatch(JSONValue.self, .init(codingPath: decoder.codingPath, debugDescription: "unsupported JSON value"))
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .string(let v): try c.encode(v)
        case .int(let v): try c.encode(v)
        case .double(let v): try c.encode(v)
        case .bool(let v): try c.encode(v)
        case .null: try c.encodeNil()
        }
    }

    public var stringValue: String? {
        if case .string(let value) = self { return value }
        return nil
    }
}

public protocol CompanionIdentityStore: Sendable {
    func loadOrCreate() throws -> String
}

public struct FileIdentityStore: CompanionIdentityStore, Sendable {
    public let url: URL

    public init(url: URL) {
        self.url = url
    }

    public func loadOrCreate() throws -> String {
        if let data = try? Data(contentsOf: url),
           let existing = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines),
           !existing.isEmpty {
            return existing
        }

        let id = "cmp_" + UUID().uuidString.lowercased()
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data((id + "\n").utf8).write(to: url, options: .atomic)
        return id
    }
}

public enum ExecutionDecision: Equatable, Sendable {
    case execute
    case rejectDuplicate
    case rejectSnapshotMismatch
    case rejectUnsupportedCapability
}

public struct ExecutionGuard: Sendable {
    private var completed: Set<String> = []

    public init() {}

    public mutating func decision(
        executionID: String,
        requestSnapshotID: String,
        appliedSnapshotID: String?,
        capability: String
    ) -> ExecutionDecision {
        if completed.contains(executionID) {
            return .rejectDuplicate
        }
        guard let appliedSnapshotID, requestSnapshotID == appliedSnapshotID else {
            return .rejectSnapshotMismatch
        }
        guard capability == "local.echo" else {
            return .rejectUnsupportedCapability
        }
        return .execute
    }

    public mutating func markCompleted(_ executionID: String) {
        completed.insert(executionID)
        if completed.count > 512 {
            completed.removeAll(keepingCapacity: true)
        }
    }
}

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public enum CompanionClientError: Error, CustomStringConvertible {
    case invalidMessage
    case disconnected
    case reconnectLimit

    public var description: String {
        switch self {
        case .invalidMessage: return "invalid message"
        case .disconnected: return "disconnected"
        case .reconnectLimit: return "reconnect limit reached"
        }
    }
}

public actor CompanionClient {
    private let url: URL
    private let companionID: String
    private let version: String
    private var role: String?
    private var appliedSnapshotID: String?
    private var guardState = ExecutionGuard()
    private var reconnectCount = 0
    private let maxReconnects: Int

    public init(url: URL, companionID: String, version: String = "0.1.0", maxReconnects: Int = 4) {
        self.url = url
        self.companionID = companionID
        self.version = version
        self.maxReconnects = maxReconnects
    }

    public func run() async throws {
        while reconnectCount <= maxReconnects {
            do {
                let complete = try await runOneConnection()
                if complete { return }
            } catch {
                reconnectCount += 1
                if reconnectCount > maxReconnects { throw CompanionClientError.reconnectLimit }
                try await Task.sleep(for: .milliseconds(150))
            }
        }
        throw CompanionClientError.reconnectLimit
    }

    private func runOneConnection() async throws -> Bool {
        let session = URLSession(configuration: .ephemeral)
        let socket = session.webSocketTask(with: url)
        socket.resume()
        defer {
            socket.cancel(with: .normalClosure, reason: nil)
            session.invalidateAndCancel()
        }

        try await send(
            Envelope(
                type: "companion.hello",
                companionID: companionID,
                agentVersion: version,
                platform: Self.platformName,
                runtimeSnapshotID: appliedSnapshotID,
                capabilities: ["local.echo"]
            ),
            on: socket
        )

        while true {
            let message = try await socket.receive()
            let envelope = try decode(message)
            switch envelope.type {
            case "session.ready":
                role = envelope.role
                appliedSnapshotID = envelope.runtimeSnapshotID
                print("READY role=\(role ?? "-") snapshot=\(appliedSnapshotID ?? "-") companion=\(companionID)")

            case "execution.request":
                try await handleExecution(envelope, on: socket)

            case "test.complete":
                print("TEST_COMPLETE companion=\(companionID)")
                return true

            default:
                continue
            }
        }
    }

    private func handleExecution(_ request: Envelope, on socket: URLSessionWebSocketTask) async throws {
        guard let executionID = request.executionID,
              let requestSnapshot = request.runtimeSnapshotID,
              let capability = request.capability else {
            throw CompanionClientError.invalidMessage
        }

        let decision = guardState.decision(
            executionID: executionID,
            requestSnapshotID: requestSnapshot,
            appliedSnapshotID: appliedSnapshotID,
            capability: capability
        )

        switch decision {
        case .rejectDuplicate:
            try await send(Envelope(type: "execution.result", runtimeSnapshotID: requestSnapshot, executionID: executionID, capability: capability, status: "REJECTED", errorCode: "DUPLICATE_EXECUTION"), on: socket)
        case .rejectSnapshotMismatch:
            try await send(Envelope(type: "execution.result", runtimeSnapshotID: requestSnapshot, executionID: executionID, capability: capability, status: "REJECTED", errorCode: "SNAPSHOT_MISMATCH"), on: socket)
        case .rejectUnsupportedCapability:
            try await send(Envelope(type: "execution.result", runtimeSnapshotID: requestSnapshot, executionID: executionID, capability: capability, status: "REJECTED", errorCode: "UNSUPPORTED_CAPABILITY"), on: socket)
        case .execute:
            let value = request.parameters?["message"]?.stringValue ?? ""
            guardState.markCompleted(executionID)
            try await send(
                Envelope(
                    type: "execution.result",
                    runtimeSnapshotID: requestSnapshot,
                    executionID: executionID,
                    capability: capability,
                    status: "COMPLETED",
                    ackLevel: "ACCEPTED",
                    output: ["echo": .string(value)]
                ),
                on: socket
            )
            print("EXECUTED id=\(executionID) capability=\(capability) value=\(value)")
        }
    }

    private static var platformName: String {
        #if os(macOS)
        return "macos"
        #elseif os(Linux)
        return "linux-spike"
        #else
        return "other"
        #endif
    }

    private func send(_ envelope: Envelope, on socket: URLSessionWebSocketTask) async throws {
        let encoder = JSONEncoder()
        let data = try encoder.encode(envelope)
        guard let text = String(data: data, encoding: .utf8) else { throw CompanionClientError.invalidMessage }
        try await socket.send(.string(text))
    }

    private func decode(_ message: URLSessionWebSocketTask.Message) throws -> Envelope {
        let data: Data
        switch message {
        case .string(let text): data = Data(text.utf8)
        case .data(let bytes): data = bytes
        @unknown default: throw CompanionClientError.invalidMessage
        }
        return try JSONDecoder().decode(Envelope.self, from: data)
    }
}
