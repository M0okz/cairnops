import SwiftUI

/// Grand nombre de la maquette : compteur d'Incidents, valeur d'indicateur.
///
/// La taille reste pilotee par Dynamic Type via `@ScaledMetric`, mais bornee :
/// un 56 pt libre debordait de l'ecran aux tailles d'accessibilite.
struct DisplayNumber: View {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
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
            // Les chiffres defilent comme un compteur mecanique lorsqu'ils
            // changent : le mouvement confirme la mise a jour sans la retarder,
            // et s'efface si l'utilisateur reduit les animations.
            .contentTransition(.numericText())
            .animation(
                reduceMotion ? nil : .snappy(duration: 0.35),
                value: value
            )
    }
}
