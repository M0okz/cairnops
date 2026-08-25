import SwiftUI

/// Barre d'actions flottante des ecrans de detail.
///
/// L'action principale occupe la largeur restante et porte le cuivre ;
/// l'action secondaire garde la largeur de son libelle.
struct GlassActionBar<Primary: View, Secondary: View>: View {
    private let primary: Primary
    private let secondary: Secondary

    init(
        @ViewBuilder primary: () -> Primary,
        @ViewBuilder secondary: () -> Secondary = { EmptyView() }
    ) {
        self.primary = primary()
        self.secondary = secondary()
    }

    var body: some View {
        HStack(spacing: 10) {
            primary
                .frame(maxWidth: .infinity)
            secondary
        }
        .padding(.horizontal, 16)
    }
}

/// Bouton en verre de la barre d'actions.
struct GlassActionLabel: View {
    let title: String
    let systemImage: String
    var isProminent = false

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: systemImage)
                .font(.system(size: 17, weight: .bold))
            Text(title)
                .font(isProminent ? .callout.weight(.bold) : .subheadline.weight(.semibold))
                .lineLimit(1)
                .minimumScaleFactor(0.85)
        }
        .foregroundStyle(isProminent ? AppTheme.control : AppTheme.inkStrong)
        .padding(.vertical, 14)
        .padding(.horizontal, 18)
        .frame(maxWidth: isProminent ? .infinity : nil, minHeight: 24)
        .background {
            RoundedRectangle(cornerRadius: AppTheme.actionCorner, style: .continuous)
                .fill(.ultraThinMaterial)
                .overlay {
                    RoundedRectangle(cornerRadius: AppTheme.actionCorner, style: .continuous)
                        .fill(isProminent ? AppTheme.controlProminentFill : AppTheme.glassFill)
                }
                .overlay {
                    RoundedRectangle(cornerRadius: AppTheme.actionCorner, style: .continuous)
                        .strokeBorder(AppTheme.glassStroke)
                }
                .shadow(color: AppTheme.glassShadow, radius: 16, x: 0, y: 12)
        }
        .contentShape(.rect(cornerRadius: AppTheme.actionCorner))
    }
}
