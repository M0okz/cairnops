import SwiftUI

struct IncidentDetailView: View {
    @Environment(AppModel.self) private var model

    let incidentID: Incident.ID

    var body: some View {
        Group {
            if let incident = model.incident(withID: incidentID) {
                ScrollView {
                    VStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                ResponsiveStatusHeader(
                                    text: incident.effectiveSeverity.label,
                                    color: AppTheme.severityColor(incident.effectiveSeverity),
                                    systemImage: AppTheme.severitySymbol(incident.effectiveSeverity)
                                ) {
                                    VStack(alignment: .leading, spacing: 4) {
                                        Text(incident.natureLabel)
                                            .font(AppTheme.cardTitleFont)
                                        Text(incident.primaryEvidenceLabel)
                                            .font(.body)
                                            .foregroundStyle(.secondary)
                                            .lineLimit(3)
                                    }
                                }

                                MetricGrid {
                                    MetricTile(
                                        title: "Ouvert",
                                        value: TimestampParser.relativeString(from: incident.openedAt),
                                        subtitle: TimestampParser.absoluteString(from: incident.openedAt),
                                        tone: AppTheme.info,
                                        monospaced: false
                                    )
                                    MetricTile(
                                        title: "Acquittement",
                                        value: incident.acknowledgedBy ?? "En attente",
                                        subtitle: syncLabel(for: incident.acknowledgementSyncStatus),
                                        tone: incident.isAcknowledged ? AppTheme.ok : AppTheme.warning,
                                        monospaced: false
                                    )
                                }

                                if model.canMutate && !incident.isAcknowledged && !incident.isResolved {
                                    AsyncButton {
                                        await model.acknowledge(incidentID: incident.id)
                                    } label: {
                                        Label("Acquitter l'incident", systemImage: "checkmark.circle")
                                            .frame(maxWidth: .infinity)
                                    }
                                    .buttonStyle(.borderedProminent)
                                }
                            }
                        }


                        IncidentIndicatorsPanel(incidentID: incident.id)

                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                Text("Preuves")
                                    .font(AppTheme.sectionTitleFont)

                                ForEach(incident.signals) { signal in
                                    VStack(alignment: .leading, spacing: 6) {
                                        HStack {
                                            Text(signal.name)
                                                .font(.headline)
                                            Spacer()
                                            StatusPill(
                                                text: signal.active ? "Active" : "Résolue",
                                                color: signal.active ? AppTheme.severityColor(signal.severity) : AppTheme.ok
                                            )
                                        }
                                        Text(signal.origin.capitalized)
                                            .font(.footnote)
                                            .foregroundStyle(.secondary)
                                        if let invalidationReason = signal.invalidationReason {
                                            Text("Invalidation : \(invalidationReason)")
                                                .font(.footnote)
                                                .foregroundStyle(AppTheme.warning)
                                        }
                                    }
                                }
                            }
                        }

                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                Text("Journal d’activité")
                                    .font(AppTheme.sectionTitleFont)

                                // Le serveur livre deja le journal du plus
                                // recent au plus ancien : aucun tri n'est requis
                                // pendant le rendu.
                                ForEach(incident.activity) { activity in
                                    VStack(alignment: .leading, spacing: 4) {
                                        Text(activity.message)
                                            .font(.headline)
                                        Text("\(activity.origin.capitalized) · \(TimestampParser.absoluteString(from: activity.occurredAt))")
                                            .font(.footnote)
                                            .foregroundStyle(.secondary)
                                    }
                                }
                            }
                        }
                    }
                    .padding(AppTheme.screenPadding)
                    .padding(.bottom, AppTheme.bottomScrollInset)
                }
            } else {
                ContentUnavailableView("Incident introuvable", systemImage: "exclamationmark.circle")
            }
        }
        .background(AppBackdrop())
        .navigationTitle("Incident")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable {
            await model.refresh()
        }
    }

    private func syncLabel(for value: String) -> String {
        switch value {
        case "pending":
            "En cours"
        case "synchronized":
            "Synchronisée"
        case "failed":
            "Échouée"
        default:
            "Sans propagation"
        }
    }
}
