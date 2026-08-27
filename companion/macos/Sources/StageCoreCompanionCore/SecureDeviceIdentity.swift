import Foundation

public let companionDeviceKeyAlgorithm = "P256_X963_SHA256"

public struct SecureDeviceIdentity: Sendable, Equatable {
    public let companionID: String
    public let publicKeyData: Data
    public let publicKeyAlgorithm: String

    public init(
        companionID: String,
        publicKeyData: Data,
        publicKeyAlgorithm: String = companionDeviceKeyAlgorithm
    ) {
        self.companionID = companionID
        self.publicKeyData = publicKeyData
        self.publicKeyAlgorithm = publicKeyAlgorithm
    }

    public var publicKeyBase64: String {
        publicKeyData.base64EncodedString()
    }
}

public protocol SecureDeviceIdentityStore: Sendable {
    func loadOrCreateIdentity() throws -> SecureDeviceIdentity
    func signAuthenticationChallenge(_ message: Data) throws -> Data
}

public enum SecureDeviceIdentityError: Error, Equatable {
    case secureStorageUnavailable
    case incompleteIdentity
    case invalidStoredIdentity
    case keyGenerationFailed
    case keychainFailure(Int32)
    case signingFailed
}
