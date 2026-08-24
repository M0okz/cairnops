import SwiftUI

/// Pastille d'etat avec son halo.
///
/// Le halo remplace l'ancienne pastille pleine a fond colore : il signale
/// l'etat sans poser un second bloc dans la ligne.
struct StatusDot: View {
    @Environment(\.colorScheme) private var colorScheme

    let tone: Color
    var size = 8.0
    var haloWidth = 4.0

    var body: some View {
        Circle()
            .fill(tone)
            .frame(width: size, height: size)
            .background(
                Circle()
                    .fill(tone.opacity(colorScheme == .dark ? 0.18 : 0.12))
                    .padding(-haloWidth)
            )
            // Le halo deborde du cadre : sans cette reserve il rognerait sur le
            // texte voisin au lieu de se poser par-dessus le fond.
            .frame(width: size + haloWidth, height: size + haloWidth)
            .accessibilityHidden(true)
    }
}
