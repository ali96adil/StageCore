import Foundation

#if canImport(Security)
import Security

public final class KeychainDeviceIdentityStore: SecureDeviceIdentityStore, @unchecked Sendable {
    private let service: String
    private let account: String
    private let keyTag: Data
    private let lock = NSLock()

    public init(service: String = "com.stagecore.companion.identity") {
        self.service = service
        self.account = "companion-id"
        self.keyTag = Data("\(service).device-key.p256".utf8)
    }

    public func loadOrCreateIdentity() throws -> SecureDeviceIdentity {
        try lock.withLock {
            let storedID = try loadCompanionID()
            let storedKey = try loadPrivateKey()
            switch (storedID, storedKey) {
            case let (.some(companionID), .some(privateKey)):
                guard CompanionIdentity.isCanonicalID(companionID) else {
                    throw SecureDeviceIdentityError.invalidStoredIdentity
                }
                return try identity(companionID: companionID, privateKey: privateKey)

            case (nil, nil):
                let companionID = CompanionIdentity.generateID()
                let privateKey = try createPrivateKey()
                do {
                    try storeCompanionID(companionID)
                } catch {
                    deletePrivateKey()
                    throw error
                }
                return try identity(companionID: companionID, privateKey: privateKey)

            default:
                // Losing only one half of identity is a security recovery event.
                // Never silently replace the remaining trusted material.
                throw SecureDeviceIdentityError.incompleteIdentity
            }
        }
    }

    public func signAuthenticationChallenge(_ message: Data) throws -> Data {
        try lock.withLock {
            guard let privateKey = try loadPrivateKey() else {
                throw SecureDeviceIdentityError.incompleteIdentity
            }
            var error: Unmanaged<CFError>?
            guard let signature = SecKeyCreateSignature(
                privateKey,
                .ecdsaSignatureMessageX962SHA256,
                message as CFData,
                &error
            ) as Data? else {
                throw SecureDeviceIdentityError.signingFailed
            }
            return signature
        }
    }

    private func loadCompanionID() throws -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess, let data = item as? Data,
              let value = String(data: data, encoding: .utf8) else {
            throw SecureDeviceIdentityError.keychainFailure(status)
        }
        return value
    }

    private func storeCompanionID(_ companionID: String) throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
            kSecValueData as String: Data(companionID.utf8),
        ]
        let status = SecItemAdd(query as CFDictionary, nil)
        guard status == errSecSuccess else {
            throw SecureDeviceIdentityError.keychainFailure(status)
        }
    }

    private func loadPrivateKey() throws -> SecKey? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassKey,
            kSecAttrApplicationTag as String: keyTag,
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
            kSecReturnRef as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess, let key = item else {
            throw SecureDeviceIdentityError.keychainFailure(status)
        }
        return (key as! SecKey)
    }

    private func createPrivateKey() throws -> SecKey {
        let privateAttributes: [String: Any] = [
            kSecAttrIsPermanent as String: true,
            kSecAttrApplicationTag as String: keyTag,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        let attributes: [String: Any] = [
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
            kSecAttrKeySizeInBits as String: 256,
            kSecPrivateKeyAttrs as String: privateAttributes,
        ]
        var error: Unmanaged<CFError>?
        guard let key = SecKeyCreateRandomKey(attributes as CFDictionary, &error) else {
            throw SecureDeviceIdentityError.keyGenerationFailed
        }
        return key
    }

    private func identity(companionID: String, privateKey: SecKey) throws -> SecureDeviceIdentity {
        guard let publicKey = SecKeyCopyPublicKey(privateKey) else {
            throw SecureDeviceIdentityError.invalidStoredIdentity
        }
        var error: Unmanaged<CFError>?
        guard let publicData = SecKeyCopyExternalRepresentation(publicKey, &error) as Data? else {
            throw SecureDeviceIdentityError.invalidStoredIdentity
        }
        return SecureDeviceIdentity(companionID: companionID, publicKeyData: publicData)
    }

    private func deletePrivateKey() {
        let query: [String: Any] = [
            kSecClass as String: kSecClassKey,
            kSecAttrApplicationTag as String: keyTag,
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
        ]
        SecItemDelete(query as CFDictionary)
    }
}

#else

public final class KeychainDeviceIdentityStore: SecureDeviceIdentityStore, @unchecked Sendable {
    public init(service: String = "com.stagecore.companion.identity") {}

    public func loadOrCreateIdentity() throws -> SecureDeviceIdentity {
        throw SecureDeviceIdentityError.secureStorageUnavailable
    }

    public func signAuthenticationChallenge(_ message: Data) throws -> Data {
        throw SecureDeviceIdentityError.secureStorageUnavailable
    }
}

#endif
