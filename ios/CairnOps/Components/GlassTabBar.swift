import SwiftUI

/// Barre d'onglets flottante en verre.
///
/// Elle remplace la barre systeme pour reprendre la forme de la maquette :
/// pastille arrondie posee au-dessus du contenu, qui defile dessous, et route
/// courante marquee par le cuivre plutot que par la teinte systeme.
struct GlassTabBar: View {
    @Binding var selection: AppTab

    var body: some View {
        HStack(spacing: 0) {
            ForEach(AppTab.allCases) { tab in
                Button {
                    selection = tab
                } label: {
                    item(tab)
                }
                .buttonStyle(.plain)
                .accessibilityLabel(tab.title)
                .accessibilityAddTraits(tab == selection ? [.isSelected, .isButton] : .isButton)
            }
        }
        .padding(.horizontal, 6)
        .frame(height: AppTheme.tabBarHeight)
        .background {
            RoundedRectangle(cornerRadius: AppTheme.barCorner, style: .continuous)
                .fill(.ultraThinMaterial)
                .overlay {
                    RoundedRectangle(cornerRadius: AppTheme.barCorner, style: .continuous)
                        .fill(AppTheme.glassFill)
                }
                .overlay {
                    RoundedRectangle(cornerRadius: AppTheme.barCorner, style: .continuous)
                        .strokeBorder(AppTheme.glassStroke)
                }
                .shadow(color: AppTheme.glassShadow, radius: 20, x: 0, y: 14)
        }
        .padding(.horizontal, AppTheme.barInset)
    }

    private func item(_ tab: AppTab) -> some View {
        let isSelected = tab == selection

        return VStack(spacing: 3) {
            Image(systemName: tab.symbolName)
                .font(.system(size: 19, weight: .medium))
            Text(tab.title)
                .font(AppTheme.tabLabelFont)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
        }
        .foregroundStyle(isSelected ? AppTheme.accent : AppTheme.inkMuted)
        .frame(maxWidth: .infinity)
        .padding(.vertical, 8)
        .background {
            if isSelected {
                RoundedRectangle(cornerRadius: 20, style: .continuous)
                    .fill(AppTheme.accentSolid.opacity(0.16))
                    .padding(.horizontal, 4)
            }
        }
        .contentShape(.rect)
    }
}
