import SwiftUI

struct TargetDetailView: View {
    @Environment(AppModel.self) private var model

    let targetID: Target.ID

    var body: some View {
        Group {
            if let target = model.target(withID: targetID) {
                ScrollView {
                    VStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                // Un seul calcul de sante au lieu de plusieurs
                                // appels identiques pendant le rendu.
                                let health = model.snapshot.health(for: target)
                                ResponsiveStatusHeader(
                                    text: AppTheme.targetHealthLabel(health),
                                    color: AppTheme.targetHealthColor(health),
                                    systemImage: AppTheme.targetHealthSymbol(health)
                                ) {
                                    VStack(alignment: .leading, spacing: 4) {
                                        Text(target.name)
                                            .font(AppTheme.cardTitleFont)
                                        if let description = target.displayDescription {
                                            Text(description)
                                                .font(.body)
                                                .foregroundStyle(.secondary)
                                        }
                                    }
                                }

                                MetricGrid {
                                    MetricTile(
                                        title: "Dernière observation",
                                        value: TimestampParser.relativeString(from: measures?.latestObservedAt),
                                        subtitle: TimestampParser.absoluteString(from: measures?.latestObservedAt),
                                        tone: AppTheme.info,
                                        monospaced: false
                                    )
                                    MetricTile(
                                        title: "Sources",
                                        value: "\(target.totalSourceCount)",
                                        subtitle: target.totalSourceCount == 1 ? "source active" : "sources actives"
                                    )
                                    MetricTile(
                                        title: "Disponibilité 24 h",
                                        value: measures?.last24Hours?.availabilityDisplayValue ?? "Non mesurée",
                                        subtitle: measures?.last24Hours?.coverageDisplayValue.map { "Couverture \($0)" } ?? availabilityHint,
                                        tone: measures?.last24Hours?.availabilityDisplayValue == nil ? AppTheme.info : .primary,
                                        monospaced: false
                                    )
                                    MetricTile(
                                        title: "Latence moyenne",
                                        value: measures?.last24Hours?.averageLatencyMilliseconds.map { "\($0.formatted(.number.precision(.fractionLength(0)))) ms" } ?? "—",
                                        subtitle: measures?.last24Hours?.maximumLatencyMilliseconds.map { "Pic \($0.formatted(.number.precision(.fractionLength(0)))) ms" },
                                        tone: AppTheme.warning
                                    )
                                }
                            }
                        }

                        if let importedContext = importedContext(for: target) {
                            Panel {
                                VStack(alignment: .leading, spacing: 10) {
                                    Text("Supervision importée")
                                        .font(AppTheme.sectionTitleFont)
                                    Text(importedContext)
                                        .foregroundStyle(.secondary)
                                }
                            }
                        }

                        TargetIndicatorsPanel(targetID: target.id)

                        if !target.sources.isEmpty {
                            Panel {
                                VStack(alignment: .leading, spacing: 14) {
                                    Text("Sources")
                                        .font(AppTheme.sectionTitleFont)

                                    ForEach(target.sources) { source in
                                        ComponentHealthRow(
                                            title: source.name,
                                            statusText: source.enabled ? source.kind.rawValue.uppercased() : "Suspendue",
                                            statusColor: source.enabled ? AppTheme.info : AppTheme.warning,
                                            statusSymbol: source.enabled ? "sensor.tag.radiowaves.forward.fill" : "pause.circle.fill",
                                            detail: "Intervalle \(source.intervalSeconds)s · seuil \(source.failureThreshold)/\(source.recoveryThreshold)"
                                        )
                                    }
                                }
                            }
                        }

                        let incidents = model.snapshot.incidents(forTargetID: target.id)
                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                Text("Incidents")
                                    .font(AppTheme.sectionTitleFont)

                                if incidents.isEmpty {
                                    Label("Aucun incident actif ou résolu à afficher pour cette cible.", systemImage: "checkmark.circle.fill")
                                        .font(.subheadline)
                                        .foregroundStyle(AppTheme.ok)
                                } else {
                                    ForEach(incidents) { incident in
                                        NavigationLink {
                                            IncidentDetailView(incidentID: incident.id)
                                        } label: {
                                            IncidentRow(incident: incident)
                                        }
                                        .buttonStyle(.plain)

                                        if incident.id != incidents.last?.id {
                                            Divider()
                                        }
                                    }
                                }
                            }
                        }
                    }
                    .padding(AppTheme.screenPadding)
                    .padding(.bottom, AppTheme.bottomScrollInset)
                }
            } else {
                ContentUnavailableView("Cible introuvable", systemImage: "questionmark.circle")
            }
        }
        // Un seul fond pour l'ecran : il etait auparavant pose deux fois, ce qui
        // doublait le cout de composition a chaque image.
        .background(AppBackdrop())
        .navigationTitle("Cible")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable {
            await model.refresh()
        }
    }

    private var measures: TargetMeasures? {
        model.snapshot.measures[targetID]
    }

    private var availabilityHint: String {
        if measures?.hasImportedOnlySignals == true {
            return "Mesure importée"
        }
        return "En attente de données"
    }

    private func importedContext(for target: Target) -> String? {
        guard target.sources.isEmpty, let summary = target.sourceOriginSummary else {
            return nil
        }

        return "\(summary). CairnOps reprend l’état courant et la dernière observation, mais certaines fenêtres de disponibilité peuvent rester non mesurées."
    }
}
