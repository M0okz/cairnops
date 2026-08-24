import SwiftUI

/// Repartition de la flotte en une seule barre proportionnelle.
///
/// Elle dit d'un coup d'oeil ce que trois compteurs separes obligeaient a
/// recomposer mentalement : la part de Cibles saines, degradees et injoignables.
struct FleetBar: View {

    struct Segment: Identifiable {
        let id: String
        let label: String
        let count: Int
        let tone: Color
    }

    let segments: [Segment]

    private var total: Int {
        segments.reduce(0) { $0 + $1.count }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            bar
            legend
        }
    }

    private var bar: some View {
        GeometryReader { proxy in
            let gap = 2.0
            let visible = segments.filter { $0.count > 0 }
            let available = max(proxy.size.width - gap * Double(max(visible.count - 1, 0)), 0)

            HStack(spacing: gap) {
                ForEach(visible) { segment in
                    Rectangle()
                        .fill(segment.tone)
                        .frame(width: available * Double(segment.count) / Double(max(total, 1)))
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .frame(height: 6)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Répartition de la flotte")
        .accessibilityValue(
            segments
                .map { "\($0.count) \($0.label)" }
                .joined(separator: ", ")
        )
    }

    private var legend: some View {
        // Les trois legendes ne tiennent pas toujours sur une ligne aux grandes
        // tailles de texte : elles passent alors a la ligne au lieu d'etre
        // comprimees.
        ViewThatFits(in: .horizontal) {
            HStack(spacing: 16) {
                ForEach(segments) { entry($0) }
            }

            VStack(alignment: .leading, spacing: 6) {
                ForEach(segments) { entry($0) }
            }
        }
        .accessibilityHidden(true)
    }

    private func entry(_ segment: FleetBar.Segment) -> some View {
        HStack(spacing: 6) {
            Circle()
                .fill(segment.tone)
                .frame(width: 7, height: 7)

            Text(segment.label)
                .font(AppTheme.metaFont)
                .foregroundStyle(AppTheme.inkStrong)

            Text("\(segment.count)")
                .font(AppTheme.metaFont.weight(.bold))
                .monospacedDigit()
                .foregroundStyle(AppTheme.ink)
        }
    }
}
