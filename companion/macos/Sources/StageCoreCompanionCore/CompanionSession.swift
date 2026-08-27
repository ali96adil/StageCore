import Foundation

public struct CompanionCapabilityOutcome: Sendable, Equatable {
    public var status: CompanionExecutionStatus
    public var ackLevel: CompanionAckLevel
    public var errorCode: String?
    public var responseSummary: String
    public var output: [String: JSONValue]

    public init(
        status: CompanionExecutionStatus,
        ackLevel: CompanionAckLevel,
        errorCode: String? = nil,
        responseSummary: String,
        output: [String: JSONValue] = [:]
    ) {
        self.status = status
        self.ackLevel = ackLevel
        self.errorCode = errorCode
        self.responseSummary = responseSummary
        self.output = output
    }
}

public protocol CompanionCapabilityExecutor: Sendable {
    var capabilityKey: String { get }
    func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome
}

public struct LocalEchoExecutor: CompanionCapabilityExecutor {
    public let capabilityKey = "local.echo"

    public init() {}

    public func execute(parameters: [String: JSONValue]) async -> CompanionCapabilityOutcome {
        CompanionCapabilityOutcome(
            status: .completed,
            ackLevel: .accepted,
            responseSummary: "local echo completed",
            output: ["echo": parameters["message"] ?? .null]
        )
    }
}

public struct CompanionSessionConfiguration: Sendable, Equatable {
    public var companionID: String
    public var displayName: String
    public var hostname: String
    public var agentVersion: String
    public var platform: String
    public var architecture: String
    public var configHash: String
    public var readiness: CompanionReadiness
    public var requiresAuthenticatedSession: Bool

    public init(
        companionID: String,
        displayName: String = "",
        hostname: String = "",
        agentVersion: String,
        platform: String,
        architecture: String,
        configHash: String = "",
        readiness: CompanionReadiness = .unknown,
        requiresAuthenticatedSession: Bool = true
    ) {
        self.companionID = companionID
        self.displayName = displayName
        self.hostname = hostname
        self.agentVersion = agentVersion
        self.platform = platform
        self.architecture = architecture
        self.configHash = configHash
        self.readiness = readiness
        self.requiresAuthenticatedSession = requiresAuthenticatedSession
    }
}

public enum CompanionSessionError: Error, Equatable {
    case unsupportedSchemaVersion(Int)
    case unexpectedMessage(CompanionMessageType)
    case unauthenticatedSession
}

private struct CompanionMessageHeader: Decodable {
    let type: CompanionMessageType
    let schemaVersion: Int

    enum CodingKeys: String, CodingKey {
        case type
        case schemaVersion = "schema_version"
    }
}

public actor CompanionSession {
    private let configuration: CompanionSessionConfiguration
    private var state: CompanionRuntimeState
    private var guardState: ExecutionGuard
    private let executors: [String: any CompanionCapabilityExecutor]
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    public init(
        configuration: CompanionSessionConfiguration,
        executors: [any CompanionCapabilityExecutor],
        duplicateCapacity: Int = 512
    ) {
        self.configuration = configuration
        self.state = CompanionRuntimeState(
            companionID: configuration.companionID,
            configHash: configuration.configHash,
            capabilities: Set(executors.map(\.capabilityKey)),
            readiness: configuration.readiness
        )
        self.guardState = ExecutionGuard(capacity: duplicateCapacity)
        var registry: [String: any CompanionCapabilityExecutor] = [:]
        for executor in executors {
            registry[executor.capabilityKey] = executor
        }
        self.executors = registry
    }

    public func helloData() throws -> Data {
        try encoder.encode(
            CompanionHello(
                companionID: configuration.companionID,
                displayName: configuration.displayName,
                hostname: configuration.hostname,
                agentVersion: configuration.agentVersion,
                platform: configuration.platform,
                architecture: configuration.architecture,
                capabilities: state.capabilities.sorted(),
                machineRoleID: state.machineRoleID,
                roleKey: state.roleKey,
                appliedRuntimeSnapshotID: state.appliedRuntimeSnapshotID,
                configHash: state.configHash,
                readiness: state.readiness.rawValue
            )
        )
    }

    public func runtimeState() -> CompanionRuntimeState {
        state
    }

    public func establishAuthenticatedSession(_ credential: CompanionRuntimeCredential) {
        state.authenticate(sessionID: credential.sessionID, expiresAt: credential.expiresAt)
    }

    public func invalidateAuthenticatedSession() {
        state.clearAuthentication()
    }

    /// Handles one Hub message. A non-nil return is a wire response that the
    /// transport must send before reading the next execution request.
    public func handle(_ data: Data) async throws -> Data? {
        let header = try decoder.decode(CompanionMessageHeader.self, from: data)
        guard header.schemaVersion == 1 else {
            throw CompanionSessionError.unsupportedSchemaVersion(header.schemaVersion)
        }

        switch header.type {
        case .sessionReady:
            guard !configuration.requiresAuthenticatedSession || state.isAuthenticated() else {
                throw CompanionSessionError.unauthenticatedSession
            }
            let ready = try decoder.decode(SessionReady.self, from: data)
            state.apply(ready)
            return nil

        case .executionRequest:
            let request = try decoder.decode(CompanionExecutionRequest.self, from: data)
            return try encoder.encode(await handleExecution(request))

        case .hello, .executionResult:
            throw CompanionSessionError.unexpectedMessage(header.type)
        }
    }

    private func handleExecution(_ request: CompanionExecutionRequest) async -> CompanionExecutionResult {
        guard !configuration.requiresAuthenticatedSession || state.isAuthenticated() else {
            guardState.markTerminal(request.executionID)
            return rejection(request, code: "SESSION_UNAUTHENTICATED", summary: "authenticated runtime session is required")
        }
        let decision = guardState.decision(for: request, state: state)
        switch decision {
        case .rejectDuplicate:
            return rejection(request, code: "DUPLICATE_EXECUTION", summary: "execution_id was already terminal")

        case .rejectSnapshotMismatch:
            guardState.markTerminal(request.executionID)
            return rejection(request, code: "SNAPSHOT_MISMATCH", summary: "request Runtime Snapshot does not match applied state")

        case .rejectRoleMismatch:
            guardState.markTerminal(request.executionID)
            return rejection(request, code: "MACHINE_ROLE_MISMATCH", summary: "request Machine Role does not match assigned state")

        case .rejectUnsupportedCapability:
            guardState.markTerminal(request.executionID)
            return rejection(request, code: "UNSUPPORTED_CAPABILITY", summary: "capability is not available on this Companion")

        case .execute:
            guard let executor = executors[request.capability] else {
                guardState.markTerminal(request.executionID)
                return rejection(request, code: "UNSUPPORTED_CAPABILITY", summary: "capability executor is unavailable")
            }
            let outcome = await executeBounded(executor, request: request)
            // Mark before transport send. If the socket disappears after local
            // execution but before result delivery, a repeated execution_id on
            // reconnect is rejected rather than executed twice.
            guardState.markTerminal(request.executionID)
            return CompanionExecutionResult(
                executionID: request.executionID,
                status: outcome.status,
                ackLevel: outcome.ackLevel,
                errorCode: outcome.errorCode,
                responseSummary: outcome.responseSummary,
                output: outcome.output
            )
        }
    }

    private func rejection(
        _ request: CompanionExecutionRequest,
        code: String,
        summary: String
    ) -> CompanionExecutionResult {
        CompanionExecutionResult(
            executionID: request.executionID,
            status: .rejected,
            ackLevel: .none,
            errorCode: code,
            responseSummary: summary
        )
    }

    private func executeBounded(
        _ executor: any CompanionCapabilityExecutor,
        request: CompanionExecutionRequest
    ) async -> CompanionCapabilityOutcome {
        guard request.timeoutMS > 0 else {
            return await executor.execute(parameters: request.parameters)
        }

        return await withTaskGroup(of: CompanionCapabilityOutcome.self) { group in
            group.addTask {
                await executor.execute(parameters: request.parameters)
            }
            group.addTask {
                do {
                    try await Task.sleep(for: .milliseconds(request.timeoutMS))
                } catch {
                    return CompanionCapabilityOutcome(
                        status: .cancelled,
                        ackLevel: .none,
                        errorCode: "COMPANION_EXECUTION_CANCELLED",
                        responseSummary: "execution timer was cancelled"
                    )
                }
                return CompanionCapabilityOutcome(
                    status: .timedOut,
                    ackLevel: .none,
                    errorCode: "COMPANION_EXECUTION_TIMEOUT",
                    responseSummary: "local capability did not complete before timeout"
                )
            }

            let first = await group.next() ?? CompanionCapabilityOutcome(
                status: .failed,
                ackLevel: .none,
                errorCode: "COMPANION_EXECUTION_FAILED",
                responseSummary: "local capability produced no result"
            )
            group.cancelAll()
            return first
        }
    }
}
