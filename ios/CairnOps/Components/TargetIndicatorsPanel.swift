import SwiftUI

/// Indicateurs contextuels d'une Cible, importes depuis ses Connecteurs.
///
/// Chaque indicateur devient une dalle chiffree avec sa tendance, comme les
/// mesures natives : la Cible se lit d'un bloc, sans distinguer visuellement ce
/// que CairnOps mesure de ce qu'une Integration rapporte. L'origine reste dite
/// en toutes lettres sous la section.
struct TargetIndicatorsPanel: View {
    @Environment(AppModel.self) private var model

    let targetID: Target.ID
    var previewProjection: TargetIndicators?

    @State private var window = "24h"
    @State private var projection: TargetIndicators?
    @State private var errorMessage: String?
    @State private var isLoading = false

    var body: some View {
        let displayed = previewProjection ?? projection

        VStack(alignment: .leading, spacing: 0) {
            header

            if isLoading, displayed == nil {
                ProgressView()
                    .frame(maxWidth: .infinity, minHeight: 90)
            } else if let errorMessage, displayed == nil {
                message(errorMessage, tone: AppTheme.warningInk) {
                    Button("Réessayer") { Task { await load() } }
                        .font(AppTheme.fieldValueFont)
                        .foregroundStyle(AppTheme.accent)
                }
            } else if let displayed, displayed.indicators.isEmpty {
                message(
                    "Aucun indicateur sélectionné. La sélection se configure depuis la fiche du Connecteur sur le Web.",
                    tone: AppTheme.inkMuted
                )
            } else if let displayed {
                grid(displayed)

                Text("Contexte uniquement · les seuils et alertes restent sous l’autorité du produit d’origine.")
                    .font(.caption2)
                    .foregroundStyle(AppTheme.inkMuted)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 12)
            }
        }
        .padding(.top, 18)
        .hairlineTop()
        .task(id: "\(targetID)#\(window)") {
            guard previewProjection == nil else { return }
            await load()
        }
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Text("Indicateurs".uppercased())
                .font(AppTheme.sectionLabelFont)
                .tracking(AppTheme.labelTracking)
                .foregroundStyle(AppTheme.inkFaint)
                .accessibilityAddTraits(.isHeader)

            Spacer(minLength: 8)

            UnderlineTabs(
                selection: $window,
                items: [.init("24h", "24 h"), .init("7d", "7 j")]
            )
            .fixedSize(horizontal: true, vertical: false)
        }
        .padding(.bottom, 2)
    }

    private func grid(_ projection: TargetIndicators) -> some View {
        LazyVGrid(
            columns: [GridItem(.flexible(), spacing: 20, alignment: .topLeading),
                      GridItem(.flexible(), spacing: 20, alignment: .topLeading)],
            alignment: .leading,
            spacing: 0
        ) {
            ForEach(projection.indicators) { indicator in
                MetricCell(
                    label: indicator.displayLabel,
                    value: valueText(indicator),
                    unit: unitText(indicator),
                    delta: indicator.lastError == nil ? nil : "!",
                    tone: tone(for: indicator),
                    series: (projection.series?[indicator.id] ?? []).map(\.value),
                    lowerBound: indicator.unit == .percent ? 0 : nil,
                    upperBound: indicator.unit == .percent ? 100 : nil
                )
            }
        }
    }

    /// La dalle separe le nombre de son unite : `IndicatorUnit.format` les colle,
    /// on ne garde donc que la partie chiffree quand l'unite est connue.
    private func valueText(_ indicator: ContextIndicator) -> String {
        guard let value = indicator.lastValue else {
            return "—"
        }

        switch indicator.unit {
        case .percent, .milliseconds, .days, .count:
            return value.formatted(.number.precision(.fractionLength(0)))
        case .bytesPerSecond, .boolean, .seconds:
            return indicator.unit.format(value)
        }
    }

    private func unitText(_ indicator: ContextIndicator) -> String? {
        guard indicator.lastValue != nil else {
            return nil
        }

        switch indicator.unit {
        case .percent:
            return "%"
        case .milliseconds:
            return "ms"
        case .days:
            return "j"
        case .count:
            return "unités"
        case .bytesPerSecond, .boolean, .seconds:
            return nil
        }
    }

    private func tone(for indicator: ContextIndicator) -> Color {
        guard indicator.lastError == nil else {
            return AppTheme.warningInk
        }
        guard indicator.unit == .percent, let value = indicator.lastValue else {
            return AppTheme.ink
        }
        if value >= 90 {
            return AppTheme.criticalInk
        }
        if value >= 75 {
            return AppTheme.warningInk
        }
        return AppTheme.ink
    }

    @ViewBuilder
    private func message<Action: View>(
        _ text: String,
        tone: Color,
        @ViewBuilder action: () -> Action = { EmptyView() }
    ) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(text)
                .font(.subheadline)
                .foregroundStyle(tone)
                .fixedSize(horizontal: false, vertical: true)
            action()
        }
        .padding(.vertical, 14)
    }

    private func load() async {
        isLoading = true
        errorMessage = nil
        do {
            projection = try await model.fetchTargetIndicators(targetID: targetID, window: window)
        } catch is CancellationError {
            return
        } catch {
            errorMessage = error.localizedDescription
        }
        isLoading = false
    }
}
