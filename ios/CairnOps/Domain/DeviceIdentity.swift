import CryptoKit
import Foundation

struct PendingDevicePairing: Codable, Equatable, Sendable {
    enum KeyMaterialError: LocalizedError, Equatable, Sendable {
        case invalidPrivateKey

        var errorDescription: String? {
            "L’identité cryptographique locale de l’appareil est invalide."
        }
    }

    let serverBaseURL: String
    let pairingToken: String
    let deviceName: String
    let appVersion: String
    let locale: String
    let notificationContent: String
    let encryptionPrivateKey: Data
    let pushRecipient: String
    let createdAt: Date

    static func make(
        from link: DevicePairingLink,
        deviceName: String,
        appVersion: String,
        locale: String,
        now: Date = .now
    ) -> PendingDevicePairing {
        let privateKey = Curve25519.KeyAgreement.PrivateKey()
        let provisionalPushRecipient = SymmetricKey(size: .bits256).withUnsafeBytes {
            Data($0).base64URLEncodedString()
        }
        let trimmedName = deviceName.trimmingCharacters(in: .whitespacesAndNewlines)
        return PendingDevicePairing(
            serverBaseURL: link.instanceURL.absoluteString,
            pairingToken: link.token,
            deviceName: trimmedName.isEmpty
                ? "iPhone"
                : trimmedName.prefixingUnicodeScalars(100),
            appVersion: appVersion.prefixingUnicodeScalars(64),
            locale: locale == "en" ? "en" : "fr",
            notificationContent: "complete",
            encryptionPrivateKey: privateKey.rawRepresentation,
            // L’inscription au Relais Push fera tourner cette capacité via PATCH.
            // Elle doit déjà être stable et opaque pour satisfaire le contrat
            // d’identité, même lorsque le Relais est désactivé sur l’instance.
            pushRecipient: provisionalPushRecipient,
            createdAt: now
        )
    }

    func encryptionPublicKey() throws -> String {
        guard let privateKey = try? Curve25519.KeyAgreement.PrivateKey(
            rawRepresentation: encryptionPrivateKey
        ) else {
            throw KeyMaterialError.invalidPrivateKey
        }
        return privateKey.publicKey.rawRepresentation.base64URLEncodedString()
    }
}

struct DeviceIdentity: Codable, Equatable, Sendable {
    let serverBaseURL: String
    let deviceID: String
    let deviceToken: String
    let encryptionPrivateKey: Data
    let pushRecipient: String
}

struct DeviceCredentialState: Codable, Equatable, Sendable {
    var pendingPairing: PendingDevicePairing?
    var identity: DeviceIdentity?

    init(
        pendingPairing: PendingDevicePairing? = nil,
        identity: DeviceIdentity? = nil
    ) {
        self.pendingPairing = pendingPairing
        self.identity = identity
    }
}

private extension Data {
    func base64URLEncodedString() -> String {
        base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }
}

private extension String {
    func prefixingUnicodeScalars(_ maximumCount: Int) -> String {
        String(unicodeScalars.prefix(maximumCount))
    }
}
