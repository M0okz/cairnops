import SwiftUI

struct IncidentIndicatorsPanel: View {
    @Environment(AppModel.self) private var model

    let incidentID: Incident.ID
    var previewProjection: IncidentIndicators?

    @State private var projection: IncidentIndicators?
    @State private var errorMessage: String?
    @State private var isLoading = false

    var body: some View {
        let displayedProjection = previewProjection ?? projection

        Panel {
            VStack(alignment: .leading, spacing: 14) {
                VStack(alignment: .leading, spacing: 3) {
                    Text("Contexte autour de l’incident")
                        .font(AppTheme.sectionTitleFont)
                    Text("Fenêtre de deux heures avant et après l’ouverture")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }

                if isLoading, displayedProjection == nil {
                    ProgressView("Lecture du contexte…")
                        .frame(maxWidth: .infinity, minHeight: 100)
                } else if let errorMessage, displayedProjection == nil {
                    ContentUnavailableView {
                        Label("Contexte indisponible", systemImage: "chart.xyaxis.line")
                    } description: {
                        Text(errorMessage)
                    } actions: {
                        Button("Réessayer") { Task { await load() } }
                    }
                } else if let displayedProjection, displayedProjection.indicators.isEmpty, displayedProjection.snapshots.isEmpty {
                    ContentUnavailableView(
                        "Aucun indicateur à cette date",
                        systemImage: "clock.arrow.circlepath",
                        description: Text("Les données expirent naturellement selon leur durée de conservation.")
                    )
                } else if let displayedProjection {
                    if !displayedProjection.snapshots.isEmpty {
                        MetricGrid {
                            ForEach(displayedProjection.snapshots) { snapshot in
                                MetricTile(
                                    title: snapshot.label,
                                    value: snapshot.unit.format(snapshot.value),
                                    subtitle: "À l’ouverture",
                                    tone: AppTheme.accent
                                )
                            }
                        }
                    }

                    ForEach(displayedProjection.indicators) { indicator in
                        VStack(alignment: .leading, spacing: 8) {
                            HStack {
                                Text(indicator.displayLabel)
                                    .font(.headline)
                                Spacer()
                                Text(indicator.displayValue)
                                    .font(.headline.monospacedDigit())
                            }
                            IndicatorChart(
                                points: displayedProjection.series[indicator.id] ?? [],
                                unit: indicator.unit
                            )
                        }

                        if indicator.id != displayedProjection.indicators.last?.id {
                            Divider()
                        }
                    }

                    Label(displayedProjection.disclaimer, systemImage: "info.circle.fill")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .task(id: incidentID) {
            guard previewProjection == nil else { return }
            await load()
        }
    }

    private func load() async {
        isLoading = true
        errorMessage = nil
        do {
            projection = try await model.fetchIncidentIndicators(incidentID: incidentID)
        } catch is CancellationError {
            return
        } catch {
            errorMessage = error.localizedDescription
        }
        isLoading = false
    }
}
