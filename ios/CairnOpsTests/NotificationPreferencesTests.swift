import Foundation
import Testing
@testable import CairnOps

@MainActor
struct NotificationPreferencesTests {
    @Test("Les réglages initiaux restent sobres et sans répétition implicite")
    func defaultsAreSafe() {
        let preferences = NotificationPreferences()

        #expect(preferences.incidentSound == .system)
        #expect(preferences.recoverySound == .system)
        #expect(preferences.criticalAlertsEnabled == false)
        #expect(preferences.repeatPolicy == .disabled)
    }

    @Test("Le son d’incident et le son de rétablissement sont indépendants")
    func eventKindSelectsItsSound() {
        let preferences = NotificationPreferences(
            incidentSound: .silent,
            recoverySound: .system,
            criticalAlertsEnabled: false,
            repeatPolicy: .disabled
        )

        #expect(preferences.deliverySound(eventKind: "firing", severity: "major") == .silent)
        #expect(preferences.deliverySound(eventKind: "resolved", severity: "major") == .system)
    }

    @Test("Une alerte critique ne remplace le son que pour un incident critique actif")
    func criticalSoundIsScopedToCriticalFiringEvents() {
        let preferences = NotificationPreferences(
            incidentSound: .silent,
            recoverySound: .silent,
            criticalAlertsEnabled: true,
            repeatPolicy: .disabled
        )

        #expect(preferences.deliverySound(eventKind: "firing", severity: "critical") == .critical)
        #expect(preferences.deliverySound(eventKind: "firing", severity: "major") == .silent)
        #expect(preferences.deliverySound(eventKind: "resolved", severity: "critical") == .silent)
    }

    @Test("La politique standard suit les délais annoncés et ne répète que le dernier palier")
    func standardReminderSchedule() {
        #expect(NotificationRepeatPolicy.standard.stages == [
            NotificationReminderStage(id: "5m", delay: 5 * 60, repeats: false),
            NotificationReminderStage(id: "15m", delay: 15 * 60, repeats: false),
            NotificationReminderStage(id: "1h", delay: 60 * 60, repeats: false),
            NotificationReminderStage(id: "4h", delay: 4 * 60 * 60, repeats: true),
        ])
        #expect(NotificationRepeatPolicy.disabled.stages.isEmpty)
    }

    @Test("Les identifiants permettent d’annuler tous les rappels d’un incident")
    func reminderIdentifiersAreStable() {
        let identifiers = NotificationReminderIdentifier.identifiers(incidentID: "incident-42")

        #expect(identifiers == [
            "fr.cairnops.reminder.incident-42.5m",
            "fr.cairnops.reminder.incident-42.15m",
            "fr.cairnops.reminder.incident-42.1h",
            "fr.cairnops.reminder.incident-42.4h",
        ])
        #expect(identifiers.allSatisfy(NotificationReminderIdentifier.isReminder))
        #expect(!NotificationReminderIdentifier.isReminder("unrelated"))
    }

    @Test("Les préférences survivent à la reconstruction du store")
    func persistsPreferences() throws {
        let dataStore = MemoryNotificationPreferencesDataStore()
        let expected = NotificationPreferences(
            incidentSound: .silent,
            recoverySound: .system,
            criticalAlertsEnabled: true,
            repeatPolicy: .standard
        )

        try NotificationPreferencesStore(dataStore: dataStore).save(expected)

        let reloaded = try NotificationPreferencesStore(dataStore: dataStore).load()
        #expect(reloaded == expected)
    }

    @Test("Un ancien enregistrement incomplet reçoit les nouveaux réglages par défaut")
    func decodesOlderPreferences() throws {
        let data = try #require(#"{"incidentSound":"silent"}"#.data(using: .utf8))

        let preferences = try JSONDecoder().decode(NotificationPreferences.self, from: data)

        #expect(preferences.incidentSound == .silent)
        #expect(preferences.recoverySound == .system)
        #expect(preferences.criticalAlertsEnabled == false)
        #expect(preferences.repeatPolicy == .disabled)
    }
}

private final class MemoryNotificationPreferencesDataStore: NotificationPreferencesDataStore {
    private var data: Data?

    func read() throws -> Data? {
        data
    }

    func write(_ data: Data) throws {
        self.data = data
    }
}
