import Foundation

enum CairnOpsNotification {
    static let incidentCategoryIdentifier = "CAIRNOPS_INCIDENT"
    static let acknowledgeActionIdentifier = "CAIRNOPS_ACKNOWLEDGE"

    static let incidentIDKey = "cairnops_incident_id"
    static let instanceURLKey = "cairnops_instance_url"
    static let eventKindKey = "cairnops_event_kind"
}

struct NotificationActionRequest: Equatable, Sendable {
    let incidentID: String
    let instanceURL: String
    let eventKind: String

    init?(userInfo: [AnyHashable: Any]) {
        guard let incidentID = userInfo[CairnOpsNotification.incidentIDKey] as? String,
              !incidentID.isEmpty,
              let instanceURL = userInfo[CairnOpsNotification.instanceURLKey] as? String,
              !instanceURL.isEmpty,
              let eventKind = userInfo[CairnOpsNotification.eventKindKey] as? String,
              !eventKind.isEmpty else {
            return nil
        }
        self.incidentID = incidentID
        self.instanceURL = instanceURL
        self.eventKind = eventKind
    }

    var canAcknowledge: Bool {
        eventKind == "firing"
    }
}

struct NotificationActionResult: Equatable, Identifiable, Sendable {
    enum Outcome: Equatable, Sendable {
        case opened
        case acknowledged
        case failed(message: String)
    }

    let id = UUID()
    let request: NotificationActionRequest
    let outcome: Outcome
    let authenticatedAction: Bool
}

@MainActor
struct NotificationActionService {
    enum ActionError: LocalizedError, Equatable {
        case noDeviceIdentity
        case instanceMismatch
        case incidentNoLongerActive

        var errorDescription: String? {
            switch self {
            case .noDeviceIdentity:
                AppLanguage.localized("notification.action.noIdentity")
            case .instanceMismatch:
                AppLanguage.localized("notification.action.instanceMismatch")
            case .incidentNoLongerActive:
                AppLanguage.localized("notification.action.notActive")
            }
        }
    }

    private let credentialStore: DeviceCredentialStore
    private let apiFactory: (ServerConfiguration, String?) -> CairnOpsAPI

    init(
        credentialStore: DeviceCredentialStore = DeviceCredentialStore(),
        apiFactory: @escaping (ServerConfiguration, String?) -> CairnOpsAPI = {
            CairnOpsAPI(configuration: $0, deviceToken: $1)
        }
    ) {
        self.credentialStore = credentialStore
        self.apiFactory = apiFactory
    }

    func acknowledge(_ request: NotificationActionRequest) async throws -> Incident {
        guard request.canAcknowledge else {
            throw ActionError.incidentNoLongerActive
        }
        guard let identity = try credentialStore.load().identity else {
            throw ActionError.noDeviceIdentity
        }

        let notificationConfiguration = try ServerConfiguration(
            baseURLString: request.instanceURL
        ).validated()
        let deviceConfiguration = try ServerConfiguration(
            baseURLString: identity.serverBaseURL
        ).validated()
        guard notificationConfiguration == deviceConfiguration else {
            throw ActionError.instanceMismatch
        }

        return try await apiFactory(
            deviceConfiguration,
            identity.deviceToken
        ).acknowledgeIncident(id: request.incidentID)
    }
}
