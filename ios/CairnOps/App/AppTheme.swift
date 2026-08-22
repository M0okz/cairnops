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
    // `TabView` reserve deja la zone sure de sa barre flottante. Un grand
    // remplissage manuel ajoutait une longue zone morte en fin de defilement.
    static let bottomScrollInset = 24.0
    static let headerSpacing = 20.0
    // Les styles semantiques suivent Dynamic Type, contrairement aux tailles
    // fixes qui faisaient se chevaucher les cartes aux grandes tailles de texte.
    static let pageTitleFont = Font.title2.weight(.semibold)
    static let detailTitleFont = Font.title3.weight(.semibold)
    static let heroTitleFont = Font.title3.weight(.semibold)
    static let sectionTitleFont = Font.headline
    static let cardTitleFont = Font.headline
    static let tileValueFont = Font.headline
    static let rowTitleFont = Font.body.weight(.semibold)

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
