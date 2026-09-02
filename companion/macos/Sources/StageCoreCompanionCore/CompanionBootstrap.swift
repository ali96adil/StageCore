import Foundation

public struct CompanionAppConfiguration: Codable, Sendable, Equatable {
    public var hubAPIBaseURL: URL
    public var hubRuntimeURL: URL
    public var displayName: String
    public var agentVersion: String
    public var configHash: String
    public var oscEndpoint: OSCEndpoint?
    public var mediaCacheRoot: URL?
    public var hubBinding: CompanionHubBinding?

    public init(
        hubAPIBaseURL: URL,
        hubRuntimeURL: URL,
        displayName: String,
        agentVersion: String = "0.1.0",
        configHash: String = "",
        oscEndpoint: OSCEndpoint? = nil,
        mediaCacheRoot: URL? = nil,
        hubBinding: CompanionHubBinding? = nil
    ) {
        self.hubAPIBaseURL = hubAPIBaseURL
        self.hubRuntimeURL = hubRuntimeURL
        self.displayName = displayName
        self.agentVersion = agentVersion
        self.configHash = configHash
        self.oscEndpoint = oscEndpoint
        self.mediaCacheRoot = mediaCacheRoot
        self.hubBinding = hubBinding
    }
}

public struct FileCompanionConfigurationStore: Sendable {
    public let url: URL

    public init(url: URL) {
        self.url = url
    }

    public func load() throws -> CompanionAppConfiguration {
        try JSONDecoder().decode(CompanionAppConfiguration.self, from: Data(contentsOf: url))
    }

    public func save(_ configuration: CompanionAppConfiguration) throws {
        let directory = url.deletingLastPathComponent()
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let data = try JSONEncoder().encode(configuration)
        try data.write(to: url, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: url.path)
    }
}

public enum CompanionBootstrapPhase: String, Sendable, Equatable {
    case starting = "STARTING"
    case pairingRequired = "PAIRING_REQUIRED"
    case waitingForApproval = "WAITING_FOR_APPROVAL"
    case authenticating = "AUTHENTICATING"
    case connecting = "CONNECTING"
    case running = "RUNNING"
    case stopped = "STOPPED"
    case failed = "FAILED"
}

public struct CompanionBootstrapStatus: Sendable, Equatable {
    public var phase: CompanionBootstrapPhase
    public var companionID: String
    public var displayName: String
    public var hostname: String
    public var version: String
    public var platform: String
    public var architecture: String
    public var capabilities: [String]
    public var machineRoleID: String?
    public var roleKey: String?
    public var runtimeSnapshotID: String?
    public var configHash: String
    public var readiness: CompanionReadiness
}

public enum CompanionBootstrapEvent: Sendable, Equatable {
    case pairingRequired(CompanionPairingReceipt)
    case phaseChanged(CompanionBootstrapPhase)
}

public actor CompanionBootstrap {
    private let configuration: CompanionAppConfiguration
    private let identity: SecureDeviceIdentity
    private let securityClient: HubSecurityClient
    private let companionSession: CompanionSession
    private let runtimeAgent: WebSocketCompanionAgent
    private let report: CompanionReportIdentity
    private var phase: CompanionBootstrapPhase = .starting

    public init(
        configuration: CompanionAppConfiguration,
        identityStore: any SecureDeviceIdentityStore,
        securityPolicy: CompanionTransportSecurityPolicy = .production,
        hostname: String = ProcessInfo.processInfo.hostName,
        platform: String = "macos",
        architecture: String = CompanionBootstrap.currentArchitecture
    ) throws {
        self.configuration = configuration
        self.identity = try identityStore.loadOrCreateIdentity()

        var executors: [any CompanionCapabilityExecutor] = [LocalEchoExecutor()]
        if let endpoint = configuration.oscEndpoint {
            executors.append(try OSCSendExecutor(endpoint: endpoint))
        }
        #if os(macOS)
        executors.append(try MIDISendExecutor())
        let operationProviders: [any ExecutionEnvironmentOperationProvider] = [VDMXOperationProvider()]
        #else
        let operationProviders: [any ExecutionEnvironmentOperationProvider] = []
        #endif
        // F-025 keeps the operation capability generic while registration is
        // explicit per adapter. Unknown adapters and unsupported operation
        // kinds fail truthfully; there is never a command/shell fallback.
        executors.append(try ExecutionEnvironmentOperationExecutor(providers: operationProviders))
        let capabilities = executors.map(\.capabilityKey).sorted()

        let report = CompanionReportIdentity(
            displayName: configuration.displayName,
            hostname: hostname,
            platform: platform,
            architecture: architecture,
            version: configuration.agentVersion,
            capabilities: capabilities
        )
        self.report = report
        let certificatePin = configuration.hubBinding?.tlsCertificateSHA256
        let securityClient = try HubSecurityClient(
            apiBaseURL: configuration.hubAPIBaseURL,
            securityPolicy: securityPolicy,
            identityStore: identityStore,
            report: report,
            session: HubTLS.makeSession(pinnedCertificateSHA256: certificatePin)
        )
        self.securityClient = securityClient

        let mediaSynchronizer: (any CompanionMediaSynchronizer)?
        #if os(macOS)
        let cacheRoot = configuration.mediaCacheRoot ?? FileManager.default.urls(
            for: .cachesDirectory,
            in: .userDomainMask
        ).first!.appendingPathComponent("StageCore/media", isDirectory: true)
        mediaSynchronizer = try MediaCacheSynchronizer(
            apiBaseURL: configuration.hubAPIBaseURL,
            cacheRoot: cacheRoot,
            securityPolicy: securityPolicy,
            session: HubTLS.makeSession(pinnedCertificateSHA256: certificatePin)
        )
        #else
        mediaSynchronizer = nil
        #endif

        let companionSession = CompanionSession(
            configuration: CompanionSessionConfiguration(
                companionID: identity.companionID,
                displayName: report.displayName,
                hostname: report.hostname,
                agentVersion: report.version,
                platform: report.platform,
                architecture: report.architecture,
                configHash: configuration.configHash,
                readiness: .unknown,
                requiresAuthenticatedSession: true
            ),
            executors: executors,
            mediaSynchronizer: mediaSynchronizer
        )
        self.companionSession = companionSession

        #if os(macOS)
        let inspectionProviders: [any CompanionInspectionProvider] = [VDMXInspectionProvider()]
        #else
        let inspectionProviders: [any CompanionInspectionProvider] = []
        #endif
        self.runtimeAgent = try WebSocketCompanionAgent(
            url: configuration.hubRuntimeURL,
            securityPolicy: securityPolicy,
            session: companionSession,
            authenticator: securityClient,
            inspectionProviders: inspectionProviders,
            tlsCertificateSHA256: certificatePin
        )
    }

    public func run(event: @Sendable (CompanionBootstrapEvent) -> Void = { _ in }) async throws {
        do {
            phase = .authenticating
            event(.phaseChanged(phase))
            do {
                _ = try await securityClient.authenticate()
            } catch HubSecurityClientError.hubRejected(let code) where code == "COMPANION_UNPAIRED" {
                let receipt = try await securityClient.requestPairing()
                phase = .pairingRequired
                event(.pairingRequired(receipt))
                phase = .waitingForApproval
                event(.phaseChanged(phase))
                try await waitForPairingApproval(receipt)
            }

            phase = .connecting
            event(.phaseChanged(phase))
            phase = .running
            event(.phaseChanged(phase))
            try await runtimeAgent.run()
            phase = .stopped
            event(.phaseChanged(phase))
        } catch {
            phase = .failed
            event(.phaseChanged(phase))
            throw error
        }
    }

    public func status() async -> CompanionBootstrapStatus {
        let runtime = await companionSession.runtimeState()
        return CompanionBootstrapStatus(
            phase: phase,
            companionID: identity.companionID,
            displayName: report.displayName,
            hostname: report.hostname,
            version: report.version,
            platform: report.platform,
            architecture: report.architecture,
            capabilities: report.capabilities,
            machineRoleID: runtime.machineRoleID,
            roleKey: runtime.roleKey,
            runtimeSnapshotID: runtime.appliedRuntimeSnapshotID,
            configHash: runtime.configHash,
            readiness: runtime.readiness
        )
    }

    private func waitForPairingApproval(_ receipt: CompanionPairingReceipt) async throws {
        while Date() < receipt.expiresAt {
            switch try await securityClient.pairingStatus(receipt: receipt) {
            case .approved:
                return
            case .rejected:
                throw HubSecurityClientError.hubRejected("PAIRING_REJECTED")
            case .expired:
                throw HubSecurityClientError.hubRejected("PAIRING_REQUEST_EXPIRED")
            case .pending:
                try await Task.sleep(for: .seconds(1))
            }
        }
        throw HubSecurityClientError.hubRejected("PAIRING_REQUEST_EXPIRED")
    }

    public static var currentArchitecture: String {
        #if arch(arm64)
        return "arm64"
        #elseif arch(x86_64)
        return "x86_64"
        #else
        return "unknown"
        #endif
    }
}
