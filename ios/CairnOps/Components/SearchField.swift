import SwiftUI

/// Champ de recherche pose dans le contenu.
///
/// La barre de navigation etant masquee, `searchable` n'a plus de support :
/// la maquette dessine de toute facon la recherche comme une ligne du contenu,
/// entre deux filets.
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
        .padding(.vertical, 13)
        // La ligne entiere reste une cible tactile confortable.
        .frame(minHeight: 44)
        .hairlineTop()
    }
}

/// Filtre secondaire en pastille, active ou non.
struct FilterChip: View {
    let title: String
    let isActive: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            HStack(spacing: 5) {
                Text(title)
                if isActive {
                    Image(systemName: "xmark")
                        .font(.system(size: 9, weight: .heavy))
                }
            }
            .font(AppTheme.metaFont.weight(isActive ? .bold : .regular))
            .foregroundStyle(isActive ? AppTheme.accent : AppTheme.inkMuted)
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(isActive ? [.isSelected, .isButton] : .isButton)
    }
}
