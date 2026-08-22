import Foundation

struct PushRelayError: LocalizedError, Equatable, Sendable {
	let statusCode: Int
	let message: String

	var errorDescription: String? { message }
}

struct PushRelayClient: Sendable {
	private struct RegistrationRequest: Encodable {
		let platform = "ios"
		let environment: String
		let deviceToken: String

		private enum CodingKeys: String, CodingKey {
			case platform
			case environment
			case deviceToken = "device_token"
		}
	}

	private struct ErrorEnvelope: Decodable {
		let error: String
	}

	private let baseURL: URL
	private let environment: String
	private let transport: any CairnOpsHTTPTransport
	private let encoder = JSONEncoder()
	private let decoder = JSONDecoder()

	init(
		baseURL: URL,
		environment: String = "sandbox",
		transport: (any CairnOpsHTTPTransport)? = nil
	) {
		self.baseURL = baseURL
		self.environment = environment
		self.transport = transport ?? CairnOpsAPI.makeSession(acceptsCookies: false)
	}

	static func configured(bundle: Bundle = .main) throws -> PushRelayClient {
		guard let rawURL = bundle.object(forInfoDictionaryKey: "CairnOpsPushRelayURL") as? String,
			let url = URL(string: rawURL),
			url.scheme == "https",
			url.host != nil else {
			throw PushRelayError(
				statusCode: -1,
				message: "L’adresse du Relais Push n’est pas configurée."
			)
		}
		let apnsEnvironment = bundle.object(
			forInfoDictionaryKey: "CairnOpsAPNSEnvironment"
		) as? String
		let environment = switch apnsEnvironment {
		case "development": "sandbox"
		case "production": "production"
		default: ""
		}
		guard !environment.isEmpty else {
			throw PushRelayError(
				statusCode: -1,
				message: "L’environnement APNs de CairnOps n’est pas configuré."
			)
		}
		return PushRelayClient(baseURL: url, environment: environment)
	}

	func register(deviceToken: String) async throws -> PushRelayRegistration {
		var request = try makeRequest(
			path: "v1/registrations",
			method: "POST",
			body: encoder.encode(RegistrationRequest(environment: environment, deviceToken: deviceToken))
		)
		request.setValue("no-store", forHTTPHeaderField: "Cache-Control")
		return try await perform(request)
	}

	func rotate(
		_ registration: PushRelayRegistration,
		deviceToken: String
	) async throws {
		let request = try makeRequest(
			path: "v1/registrations/\(registration.recipient)",
			method: "PUT",
			body: encoder.encode(RegistrationRequest(environment: environment, deviceToken: deviceToken)),
			managementToken: registration.managementToken
		)
		try await performVoid(request)
	}

	func remove(_ registration: PushRelayRegistration) async throws {
		let request = try makeRequest(
			path: "v1/registrations/\(registration.recipient)",
			method: "DELETE",
			managementToken: registration.managementToken
		)
		try await performVoid(request)
	}

	private func makeRequest(
		path: String,
		method: String,
		body: Data? = nil,
		managementToken: String? = nil
	) throws -> URLRequest {
		var request = URLRequest(url: baseURL.appending(path: path))
		request.httpMethod = method
		request.setValue("application/json", forHTTPHeaderField: "Accept")
		request.timeoutInterval = 15
		if let body {
			request.httpBody = body
			request.setValue("application/json", forHTTPHeaderField: "Content-Type")
		}
		if let managementToken {
			request.setValue("Bearer \(managementToken)", forHTTPHeaderField: "Authorization")
		}
		return request
	}

	private func perform<T: Decodable>(_ request: URLRequest) async throws -> T {
		let (data, response) = try await transport.perform(request)
		try validate(data: data, response: response)
		return try decoder.decode(T.self, from: data)
	}

	private func performVoid(_ request: URLRequest) async throws {
		let (data, response) = try await transport.perform(request)
		try validate(data: data, response: response)
	}

	private func validate(data: Data, response: URLResponse) throws {
		guard let response = response as? HTTPURLResponse else {
			throw PushRelayError(statusCode: -1, message: "Réponse invalide du Relais Push.")
		}
		guard (200..<300).contains(response.statusCode) else {
			let message = (try? decoder.decode(ErrorEnvelope.self, from: data).error)
				?? "Le Relais Push a refusé la requête."
			throw PushRelayError(statusCode: response.statusCode, message: message)
		}
	}
}
