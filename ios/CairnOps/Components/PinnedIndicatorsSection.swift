import SwiftUI

/// Indicateurs epingles, remontes sur la Vue d'ensemble.
///
/// Ils sont limites a quatre : au-dela, la section concurrencerait les
/// Incidents qui, eux, demandent une action.
struct PinnedIndicatorsSection: View {
    let projections: [TargetIndicators]
    let targetName: (String) -> String

    private var rows: [(indicator: ContextIndicator, projection: TargetIndicators)] {
        projections
            .flatMap { projection in
                projection.indicators.map { ($0, projection) }
            }
            .filter(\.0.pinned)
            .sorted { ($0.0.pinPosition ?? 99) < ($1.0.pinPosition ?? 99) }
            .prefix(4)
            .map { $0 }
    }

    var body: some View {
        if !rows.isEmpty {
            VStack(alignment: .leading, spacing: 0) {
                SectionLabel("Indicateurs épinglés", detail: "\(rows.count)/4")
                    .padding(.bottom, 4)

                ForEach(rows, id: \.indicator.id) { row in
                    NavigationLink {
                        TargetDetailView(targetID: row.indicator.targetID)
                    } label: {
                        pinnedRow(row)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.top, 16)
            .hairlineTop()
        }
    }

    private func pinnedRow(
        _ row: (indicator: ContextIndicator, projection: TargetIndicators)
    ) -> some View {
        HStack(alignment: .center, spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                Text(row.indicator.displayLabel)
                    .font(.subheadline.weight(.semibold))
                    .tracking(-0.15)
                    .foregroundStyle(AppTheme.ink)
                    .lineLimit(1)

                Text(targetName(row.indicator.targetID))
                    .font(.caption2)
                    .foregroundStyle(AppTheme.inkMuted)
                    .lineLimit(1)
            }

            Spacer(minLength: 8)

            // Cadrage automatique : borner a 0-100 ecraserait la variation
            // d'un indicateur stable en une ligne plate.
            Sparkline(
                values: (row.projection.series?[row.indicator.id] ?? []).map(\.value),
                tone: AppTheme.control,
                height: 22,
                showsArea: false
            )
            .frame(width: 64)

            Text(row.indicator.displayValue)
                .font(.footnote.weight(.bold))
                .monospacedDigit()
                .foregroundStyle(row.indicator.lastError == nil ? AppTheme.ink : AppTheme.warningInk)
                .frame(minWidth: 52, alignment: .trailing)
        }
        .padding(.vertical, 12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .hairlineTop()
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
    }
}
