import SwiftUI

/// Fond decoratif de l'application.
///
/// La version initiale empilait deux `Circle` flous. `blur(radius:)` force un
/// rendu hors-ecran recalcule a chaque image, y compris pendant le defilement,
/// et ce fond est pose derriere chaque ecran. Deux degrades radials donnent le
/// meme halo pour un cout negligeable, sans passe de compositing.
struct AppBackdrop: View {
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        ZStack {
            AppTheme.background

            RadialGradient(
                colors: [
                    AppTheme.accent.opacity(colorScheme == .dark ? 0.16 : 0.08),
                    AppTheme.accent.opacity(0),
                ],
                center: .init(x: 0.82, y: 0.12),
                startRadius: 0,
                endRadius: 260
            )

            RadialGradient(
                colors: [
                    AppTheme.info.opacity(colorScheme == .dark ? 0.12 : 0.05),
                    AppTheme.info.opacity(0),
                ],
                center: .init(x: 0.12, y: 0.78),
                startRadius: 0,
                endRadius: 230
            )
        }
        .ignoresSafeArea()
        .allowsHitTesting(false)
    }
}
