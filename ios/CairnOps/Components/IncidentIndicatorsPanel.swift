import SwiftUI

/// Contexte mesure autour de l'ouverture d'un Incident.
///
/// La fenetre couvre deux heures avant et apres : elle situe l'Incident sans
/// pretendre l'expliquer, et les valeurs relevees a l'ouverture sont donnees
/// separement de la courbe.
struct IncidentIndicatorsPanel: View {
    @Environment(AppModel.self) private var model

    let incidentID: Incident.ID
    var previewProjection: IncidentIndicators?

    @State private var projection: IncidentIndicators?
    @State private var errorMessage: String?
    @State private var isLoading = false

    var body: some View {
        let displayed = previewProjection ?? projection

        VStack(alignment: .leading, spacing: 0) {
            if isLoading, displayed == nil {
                sectionLabel
                ProgressView()
                    .frame(maxWidth: .infinity, minHeight: 80)
            } else if let errorMessage, displayed == nil {
                sectionLabel
                VStack(alignment: .leading, spacing: 10) {
                    Text(errorMessage)
                        .font(.subheadline)
                        .foregroundStyle(AppTheme.warningInk)
                        .fixedSize(horizontal: false, vertical: true)

                    Button("Réessayer") { Task { await load() } }
                        .font(AppTheme.fieldValueFont)
                        .foregroundStyle(AppTheme.control)
                }
                .padding(.vertical, 14)
            } else if let displayed, !displayed.indicators.isEmpty || !displayed.snapshots.isEmpty {
                sectionLabel
                content(displayed)
            }
        }
        .task(id: incidentID) {
            guard previewProjection == nil else { return }
            await load()
        }
    }

    /// La section disparait entierement lorsqu'aucun Connecteur ne fournit de
    /// contexte : une dalle vide n'apprendrait rien.
    private var sectionLabel: some View {
        SectionLabel("Contexte", detail: "± 2 h autour de l’ouverture")
            .padding(.top, 18)
            .padding(.bottom, 2)
            .hairlineTop()
    }

    private func content(_ projection: IncidentIndicators) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            if !projection.snapshots.isEmpty {
                LazyVGrid(
                    columns: [GridItem(.flexible(), spacing: 20, alignment: .topLeading),
                              GridItem(.flexible(), spacing: 20, alignment: .topLeading)],
                    alignment: .leading,
                    spacing: 0
                ) {
                    ForEach(projection.snapshots) { snapshot in
                        MetricCell(
                            label: snapshot.label,
                            value: snapshot.unit.format(snapshot.value),
                            unit: "à l’ouverture",
                            tone: AppTheme.ink,
                            series: projection.series[snapshot.id]?.map(\.value) ?? []
                        )
                    }
                }
            }

            ForEach(projection.indicators) { indicator in
                MetaRow(
                    title: indicator.displayLabel,
                    subtitle: TimestampParser.relativeString(from: indicator.lastObservedAt),
                    state: indicator.displayValue,
                    tone: indicator.lastError == nil ? AppTheme.info : AppTheme.warning,
                    stateInk: AppTheme.ink
                )
            }

            Text(projection.disclaimer)
                .font(.caption2)
                .foregroundStyle(AppTheme.inkMuted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, 12)
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
