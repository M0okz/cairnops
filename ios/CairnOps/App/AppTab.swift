import Foundation

enum AppTab: String, CaseIterable, Identifiable {
    case overview
    case targets
    case incidents
    case health
    case settings

    var id: String { rawValue }

    var title: String {
        switch self {
        case .overview:
            "Aperçu"
        case .targets:
            "Cibles"
        case .incidents:
            "Incidents"
        case .health:
            "Santé"
        case .settings:
            "Réglages"
        }
    }

    var symbolName: String {
        switch self {
        case .overview:
            "square.grid.2x2"
        case .targets:
            "dot.scope"
        case .incidents:
            "exclamationmark.triangle"
        case .health:
            "waveform.path.ecg"
        case .settings:
            "gearshape"
        }
    }
}
