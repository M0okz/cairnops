import SwiftUI

/// Titre d'ecran de la maquette : un titre moyen qui partage sa ligne.
///
/// Le grand titre systeme a ete abandonne : il occupait un tiers de la hauteur
/// utile sans rien apprendre. Ici la ligne porte aussi le compteur et les
/// commandes, et les filtres suivent immediatement dessous.
struct PageTitle<Trailing: View>: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    let title: String
    var detail: String?
    private let trailing: Trailing

    init(
        _ title: String,
        detail: String? = nil,
        @ViewBuilder trailing: () -> Trailing = { EmptyView() }
    ) {
        self.title = title
        self.detail = detail
        self.trailing = trailing()
    }

    var body: some View {
        // Aux tailles d'accessibilite, partager la ligne coupe le titre en
        // plein mot : le compteur et les commandes passent alors dessous.
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: 8) {
                titleText
                HStack(alignment: .firstTextBaseline, spacing: 10) {
                    detailText
                    Spacer(minLength: 8)
                    trailing
                }
            }
        } else {
            HStack(alignment: .firstTextBaseline, spacing: 10) {
                titleText
                detailText
                Spacer(minLength: 8)
                trailing
            }
        }
    }

    private var titleText: some View {
        Text(title)
            .font(AppTheme.pageTitleFont)
            .tracking(AppTheme.titleTracking)
            .foregroundStyle(AppTheme.ink)
            .fixedSize(horizontal: false, vertical: true)
            .accessibilityAddTraits(.isHeader)
    }

    @ViewBuilder
    private var detailText: some View {
        if let detail, !detail.isEmpty {
            Text(detail)
                .font(AppTheme.metaFont)
                .foregroundStyle(AppTheme.inkMuted)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
        }
    }
}

/// Pastille d'identite : initiale de l'utilisateur sur fond cuivre.
struct AvatarBadge: View {
    let name: String
    var size = 34.0

    private var initial: String {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.first.map { String($0).uppercased() } ?? "?"
    }

    var body: some View {
        Text(initial)
            .font(.system(size: size * 0.41, weight: .heavy))
            .foregroundStyle(Color(hex: 0xF7ECDF))
            .frame(width: size, height: size)
            .background(Circle().fill(Color(hex: 0xA9631F)))
            .accessibilityHidden(true)
    }
}
