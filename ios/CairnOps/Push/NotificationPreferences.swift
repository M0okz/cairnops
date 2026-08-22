import Foundation
import Security

enum IncidentNotificationSound: String, CaseIterable, Codable, Identifiable, Sendable {
    case system
    case silent

    var id: Self { self }

    var label: String {
        switch self {
        case .system:
            "Par défaut"
        case .silent:
            "Silencieux"
        }
    }
}

enum NotificationDeliverySound: Equatable, Sendable {
    case system
    case silent
    case critical
}

struct NotificationReminderStage: Equatable, Sendable {
    let id: String
    let delay: TimeInterval
    let repeats: Bool
}

enum NotificationReminderIdentifier {
    static let prefix = "fr.cairnops.reminder."

    static func identifier(incidentID: String, stageID: String) -> String {
        "\(prefix)\(incidentID).\(stageID)"
    }

    static func identifiers(incidentID: String) -> [String] {
        NotificationRepeatPolicy.allCases
            .flatMap(\.stages)
            .reduce(into: [NotificationReminderStage]()) { result, stage in
                guard !result.contains(where: { $0.id == stage.id }) else {
                    return
                }
                result.append(stage)
            }
            .map { identifier(incidentID: incidentID, stageID: $0.id) }
    }

    static func isReminder(_ identifier: String) -> Bool {
        identifier.hasPrefix(prefix)
    }
}

enum NotificationRepeatPolicy: String, CaseIterable, Codable, Identifiable, Sendable {
    case disabled
    case standard

    var id: Self { self }

    var label: String {
        switch self {
        case .disabled:
            "Aucun"
        case .standard:
            "Standard"
        }
    }

    var detail: String {
        switch self {
        case .disabled:
            "Une seule notification par événement"
        case .standard:
            "Immédiat → 5 min → 15 min → 1 h → 4 h (répété)"
        }
    }

    var stages: [NotificationReminderStage] {
        switch self {
        case .disabled:
            []
        case .standard:
            [
                NotificationReminderStage(id: "5m", delay: 5 * 60, repeats: false),
                NotificationReminderStage(id: "15m", delay: 15 * 60, repeats: false),
                NotificationReminderStage(id: "1h", delay: 60 * 60, repeats: false),
                NotificationReminderStage(id: "4h", delay: 4 * 60 * 60, repeats: true),
            ]
        }
    }
}

struct NotificationPreferences: Codable, Equatable, Sendable {
    var incidentSound: IncidentNotificationSound
    var recoverySound: IncidentNotificationSound
    var criticalAlertsEnabled: Bool
    var repeatPolicy: NotificationRepeatPolicy

    init(
        incidentSound: IncidentNotificationSound = .system,
        recoverySound: IncidentNotificationSound = .system,
        criticalAlertsEnabled: Bool = false,
        repeatPolicy: NotificationRepeatPolicy = .disabled
    ) {
        self.incidentSound = incidentSound
        self.recoverySound = recoverySound
        self.criticalAlertsEnabled = criticalAlertsEnabled
        self.repeatPolicy = repeatPolicy
    }

    func deliverySound(eventKind: String, severity: String) -> NotificationDeliverySound {
        if criticalAlertsEnabled, eventKind == "firing", severity == "critical" {
            return .critical
        }
        let selected = eventKind == "resolved" ? recoverySound : incidentSound
        switch selected {
        case .system:
            return .system
        case .silent:
            return .silent
        }
    }

    private enum CodingKeys: String, CodingKey {
        case incidentSound
        case recoverySound
        case criticalAlertsEnabled
        case repeatPolicy
    }

    init(from decoder: any Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        incidentSound = try values.decodeIfPresent(IncidentNotificationSound.self, forKey: .incidentSound) ?? .system
        recoverySound = try values.decodeIfPresent(IncidentNotificationSound.self, forKey: .recoverySound) ?? .system
        criticalAlertsEnabled = try values.decodeIfPresent(Bool.self, forKey: .criticalAlertsEnabled) ?? false
        repeatPolicy = try values.decodeIfPresent(NotificationRepeatPolicy.self, forKey: .repeatPolicy) ?? .disabled
    }
}

protocol NotificationPreferencesDataStore {
    func read() throws -> Data?
    func write(_ data: Data) throws
}

struct NotificationPreferencesStore {
    enum StoreError: LocalizedError {
        case unreadablePreferences

        var errorDescription: String? {
            "Les réglages de notification enregistrés sont illisibles."
        }
    }

    private let dataStore: any NotificationPreferencesDataStore

    init(dataStore: any NotificationPreferencesDataStore = KeychainNotificationPreferencesDataStore()) {
        self.dataStore = dataStore
    }

    func load() throws -> NotificationPreferences {
        guard let data = try dataStore.read() else {
            return NotificationPreferences()
        }
        guard let preferences = try? JSONDecoder().decode(NotificationPreferences.self, from: data) else {
            throw StoreError.unreadablePreferences
        }
        return preferences
    }

    func save(_ preferences: NotificationPreferences) throws {
        try dataStore.write(JSONEncoder().encode(preferences))
    }
}

private final class KeychainNotificationPreferencesDataStore: NotificationPreferencesDataStore {
    private static let service = "fr.cairnops.ios.notification-preferences"
    private static let account = "global"

    func read() throws -> Data? {
        var query = baseQuery
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
        return data
    }

    func write(_ data: Data) throws {
        let attributes: [String: Any] = [
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        let updateStatus = SecItemUpdate(baseQuery as CFDictionary, attributes as CFDictionary)
        if updateStatus == errSecItemNotFound {
            var insertion = baseQuery
            attributes.forEach { insertion[$0.key] = $0.value }
            let insertionStatus = SecItemAdd(insertion as CFDictionary, nil)
            guard insertionStatus == errSecSuccess else {
                throw NSError(domain: NSOSStatusErrorDomain, code: Int(insertionStatus))
            }
            return
        }
        guard updateStatus == errSecSuccess else {
            throw NSError(domain: NSOSStatusErrorDomain, code: Int(updateStatus))
        }
    }

    private var baseQuery: [String: Any] {
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: Self.service,
            kSecAttrAccount as String: Self.account,
            kSecAttrSynchronizable as String: false,
        ]
        if let accessGroup = SharedDeviceCredentials.accessGroup {
            query[kSecAttrAccessGroup as String] = accessGroup
        }
        return query
    }
}
