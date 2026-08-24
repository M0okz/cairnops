import SwiftUI

/// Grand nombre de la maquette : compteur d'Incidents, valeur d'indicateur.
///
/// La taille reste pilotee par Dynamic Type via `@ScaledMetric`, mais bornee :
/// un 56 pt libre debordait de l'ecran aux tailles d'accessibilite.
struct DisplayNumber: View {
    @ScaledMetric(relativeTo: .largeTitle) private var scale = 1

    let value: String
    var size = 56.0
    var tone: Color = AppTheme.ink

    /// Facteur maximal applique a la taille de base.
    private var cappedScale: Double {
        min(scale, 1.45)
    }

    var body: some View {
        Text(value)
            .font(.system(size: size * cappedScale, weight: .heavy))
            .monospacedDigit()
            .tracking(AppTheme.displayTracking)
            .foregroundStyle(tone)
            .lineLimit(1)
            .minimumScaleFactor(0.6)
    }
}
