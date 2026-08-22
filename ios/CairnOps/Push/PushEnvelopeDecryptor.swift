import CryptoKit
import Foundation

struct PushEnvelope: Codable, Equatable, Sendable {
	let version: Int
	let ephemeralPublicKey: String
	let nonce: String
	let ciphertext: String

	private enum CodingKeys: String, CodingKey {
		case version
		case ephemeralPublicKey = "ephemeral_public_key"
		case nonce
		case ciphertext
	}
}

struct PushMessage: Decodable, Equatable, Sendable {
	struct Presentation: Decodable, Equatable, Sendable {
		let title: String
		let body: String
	}

	let version: Int
	let eventKind: String
	let incidentID: String
	let severity: String
	let occurredAt: String
	let instanceURL: String
	let presentation: Presentation

	private enum CodingKeys: String, CodingKey {
		case version
		case eventKind = "event_kind"
		case incidentID = "incident_id"
		case severity
		case occurredAt = "occurred_at"
		case instanceURL = "instance_url"
		case presentation
	}
}

enum PushEnvelopeDecryptor {
	enum DecryptionError: Error, Equatable {
		case invalidEnvelope
		case unsupportedVersion
	}

	private static let context = Data("cairnops-push-envelope-v1".utf8)

	static func decrypt(_ envelope: PushEnvelope, privateKey: Data) throws -> PushMessage {
		guard envelope.version == 1 else {
			throw DecryptionError.unsupportedVersion
		}
		guard privateKey.count == 32,
			let ephemeralKeyData = Data(base64URL: envelope.ephemeralPublicKey),
			ephemeralKeyData.count == 32,
			let nonce = Data(base64URL: envelope.nonce),
			nonce.count == 24,
			let sealed = Data(base64URL: envelope.ciphertext),
			sealed.count >= 16 else {
			throw DecryptionError.invalidEnvelope
		}

		let localKey = try Curve25519.KeyAgreement.PrivateKey(rawRepresentation: privateKey)
		let ephemeralKey = try Curve25519.KeyAgreement.PublicKey(rawRepresentation: ephemeralKeyData)
		let sharedSecret = try localKey.sharedSecretFromKeyAgreement(with: ephemeralKey)
		let xChaChaKey = sharedSecret.hkdfDerivedSymmetricKey(
			using: SHA256.self,
			salt: Data(),
			sharedInfo: context,
			outputByteCount: 32
		)
		let keyBytes = xChaChaKey.withUnsafeBytes { Data($0) }
		let subkey = try HChaCha20.deriveKey(key: keyBytes, nonce: nonce.prefix(16))
		let ietfNonce = Data(repeating: 0, count: 4) + nonce.suffix(8)
		let cryptoNonce = try ChaChaPoly.Nonce(data: ietfNonce)
		let box = try ChaChaPoly.SealedBox(
			nonce: cryptoNonce,
			ciphertext: sealed.dropLast(16),
			tag: sealed.suffix(16)
		)
		let plaintext = try ChaChaPoly.open(
			box,
			using: SymmetricKey(data: subkey),
			authenticating: context
		)
		return try JSONDecoder().decode(PushMessage.self, from: plaintext)
	}
}

private enum HChaCha20 {
	static func deriveKey(key: Data, nonce: Data.SubSequence) throws -> Data {
		guard key.count == 32, nonce.count == 16 else {
			throw PushEnvelopeDecryptor.DecryptionError.invalidEnvelope
		}
		let keyBytes = [UInt8](key)
		let nonceBytes = [UInt8](nonce)
		var state: [UInt32] = [
			0x61707865, 0x3320646e, 0x79622d32, 0x6b206574,
		]
		for offset in stride(from: 0, to: keyBytes.count, by: 4) {
			state.append(word(keyBytes, at: offset))
		}
		for offset in stride(from: 0, to: nonceBytes.count, by: 4) {
			state.append(word(nonceBytes, at: offset))
		}

		for _ in 0..<10 {
			quarterRound(&state, 0, 4, 8, 12)
			quarterRound(&state, 1, 5, 9, 13)
			quarterRound(&state, 2, 6, 10, 14)
			quarterRound(&state, 3, 7, 11, 15)
			quarterRound(&state, 0, 5, 10, 15)
			quarterRound(&state, 1, 6, 11, 12)
			quarterRound(&state, 2, 7, 8, 13)
			quarterRound(&state, 3, 4, 9, 14)
		}

		return [state[0], state[1], state[2], state[3], state[12], state[13], state[14], state[15]]
			.reduce(into: Data()) { output, value in
				var littleEndian = value.littleEndian
				withUnsafeBytes(of: &littleEndian) { output.append(contentsOf: $0) }
			}
	}

	private static func word(_ bytes: [UInt8], at offset: Int) -> UInt32 {
		UInt32(bytes[offset])
			| UInt32(bytes[offset + 1]) << 8
			| UInt32(bytes[offset + 2]) << 16
			| UInt32(bytes[offset + 3]) << 24
	}

	private static func quarterRound(
		_ state: inout [UInt32],
		_ a: Int,
		_ b: Int,
		_ c: Int,
		_ d: Int
	) {
		state[a] &+= state[b]
		state[d] = rotateLeft(state[d] ^ state[a], by: 16)
		state[c] &+= state[d]
		state[b] = rotateLeft(state[b] ^ state[c], by: 12)
		state[a] &+= state[b]
		state[d] = rotateLeft(state[d] ^ state[a], by: 8)
		state[c] &+= state[d]
		state[b] = rotateLeft(state[b] ^ state[c], by: 7)
	}

	private static func rotateLeft(_ value: UInt32, by count: UInt32) -> UInt32 {
		(value << count) | (value >> (32 - count))
	}
}

private extension Data {
	init?(base64URL value: String) {
		var encoded = value
			.replacingOccurrences(of: "-", with: "+")
			.replacingOccurrences(of: "_", with: "/")
		let padding = (4 - encoded.count % 4) % 4
		encoded += String(repeating: "=", count: padding)
		self.init(base64Encoded: encoded)
	}
}
