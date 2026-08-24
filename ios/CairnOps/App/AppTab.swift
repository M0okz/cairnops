import Foundation

/// Routes de la barre d'onglets.
///
/// Les Reglages n'y figurent plus : la maquette les atteint depuis l'identite
/// de la Vue d'ensemble, et la barre reste ainsi consacree aux quatre vues
/// operationnelles.
enum AppTab: String, CaseIterable, Identifiable {
    case overview
    case incidents
    case targets
    case health

    var id: String { rawValue }

    var title: String {
        switch self {
        case .overview:
            "Vue"
        case .incidents:
            "Incidents"
        case .targets:
            "Cibles"
        case .health:
            "Santé"
        }
    }

    var symbolName: String {
        switch self {
        case .overview:
            "square.grid.2x2"
        case .incidents:
            "exclamationmark.triangle"
        case .targets:
            "dot.scope"
        case .health:
            "waveform.path.ecg"
        }
    }
}

import SwiftUI

extension EnvironmentValues {

    /// Bascule vers un autre onglet depuis le contenu d'un ecran.
    ///
    /// La Vue d'ensemble renvoie vers la liste complete des Incidents ; sans ce
    /// passage, le lien « Tout voir » devrait empiler un second exemplaire de
    /// l'ecran deja present dans la barre.
    @Entry var selectTab: (AppTab) -> Void = { _ in }
}
