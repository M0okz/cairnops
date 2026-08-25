import SwiftUI

/// Choix de theme de l'utilisateur.
///
/// Les deux themes sont de qualite equivalente : le sombre est la reference de
/// conception, le clair n'en est pas une degradation. Le reglage permet donc de
/// forcer l'un ou l'autre, sans qu'aucun ne soit un mode secondaire.
enum AppearancePreference: String, CaseIterable, Identifiable {
    case system
    case dark
    case light

    static let storageKey = "cairnops.appearance"

    var id: String { rawValue }

    var title: String {
        switch self {
        case .system:
            AppLanguage.localized("appearance.system")
        case .dark:
            AppLanguage.localized("appearance.dark")
        case .light:
            AppLanguage.localized("appearance.light")
        }
    }

    /// `nil` laisse le systeme trancher.
    var colorScheme: ColorScheme? {
        switch self {
        case .system:
            nil
        case .dark:
            .dark
        case .light:
            .light
        }
    }
}
