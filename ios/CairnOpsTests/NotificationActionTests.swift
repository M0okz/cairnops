import Foundation
import Testing
@testable import CairnOps

@MainActor
struct NotificationActionTests {
    @Test("L’action extrait uniquement une enveloppe Push complète")
    func parsesNotificationEnvelope() throws {
        let request = try #require(NotificationActionRequest(userInfo: [
            CairnOpsNotification.incidentIDKey: "incident-1",
            CairnOpsNotification.instanceURLKey: "https://cairnops.example.net/base",
            CairnOpsNotification.eventKindKey: "firing",
        ]))

        #expect(request.incidentID == "incident-1")
        #expect(request.canAcknowledge)
        #expect(NotificationActionRequest(userInfo: [
            CairnOpsNotification.incidentIDKey: "incident-1",
        ]) == nil)
    }

    @Test("Une notification de rétablissement ne peut pas acquitter")
    func rejectsResolvedNotification() async throws {
        let service = NotificationActionService(
            credentialStore: DeviceCredentialStore(
                secureDataStore: NotificationActionMemorySecureDataStore()
            )
        )
        let request = try #require(NotificationActionRequest(userInfo: [
            CairnOpsNotification.incidentIDKey: "incident-1",
            CairnOpsNotification.instanceURLKey: "https://cairnops.example.net/base",
            CairnOpsNotification.eventKindKey: "resolved",
        ]))

        await #expect(throws: NotificationActionService.ActionError.incidentNoLongerActive) {
            try await service.acknowledge(request)
        }
    }

    @Test("L’acquittement utilise l’identité de l’appareil et revalide côté serveur")
    func acknowledgesWithDeviceIdentity() async throws {
        let secureDataStore = NotificationActionMemorySecureDataStore()
        let credentialStore = DeviceCredentialStore(secureDataStore: secureDataStore)
        try credentialStore.save(identity: DeviceIdentity(
            serverBaseURL: "https://cairnops.example.net/base",
            deviceID: "device-1",
            deviceToken: "device-token",
            encryptionPrivateKey: Data(repeating: 7, count: 32),
            pushRegistration: nil
        ))
        let transport = NotificationActionHTTPTransportSpy(body: Self.incidentBody)
        let service = NotificationActionService(
            credentialStore: credentialStore,
            apiFactory: { configuration, token in
                CairnOpsAPI(
                    configuration: configuration,
                    deviceToken: token,
                    transport: transport
                )
            }
        )
        let request = try #require(NotificationActionRequest(userInfo: [
            CairnOpsNotification.incidentIDKey: "incident-1",
            CairnOpsNotification.instanceURLKey: "https://cairnops.example.net/base",
            CairnOpsNotification.eventKindKey: "firing",
        ]))

        let incident = try await service.acknowledge(request)

        #expect(incident.id == "incident-1")
        let recorded = try #require(await transport.recordedRequest())
        #expect(recorded.httpMethod == "POST")
        #expect(recorded.url?.path == "/base/api/v1/incidents/incident-1/acknowledgement")
        #expect(recorded.value(forHTTPHeaderField: "Authorization") == "Bearer device-token")
    }

    @Test("Une notification d’une autre instance est refusée")
    func rejectsAnotherInstance() async throws {
        let secureDataStore = NotificationActionMemorySecureDataStore()
        let credentialStore = DeviceCredentialStore(secureDataStore: secureDataStore)
        try credentialStore.save(identity: DeviceIdentity(
            serverBaseURL: "https://cairnops.example.net/base",
            deviceID: "device-1",
            deviceToken: "device-token",
            encryptionPrivateKey: Data(repeating: 7, count: 32),
            pushRegistration: nil
        ))
        let service = NotificationActionService(credentialStore: credentialStore)
        let request = try #require(NotificationActionRequest(userInfo: [
            CairnOpsNotification.incidentIDKey: "incident-1",
            CairnOpsNotification.instanceURLKey: "https://other.example.net",
            CairnOpsNotification.eventKindKey: "firing",
        ]))

        await #expect(throws: NotificationActionService.ActionError.instanceMismatch) {
            try await service.acknowledge(request)
        }
    }

    private static let incidentBody = #"{"id":"incident-1","target_id":"target-1","target_name":"API","nature_key":"availability","nature_label":"Indisponibilité","status":"active","source_severity":"critical","effective_severity":"critical","opened_at":"2026-08-25T08:00:00Z","resolved_at":null,"acknowledged_at":"2026-08-25T08:01:00Z","acknowledged_by":"Ops","acknowledgement_origin":"local","acknowledgement_sync_status":"not_applicable","acknowledgement_sync_error":null,"maintenance_active":false,"maintenance_ends_at":null,"signals":[],"activity":[],"created_at":"2026-08-25T08:00:00Z","updated_at":"2026-08-25T08:01:00Z"}"#
}

@MainActor
private final class NotificationActionMemorySecureDataStore: SecureDataStore {
    private var data: Data?

    func read() throws -> Data? { data }
    func write(_ data: Data) throws { self.data = data }
    func delete() throws { data = nil }
}

private actor NotificationActionHTTPTransportSpy: CairnOpsHTTPTransport {
    let body: String
    private var request: URLRequest?

    init(body: String) {
        self.body = body
    }

    func perform(_ request: URLRequest) async throws -> (Data, URLResponse) {
        self.request = request
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: 200,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        return (Data(body.utf8), response)
    }

    func recordedRequest() -> URLRequest? { request }
}
