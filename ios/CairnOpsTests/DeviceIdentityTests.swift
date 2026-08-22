import Foundation
import Testing
@testable import CairnOps

struct DeviceIdentityTests {
    private let token = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

    @Test("L’identité X25519 respecte le contrat base64url")
    func createsValidPublicKey() throws {
        let pending = try makePending()

        let publicKey = try pending.encryptionPublicKey()

        #expect(publicKey.count == 43)
        #expect(!publicKey.contains("="))
        #expect(publicKey.allSatisfy { $0.isLetter || $0.isNumber || $0 == "-" || $0 == "_" })
    }

    @Test("Les champs mobiles respectent les limites Unicode du serveur")
    func boundsDeviceMetadataByUnicodeScalar() throws {
        let link = try DevicePairingLink(
            string: "cairnops://pair?instance=https%3A%2F%2Fcairnops.example.net&token=\(token)"
        )
        let pending = PendingDevicePairing.make(
            from: link,
            deviceName: String(repeating: "🇫🇷", count: 60),
            appVersion: String(repeating: "é", count: 80),
            locale: "de"
        )

		#expect(pending.deviceName.unicodeScalars.count == 100)
		#expect(pending.appVersion.unicodeScalars.count == 64)
		#expect(pending.locale == "fr")
    }

    private func makePending() throws -> PendingDevicePairing {
        let link = try DevicePairingLink(
            string: "cairnops://pair?instance=https%3A%2F%2Fcairnops.example.net&token=\(token)"
        )
        return PendingDevicePairing.make(
            from: link,
            deviceName: "iPhone de Gregory",
            appVersion: "0.1.0 (1)",
            locale: "fr"
        )
    }
}
