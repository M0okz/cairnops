import Foundation
import Testing
@testable import CairnOps

@MainActor
struct DeviceCredentialStoreTests {
    @Test("L’appairage en attente survit à une relance")
    func persistsPendingPairing() throws {
        let secureData = MemorySecureDataStore()
        let store = DeviceCredentialStore(secureDataStore: secureData)
        let pending = fixturePendingPairing()

        try store.save(pendingPairing: pending)

        let reloaded = try DeviceCredentialStore(secureDataStore: secureData).load()
        #expect(reloaded.pendingPairing == pending)
        #expect(reloaded.identity == nil)
    }

    @Test("La confirmation remplace le secret temporaire par l’identité durable")
    func promotesPairingToIdentity() throws {
        let secureData = MemorySecureDataStore()
        let store = DeviceCredentialStore(secureDataStore: secureData)
        let pending = fixturePendingPairing()
        try store.save(pendingPairing: pending)
        let identity = DeviceIdentity(
            serverBaseURL: pending.serverBaseURL,
            deviceID: "device-id",
            deviceToken: "device-token",
            encryptionPrivateKey: pending.encryptionPrivateKey,
            pushRecipient: pending.pushRecipient
        )

        try store.save(identity: identity)

        let reloaded = try store.load()
        #expect(reloaded.pendingPairing == nil)
        #expect(reloaded.identity == identity)
    }

    @Test("La suppression locale efface tout secret d’appareil")
    func clearsCredentials() throws {
        let secureData = MemorySecureDataStore()
        let store = DeviceCredentialStore(secureDataStore: secureData)
        try store.save(pendingPairing: fixturePendingPairing())

        try store.clear()

        #expect(try store.load() == DeviceCredentialState())
    }

    private func fixturePendingPairing() -> PendingDevicePairing {
        PendingDevicePairing(
            serverBaseURL: "https://cairnops.example.net",
            pairingToken: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
            deviceName: "iPhone de Gregory",
            appVersion: "0.1.0 (1)",
            locale: "fr",
            notificationContent: "complete",
            encryptionPrivateKey: Data(repeating: 7, count: 32),
            pushRecipient: "opaque-recipient-1234",
            createdAt: Date(timeIntervalSince1970: 1_700_000_000)
        )
    }
}

@MainActor
private final class MemorySecureDataStore: SecureDataStore {
    private var data: Data?

    func read() throws -> Data? {
        data
    }

    func write(_ data: Data) throws {
        self.data = data
    }

    func delete() throws {
        data = nil
    }
}
