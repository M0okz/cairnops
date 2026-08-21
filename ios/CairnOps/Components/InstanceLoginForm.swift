import SwiftUI

struct InstanceLoginForm: View {
    @Environment(AppModel.self) private var model

    let title: String
    let subtitle: String

    var body: some View {
        @Bindable var bindableModel = model

        Panel {
            VStack(alignment: .leading, spacing: 16) {
                VStack(alignment: .leading, spacing: 6) {
                    Text(title)
                        .font(AppTheme.sectionTitleFont)
                    if !subtitle.isEmpty {
                        Text(subtitle)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                }

                VStack(alignment: .leading, spacing: 12) {
                    TextField("https://cairnops.example.net", text: $bindableModel.serverURLText)
                        .textInputAutocapitalization(.never)
                        .textContentType(.URL)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                        .padding(.horizontal, 14)
                        .padding(.vertical, 12)
                        .background(
                            RoundedRectangle(cornerRadius: 12)
                                .fill(AppTheme.background)
                        )

                    TextField("Identifiant", text: $bindableModel.usernameText)
                        .textInputAutocapitalization(.never)
                        .textContentType(.username)
                        .autocorrectionDisabled()
                        .padding(.horizontal, 14)
                        .padding(.vertical, 12)
                        .background(
                            RoundedRectangle(cornerRadius: 12)
                                .fill(AppTheme.background)
                        )

                    SecureField("Mot de passe", text: $bindableModel.passwordText)
                        .textContentType(.password)
                        .padding(.horizontal, 14)
                        .padding(.vertical, 12)
                        .background(
                            RoundedRectangle(cornerRadius: 12)
                                .fill(AppTheme.background)
                        )
                }

                if let loginError = model.loginError {
                    Text(loginError)
                        .font(.footnote)
                        .foregroundStyle(AppTheme.critical)
                }

                AsyncButton {
                    await model.login()
                } label: {
                    Text("Ouvrir la session")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
            }
        }
    }
}
