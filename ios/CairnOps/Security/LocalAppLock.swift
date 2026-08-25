import Foundation
import LocalAuthentication
import Observation

@MainActor
protocol LocalAuthenticating {
    func canAuthenticate() -> Bool
    func authenticate(reason: String) async throws -> Bool
}

@MainActor
struct SystemLocalAuthenticator: LocalAuthenticating {
    func canAuthenticate() -> Bool {
        LAContext().canEvaluatePolicy(.deviceOwnerAuthentication, error: nil)
    }

    func authenticate(reason: String) async throws -> Bool {
        try await LAContext().evaluatePolicy(
            .deviceOwnerAuthentication,
            localizedReason: reason
        )
    }
}

protocol LocalLockPreferenceStore {
    var isEnabled: Bool { get set }
}

struct UserDefaultsLocalLockPreferenceStore: LocalLockPreferenceStore {
    static let storageKey = "fr.cairnops.local-lock-enabled"

    private let defaults: UserDefaults

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    var isEnabled: Bool {
        get { defaults.bool(forKey: Self.storageKey) }
        nonmutating set { defaults.set(newValue, forKey: Self.storageKey) }
    }
}

@MainActor
@Observable
final class LocalAppLock {
    private(set) var isEnabled: Bool
    private(set) var isLocked: Bool
    private(set) var errorMessage: String?

    @ObservationIgnored private let authenticator: any LocalAuthenticating
    @ObservationIgnored private var preferenceStore: any LocalLockPreferenceStore

    init(
        authenticator: any LocalAuthenticating = SystemLocalAuthenticator(),
        preferenceStore: any LocalLockPreferenceStore = UserDefaultsLocalLockPreferenceStore()
    ) {
        self.authenticator = authenticator
        self.preferenceStore = preferenceStore
        isEnabled = preferenceStore.isEnabled
        isLocked = preferenceStore.isEnabled
    }

    var canEnable: Bool {
        authenticator.canAuthenticate()
    }

    @discardableResult
    func setEnabled(_ enabled: Bool) async -> Bool {
        guard enabled != isEnabled else {
            return isEnabled
        }

        if enabled {
            guard canEnable else {
                errorMessage = AppLanguage.localized("lock.unavailable")
                return false
            }
            guard await authenticate(reasonKey: "lock.enableReason") else {
                return false
            }
        }

        preferenceStore.isEnabled = enabled
        isEnabled = enabled
        isLocked = false
        errorMessage = nil
        return isEnabled
    }

    func lockWhenLeavingForeground() {
        guard isEnabled else {
            return
        }
        isLocked = true
        errorMessage = nil
    }

    @discardableResult
    func unlock() async -> Bool {
        guard isEnabled, isLocked else {
            return true
        }
        guard canEnable else {
            errorMessage = AppLanguage.localized("lock.unavailable")
            return false
        }
        guard await authenticate(reasonKey: "lock.unlockReason") else {
            return false
        }
        isLocked = false
        errorMessage = nil
        return true
    }

    func unlockFromAuthenticatedNotificationAction() {
        guard isEnabled else {
            return
        }
        isLocked = false
        errorMessage = nil
    }

    private func authenticate(reasonKey: String) async -> Bool {
        do {
            let authenticated = try await authenticator.authenticate(
                reason: AppLanguage.localized(reasonKey)
            )
            if !authenticated {
                errorMessage = AppLanguage.localized("lock.authenticationFailed")
            }
            return authenticated
        } catch let error as LAError where error.code == .userCancel || error.code == .appCancel {
            errorMessage = nil
            return false
        } catch {
            errorMessage = error.localizedDescription
            return false
        }
    }
}
