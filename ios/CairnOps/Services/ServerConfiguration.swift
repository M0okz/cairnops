import Foundation

struct ServerConfiguration: Codable, Equatable, Sendable {
    enum ConfigurationError: LocalizedError, Sendable {
        case missingBaseURL
        case invalidBaseURL
        case notInitialized

        var errorDescription: String? {
            switch self {
            case .missingBaseURL:
                "Renseignez l'URL de l'instance CairnOps."
            case .invalidBaseURL:
                "L'URL de l'instance est invalide."
            case .notInitialized:
                "Cette instance n'est pas initialisee. La mise en service initiale reste cote Web."
            }
        }
    }

    var baseURLString = ""

    func validated() throws -> ServerConfiguration {
        let trimmedBaseURL = baseURLString.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedBaseURL.isEmpty else {
            throw ConfigurationError.missingBaseURL
        }

        var candidate = trimmedBaseURL
        if !candidate.contains("://") {
            candidate = "https://\(candidate)"
        }

        guard var components = URLComponents(string: candidate) else {
            throw ConfigurationError.invalidBaseURL
        }
        guard let scheme = components.scheme?.lowercased(),
              scheme == "https" || scheme == "http",
              let host = components.host,
              !host.isEmpty,
              components.user == nil,
              components.password == nil else {
            throw ConfigurationError.invalidBaseURL
        }
        components.scheme = scheme

        components.query = nil
        components.fragment = nil

        if components.path == "/" {
            components.path = ""
        }
        while components.path.count > 1 && components.path.hasSuffix("/") {
            components.path.removeLast()
        }

        guard let normalizedURL = components.url else {
            throw ConfigurationError.invalidBaseURL
        }

        return ServerConfiguration(baseURLString: normalizedURL.absoluteString)
    }

    func resolvedBaseURL() throws -> URL {
        let normalized = try validated()
        guard let url = URL(string: normalized.baseURLString) else {
            throw ConfigurationError.invalidBaseURL
        }
        return url
    }

    func eventsURL(after version: Int64?) throws -> URL {
        var components = URLComponents(
            url: try resolvedBaseURL().appending(path: "api/v1/events"),
            resolvingAgainstBaseURL: false
        )
        if let version {
            components?.queryItems = [URLQueryItem(name: "after", value: String(version))]
        }
        guard let url = components?.url else {
            throw ConfigurationError.invalidBaseURL
        }
        return url
    }
}
