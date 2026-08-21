import SwiftUI

struct IncidentDetailView: View {
    @Environment(AppModel.self) private var model

    let incidentID: Incident.ID

    var body: some View {
        Group {
            if let incident = model.incident(withID: incidentID) {
                ScrollView {
                    VStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                        DetailHeader(
                            title: "Incident",
                            subtitle: incident.targetName
                        )

                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                HStack(alignment: .top) {
                                    VStack(alignment: .leading, spacing: 4) {
                                        Text(incident.natureLabel)
                                            .font(AppTheme.cardTitleFont)
                                        Text(incident.primaryEvidenceLabel)
                                            .font(.body)
                                            .foregroundStyle(.secondary)
                                            .lineLimit(3)
                                    }

                                    Spacer(minLength: 12)

                                    StatusPill(
                                        text: incident.effectiveSeverity.label,
                                        color: AppTheme.severityColor(incident.effectiveSeverity),
                                        systemImage: AppTheme.severitySymbol(incident.effectiveSeverity)
                                    )
                                }

                                HStack(spacing: 12) {
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

                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                Text("Preuves actives")
                                    .font(AppTheme.sectionTitleFont)

                                ForEach(incident.signals) { signal in
                                    VStack(alignment: .leading, spacing: 6) {
                                        HStack {
                                            Text(signal.name)
                                                .font(.headline)
                                            Spacer()
                                            StatusPill(
                                                text: signal.active ? "Active" : "Resolue",
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
                                Text("Journal d'activite")
                                    .font(AppTheme.sectionTitleFont)

                                ForEach(incident.activity.sorted(by: { $0.id > $1.id })) { activity in
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
                .background(AppBackdrop())
                .toolbar(.hidden, for: .navigationBar)
            } else {
                ContentUnavailableView("Incident introuvable", systemImage: "exclamationmark.circle")
            }
        }
        .background(AppBackdrop())
    }

    private func syncLabel(for value: String) -> String {
        switch value {
        case "pending":
            "En cours"
        case "synchronized":
            "Synchronisee"
        case "failed":
            "Echouee"
        default:
            "Sans propagation"
        }
    }
}
