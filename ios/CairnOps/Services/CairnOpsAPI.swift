import Foundation

protocol CairnOpsHTTPTransport: Sendable {
    func perform(_ request: URLRequest) async throws -> (Data, URLResponse)
}

extension URLSession: CairnOpsHTTPTransport {
    func perform(_ request: URLRequest) async throws -> (Data, URLResponse) {
        try await data(for: request)
    }
}

struct CairnOpsAPIError: LocalizedError, Equatable {
    let statusCode: Int
    let message: String

    var errorDescription: String? { message }
}

struct CairnOpsAPI {
    enum DevicePairingStatus: String, Decodable, Equatable, Sendable {
        case awaitingScan = "awaiting_scan"
        case awaitingConfirmation = "awaiting_confirmation"
        case confirmed
    }

    struct DevicePairingClaim: Encodable, Equatable, Sendable {
        let name: String
        let platform: String
        let appVersion: String
        let locale: String
        let notificationContent: String
        let encryptionPublicKey: String
		let pushRecipient: String?

        private enum CodingKeys: String, CodingKey {
            case name
            case platform
            case appVersion = "app_version"
            case locale
            case notificationContent = "notification_content"
            case encryptionPublicKey = "encryption_public_key"
            case pushRecipient = "push_recipient"
        }
    }

    struct DevicePairingResult: Decodable, Equatable, Sendable {
        let status: DevicePairingStatus
        let deviceID: String?
        let deviceToken: String?

        private enum CodingKeys: String, CodingKey {
            case status
            case deviceID = "device_id"
            case deviceToken = "device_token"
        }
    }

    struct SetupStatus: Decodable {
        let initialized: Bool
        let name: String
    }

    struct InboxPayload: Decodable {
        let entries: [InboxEntry]
        let unread: Int
    }

    struct OperationalProjection {
        let targets: [Target]
        let incidents: [Incident]
        let measures: [TargetMeasures]
        let inbox: InboxPayload
        let indicatorTargets: [TargetIndicators]

        init(
            targets: [Target],
            incidents: [Incident],
            measures: [TargetMeasures],
            inbox: InboxPayload,
            indicatorTargets: [TargetIndicators] = []
        ) {
            self.targets = targets
            self.incidents = incidents
            self.measures = measures
            self.inbox = inbox
            self.indicatorTargets = indicatorTargets
        }
    }

    private struct SessionEnvelope: Decodable {
        let user: User
    }

    private struct TargetsEnvelope: Decodable {
        let targets: [Target]
    }

    private struct IncidentsEnvelope: Decodable {
        let incidents: [Incident]
    }

    private struct MeasuresEnvelope: Decodable {
        let targets: [TargetMeasures]
    }

    private struct IndicatorsEnvelope: Decodable {
        let targets: [TargetIndicators]
    }

    private struct VersionEnvelope: Decodable {
        let version: String
    }

    private struct ErrorEnvelope: Decodable {
        let error: String
    }

    private let configuration: ServerConfiguration
    private let session: URLSession
    private let transport: any CairnOpsHTTPTransport
    private let cookieStorage: HTTPCookieStorage
    private let deviceToken: String?
    private let decoder = JSONDecoder()
    private let encoder = JSONEncoder()

    init(
        configuration: ServerConfiguration,
        deviceToken: String? = nil,
        session: URLSession? = nil,
        transport: (any CairnOpsHTTPTransport)? = nil,
        cookieStorage: HTTPCookieStorage = .shared
    ) {
        let resolvedSession = session ?? CairnOpsAPI.makeSession(acceptsCookies: deviceToken == nil)
        self.configuration = configuration
        self.deviceToken = deviceToken
        self.session = resolvedSession
        self.transport = transport ?? resolvedSession
        self.cookieStorage = cookieStorage
    }

    static func makeSession(acceptsCookies: Bool = true) -> URLSession {
        let configuration = URLSessionConfiguration.default
        configuration.httpShouldSetCookies = acceptsCookies
        configuration.httpCookieAcceptPolicy = acceptsCookies ? .always : .never
        configuration.httpCookieStorage = acceptsCookies ? .shared : nil
        configuration.waitsForConnectivity = true
        configuration.timeoutIntervalForRequest = 30
        configuration.timeoutIntervalForResource = 60
        return URLSession(configuration: configuration)
    }

    func getSetupStatus() async throws -> SetupStatus {
        try await request(path: "api/v1/setup/status")
    }

    func getVersion() async throws -> String {
        let payload: VersionEnvelope = try await request(path: "api/v1/version")
        return payload.version
    }

    func getCurrentSession() async throws -> User {
        let payload: SessionEnvelope = try await request(path: "api/v1/session")
        return payload.user
    }

    func logout() async throws {
        try await requestVoid(path: "api/v1/session", method: "DELETE")
    }

    func claimDevicePairing(
        pairingToken: String,
        claim: DevicePairingClaim
    ) async throws -> DevicePairingResult {
        try await request(
            path: "api/v1/device-pairings/claim",
            method: "POST",
            body: try encoder.encode(claim),
            bearerToken: pairingToken
        )
    }

	func getDevicePairingResult(pairingToken: String) async throws -> DevicePairingResult {
        try await request(
            path: "api/v1/device-pairings/result",
            bearerToken: pairingToken
        )
	}

	func updatePushRecipient(deviceID: String, recipient: String) async throws {
		struct Update: Encodable {
			let pushRecipient: String

			private enum CodingKeys: String, CodingKey {
				case pushRecipient = "push_recipient"
			}
		}

		try await requestVoid(
			path: "api/v1/devices/\(deviceID)",
			method: "PATCH",
			body: try encoder.encode(Update(pushRecipient: recipient))
		)
	}

    func fetchTargets() async throws -> [Target] {
        let payload: TargetsEnvelope = try await request(path: "api/v1/targets")
        return payload.targets
    }

    func fetchIncidents() async throws -> [Incident] {
        let payload: IncidentsEnvelope = try await request(path: "api/v1/incidents")
        return payload.incidents
    }

    func fetchTargetMeasures() async throws -> [TargetMeasures] {
        let payload: MeasuresEnvelope = try await request(path: "api/v1/metrics/targets")
        return payload.targets
    }

    func fetchInbox() async throws -> InboxPayload {
        try await request(path: "api/v1/notifications")
    }

    func fetchPinnedIndicators() async throws -> [TargetIndicators] {
        let payload: IndicatorsEnvelope = try await request(path: "api/v1/indicators/targets")
        return payload.targets
    }

    func fetchTargetIndicators(targetID: String, window: String) async throws -> TargetIndicators {
        try await request(
            path: "api/v1/targets/\(targetID)/indicators",
            queryItems: [URLQueryItem(name: "window", value: window)]
        )
    }

    func fetchIncidentIndicators(incidentID: String) async throws -> IncidentIndicators {
        try await request(path: "api/v1/incidents/\(incidentID)/indicators")
    }

    func fetchOperationalProjection() async throws -> OperationalProjection {
        async let targets = fetchTargets()
        async let incidents = fetchIncidents()
        async let measures = fetchTargetMeasures()
        async let inbox = fetchInbox()
        async let indicatorTargets = fetchPinnedIndicators()

        return try await OperationalProjection(
            targets: targets,
            incidents: incidents,
            measures: measures,
            inbox: inbox,
            indicatorTargets: indicatorTargets
        )
    }

    func fetchSystemHealth() async throws -> SystemHealth? {
        do {
            return try await request(path: "api/v1/system/health")
        } catch let error as CairnOpsAPIError where error.statusCode == 503 {
            return nil
        }
    }

    func acknowledgeIncident(id: Incident.ID) async throws -> Incident {
        try await request(
            path: "api/v1/incidents/\(id)/acknowledgement",
            method: "POST"
        )
    }

    func makeRealtimeTask(after version: Int64?) throws -> URLSessionWebSocketTask {
        try session.webSocketTask(with: makeRealtimeRequest(after: version))
    }

    func makeRealtimeRequest(after version: Int64?) throws -> URLRequest {
        let eventsURL = try configuration.eventsURL(after: version)
        guard var components = URLComponents(url: eventsURL, resolvingAgainstBaseURL: false) else {
            throw ServerConfiguration.ConfigurationError.invalidBaseURL
        }
        components.scheme = components.scheme == "https" ? "wss" : "ws"
        guard let webSocketURL = components.url else {
            throw ServerConfiguration.ConfigurationError.invalidBaseURL
        }

        var request = URLRequest(url: webSocketURL)
        request.timeoutInterval = 60

        if let deviceToken {
            request.setValue("Bearer \(deviceToken)", forHTTPHeaderField: "Authorization")
        } else if let baseURL = try? configuration.resolvedBaseURL(),
                  let cookies = cookieStorage.cookies(for: baseURL),
                  !cookies.isEmpty {
            HTTPCookie.requestHeaderFields(with: cookies).forEach { header, value in
                request.setValue(value, forHTTPHeaderField: header)
            }
        }

        return request
    }

    func receiveRealtimeMessage(from task: URLSessionWebSocketTask) async throws -> RealtimeMessage {
        let response = try await task.receive()
        let data: Data = switch response {
        case .data(let data):
            data
        case .string(let string):
            Data(string.utf8)
        @unknown default:
            throw CairnOpsAPIError(statusCode: -1, message: "Trame WebSocket inconnue.")
        }

        return try decoder.decode(RealtimeMessage.self, from: data)
    }

    func clearCookies() {
        guard let host = try? configuration.resolvedBaseURL().host() else {
            return
        }

        cookieStorage.cookies?.forEach { cookie in
            if cookie.domain.hasSuffix(host) {
                cookieStorage.deleteCookie(cookie)
            }
        }
    }

    private func request<T: Decodable>(
        path: String,
        method: String = "GET",
        body: Data? = nil,
        bearerToken: String? = nil,
        queryItems: [URLQueryItem] = []
    ) async throws -> T {
        let request = try makeRequest(
            path: path,
            method: method,
            body: body,
            bearerToken: bearerToken,
            queryItems: queryItems
        )
        let (data, response) = try await transport.perform(request)
        return try decodeResponse(data: data, response: response)
    }

    private func requestVoid(
        path: String,
        method: String = "GET",
        body: Data? = nil,
        bearerToken: String? = nil
    ) async throws {
        let request = try makeRequest(
            path: path,
            method: method,
            body: body,
            bearerToken: bearerToken,
            queryItems: []
        )
        let (data, response) = try await transport.perform(request)
        let _: EmptyResponse = try decodeResponse(data: data, response: response)
    }

    private func makeRequest(
        path: String,
        method: String,
        body: Data?,
        bearerToken: String?,
        queryItems: [URLQueryItem]
    ) throws -> URLRequest {
        let baseURL = try configuration.resolvedBaseURL()
        let endpoint = baseURL.appending(path: path)
        guard var components = URLComponents(url: endpoint, resolvingAgainstBaseURL: false) else {
            throw ServerConfiguration.ConfigurationError.invalidBaseURL
        }
        components.queryItems = queryItems.isEmpty ? nil : queryItems
        guard let requestURL = components.url else {
            throw ServerConfiguration.ConfigurationError.invalidBaseURL
        }
        var request = URLRequest(url: requestURL)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")

        if let bearerToken = bearerToken ?? deviceToken {
            request.setValue("Bearer \(bearerToken)", forHTTPHeaderField: "Authorization")
        }

        if let body {
            request.httpBody = body
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }

        return request
    }

    private func decodeResponse<T: Decodable>(data: Data, response: URLResponse) throws -> T {
        guard let response = response as? HTTPURLResponse else {
            throw CairnOpsAPIError(statusCode: -1, message: "Reponse serveur invalide.")
        }

        guard (200..<300).contains(response.statusCode) else {
            if let payload = try? decoder.decode(ErrorEnvelope.self, from: data) {
                throw CairnOpsAPIError(statusCode: response.statusCode, message: payload.error)
            }
            throw CairnOpsAPIError(
                statusCode: response.statusCode,
                message: "La requete a echoue (\(response.statusCode))."
            )
        }

        if T.self == EmptyResponse.self {
            return EmptyResponse() as! T
        }

        return try decoder.decode(T.self, from: data)
    }
}

private struct EmptyResponse: Decodable {}
