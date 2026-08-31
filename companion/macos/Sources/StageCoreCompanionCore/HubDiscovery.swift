import Foundation
#if canImport(Network)
import Network
#endif

public enum HubDiscoveryError: Error, Equatable {
    case unsupportedPlatform
    case malformedAdvertisement
    case noHubFound
    case multipleHubsFound
    case rememberedHubUnavailable
    case hubIdentityMismatch
    case verificationFailed
}

public struct CompanionHubBinding: Codable, Sendable, Equatable {
    public var hubID: String
    public var fingerprint: String
    public var tlsCertificateSHA256: String

    public init(hubID: String, fingerprint: String, tlsCertificateSHA256: String) {
        self.hubID = hubID.lowercased()
        self.fingerprint = fingerprint
        self.tlsCertificateSHA256 = tlsCertificateSHA256.lowercased()
    }

    public func matches(_ hub: DiscoveredHub) -> Bool {
        hubID == hub.hubID.lowercased()
            && fingerprint == hub.fingerprint
            && tlsCertificateSHA256 == hub.tlsCertificateSHA256.lowercased()
    }
}

public struct DiscoveredHub: Sendable, Equatable, Hashable {
    public let hubID: String
    public let displayName: String
    public let fingerprint: String
    public let tlsCertificateSHA256: String
    public let host: String
    public let port: Int
    public let apiPath: String
    public let runtimePath: String

    public init(txt: [String: String]) throws {
        guard txt["v"] == "1",
              let rawHubID = txt["hub_id"]?.trimmingCharacters(in: .whitespacesAndNewlines),
              let uuid = UUID(uuidString: rawHubID),
              let rawName = txt["name"]?.trimmingCharacters(in: .whitespacesAndNewlines),
              !rawName.isEmpty,
              rawName.utf8.count <= 96,
              let fingerprint = txt["hub_fp"]?.trimmingCharacters(in: .whitespacesAndNewlines),
              !fingerprint.isEmpty,
              fingerprint.utf8.count <= 200,
              let rawPin = txt["tls_sha256"]?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased(),
              HubTLS.isValidCertificateSHA256(rawPin),
              let rawHost = txt["host"]?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased(),
              Self.validLocalHost(rawHost),
              let rawPort = txt["port"],
              let port = Int(rawPort),
              (1...65_535).contains(port),
              txt["api_path"] == "/",
              txt["runtime_path"] == "/api/v1/companion/runtime"
        else {
            throw HubDiscoveryError.malformedAdvertisement
        }

        hubID = uuid.uuidString.lowercased()
        displayName = rawName
        self.fingerprint = fingerprint
        tlsCertificateSHA256 = rawPin
        host = rawHost
        self.port = port
        apiPath = "/"
        runtimePath = "/api/v1/companion/runtime"
    }

    public var apiBaseURL: URL {
        var components = URLComponents()
        components.scheme = "https"
        components.host = host
        components.port = port
        components.path = apiPath
        return components.url!
    }

    public var runtimeURL: URL {
        var components = URLComponents()
        components.scheme = "wss"
        components.host = host
        components.port = port
        components.path = runtimePath
        return components.url!
    }

    public var binding: CompanionHubBinding {
        CompanionHubBinding(
            hubID: hubID,
            fingerprint: fingerprint,
            tlsCertificateSHA256: tlsCertificateSHA256
        )
    }

    private static func validLocalHost(_ host: String) -> Bool {
        guard !host.isEmpty,
              host.utf8.count <= 253,
              host.hasSuffix(".local"),
              !host.contains("/"),
              !host.contains("\\"),
              !host.contains(":"),
              !host.contains("@"),
              !host.unicodeScalars.contains(where: { CharacterSet.whitespacesAndNewlines.contains($0) })
        else { return false }

        let labels = host.split(separator: ".", omittingEmptySubsequences: false)
        guard labels.count >= 2 else { return false }
        for label in labels.dropLast() {
            guard !label.isEmpty, label.utf8.count <= 63 else { return false }
            for byte in label.utf8 {
                let letter = (byte >= 97 && byte <= 122)
                let digit = (byte >= 48 && byte <= 57)
                if !letter && !digit && byte != 45 { return false }
            }
            if label.first == "-" || label.last == "-" { return false }
        }
        return labels.last == "local"
    }
}

public protocol HubDiscovering: Sendable {
    func discover(timeout: Duration) async throws -> [DiscoveredHub]
}

public struct BonjourHubDiscovery: HubDiscovering, Sendable {
    public init() {}

    public func discover(timeout: Duration = .seconds(2)) async throws -> [DiscoveredHub] {
        #if canImport(Network)
        let session = BonjourBrowseSession()
        session.start()
        do {
            try await Task.sleep(for: timeout)
        } catch {
            session.stop()
            throw error
        }
        session.stop()
        if let error = session.failure {
            throw error
        }
        return session.snapshot()
        #else
        _ = timeout
        throw HubDiscoveryError.unsupportedPlatform
        #endif
    }
}

#if canImport(Network)
private final class BonjourBrowseSession: @unchecked Sendable {
    private let lock = NSLock()
    private var hubs: [String: DiscoveredHub] = [:]
    private var browserFailure: Error?
    private var browser: NWBrowser?
    private let queue = DispatchQueue(label: "com.stagecore.companion.hub-discovery")

    var failure: Error? {
        lock.lock()
        defer { lock.unlock() }
        return browserFailure
    }

    func start() {
        let browser = NWBrowser(
            for: .bonjourWithTXTRecord(type: "_stagecore-hub._tcp", domain: "local."),
            using: .tcp
        )
        self.browser = browser
        browser.stateUpdateHandler = { [weak self] state in
            guard let self else { return }
            if case .failed(let error) = state {
                self.lock.lock()
                self.browserFailure = error
                self.lock.unlock()
            }
        }
        browser.browseResultsChangedHandler = { [weak self] results, _ in
            self?.consume(results)
        }
        browser.start(queue: queue)
    }

    func stop() {
        browser?.cancel()
        browser = nil
    }

    func snapshot() -> [DiscoveredHub] {
        lock.lock()
        defer { lock.unlock() }
        return hubs.values.sorted {
            if $0.displayName == $1.displayName { return $0.hubID < $1.hubID }
            return $0.displayName.localizedCaseInsensitiveCompare($1.displayName) == .orderedAscending
        }
    }

    private func consume(_ results: Set<NWBrowser.Result>) {
        var next: [String: DiscoveredHub] = [:]
        for result in results {
            guard case .bonjour(let txtRecord) = result.metadata,
                  let hub = try? DiscoveredHub(txt: txtRecord.dictionary)
            else { continue }
            let key = "\(hub.hubID)|\(hub.fingerprint)|\(hub.tlsCertificateSHA256)"
            next[key] = hub
        }
        lock.lock()
        hubs = next
        lock.unlock()
    }
}
#endif

public enum HubDiscoverySelection {
    public static func firstPairCandidate(from hubs: [DiscoveredHub]) throws -> DiscoveredHub {
        switch hubs.count {
        case 0:
            throw HubDiscoveryError.noHubFound
        case 1:
            return hubs[0]
        default:
            throw HubDiscoveryError.multipleHubsFound
        }
    }

    public static func remembered(_ binding: CompanionHubBinding, from hubs: [DiscoveredHub]) throws -> DiscoveredHub {
        let matches = hubs.filter(binding.matches)
        guard matches.count == 1 else {
            throw HubDiscoveryError.rememberedHubUnavailable
        }
        return matches[0]
    }
}
