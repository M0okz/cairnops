import SwiftUI

struct LoginView: View {
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                VStack(alignment: .leading, spacing: 10) {
                    Text("CairnOps sur iPhone")
                        .font(.system(size: 28, weight: .semibold, design: .default))
                    Text("Une projection mobile native du parcours incident V1 : vue d'ensemble, cibles, incidents, sante et actions immediates.")
                        .font(.body)
                        .foregroundStyle(.secondary)
                }

                InstanceLoginForm(
                    title: "Connexion directe a l'instance",
                    subtitle: "Le client mobile parle directement a votre instance CairnOps et reste en lecture seule hors ligne."
                )
            }
            .padding(AppTheme.screenPadding)
            .padding(.top, 24)
        }
        .background(AppBackdrop())
    }
}
