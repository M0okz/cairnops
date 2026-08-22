import Foundation
import Security

enum SharedDeviceCredentials {
	static let service = "fr.cairnops.ios.device-credentials"
	static let account = "current-device"

	private struct CredentialState: Decodable {
		struct Identity: Decodable {
			let encryptionPrivateKey: Data
		}

		let identity: Identity?
	}

	static var accessGroup: String? {
		guard let value = Bundle.main.object(
			forInfoDictionaryKey: "CairnOpsKeychainAccessGroup"
		) as? String,
			!value.isEmpty,
			!value.contains("$(") else {
			return nil
		}
		return value
	}

	static func query(shared: Bool) -> [String: Any] {
		var query: [String: Any] = [
			kSecClass as String: kSecClassGenericPassword,
			kSecAttrService as String: service,
			kSecAttrAccount as String: account,
			kSecAttrSynchronizable as String: false,
		]
		if shared, let accessGroup {
			query[kSecAttrAccessGroup as String] = accessGroup
		}
		return query
	}

	static func encryptionPrivateKey() throws -> Data? {
		var query = query(shared: true)
		query[kSecReturnData as String] = true
		query[kSecMatchLimit as String] = kSecMatchLimitOne
		var result: CFTypeRef?
		let status = SecItemCopyMatching(query as CFDictionary, &result)
		if status == errSecItemNotFound {
			return nil
		}
		guard status == errSecSuccess, let data = result as? Data else {
			throw NSError(domain: NSOSStatusErrorDomain, code: Int(status))
		}
		return try JSONDecoder().decode(CredentialState.self, from: data).identity?.encryptionPrivateKey
	}
}
