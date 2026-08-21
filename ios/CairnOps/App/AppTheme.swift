import SwiftUI

enum AppTheme {
    static let background = Color("BrandBackground")
    static let panel = Color("BrandPanel")
    static let accent = Color("BrandCopper")
    static let ok = Color("BrandOk")
    static let warning = Color("BrandWarning")
    static let critical = Color("BrandCritical")
    static let info = Color("BrandInfo")
    static let line = Color.primary.opacity(0.08)
    static let subpanel = background
    static let shadow = Color.black.opacity(0.06)

    static let screenPadding = 16.0
    static let sectionSpacing = 14.0
    static let cardSpacing = 10.0
    static let cardCorner = 16.0
    static let bottomScrollInset = 96.0
    static let headerSpacing = 20.0
    static let pageTitleFont = Font.system(size: 22, weight: .semibold, design: .default)
    static let detailTitleFont = Font.system(size: 21, weight: .semibold, design: .default)
    static let heroTitleFont = Font.system(size: 19, weight: .semibold, design: .default)
    static let sectionTitleFont = Font.system(size: 18, weight: .semibold, design: .default)
    static let cardTitleFont = Font.system(size: 17, weight: .semibold, design: .default)
    static let tileValueFont = Font.system(size: 17, weight: .semibold, design: .default)
    static let rowTitleFont = Font.system(size: 16, weight: .semibold, design: .default)

    static func severityColor(_ severity: IncidentSeverity) -> Color {
        switch severity {
        case .information:
            info
        case .warning:
            warning
        case .major, .critical:
            critical
        }
    }

    static func severitySymbol(_ severity: IncidentSeverity) -> String {
        switch severity {
        case .information:
            "info.circle.fill"
        case .warning:
            "exclamationmark.triangle.fill"
        case .major:
            "bolt.horizontal.circle.fill"
        case .critical:
            "xmark.octagon.fill"
        }
    }

    static func targetHealthColor(_ health: AppSnapshot.TargetHealth) -> Color {
        switch health {
        case .ok:
            ok
        case .degraded, .maintenance:
            warning
        case .down:
            critical
        case .unknown:
            info
        }
    }

    static func targetHealthLabel(_ health: AppSnapshot.TargetHealth) -> String {
        switch health {
        case .ok:
            "Opérationnelle"
        case .degraded:
            "Dégradée"
        case .down:
            "Indisponible"
        case .maintenance:
            "Maintenance"
        case .unknown:
            "Inconnue"
        }
    }

    static func targetHealthSymbol(_ health: AppSnapshot.TargetHealth) -> String {
        switch health {
        case .ok:
            "checkmark.circle.fill"
        case .degraded:
            "exclamationmark.triangle.fill"
        case .down:
            "xmark.octagon.fill"
        case .maintenance:
            "wrench.and.screwdriver.fill"
        case .unknown:
            "questionmark.circle.fill"
        }
    }

    static func globalStatusColor(_ status: AppSnapshot.GlobalStatus) -> Color {
        switch status {
        case .allOperational:
            ok
        case .ongoingIncident:
            critical
        case .degradedServices:
            warning
        case .incompleteMonitoring:
            info
        case .notConfigured:
            accent
        }
    }

    static func globalStatusLabel(_ status: AppSnapshot.GlobalStatus) -> String {
        switch status {
        case .allOperational:
            "Tout est opérationnel"
        case .ongoingIncident:
            "Incident en cours"
        case .degradedServices:
            "Services dégradés"
        case .incompleteMonitoring:
            "Supervision incomplète"
        case .notConfigured:
            "Supervision non configurée"
        }
    }

    static func globalStatusSymbol(_ status: AppSnapshot.GlobalStatus) -> String {
        switch status {
        case .allOperational:
            "checkmark.circle.fill"
        case .ongoingIncident:
            "xmark.octagon.fill"
        case .degradedServices:
            "exclamationmark.triangle.fill"
        case .incompleteMonitoring:
            "questionmark.circle.fill"
        case .notConfigured:
            "slider.horizontal.3"
        }
    }
}
