import Foundation

struct CairnOpsAPIError: LocalizedError, Equatable {
    let statusCode: Int
    let message: String

    var errorDescription: String? { message }
}

struct CairnOpsAPI {
    struct SetupStatus: Decodable {
        let initialized: Bool
        let name: String
    }

    struct AuthenticatedSession: Decodable {
        let user: User
        let expiresAt: String

        private enum CodingKeys: String, CodingKey {
            case user
            case expiresAt = "expires_at"
        }
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

    private struct VersionEnvelope: Decodable {
        let version: String
    }

    private struct ErrorEnvelope: Decodable {
        let error: String
    }

    private let configuration: ServerConfiguration
    private let session: URLSession
    private let cookieStorage: HTTPCookieStorage
    private let decoder = JSONDecoder()
    private let encoder = JSONEncoder()

    init(
        configuration: ServerConfiguration,
        session: URLSession = CairnOpsAPI.makeSession(),
        cookieStorage: HTTPCookieStorage = .shared
    ) {
        self.configuration = configuration
        self.session = session
        self.cookieStorage = cookieStorage
    }

    static func makeSession() -> URLSession {
        let configuration = URLSessionConfiguration.default
        configuration.httpShouldSetCookies = true
        configuration.httpCookieAcceptPolicy = .always
        configuration.httpCookieStorage = .shared
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

    func login(username: String, password: String) async throws -> AuthenticatedSession {
        try await request(
            path: "api/v1/session",
            method: "POST",
            body: try encoder.encode([
                "username": username,
                "password": password,
            ])
        )
    }

    func logout() async throws {
        try await requestVoid(path: "api/v1/session", method: "DELETE")
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

    func fetchOperationalProjection() async throws -> OperationalProjection {
        async let targets = fetchTargets()
        async let incidents = fetchIncidents()
        async let measures = fetchTargetMeasures()
        async let inbox = fetchInbox()

        return try await OperationalProjection(
            targets: targets,
            incidents: incidents,
            measures: measures,
            inbox: inbox
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
        let url = try configuration.eventsURL(after: version)
        var request = URLRequest(url: url)
        request.timeoutInterval = 60

        if let baseURL = try? configuration.resolvedBaseURL(),
           let cookies = cookieStorage.cookies(for: baseURL),
           !cookies.isEmpty {
            HTTPCookie.requestHeaderFields(with: cookies).forEach { header, value in
                request.setValue(value, forHTTPHeaderField: header)
            }
        }

        return session.webSocketTask(with: request)
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
        body: Data? = nil
    ) async throws -> T {
        let request = try makeRequest(path: path, method: method, body: body)
        let (data, response) = try await session.data(for: request)
        return try decodeResponse(data: data, response: response)
    }

    private func requestVoid(
        path: String,
        method: String = "GET",
        body: Data? = nil
    ) async throws {
        let request = try makeRequest(path: path, method: method, body: body)
        let (data, response) = try await session.data(for: request)
        let _: EmptyResponse = try decodeResponse(data: data, response: response)
    }

    private func makeRequest(path: String, method: String, body: Data?) throws -> URLRequest {
        let baseURL = try configuration.resolvedBaseURL()
        var request = URLRequest(url: baseURL.appending(path: path))
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")

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
