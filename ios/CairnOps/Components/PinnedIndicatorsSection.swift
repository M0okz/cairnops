import SwiftUI

struct PinnedIndicatorsSection: View {
    let projections: [TargetIndicators]
    let targetName: (String) -> String

    private var rows: [(indicator: ContextIndicator, projection: TargetIndicators)] {
        projections
            .flatMap { projection in
                projection.indicators.map { ($0, projection) }
            }
            .filter { $0.0.pinned }
            .sorted { ($0.0.pinPosition ?? 99) < ($1.0.pinPosition ?? 99) }
            .prefix(4)
            .map { $0 }
    }

    var body: some View {
        if !rows.isEmpty {
            VStack(alignment: .leading, spacing: 12) {
                HStack(alignment: .firstTextBaseline) {
                    VStack(alignment: .leading, spacing: 3) {
                        Text("Indicateurs épinglés")
                            .font(AppTheme.sectionTitleFont)
                        Text("Métriques contextuelles importées depuis les Connecteurs")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    Text("\(rows.count)/4")
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                }

                LazyVGrid(columns: [GridItem(.adaptive(minimum: 250), spacing: 12)], spacing: 12) {
                    ForEach(rows, id: \.indicator.id) { row in
                        NavigationLink {
                            TargetDetailView(targetID: row.indicator.targetID)
                        } label: {
                            Panel {
                                VStack(alignment: .leading, spacing: 8) {
                                    HStack(alignment: .firstTextBaseline) {
                                        VStack(alignment: .leading, spacing: 2) {
                                            Text(row.indicator.displayLabel)
                                                .font(.headline)
                                                .lineLimit(1)
                                            Text(targetName(row.indicator.targetID))
                                                .font(.caption)
                                                .foregroundStyle(.secondary)
                                        }
                                        Spacer()
                                        Text(row.indicator.displayValue)
                                            .font(.headline.monospacedDigit())
                                    }
                                    IndicatorChart(
                                        points: row.projection.series?[row.indicator.id] ?? [],
                                        unit: row.indicator.unit,
                                        compact: true
                                    )
                                    Text(TimestampParser.relativeString(from: row.indicator.lastObservedAt))
                                        .font(.caption)
                                        .foregroundStyle(row.indicator.lastError == nil ? .secondary : AppTheme.warning)
                                }
                            }
                        }
                        .buttonStyle(.plain)
                    }
                }

                Text("Contexte uniquement · les seuils et alertes restent sous l’autorité du produit d’origine.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
    }
}
