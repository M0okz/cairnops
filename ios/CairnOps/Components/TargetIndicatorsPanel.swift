import SwiftUI

struct TargetIndicatorsPanel: View {
    @Environment(AppModel.self) private var model

    let targetID: Target.ID
    var previewProjection: TargetIndicators?

    @State private var window = "24h"
    @State private var projection: TargetIndicators?
    @State private var errorMessage: String?
    @State private var isLoading = false

    var body: some View {
        let displayedProjection = previewProjection ?? projection

        Panel {
            VStack(alignment: .leading, spacing: 14) {
                ViewThatFits(in: .horizontal) {
                    HStack(alignment: .firstTextBaseline) {
                        heading
                        Spacer()
                        windowPicker
                    }
                    VStack(alignment: .leading, spacing: 12) {
                        heading
                        windowPicker
                    }
                }

                if isLoading, displayedProjection == nil {
                    ProgressView("Lecture des indicateurs…")
                        .frame(maxWidth: .infinity, minHeight: 120)
                } else if let errorMessage, displayedProjection == nil {
                    ContentUnavailableView {
                        Label("Indicateurs indisponibles", systemImage: "chart.xyaxis.line")
                    } description: {
                        Text(errorMessage)
                    } actions: {
                        Button("Réessayer") { Task { await load() } }
                    }
                } else if let displayedProjection, displayedProjection.indicators.isEmpty {
                    ContentUnavailableView(
                        "Aucun indicateur sélectionné",
                        systemImage: "slider.horizontal.3",
                        description: Text("La sélection se configure depuis la fiche du Connecteur sur le Web.")
                    )
                } else if let displayedProjection {
                    ForEach(displayedProjection.indicators) { indicator in
                        VStack(alignment: .leading, spacing: 8) {
                            HStack(alignment: .firstTextBaseline) {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(indicator.displayLabel)
                                        .font(.headline)
                                    Text(TimestampParser.relativeString(from: indicator.lastObservedAt))
                                        .font(.caption)
                                        .foregroundStyle(indicator.lastError == nil ? .secondary : AppTheme.warning)
                                }
                                Spacer()
                                Text(indicator.displayValue)
                                    .font(.headline.monospacedDigit())
                            }

                            IndicatorChart(
                                points: displayedProjection.series?[indicator.id] ?? [],
                                unit: indicator.unit
                            )

                            if let lastError = indicator.lastError, !lastError.isEmpty {
                                Label(lastError, systemImage: "exclamationmark.triangle.fill")
                                    .font(.caption)
                                    .foregroundStyle(AppTheme.warning)
                            }
                        }
                        .accessibilityElement(children: .contain)

                        if indicator.id != displayedProjection.indicators.last?.id {
                            Divider()
                        }
                    }
                }

                Text("Contexte uniquement · les alertes restent sous l’autorité du produit d’origine. Les relevés détaillés expirent après 24 h et les agrégats horaires après 7 j.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .task(id: "\(targetID)#\(window)") {
            guard previewProjection == nil else { return }
            await load()
        }
    }

    private var heading: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text("Indicateurs")
                .font(AppTheme.sectionTitleFont)
            Text("Métriques contextuelles importées depuis les Connecteurs")
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    private var windowPicker: some View {
        Picker("Fenêtre", selection: $window) {
            Text("24 h").tag("24h")
            Text("7 j").tag("7d")
        }
        .pickerStyle(.segmented)
        .fixedSize()
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
