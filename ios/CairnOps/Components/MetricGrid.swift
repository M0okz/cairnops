import SwiftUI

/// Grille adaptee aux dalles de mesure.
///
/// Les anciens `HStack` imposaient jusqu'a trois colonnes sur un iPhone et
/// comprimaient les libelles. La grille conserve des cellules lisibles, passe
/// naturellement a davantage de colonnes sur iPad et a une seule colonne aux
/// tailles de texte d'accessibilite.
struct MetricGrid<Content: View>: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private let content: Content

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
    }

    var body: some View {
        LazyVGrid(columns: columns, alignment: .leading, spacing: 12) {
            content
        }
    }

    private var columns: [GridItem] {
        [
            GridItem(
                .adaptive(minimum: dynamicTypeSize.isAccessibilitySize ? 220 : 132),
                spacing: 12,
                alignment: .top
            ),
        ]
    }
}
