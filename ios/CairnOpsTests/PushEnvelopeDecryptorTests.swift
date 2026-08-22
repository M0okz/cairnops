import Foundation
import Testing
@testable import CairnOps

struct PushEnvelopeDecryptorTests {
	@Test("L’enveloppe X25519/XChaCha20 produite par Go est déchiffrée sur iOS")
	func decryptsGoCompatibilityVector() throws {
		let privateKey = try #require(Data(base64URL: "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA"))
		let envelope = PushEnvelope(
			version: 1,
			ephemeralPublicKey: "WGmv9FBUlzLLqu1eXfmzCm2jHLDldCutWtShp2jxpns",
			nonce: "QUJDREVGR0hJSktMTU5PUFFSU1RVVldY",
			ciphertext: "LNa8N6yum6zSa5ozPKK627c8TXhSKCgAwtjON4I74h5VzLRD-XYNX57HBkMOWKFn3PuKF0PCfjl5XDUNgqI_Dovhw5hjNg2jXe8ZYQrm34CcmT45roP6iN-4AMNF8YOM0QhF-gysHUwJMnAyDHhqKfs6yoEJkOJr4x4KUkdhjc6bLthjmgJYgKRqYW47EbXoQ8lNzGPrHu2ZOY1GVk7aAD3feMvM1xIdmrBWCjLUSwIPjJycE5hUjkRdDsZubMd-qzE7uhPbUREhjzeU-vklz9zQ45NrdkL66q1yG-0WsEtsxN3bkgiVf0L_fUztfJ8SJlFvtIEEIPghFg"
		)

		let message = try PushEnvelopeDecryptor.decrypt(envelope, privateKey: privateKey)

		#expect(message.version == 1)
		#expect(message.eventKind == "firing")
		#expect(message.incidentID == "incident-vector")
		#expect(message.severity == "critical")
		#expect(message.presentation.title == "Routeur")
		#expect(message.presentation.body == "Indisponibilité")
	}
}

private extension Data {
	init?(base64URL value: String) {
		var encoded = value.replacingOccurrences(of: "-", with: "+")
			.replacingOccurrences(of: "_", with: "/")
		encoded += String(repeating: "=", count: (4 - encoded.count % 4) % 4)
		self.init(base64Encoded: encoded)
	}
}
