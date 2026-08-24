import SwiftUI

/// Ecran d'accueil tant qu'aucun appareil n'est associe.
struct LoginView: View {
    var body: some View {
        BareScreen(bottomInset: 40) {
            VStack(alignment: .leading, spacing: 10) {
                Text("CairnOps sur iPhone")
                    .font(.largeTitle.weight(.heavy))
                    .tracking(-1.0)
                    .foregroundStyle(AppTheme.ink)
                    .accessibilityAddTraits(.isHeader)

                Text("Associez cet appareil depuis votre session Web, sans saisir votre mot de passe dans l’application.")
                    .font(.body)
                    .foregroundStyle(AppTheme.inkMuted)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .padding(.top, 24)
            .padding(.bottom, 28)

            DevicePairingPanel()
        }
    }
}

#Preview {
    LoginView()
        .environment(AppModel())
}
