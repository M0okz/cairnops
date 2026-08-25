import SwiftUI

/// Selecteur de portee, pose sur une piste arrondie.
///
/// Il remplace le soulignement cuivre : celui-ci ne se distinguait pas d'un
/// titre de section et supportait mal d'etre empile avec un second filtre. La
/// pastille selectionnee reprend le traitement de la route courante dans la
/// barre d'onglets, si bien que « ce qui est actif » se lit partout pareil.
struct SegmentedBubble<Value: Hashable>: View {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Namespace private var selectionNamespace

    struct Item: Identifiable {
        let value: Value
        let title: String
        let count: Int?

        var id: Value { value }

        init(_ value: Value, _ title: String, count: Int? = nil) {
            self.value = value
            self.title = title
            self.count = count
        }
    }

    @Binding var selection: Value
    let items: [Item]

    /// En ligne, le selecteur partage sa rangee avec un intitule et se limite a
    /// la largeur de ses libelles.
    var isCompact = false

    var body: some View {
        HStack(spacing: 4) {
            ForEach(items) { item in
                Button {
                    selection = item.value
                } label: {
                    label(for: item)
                }
                .buttonStyle(.plain)
                .accessibilityAddTraits(item.value == selection ? [.isSelected, .isButton] : .isButton)
            }
        }
        .padding(4)
        .background(
            Capsule(style: .continuous)
                .fill(AppTheme.surface)
        )
        .animation(reduceMotion ? nil : .snappy(duration: 0.28), value: selection)
    }

    private func label(for item: Item) -> some View {
        let isSelected = item.value == selection

        return HStack(spacing: 5) {
            Text(AppLanguage.localized(item.title))
            if let count = item.count {
                Text("\(count)")
                    .monospacedDigit()
                    .opacity(isSelected ? 1 : 0.75)
            }
        }
        .font(isCompact ? .caption.weight(.semibold) : .subheadline.weight(.semibold))
        .foregroundStyle(isSelected ? AppTheme.accent : AppTheme.inkMuted)
        .lineLimit(1)
        .minimumScaleFactor(0.85)
        .padding(.vertical, isCompact ? 5 : 7)
        .padding(.horizontal, isCompact ? 10 : 12)
        .frame(maxWidth: isCompact ? nil : .infinity)
        .background {
            if isSelected {
                Capsule(style: .continuous)
                    .fill(AppTheme.accentSolid.opacity(0.18))
                    // La pastille glisse d'un segment a l'autre plutot que de
                    // disparaitre puis reapparaitre ailleurs.
                    .matchedGeometryEffect(id: "selection", in: selectionNamespace)
            }
        }
        .contentShape(.rect)
    }
}

/// Bouton de filtre secondaire, pose a cote de la recherche.
///
/// Le second filtre occupait sa propre rangee de texte nu sans jamais se
/// designer comme un filtre. Replie dans un menu, il libere la page et signale
/// par sa pastille cuivre qu'un filtre est actif.
struct FilterMenu<Content: View>: View {
    let isActive: Bool
    private let content: Content

    init(isActive: Bool, @ViewBuilder content: () -> Content) {
        self.isActive = isActive
        self.content = content()
    }

    var body: some View {
        Menu {
            content
        } label: {
            Image(systemName: "line.3.horizontal.decrease")
                .font(.system(size: 16, weight: .semibold))
                .foregroundStyle(isActive ? AppTheme.accent : AppTheme.inkMuted)
                .frame(width: 44, height: 44)
                .background(
                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                        .fill(isActive ? AppTheme.accentSolid.opacity(0.18) : AppTheme.surface)
                )
                .contentShape(.rect)
        }
        .accessibilityLabel(isActive ? "Filtres, actifs" : "Filtres")
    }
}
