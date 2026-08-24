import SwiftUI

/// Filtres d'ecran soulignes de cuivre.
///
/// Ils remplacent le `Picker` segmente : la maquette veut un filet de deux
/// points sous l'onglet courant, pas une capsule pleine.
struct UnderlineTabs<Value: Hashable>: View {

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

    /// En ligne, les onglets partagent leur rangee avec un intitule : ils ne
    /// defilent pas et ne debordent pas de la gouttiere, sinon le dernier
    /// onglet se ferait rogner par le bord de l'ecran.
    var isInline = false

    var body: some View {
        if isInline {
            HStack(spacing: 14) {
                buttons
            }
        } else {
            ScrollView(.horizontal) {
                HStack(spacing: 22) {
                    buttons
                }
                .padding(.horizontal, AppTheme.screenPadding)
            }
            .scrollIndicators(.hidden)
            // Le contenu porte sa propre gouttiere : sans cela, le defilement
            // horizontal se couperait net au bord de l'ecran.
            .padding(.horizontal, -AppTheme.screenPadding)
        }
    }

    @ViewBuilder
    private var buttons: some View {
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

    private func label(for item: Item) -> some View {
        let isSelected = item.value == selection

        return HStack(spacing: 6) {
            Text(item.title)
            if let count = item.count {
                Text("\(count)")
                    .monospacedDigit()
            }
        }
        .font(isSelected ? AppTheme.filterFont : AppTheme.filterIdleFont)
        .foregroundStyle(isSelected ? AppTheme.ink : AppTheme.inkMuted)
        .padding(.bottom, 8)
        .overlay(alignment: .bottom) {
            Rectangle()
                .fill(isSelected ? AppTheme.accentSolid : .clear)
                .frame(height: 2)
        }
        .contentShape(.rect)
    }
}
