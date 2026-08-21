import Foundation

enum IncidentSeverity: String, Codable, CaseIterable, Sendable {
    case information
    case warning
    case major
    case critical

    var label: String {
        switch self {
        case .information:
            "Information"
        case .warning:
            "Avertissement"
        case .major:
            "Majeur"
        case .critical:
            "Critique"
        }
    }
}
