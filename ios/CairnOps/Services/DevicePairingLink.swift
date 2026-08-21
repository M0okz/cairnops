import Foundation

struct DevicePairingLink: Equatable, Sendable {
    enum ValidationError: LocalizedError, Equatable, Sendable {
        case unsupportedURL
        case invalidInstance
        case invalidToken

        var errorDescription: String? {
            switch self {
            case .unsupportedURL:
                "Ce lien n’est pas une invitation d’appairage CairnOps."
            case .invalidInstance:
                "L’adresse de l’instance contenue dans le QR code est invalide."
            case .invalidToken:
                "Le secret d’appairage contenu dans le QR code est invalide."
            }
        }
    }

    let instanceURL: URL
    let token: String

    init(url: URL) throws {
        guard let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              components.scheme?.lowercased() == "cairnops",
              components.host?.lowercased() == "pair",
              components.path.isEmpty || components.path == "/",
              components.user == nil,
              components.password == nil,
              components.port == nil,
              components.fragment == nil else {
            throw ValidationError.unsupportedURL
        }

        let queryItems = components.queryItems ?? []
        let instances = queryItems.filter { $0.name == "instance" }.compactMap(\.value)
        let tokens = queryItems.filter { $0.name == "token" }.compactMap(\.value)
        guard instances.count == 1, tokens.count == 1 else {
            throw ValidationError.unsupportedURL
        }

        guard let instanceComponents = URLComponents(string: instances[0]),
              let instanceScheme = instanceComponents.scheme?.lowercased(),
              instanceScheme == "https" || instanceScheme == "http",
              let instanceHost = instanceComponents.host,
              !instanceHost.isEmpty,
              instanceComponents.query == nil,
              instanceComponents.fragment == nil else {
            throw ValidationError.invalidInstance
        }

        let configuration: ServerConfiguration
        do {
            configuration = try ServerConfiguration(baseURLString: instances[0]).validated()
        } catch {
            throw ValidationError.invalidInstance
        }

        let candidateToken = tokens[0]
        guard Self.isValidToken(candidateToken) else {
            throw ValidationError.invalidToken
        }

        instanceURL = try configuration.resolvedBaseURL()
        token = candidateToken
    }

    init(string: String) throws {
        guard let url = URL(string: string.trimmingCharacters(in: .whitespacesAndNewlines)) else {
            throw ValidationError.unsupportedURL
        }
        try self.init(url: url)
    }

    private static func isValidToken(_ token: String) -> Bool {
        guard token.count == 43,
              token.unicodeScalars.allSatisfy({ scalar in
                  switch scalar.value {
                  case 45, 48...57, 65...90, 95, 97...122:
                      true
                  default:
                      false
                  }
              }) else {
            return false
        }

        var base64 = token.replacingOccurrences(of: "-", with: "+")
            .replacingOccurrences(of: "_", with: "/")
        base64.append("=")
        return Data(base64Encoded: base64)?.count == 32
    }
}
