import SwiftUI

struct TargetRow: View {
    let target: Target
    let health: AppSnapshot.TargetHealth
    let measures: TargetMeasures?

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: AppTheme.targetHealthSymbol(health))
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(AppTheme.targetHealthColor(health))
                .frame(width: 24, height: 24)
                .background(
                    Circle()
                        .fill(AppTheme.targetHealthColor(health).opacity(0.12))
                )

            VStack(alignment: .leading, spacing: 8) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(target.name)
                        .font(AppTheme.rowTitleFont)
                    if let summary = target.sourceOriginSummary ?? target.displayDescription {
                        Text(summary)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                            .lineLimit(2)
                    }
                }

                // `ViewThatFits` construisait et mesurait les deux dispositions
                // a chaque rendu de chaque ligne. Un `FlowLayout` natif via
                // `HStack` a retour automatique n'existe pas ici, mais ces trois
                // etiquettes tiennent sur une ligne avec `minimumScaleFactor`,
                // et le repli vertical est gere par le retour a la ligne du
                // conteneur.
                HStack(spacing: 10) {
                    metaLabel("sensor.tag.radiowaves.forward", sourceCountLabel)
                    if let availability = measures?.last24Hours?.availabilityDisplayValue {
                        metaLabel("chart.xyaxis.line", availability)
                    }
                    metaLabel("clock", observedAtLabel)
                }
                .fixedSize(horizontal: false, vertical: true)
            }

            Spacer(minLength: 12)

            VStack(alignment: .trailing, spacing: 10) {
                StatusPill(
                    text: AppTheme.targetHealthLabel(health),
                    color: AppTheme.targetHealthColor(health),
                    systemImage: AppTheme.targetHealthSymbol(health)
                )

                Image(systemName: "chevron.right")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.tertiary)
            }
        }
        .padding(12)
        .background(
            RoundedRectangle(cornerRadius: 16)
                .fill(AppTheme.subpanel)
        )
    }

    private var sourceCountLabel: String {
        target.totalSourceCount == 1 ? "1 source" : "\(target.totalSourceCount) sources"
    }

    private var observedAtLabel: String {
        TimestampParser.relativeString(from: measures?.latestObservedAt)
    }

    private func metaLabel(_ systemImage: String, _ text: String) -> some View {
        Label(text, systemImage: systemImage)
            .font(.footnote)
            .foregroundStyle(.secondary)
            .lineLimit(1)
            .minimumScaleFactor(0.85)
    }
}
