import SwiftUI

struct LoginView: View {
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                VStack(alignment: .leading, spacing: 10) {
                    Text("CairnOps sur iPhone")
                        .font(.title.weight(.semibold))
                    Text("Associez cet appareil depuis votre session Web, sans saisir votre mot de passe dans l’application.")
                        .font(.body)
                        .foregroundStyle(.secondary)
                }

                DevicePairingPanel()
            }
            .padding(AppTheme.screenPadding)
            .padding(.top, 24)
        }
        .background(AppBackdrop())
    }
}

#Preview {
    LoginView()
        .environment(AppModel())
}
