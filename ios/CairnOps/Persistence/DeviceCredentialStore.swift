import Foundation
import Security

@MainActor
protocol SecureDataStore {
    func read() throws -> Data?
    func write(_ data: Data) throws
    func delete() throws
}

@MainActor
struct DeviceCredentialStore {
    enum StoreError: LocalizedError {
        case unreadableCredentials

        var errorDescription: String? {
            "Les identifiants sécurisés de cet appareil sont illisibles."
        }
    }

    private let secureDataStore: any SecureDataStore
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    init(
        secureDataStore: any SecureDataStore = KeychainSecureDataStore()
    ) {
        self.secureDataStore = secureDataStore
    }

    func load() throws -> DeviceCredentialState {
        guard let data = try secureDataStore.read() else {
            return DeviceCredentialState()
        }
        guard let state = try? decoder.decode(DeviceCredentialState.self, from: data) else {
            throw StoreError.unreadableCredentials
        }
        return state
    }

    func save(pendingPairing: PendingDevicePairing) throws {
        try write(DeviceCredentialState(pendingPairing: pendingPairing))
    }

    func save(identity: DeviceIdentity) throws {
        try write(DeviceCredentialState(identity: identity))
    }

    func clear() throws {
        try secureDataStore.delete()
    }

    private func write(_ state: DeviceCredentialState) throws {
        try secureDataStore.write(encoder.encode(state))
    }
}

@MainActor
private final class KeychainSecureDataStore: SecureDataStore {
    enum KeychainError: LocalizedError {
        case unexpectedStatus(OSStatus)
        case invalidData

        var errorDescription: String? {
            switch self {
            case .unexpectedStatus:
                "Le stockage sécurisé de l’iPhone n’est pas disponible."
            case .invalidData:
                "Le stockage sécurisé de l’iPhone a renvoyé une valeur invalide."
            }
        }
    }

    private let service = "fr.cairnops.ios.device-credentials"
    private let account = "current-device"

    func read() throws -> Data? {
        var query = baseQuery
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne

        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        switch status {
        case errSecSuccess:
            guard let data = result as? Data else {
                throw KeychainError.invalidData
            }
            return data
        case errSecItemNotFound:
            return nil
        default:
            throw KeychainError.unexpectedStatus(status)
        }
    }

    func write(_ data: Data) throws {
        let attributes: [String: Any] = [
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        let updateStatus = SecItemUpdate(baseQuery as CFDictionary, attributes as CFDictionary)

        if updateStatus == errSecItemNotFound {
            var insertion = baseQuery
            attributes.forEach { insertion[$0.key] = $0.value }
            let insertionStatus = SecItemAdd(insertion as CFDictionary, nil)
            guard insertionStatus == errSecSuccess else {
                throw KeychainError.unexpectedStatus(insertionStatus)
            }
            return
        }

        guard updateStatus == errSecSuccess else {
            throw KeychainError.unexpectedStatus(updateStatus)
        }
    }

    func delete() throws {
        let status = SecItemDelete(baseQuery as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw KeychainError.unexpectedStatus(status)
        }
    }

    private var baseQuery: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecAttrSynchronizable as String: false,
        ]
    }
}
