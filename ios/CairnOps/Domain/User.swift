import Foundation

struct User: Codable, Equatable, Identifiable, Sendable {
    enum Role: String, Codable, CaseIterable, Sendable {
        case administrator
        case `operator`
        case observer

        var canAcknowledge: Bool {
            self != .observer
        }

        var label: String {
            switch self {
            case .administrator:
                "Administrateur"
            case .operator:
                "Operateur"
            case .observer:
                "Observateur"
            }
        }
    }

    let id: String
    let username: String
    let displayName: String
    let role: Role

    private enum CodingKeys: String, CodingKey {
        case id
        case username
        case displayName = "display_name"
        case role
    }
}
