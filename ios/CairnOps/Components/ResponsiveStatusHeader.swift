import SwiftUI

/// Place un etat a droite tant que l'espace le permet, puis sous le contenu.
/// Le changement de disposition evite les collisions entre titres, pastilles
/// et tailles de texte d'accessibilite.
struct ResponsiveStatusHeader<Content: View>: View {
    let text: String
    let color: Color
    let systemImage: String?
    private let content: Content

    init(
        text: String,
        color: Color,
        systemImage: String? = nil,
        @ViewBuilder content: () -> Content
    ) {
        self.text = text
        self.color = color
        self.systemImage = systemImage
        self.content = content()
    }

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(alignment: .top, spacing: 12) {
                content
                Spacer(minLength: 12)
                status
            }

            VStack(alignment: .leading, spacing: 10) {
                content
                status
            }
        }
    }

    private var status: some View {
        StatusPill(text: text, color: color, systemImage: systemImage)
    }
}
