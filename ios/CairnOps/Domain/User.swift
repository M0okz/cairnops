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
                AppLanguage.localized("role.administrator")
            case .operator:
                AppLanguage.localized("role.operator")
            case .observer:
                AppLanguage.localized("role.observer")
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
