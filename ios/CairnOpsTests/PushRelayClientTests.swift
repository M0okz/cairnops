import Foundation
import Testing
@testable import CairnOps

@Suite(.tags(.networking))
struct PushRelayClientTests {
	@Test("L’inscription sépare le destinataire de la capacité de gestion")
	func registersAndRotatesAPNSToken() async throws {
		let transport = RelayTransportSpy(stubs: [
			.init(statusCode: 201, body: #"{"recipient":"opaque-recipient","management_token":"management-secret"}"#),
			.init(statusCode: 204, body: ""),
			.init(statusCode: 204, body: ""),
		])
		let client = PushRelayClient(
			baseURL: try #require(URL(string: "https://push.example.test/base")),
			transport: transport
		)

		let registration = try await client.register(deviceToken: "0011aabb")
		#expect(registration.recipient == "opaque-recipient")
		#expect(registration.managementToken == "management-secret")
		try await client.rotate(registration, deviceToken: "ffee")
		try await client.remove(registration)

		let requests = await transport.requests
		#expect(requests.compactMap(\.httpMethod) == ["POST", "PUT", "DELETE"])
		#expect(requests[0].url?.path == "/base/v1/registrations")
		#expect(requests[1].url?.path == "/base/v1/registrations/opaque-recipient")
		#expect(requests[1].value(forHTTPHeaderField: "Authorization") == "Bearer management-secret")
		let body = try #require(requests[0].httpBody)
		let decoded = try JSONDecoder().decode(RegistrationBody.self, from: body)
		#expect(decoded.platform == "ios")
		#expect(decoded.environment == "sandbox")
		#expect(decoded.deviceToken == "0011aabb")
	}
}

private struct RegistrationBody: Decodable {
	let platform: String
	let environment: String
	let deviceToken: String

	private enum CodingKeys: String, CodingKey {
		case platform
		case environment
		case deviceToken = "device_token"
	}
}

private actor RelayTransportSpy: CairnOpsHTTPTransport {
	struct Stub: Sendable {
		let statusCode: Int
		let body: String
	}

	private var stubs: [Stub]
	private(set) var requests: [URLRequest] = []

	init(stubs: [Stub]) {
		self.stubs = stubs
	}

	func perform(_ request: URLRequest) async throws -> (Data, URLResponse) {
		requests.append(request)
		let stub = stubs.removeFirst()
		let url = try #require(request.url)
		let response = try #require(HTTPURLResponse(
			url: url,
			statusCode: stub.statusCode,
			httpVersion: "HTTP/2",
			headerFields: ["Content-Type": "application/json"]
		))
		return (Data(stub.body.utf8), response)
	}
}
