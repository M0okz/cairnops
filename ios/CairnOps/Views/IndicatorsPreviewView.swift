import SwiftUI

struct IndicatorsPreviewView: View {
#if DEBUG
    private let targetProjection: TargetIndicators
    private let incidentProjection: IncidentIndicators

    init() {
        let now = Date.now
        let observedAt = now.ISO8601Format()
        let targetID = "11111111-1111-4111-8111-111111111111"
        let cpuID = "22222222-2222-4222-8222-222222222222"
        let networkID = "33333333-3333-4333-8333-333333333333"
        let connectorID = "44444444-4444-4444-8444-444444444444"
        let bindingID = "55555555-5555-4555-8555-555555555555"
        let cpu = ContextIndicator(
            id: cpuID,
            connectorID: connectorID,
            bindingID: bindingID,
            targetID: targetID,
            semanticKey: "cpu.utilization",
            label: "Utilisation CPU",
            externalID: "item-42",
            dimension: nil,
            unit: .percent,
            enabled: true,
            lastValue: 37.5,
            lastObservedAt: observedAt,
            lastError: nil,
            pinned: true,
            pinPosition: 0
        )
        let network = ContextIndicator(
            id: networkID,
            connectorID: connectorID,
            bindingID: bindingID,
            targetID: targetID,
            semanticKey: "network.in",
            label: "Réseau entrant",
            externalID: "item-43",
            dimension: "eth0",
            unit: .bytesPerSecond,
            enabled: true,
            lastValue: 1_420_000,
            lastObservedAt: observedAt,
            lastError: nil,
            pinned: false,
            pinPosition: nil
        )
        let cpuPoints = Self.points(values: [31, 34, 39, 35, 42, 37.5], endingAt: now)
        let networkPoints = Self.points(values: [620_000, 910_000, 840_000, 1_260_000, 1_080_000, 1_420_000], endingAt: now)
        targetProjection = TargetIndicators(
            targetID: targetID,
            generatedAt: observedAt,
            indicators: [cpu, network],
            series: [cpuID: cpuPoints, networkID: networkPoints]
        )
        incidentProjection = IncidentIndicators(
            incidentID: "66666666-6666-4666-8666-666666666666",
            targetID: targetID,
            openedAt: now.addingTimeInterval(-17 * 60).ISO8601Format(),
            snapshots: [IndicatorSnapshot(
                indicatorID: cpuID,
                semanticKey: "cpu.utilization",
                label: "Utilisation CPU",
                unit: .percent,
                value: 42,
                observedAt: now.addingTimeInterval(-17 * 60).ISO8601Format()
            )],
            indicators: [cpu],
            series: [cpuID: cpuPoints],
            disclaimer: "Corrélation temporelle uniquement — ces Indicateurs ne prouvent pas la cause de l’Incident."
        )
    }

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                PinnedIndicatorsSection(
                    projections: [targetProjection],
                    targetName: { _ in "API Homelab" }
                )
                TargetIndicatorsPanel(
                    targetID: targetProjection.targetID,
                    previewProjection: targetProjection
                )
                IncidentIndicatorsPanel(
                    incidentID: incidentProjection.incidentID,
                    previewProjection: incidentProjection
                )
            }
            .padding(AppTheme.screenPadding)
        }
        .navigationTitle("Indicateurs")
        .background(AppTheme.background)
        .environment(AppModel())
    }

    private static func points(values: [Double], endingAt: Date) -> [IndicatorPoint] {
        values.enumerated().map { index, value in
            IndicatorPoint(
                at: endingAt.addingTimeInterval(Double(index - values.count + 1) * 3600).ISO8601Format(),
                value: value,
                minimum: nil,
                maximum: nil,
                samples: 1
            )
        }
    }
#else
    var body: some View { EmptyView() }
#endif
}
