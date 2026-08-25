import Foundation
import Testing
@testable import CairnOps

@MainActor
struct LocalAppLockTests {
    @Test("Un verrouillage enregistré protège l’app dès son lancement")
    func restoresEnabledLock() {
        let preferences = LocalLockTestPreferenceStore(isEnabled: true)

        let lock = LocalAppLock(
            authenticator: LocalLockTestAuthenticator(),
            preferenceStore: preferences
        )

        #expect(lock.isEnabled)
        #expect(lock.isLocked)
    }

    @Test("L’activation exige une authentification et persiste le choix")
    func authenticatesBeforeEnabling() async {
        let authenticator = LocalLockTestAuthenticator()
        let preferences = LocalLockTestPreferenceStore()
        let lock = LocalAppLock(
            authenticator: authenticator,
            preferenceStore: preferences
        )

        let enabled = await lock.setEnabled(true)

        #expect(enabled)
        #expect(lock.isEnabled)
        #expect(!lock.isLocked)
        #expect(preferences.isEnabled)
        #expect(authenticator.reasons.count == 1)
    }

    @Test("Un refus d’authentification ne peut pas activer le verrou")
    func rejectsFailedAuthentication() async {
        let authenticator = LocalLockTestAuthenticator(result: false)
        let preferences = LocalLockTestPreferenceStore()
        let lock = LocalAppLock(
            authenticator: authenticator,
            preferenceStore: preferences
        )

        let enabled = await lock.setEnabled(true)

        #expect(!enabled)
        #expect(!lock.isEnabled)
        #expect(!preferences.isEnabled)
        #expect(lock.errorMessage != nil)
    }

    @Test("Le départ en arrière-plan reverrouille puis demande le code système")
    func locksWhenLeavingForeground() async {
        let authenticator = LocalLockTestAuthenticator()
        let lock = LocalAppLock(
            authenticator: authenticator,
            preferenceStore: LocalLockTestPreferenceStore(isEnabled: true)
        )

        _ = await lock.unlock()
        lock.lockWhenLeavingForeground()
        #expect(lock.isLocked)

        let unlocked = await lock.unlock()
        #expect(unlocked)
        #expect(!lock.isLocked)
    }

    @Test("Une action Push déjà authentifiée ouvre le verrou sans second défi")
    func authenticatedNotificationUnlocks() {
        let authenticator = LocalLockTestAuthenticator()
        let lock = LocalAppLock(
            authenticator: authenticator,
            preferenceStore: LocalLockTestPreferenceStore(isEnabled: true)
        )

        lock.unlockFromAuthenticatedNotificationAction()

        #expect(!lock.isLocked)
        #expect(authenticator.reasons.isEmpty)
    }
}

@MainActor
private final class LocalLockTestAuthenticator: LocalAuthenticating {
    let available: Bool
    let result: Bool
    private(set) var reasons: [String] = []

    init(available: Bool = true, result: Bool = true) {
        self.available = available
        self.result = result
    }

    func canAuthenticate() -> Bool {
        available
    }

    func authenticate(reason: String) async throws -> Bool {
        reasons.append(reason)
        return result
    }
}

private final class LocalLockTestPreferenceStore: LocalLockPreferenceStore {
    var isEnabled: Bool

    init(isEnabled: Bool = false) {
        self.isEnabled = isEnabled
    }
}
