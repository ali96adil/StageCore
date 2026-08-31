import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public struct PublicHubIdentity: Codable, Sendable, Equatable {
    public let schemaVersion: Int
    public let hubID: String
    public let displayName: String
    public let fingerprint: String
    public let bootstrapState: String

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case hubID = "hub_id"
        case displayName = "display_name"
        case fingerprint
        case bootstrapState = "bootstrap_state"
    }
}

public protocol HubIdentityVerifying: Sendable {
    func verify(_ hub: DiscoveredHub) async throws -> PublicHubIdentity
}

public struct HubIdentityVerifier: HubIdentityVerifying, Sendable {
    public init() {}

    public func verify(_ hub: DiscoveredHub) async throws -> PublicHubIdentity {
        let session = HubTLS.makeSession(pinnedCertificateSHA256: hub.tlsCertificateSHA256)
        defer { session.invalidateAndCancel() }
        let url = hub.apiBaseURL.appendingPathComponent("api/v1/hub/identity")
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.timeoutInterval = 3
        do {
            let (data, response) = try await session.data(for: request)
            guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
                throw HubDiscoveryError.verificationFailed
            }
            let identity = try JSONDecoder().decode(PublicHubIdentity.self, from: data)
            try Self.validate(identity, matches: hub)
            return identity
        } catch let error as HubDiscoveryError {
            throw error
        } catch {
            throw HubDiscoveryError.verificationFailed
        }
    }

    public static func validate(_ identity: PublicHubIdentity, matches hub: DiscoveredHub) throws {
        guard identity.schemaVersion == 1,
              identity.hubID.lowercased() == hub.hubID.lowercased(),
              identity.fingerprint == hub.fingerprint,
              !identity.displayName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        else {
            throw HubDiscoveryError.hubIdentityMismatch
        }
    }
}
