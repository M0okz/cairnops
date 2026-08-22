import Foundation
import Testing
@testable import CairnOps

extension Tag {
    @Tag static var networking: Self
}

@Suite(.tags(.networking))
struct CairnOpsAPIPairingTests {
    private let pairingToken = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

    @Test("La revendication utilise PairingBearer et le contrat claim")
    func claimsPairing() async throws {
        let transport = HTTPTransportSpy(stubs: [
            .json(statusCode: 202, body: #"{"status":"awaiting_confirmation"}"#),
        ])
        let api = try makeAPI(transport: transport)
        let claim = CairnOpsAPI.DevicePairingClaim(
            name: "iPhone de Gregory",
            platform: "ios",
            appVersion: "0.1.0 (1)",
            locale: "fr",
			notificationContent: "complete",
			encryptionPublicKey: "public-key",
			pushRecipient: nil
        )

        let result = try await api.claimDevicePairing(pairingToken: pairingToken, claim: claim)

        #expect(result.status == .awaitingConfirmation)
        let requests = await transport.recordedRequests()
        let request = try #require(requests.first)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/base/api/v1/device-pairings/claim")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer \(pairingToken)")

        let body = try #require(request.httpBody)
        let decoded = try JSONDecoder().decode(ClaimBody.self, from: body)
        #expect(decoded.name == claim.name)
        #expect(decoded.platform == "ios")
        #expect(decoded.encryptionPublicKey == claim.encryptionPublicKey)
		#expect(decoded.pushRecipient == nil)
    }

    @Test("Le résultat d’appairage récupère le jeton remis une seule fois")
    func fetchesPairingResult() async throws {
        let transport = HTTPTransportSpy(stubs: [
            .json(
                statusCode: 200,
                body: #"{"status":"confirmed","device_id":"device-id","device_token":"device-token"}"#
            ),
        ])
        let api = try makeAPI(transport: transport)

        let result = try await api.getDevicePairingResult(pairingToken: pairingToken)

        #expect(result.status == .confirmed)
        #expect(result.deviceID == "device-id")
        #expect(result.deviceToken == "device-token")
        let requests = await transport.recordedRequests()
        let request = try #require(requests.first)
        #expect(request.httpMethod == "GET")
        #expect(request.url?.path == "/base/api/v1/device-pairings/result")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer \(pairingToken)")
    }

    @Test("DeviceBearer authentifie REST et WebSocket")
    func appliesDeviceBearerEverywhere() async throws {
        let transport = HTTPTransportSpy(stubs: [
            .json(
                statusCode: 200,
                body: #"{"user":{"id":"user-id","username":"ops","display_name":"Ops","role":"operator"}}"#
            ),
        ])
        let api = try makeAPI(deviceToken: "durable-device-token", transport: transport)

        _ = try await api.getCurrentSession()
        let requests = await transport.recordedRequests()
        let request = try #require(requests.first)
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer durable-device-token")

        let realtimeRequest = try api.makeRealtimeRequest(after: 42)
        #expect(realtimeRequest.url?.absoluteString == "wss://cairnops.example.net/base/api/v1/events?after=42")
        #expect(realtimeRequest.value(forHTTPHeaderField: "Authorization") == "Bearer durable-device-token")
    }

    private func makeAPI(
        deviceToken: String? = nil,
        transport: HTTPTransportSpy
    ) throws -> CairnOpsAPI {
        let configuration = try ServerConfiguration(
            baseURLString: "https://cairnops.example.net/base"
        ).validated()
        return CairnOpsAPI(
            configuration: configuration,
            deviceToken: deviceToken,
            transport: transport
        )
    }
}

private struct ClaimBody: Decodable {
    let name: String
    let platform: String
    let encryptionPublicKey: String
	let pushRecipient: String?

    private enum CodingKeys: String, CodingKey {
        case name
        case platform
        case encryptionPublicKey = "encryption_public_key"
        case pushRecipient = "push_recipient"
    }
}

private actor HTTPTransportSpy: CairnOpsHTTPTransport {
    struct Stub: Sendable {
        let statusCode: Int
        let data: Data

        static func json(statusCode: Int, body: String) -> Stub {
            Stub(statusCode: statusCode, data: Data(body.utf8))
        }
    }

    enum StubError: Error {
        case missingResponse
        case invalidRequestURL
        case invalidHTTPResponse
    }

    private var stubs: [Stub]
    private var requests: [URLRequest] = []

    init(stubs: [Stub]) {
        self.stubs = stubs
    }

    func perform(_ request: URLRequest) async throws -> (Data, URLResponse) {
        requests.append(request)
        guard !stubs.isEmpty else {
            throw StubError.missingResponse
        }
        guard let url = request.url else {
            throw StubError.invalidRequestURL
        }

        let stub = stubs.removeFirst()
        guard let response = HTTPURLResponse(
            url: url,
            statusCode: stub.statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        ) else {
            throw StubError.invalidHTTPResponse
        }
        return (stub.data, response)
    }

    func recordedRequests() -> [URLRequest] {
        requests
    }
}
