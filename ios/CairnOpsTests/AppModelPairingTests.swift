import Foundation
import Testing
@testable import CairnOps

@MainActor
struct AppModelPairingTests {
    private let pairingToken = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
    private let deviceToken = "Hh0cGxoZGBcWFRQTEhEQDw4NDAsKCQgHBgUEAwIBAAA"

    @Test("La confirmation promeut le secret temporaire en identité DeviceBearer")
    func completesDurablePairing() async throws {
        let secureDataStore = PairingMemorySecureDataStore()
        let credentialStore = DeviceCredentialStore(secureDataStore: secureDataStore)
        let transport = PairingFlowTransport(deviceToken: deviceToken)
        let defaultsName = "AppModelPairingTests.\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: defaultsName))
        defer { defaults.removePersistentDomain(forName: defaultsName) }
        let snapshotStore = SnapshotStore(
            fileURL: FileManager.default.temporaryDirectory
                .appending(path: "cairnops-pairing-\(UUID().uuidString).json")
        )
        let model = AppModel(
            configurationStore: ServerConfigurationStore(defaults: defaults),
            credentialStore: credentialStore,
            snapshotStore: snapshotStore,
            pairingPollInterval: .milliseconds(1),
            apiFactory: { configuration, token in
                CairnOpsAPI(
                    configuration: configuration,
                    deviceToken: token,
                    transport: transport
                )
            }
        )
        model.debugInstallSyncHooks(
            projection: {
                CairnOpsAPI.OperationalProjection(
                    targets: [],
                    incidents: [],
                    measures: [],
                    inbox: CairnOpsAPI.InboxPayload(entries: [], unread: 0)
                )
            },
            health: { nil },
            version: { "test" }
        )

        await model.bootstrap()
        model.acceptPairingLink(
            "cairnops://pair?instance=https%3A%2F%2Fcairnops.example.net&token=\(pairingToken)"
        )
        await model.runPairingAttempt()

        let credentials = try credentialStore.load()
        #expect(credentials.pendingPairing == nil)
        #expect(credentials.identity?.deviceToken == deviceToken)
        #expect(model.user?.username == "ops")
        #expect(model.pairingState == .idle)
        #expect(model.hasDeviceIdentity)

        let requests = await transport.recordedRequests()
        let claim = try #require(
            requests.first { $0.url?.path == "/api/v1/device-pairings/claim" }
        )
        #expect(claim.value(forHTTPHeaderField: "Authorization") == "Bearer \(pairingToken)")
        let authenticated = try #require(
            requests.first { $0.url?.path == "/api/v1/session" }
        )
        #expect(authenticated.value(forHTTPHeaderField: "Authorization") == "Bearer \(deviceToken)")

        await snapshotStore.clear()
    }
}

@MainActor
private final class PairingMemorySecureDataStore: SecureDataStore {
    private var data: Data?

    func read() throws -> Data? { data }
    func write(_ data: Data) throws { self.data = data }
    func delete() throws { data = nil }
}

private actor PairingFlowTransport: CairnOpsHTTPTransport {
    private let deviceToken: String
    private var requests: [URLRequest] = []

    init(deviceToken: String) {
        self.deviceToken = deviceToken
    }

    func perform(_ request: URLRequest) async throws -> (Data, URLResponse) {
        requests.append(request)
        let path = request.url?.path ?? ""
        let body: String
        let statusCode: Int
        switch (request.httpMethod ?? "GET", path) {
        case ("GET", "/api/v1/setup/status"):
            (statusCode, body) = (200, #"{"initialized":true,"name":"CairnOps Test"}"#)
        case ("POST", "/api/v1/device-pairings/claim"):
            (statusCode, body) = (202, #"{"status":"awaiting_confirmation"}"#)
        case ("GET", "/api/v1/device-pairings/result"):
            (statusCode, body) = (
                200,
                #"{"status":"confirmed","device_id":"00000000-0000-0000-0000-000000000001","device_token":"\#(deviceToken)"}"#
            )
        case ("GET", "/api/v1/session"):
            (statusCode, body) = (
                200,
                #"{"user":{"id":"00000000-0000-0000-0000-000000000002","username":"ops","display_name":"Ops","role":"operator"}}"#
            )
        case ("GET", "/api/v1/version"):
            (statusCode, body) = (200, #"{"version":"test"}"#)
        default:
            (statusCode, body) = (500, #"{"error":"unexpected test request"}"#)
        }

        let url = try #require(request.url)
        let response = try #require(HTTPURLResponse(
            url: url,
            statusCode: statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        ))
        return (Data(body.utf8), response)
    }

    func recordedRequests() -> [URLRequest] { requests }
}
