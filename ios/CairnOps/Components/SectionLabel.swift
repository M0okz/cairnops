import SwiftUI

/// Libelle de section en petites capitales espacees.
///
/// Il remplace le titre de dalle : la maquette « a nu » separe les sections par
/// un filet et ce libelle, sans contour ni fond.
struct SectionLabel<Trailing: View>: View {
    let title: String
    private let trailing: Trailing

    init(_ title: String, @ViewBuilder trailing: () -> Trailing = { EmptyView() }) {
        self.title = title
        self.trailing = trailing()
    }

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            Text(title.uppercased())
                .font(AppTheme.sectionLabelFont)
                .tracking(AppTheme.labelTracking)
                .foregroundStyle(AppTheme.inkFaint)

            Spacer(minLength: 8)

            trailing
                .font(AppTheme.metaFont)
                .foregroundStyle(AppTheme.inkMuted)
        }
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(.isHeader)
    }
}

extension SectionLabel where Trailing == Text {

    /// Variante courante : un compteur ou une fraicheur a droite du libelle.
    init(_ title: String, detail: String) {
        self.init(title) { Text(detail) }
    }
}
