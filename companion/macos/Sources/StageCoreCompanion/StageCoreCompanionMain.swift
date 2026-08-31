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
        let arguments = Array(CommandLine.arguments.dropFirst())
        do {
            let options = try parseOptions(arguments)
            let environment = ProcessInfo.processInfo.environment
            let securityPolicy: CompanionTransportSecurityPolicy =
                environment["STAGECORE_COMPANION_ALLOW_INSECURE_LOOPBACK_FOR_TESTING"] == "1"
                ? .allowInsecureLoopbackForTesting
                : .production
            let identityService = environment["STAGECORE_COMPANION_IDENTITY_SERVICE"]
                ?? "com.stagecore.companion.identity"

            while true {
                let configuration: CompanionAppConfiguration
                do {
                    configuration = try await loadConfiguration(options: options)
                } catch {
                    if try shouldRetryRememberedDiscovery(error: error, options: options) {
                        emit("StageCore Hub unavailable; rediscovering remembered Hub...")
                        try await Task.sleep(for: .seconds(1))
                        continue
                    }
                    throw error
                }

                if let binding = configuration.hubBinding {
                    emit("StageCore Hub resolved: \(binding.hubID)")
                }
                let bootstrap = try CompanionBootstrap(
                    configuration: configuration,
                    identityStore: KeychainDeviceIdentityStore(service: identityService),
                    securityPolicy: securityPolicy
                )
                let initial = await bootstrap.status()
                emit("StageCore Companion \(initial.companionID) starting as \(initial.displayName)")
                do {
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
                    return
                } catch {
                    if configuration.hubBinding != nil && autoReconnectable(error) {
                        emit("StageCore Companion connection lost; rediscovering remembered Hub...")
                        try await Task.sleep(for: .seconds(1))
                        continue
                    }
                    throw error
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

    private static func loadConfiguration(options: CLIOptions) async throws -> CompanionAppConfiguration {
        let store = FileCompanionConfigurationStore(url: configURL(options.configPath))
        if FileManager.default.fileExists(atPath: store.url.path) {
            let existing = try store.load()
            guard let binding = existing.hubBinding else {
                // Legacy/manual configuration remains supported exactly as a
                // recovery and test path. F-004 never rewrites it into TOFU.
                return existing
            }
            let hubs = try await BonjourHubDiscovery().discover(timeout: .seconds(2))
            let discovered = try HubDiscoverySelection.remembered(binding, from: hubs)
            _ = try await HubIdentityVerifier().verify(discovered)
            var refreshed = existing
            refreshed.hubAPIBaseURL = discovered.apiBaseURL
            refreshed.hubRuntimeURL = discovered.runtimeURL
            if refreshed != existing {
                try store.save(refreshed)
            }
            return refreshed
        }

        guard !options.displayName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw BootstrapCLIError.configurationRequired
        }
        let oscEndpoint = try buildOSCEndpoint(host: options.oscHost, port: options.oscPort)

        if let apiURL = options.apiURL, let runtimeURL = options.runtimeURL {
            let configuration = CompanionAppConfiguration(
                hubAPIBaseURL: apiURL,
                hubRuntimeURL: runtimeURL,
                displayName: options.displayName,
                oscEndpoint: oscEndpoint
            )
            try store.save(configuration)
            return configuration
        }
        if options.apiURL != nil || options.runtimeURL != nil {
            throw BootstrapCLIError.configurationRequired
        }

        let hubs = try await BonjourHubDiscovery().discover(timeout: .seconds(2))
        let discovered = try HubDiscoverySelection.firstPairCandidate(from: hubs)
        let verified = try await HubIdentityVerifier().verify(discovered)
        emit("Discovered StageCore Hub: \(verified.displayName) [\(verified.fingerprint)]")
        let configuration = CompanionAppConfiguration(
            hubAPIBaseURL: discovered.apiBaseURL,
            hubRuntimeURL: discovered.runtimeURL,
            displayName: options.displayName,
            oscEndpoint: oscEndpoint,
            hubBinding: discovered.binding
        )
        try store.save(configuration)
        return configuration
    }

    private static func parseOptions(_ arguments: [String]) throws -> CLIOptions {
        var options = CLIOptions(displayName: ProcessInfo.processInfo.hostName)
        var index = 0
        while index < arguments.count {
            let value = arguments[index]
            guard index + 1 < arguments.count else { throw BootstrapCLIError.invalidArguments }
            let argument = arguments[index + 1]
            switch value {
            case "--config": options.configPath = argument
            case "--hub-api": options.apiURL = URL(string: argument)
            case "--hub-runtime": options.runtimeURL = URL(string: argument)
            case "--display-name": options.displayName = argument
            case "--osc-host": options.oscHost = argument
            case "--osc-port": options.oscPort = Int(argument)
            default: throw BootstrapCLIError.invalidArguments
            }
            index += 2
        }
        return options
    }

    private static func buildOSCEndpoint(host: String?, port: Int?) throws -> OSCEndpoint? {
        switch (host, port) {
        case (nil, nil):
            return nil
        case let (.some(host), .some(port)):
            let endpoint = OSCEndpoint(host: host, port: port)
            guard endpoint.isValid else { throw BootstrapCLIError.invalidOSCConfiguration }
            return endpoint
        default:
            throw BootstrapCLIError.invalidOSCConfiguration
        }
    }

    private static func shouldRetryRememberedDiscovery(error: Error, options: CLIOptions) throws -> Bool {
        let store = FileCompanionConfigurationStore(url: configURL(options.configPath))
        guard FileManager.default.fileExists(atPath: store.url.path),
              let configuration = try? store.load(),
              configuration.hubBinding != nil
        else { return false }

        switch error {
        case HubDiscoveryError.rememberedHubUnavailable,
             HubDiscoveryError.noHubFound,
             HubDiscoveryError.verificationFailed:
            return true
        default:
            return false
        }
    }

    private static func autoReconnectable(_ error: Error) -> Bool {
        if error is URLError { return true }
        if error is CompanionTransportError { return true }
        return false
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
        case let value as HubDiscoveryError: return String(describing: value)
        case let value as SecureDeviceIdentityError: return String(describing: value)
        case let value as CompanionTransportError: return String(describing: value)
        case let value as OSCExecutorError: return String(describing: value)
        case let value as BootstrapCLIError: return String(describing: value)
        default: return "COMPANION_STARTUP_FAILED"
        }
    }
}

private struct CLIOptions {
    var configPath: String?
    var apiURL: URL?
    var runtimeURL: URL?
    var displayName: String
    var oscHost: String?
    var oscPort: Int?
}

private enum BootstrapCLIError: Error {
    case invalidArguments
    case configurationRequired
    case invalidOSCConfiguration
}
