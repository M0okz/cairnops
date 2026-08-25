import SwiftUI

/// Champ de recherche pose dans le contenu.
///
/// Il porte sa propre surface arrondie : entre deux filets, il se confondait
/// avec les lignes qu'il sert justement a filtrer.
struct SearchField: View {
    @Binding var text: String
    let prompt: String

    var body: some View {
        HStack(spacing: 9) {
            Image(systemName: "magnifyingglass")
                .font(.system(size: 15, weight: .semibold))
                .foregroundStyle(AppTheme.inkMuted)

            TextField(text: $text) {
                Text(prompt)
                    .foregroundStyle(AppTheme.inkMuted)
            }
            .font(.subheadline)
            .foregroundStyle(AppTheme.ink)
            .textInputAutocapitalization(.never)
            .autocorrectionDisabled()
            .submitLabel(.search)

            if !text.isEmpty {
                Button {
                    text = ""
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .font(.system(size: 15))
                        .foregroundStyle(AppTheme.inkMuted)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Effacer la recherche")
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 11)
        // Le champ entier reste une cible tactile confortable.
        .frame(minHeight: 44)
        .background(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .fill(AppTheme.surface)
        )
    }
}
