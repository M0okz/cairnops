import SwiftUI

/// Pastille du bouton d'actualisation, partagee par les ecrans de liste.
///
/// Le meme empilement `Circle` + `strokeBorder` etait redefini a l'identique
/// dans quatre vues : le factoriser evite autant de sous-arbres distincts a
/// reconstruire.
struct RefreshGlyph: View {
    var body: some View {
        Image(systemName: "arrow.clockwise")
            .font(.title3.weight(.semibold))
            .frame(width: 46, height: 46)
            .background(
                Circle()
                    .fill(AppTheme.panel)
                    .overlay(
                        Circle()
                            .strokeBorder(AppTheme.line)
                    )
            )
    }
}
