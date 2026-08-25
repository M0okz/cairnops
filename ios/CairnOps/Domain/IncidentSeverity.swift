import Foundation

enum IncidentSeverity: String, Codable, CaseIterable, Sendable {
    case information
    case warning
    case major
    case critical

    var label: String {
        switch self {
        case .information:
            AppLanguage.localized("severity.information")
        case .warning:
            AppLanguage.localized("severity.warning")
        case .major:
            AppLanguage.localized("severity.major")
        case .critical:
            AppLanguage.localized("severity.critical")
        }
    }
}
