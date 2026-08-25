import SwiftUI
import UIKit
import UserNotifications

struct NotificationSettingsView: View {
    @Environment(\.openURL) private var openURL
    @Environment(PushNotificationDelegate.self) private var pushNotifications
    @State private var preferences = NotificationPreferences()
    @State private var isLoaded = false
    @State private var persistenceError: String?
    @State private var permissionMessage: String?

    private let store = NotificationPreferencesStore()

    var body: some View {
        Form {
            soundsSection
            criticalAlertsSection
            remindersSection
            if let persistenceError {
                Section {
                    Label(persistenceError, systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(AppTheme.criticalInk)
                }
            }
        }
        .scrollContentBackground(.hidden)
        .background(AppTheme.ground.ignoresSafeArea())
        // Le formulaire systeme est conserve : ses interrupteurs et selecteurs
        // portent un comportement d'accessibilite que la refonte n'a pas de
        // raison de reimplementer.
        .navigationTitle("Notifications")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            loadPreferences()
            await pushNotifications.refreshNotificationSettings()
            if pushNotifications.criticalAlertSetting == .notSupported,
                preferences.criticalAlertsEnabled {
                preferences.criticalAlertsEnabled = false
            }
        }
        .onChange(of: preferences) { oldValue, newValue in
            guard isLoaded else {
                return
            }
            persist(newValue)
            guard !oldValue.criticalAlertsEnabled, newValue.criticalAlertsEnabled else {
                if oldValue.repeatPolicy != newValue.repeatPolicy,
                    newValue.repeatPolicy == .disabled {
                    Task {
                        await pushNotifications.cancelAllIncidentReminders()
                    }
                }
                return
            }
            permissionMessage = nil
            Task {
                await enableCriticalAlerts()
            }
        }
    }

    private var soundsSection: some View {
        Section {
            Picker("Alerte d’incident", selection: $preferences.incidentSound) {
                ForEach(IncidentNotificationSound.allCases) { sound in
                    Text(AppLanguage.localized(sound.label)).tag(sound)
                }
            }
            .pickerStyle(.navigationLink)

            Picker("Rétablissement", selection: $preferences.recoverySound) {
                ForEach(IncidentNotificationSound.allCases) { sound in
                    Text(AppLanguage.localized(sound.label)).tag(sound)
                }
            }
            .pickerStyle(.navigationLink)
        } header: {
            Text("Sons")
        } footer: {
            Text("Ces choix s’appliquent sur cet iPhone aux nouvelles alertes d’incident et de rétablissement.")
        }
    }

    private var criticalAlertsSection: some View {
        Section {
            Toggle(isOn: $preferences.criticalAlertsEnabled) {
                VStack(alignment: .leading, spacing: 3) {
                    Text("Alertes critiques")
                    Text("Ignorer le mode silencieux et Concentration")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
            }
            .disabled(!pushNotifications.criticalAlertsAvailable)

            LabeledContent("État iOS") {
                Text(criticalAlertStatus)
                    .foregroundStyle(criticalAlertStatusColor)
            }

            Button {
                openSystemNotificationSettings()
            } label: {
                HStack {
                    Text("Réglages système")
                    Spacer()
                    Image(systemName: "arrow.up.forward.app")
                        .accessibilityHidden(true)
                }
                .frame(minHeight: 44)
                .contentShape(.rect)
            }
        } header: {
            Text("Comportement des alertes")
        } footer: {
            VStack(alignment: .leading, spacing: 8) {
                Text(criticalAlertsExplanation)
                if let permissionMessage {
                    Text(permissionMessage)
                        .foregroundStyle(AppTheme.warningInk)
                }
            }
        }
    }

    private var remindersSection: some View {
        Section {
            Picker("Politique de rappel", selection: $preferences.repeatPolicy) {
                ForEach(NotificationRepeatPolicy.allCases) { policy in
                    Text(AppLanguage.localized(policy.label)).tag(policy)
                }
            }
            .pickerStyle(.navigationLink)

            LabeledContent("Séquence") {
                Text(AppLanguage.localized(preferences.repeatPolicy.detail))
                    .multilineTextAlignment(.trailing)
                    .foregroundStyle(.secondary)
            }
        } header: {
            Text("Rappels")
        } footer: {
            Text("Tant qu’un incident reste actif, les rappels choisis sont programmés sur cet iPhone. Le rétablissement annule immédiatement ceux qui restent en attente.")
        }
    }

    private var criticalAlertStatus: String {
        if pushNotifications.criticalAlertSetting == .notSupported {
            return "Non disponible"
        }
        guard pushNotifications.notificationsEnabled else {
            return pushNotifications.authorizationStatus == .denied ? "Notifications coupées" : "À autoriser"
        }
        switch pushNotifications.criticalAlertSetting {
        case .enabled:
            return "Autorisées"
        case .disabled:
            return "Désactivées"
        case .notSupported:
            return "Non disponible"
        @unknown default:
            return "Inconnu"
        }
    }

    private var criticalAlertStatusColor: Color {
        pushNotifications.criticalAlertSetting == .enabled ? AppTheme.okInk : AppTheme.inkMuted
    }

    private var criticalAlertsExplanation: String {
        if pushNotifications.criticalAlertSetting == .notSupported {
            return "Apple exige une autorisation spéciale pour les alertes critiques. Le profil actuel de CairnOps ne possède pas encore cette capacité."
        }
        if pushNotifications.authorizationStatus == .denied {
            return "Les notifications sont désactivées pour CairnOps dans iOS. Ouvrez les réglages système pour les autoriser."
        }
        return "Une fois autorisées par iOS, seules les alertes d’incident de gravité Critique peuvent outrepasser le mode silencieux et Concentration."
    }

    private func loadPreferences() {
        do {
            preferences = try store.load()
            persistenceError = nil
        } catch {
            preferences = NotificationPreferences()
            persistenceError = error.localizedDescription
        }
        isLoaded = true
    }

    private func persist(_ value: NotificationPreferences) {
        do {
            try store.save(value)
            persistenceError = nil
        } catch {
            persistenceError = "Impossible d’enregistrer ces réglages : \(error.localizedDescription)"
        }
    }

    private func enableCriticalAlerts() async {
        guard await pushNotifications.requestCriticalAlertAuthorization() else {
            preferences.criticalAlertsEnabled = false
            permissionMessage = pushNotifications.criticalAlertError
                ?? "iOS n’a pas autorisé les alertes critiques."
            return
        }
        permissionMessage = nil
    }

    private func openSystemNotificationSettings() {
        guard let url = URL(string: UIApplication.openNotificationSettingsURLString) else {
            return
        }
        openURL(url)
    }
}
