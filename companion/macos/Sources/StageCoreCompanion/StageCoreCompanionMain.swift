import Foundation
import StageCoreCompanionCore
#if os(Linux)
import Glibc
#else
import Darwin
#endif

@main
enum StageCoreCompanionMain {
    static func main() async {
        do {
            let configuration = try loadConfiguration(arguments: Array(CommandLine.arguments.dropFirst()))
            let environment = ProcessInfo.processInfo.environment
            let securityPolicy: CompanionTransportSecurityPolicy =
                environment["STAGECORE_COMPANION_ALLOW_INSECURE_LOOPBACK_FOR_TESTING"] == "1"
                ? .allowInsecureLoopbackForTesting
                : .production
            let identityService = environment["STAGECORE_COMPANION_IDENTITY_SERVICE"]
                ?? "com.stagecore.companion.identity"
            let bootstrap = try CompanionBootstrap(
                configuration: configuration,
                identityStore: KeychainDeviceIdentityStore(service: identityService),
                securityPolicy: securityPolicy
            )
            let initial = await bootstrap.status()
            emit("StageCore Companion \(initial.companionID) starting as \(initial.displayName)")
            try await bootstrap.run { event in
                switch event {
                case .pairingRequired(let receipt):
                    // This is an explicit setup surface, not a normal runtime log.
                    emit("Pairing request: \(receipt.requestID)")
                    emit("Pairing code: \(receipt.pairingCode)")
                    emit("Approve it locally on the Hub before \(receipt.expiresAt.ISO8601Format())")
                case .phaseChanged(let phase):
                    emit("Companion state: \(phase.rawValue)")
                }
            }
        } catch {
            let message = Data("StageCore Companion stopped: \(safeErrorCode(error))\n".utf8)
            try? FileHandle.standardError.write(contentsOf: message)
            exit(1)
        }
    }

    private static func emit(_ line: String) {
        try? FileHandle.standardOutput.write(contentsOf: Data((line + "\n").utf8))
    }

    private static func loadConfiguration(arguments: [String]) throws -> CompanionAppConfiguration {
        var configPath: String?
        var apiURL: URL?
        var runtimeURL: URL?
        var displayName = ProcessInfo.processInfo.hostName
        var oscHost: String?
        var oscPort: Int?
        var index = 0
        while index < arguments.count {
            let value = arguments[index]
            guard index + 1 < arguments.count else { throw BootstrapCLIError.invalidArguments }
            switch value {
            case "--config": configPath = arguments[index + 1]
            case "--hub-api": apiURL = URL(string: arguments[index + 1])
            case "--hub-runtime": runtimeURL = URL(string: arguments[index + 1])
            case "--display-name": displayName = arguments[index + 1]
            case "--osc-host": oscHost = arguments[index + 1]
            case "--osc-port": oscPort = Int(arguments[index + 1])
            default: throw BootstrapCLIError.invalidArguments
            }
            index += 2
        }

        let store = FileCompanionConfigurationStore(url: configURL(configPath))
        if FileManager.default.fileExists(atPath: store.url.path) {
            return try store.load()
        }
        guard let apiURL, let runtimeURL, !displayName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw BootstrapCLIError.configurationRequired
        }

        let oscEndpoint: OSCEndpoint?
        switch (oscHost, oscPort) {
        case (nil, nil):
            oscEndpoint = nil
        case let (.some(host), .some(port)):
            let endpoint = OSCEndpoint(host: host, port: port)
            guard endpoint.isValid else { throw BootstrapCLIError.invalidOSCConfiguration }
            oscEndpoint = endpoint
        default:
            throw BootstrapCLIError.invalidOSCConfiguration
        }

        let configuration = CompanionAppConfiguration(
            hubAPIBaseURL: apiURL,
            hubRuntimeURL: runtimeURL,
            displayName: displayName,
            oscEndpoint: oscEndpoint
        )
        try store.save(configuration)
        return configuration
    }

    private static func configURL(_ explicitPath: String?) -> URL {
        if let explicitPath {
            return URL(fileURLWithPath: explicitPath)
        }
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        return base.appendingPathComponent("StageCore/Companion/config.json")
    }

    private static func safeErrorCode(_ error: Error) -> String {
        switch error {
        case let value as HubSecurityClientError: return String(describing: value)
        case let value as SecureDeviceIdentityError: return String(describing: value)
        case let value as CompanionTransportError: return String(describing: value)
        case let value as OSCExecutorError: return String(describing: value)
        case let value as BootstrapCLIError: return String(describing: value)
        default: return "COMPANION_STARTUP_FAILED"
        }
    }
}

private enum BootstrapCLIError: Error {
    case invalidArguments
    case configurationRequired
    case invalidOSCConfiguration
}
