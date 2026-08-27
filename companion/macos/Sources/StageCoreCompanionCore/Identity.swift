import Foundation

public protocol CompanionIdentityStore: Sendable {
    func loadOrCreateID() throws -> String
}

public enum CompanionIdentity {
    /// StageCore Hub persistence currently uses canonical UUID text for
    /// `companion_id` (36 characters). Keep the generated public identity
    /// separate from future private key material, which belongs in Keychain.
    public static func generateID() -> String {
        UUID().uuidString.lowercased()
    }

    public static func isCanonicalID(_ value: String) -> Bool {
        guard value.count == 36, let uuid = UUID(uuidString: value) else {
            return false
        }
        return uuid.uuidString.lowercased() == value.lowercased()
    }
}
