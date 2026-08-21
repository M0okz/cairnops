import SwiftUI

struct OverviewView: View {
    @Environment(AppModel.self) private var model

    /// Chiffres derives de la projection.
    ///
    /// Chaque valeur affichee sur cet ecran declenchait auparavant son propre
    /// parcours de `targets` ou `incidents`, plusieurs fois par rendu. On les
    /// calcule ensemble, une fois par passe de mise en page.
    private struct Summary {
        var unacknowledgedCount = 0
        var actionableCount = 0
        var targetCount = 0
        var healthyTargetCount = 0
        var highlightedIncidents: [Incident] = []
        var problematicTargets: [Target] = []
    }

    private func makeSummary() -> Summary {
        let snapshot = model.snapshot
        var summary = Summary()

        summary.targetCount = snapshot.targets.count
        summary.actionableCount = snapshot.actionableIncidents.count

        let unacknowledged = snapshot.unacknowledgedIncidents
        summary.unacknowledgedCount = unacknowledged.count
        summary.highlightedIncidents = Array(unacknowledged.prefix(4))

        for target in snapshot.sortedTargets {
            if snapshot.health(for: target) == .ok {
                summary.healthyTargetCount += 1
            } else if summary.problematicTargets.count < 4 {
                summary.problematicTargets.append(target)
            }
        }

        return summary
    }

    var body: some View {
        let summary = makeSummary()

        return ScrollView {
            LazyVStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                ScreenHeader(
                    title: "Vue d’ensemble",
                    subtitle: "Projection partagée de \(model.instanceLabel)"
                ) {
                    refreshButton
                }

                heroCard(summary)
                summaryPanel(summary)

                if summary.targetCount == 0 && model.snapshot.incidents.isEmpty {
                    ContentUnavailableView(
                        "Aucune cible active",
                        systemImage: "dot.scope",
                        description: Text("La projection mobile apparaitra ici des qu'une premiere cible sera supervisee.")
                    )
                    .frame(maxWidth: .infinity)
                    .padding(.top, 24)
                }

                if !summary.highlightedIncidents.isEmpty {
                    Panel {
                        VStack(alignment: .leading, spacing: 14) {
                            header("Incidents non acquittés", count: summary.unacknowledgedCount)

                            ForEach(summary.highlightedIncidents) { incident in
                                NavigationLink {
                                    IncidentDetailView(incidentID: incident.id)
                                } label: {
                                    IncidentRow(incident: incident)
                                }
                                .buttonStyle(.plain)

                                if incident.id != summary.highlightedIncidents.last?.id {
                                    Color.clear.frame(height: 2)
                                }
                            }
                        }
                    }
                }

                if !summary.problematicTargets.isEmpty {
                    Panel {
                        VStack(alignment: .leading, spacing: 14) {
                            header("Cibles à surveiller", count: summary.problematicTargets.count)

                            ForEach(summary.problematicTargets) { target in
                                NavigationLink {
                                    TargetDetailView(targetID: target.id)
                                } label: {
                                    TargetRow(
                                        target: target,
                                        health: model.snapshot.health(for: target),
                                        measures: model.snapshot.measures[target.id]
                                    )
                                }
                                .buttonStyle(.plain)

                                if target.id != summary.problematicTargets.last?.id {
                                    Color.clear.frame(height: 2)
                                }
                            }
                        }
                    }
                }

                if let systemHealth = model.snapshot.systemHealth {
                    Panel {
                        VStack(alignment: .leading, spacing: 10) {
                            HStack {
                                Text("Sante de CairnOps")
                                    .font(AppTheme.sectionTitleFont)
                                Spacer()
                                StatusPill(
                                    text: systemHealth.status == "operational" ? "Opérationnelle" : "Dégradée",
                                    color: systemHealth.status == "operational" ? AppTheme.ok : AppTheme.warning,
                                    systemImage: systemHealth.status == "operational" ? "heart.text.square.fill" : "waveform.path.ecg"
                                )
                            }

                            Text("Contrôle le \(TimestampParser.absoluteString(from: systemHealth.checkedAt)).")
                                .font(.footnote)
                                .foregroundStyle(.secondary)

                            HStack(spacing: 12) {
                                MetricTile(
                                    title: "Observations 24 h",
                                    value: "\(summary.unacknowledgedCount)",
                                    subtitle: "Incidents demandant une action",
                                    tone: AppTheme.critical,
                                    systemImage: "exclamationmark.triangle.fill"
                                )
                                MetricTile(
                                    title: "Cibles saines",
                                    value: "\(summary.healthyTargetCount)",
                                    subtitle: "\(summary.targetCount) cibles suivies",
                                    tone: AppTheme.ok,
                                    systemImage: "checkmark.circle.fill"
                                )
                            }

                            ForEach(systemHealth.components) { component in
                                ComponentHealthRow(
                                    title: componentLabel(component.name),
                                    statusText: componentStatusText(component.status),
                                    statusColor: component.status == "operational" ? AppTheme.ok : AppTheme.warning,
                                    statusSymbol: component.status == "operational" ? "checkmark.circle.fill" : "exclamationmark.triangle.fill",
                                    detail: "\(component.instances) instance(s)"
                                )
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
    }

    private func summaryPanel(_ summary: Summary) -> some View {
        Panel {
            VStack(alignment: .leading, spacing: 16) {
                HStack(alignment: .top, spacing: 12) {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("État global")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(.secondary)
                            .textCase(.uppercase)
                        Text(model.instanceLabel)
                            .font(AppTheme.cardTitleFont)
                        Text(verdictLine)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }

                    Spacer(minLength: 12)

                    StatusPill(
                        text: AppTheme.globalStatusLabel(model.snapshot.globalStatus),
                        color: AppTheme.globalStatusColor(model.snapshot.globalStatus),
                        systemImage: AppTheme.globalStatusSymbol(model.snapshot.globalStatus)
                    )
                }

                HStack(spacing: 12) {
                    MetricTile(
                        title: "Non acquittés",
                        value: "\(summary.unacknowledgedCount)",
                        tone: AppTheme.critical,
                        systemImage: "exclamationmark.triangle.fill"
                    )
                    MetricTile(
                        title: "Actifs",
                        value: "\(summary.actionableCount)",
                        tone: AppTheme.warning,
                        systemImage: "bolt.horizontal.circle.fill"
                    )
                    MetricTile(
                        title: "Cibles",
                        value: "\(summary.targetCount)",
                        tone: AppTheme.info,
                        systemImage: "dot.scope"
                    )
                }

                HStack(spacing: 12) {
                    Label(TimestampParser.relativeString(from: model.snapshot.freshestObservationAt), systemImage: "clock")
                    Label(realtimeLabel, systemImage: "dot.radiowaves.left.and.right")
                }
                .font(.footnote)
                .foregroundStyle(.secondary)
            }
        }
    }

    private func heroCard(_ summary: Summary) -> some View {
        let hasActionableIncidents = summary.unacknowledgedCount > 0

        return HStack(alignment: .center, spacing: 16) {
            Image(systemName: hasActionableIncidents ? "exclamationmark.triangle.fill" : "checkmark.circle.fill")
                .font(.title2.weight(.bold))
                .foregroundStyle(hasActionableIncidents ? AppTheme.critical : AppTheme.ok)
                .frame(width: 56, height: 56)
                .background(
                    Circle()
                        .fill((hasActionableIncidents ? AppTheme.critical : AppTheme.ok).opacity(0.12))
                )

            VStack(alignment: .leading, spacing: 6) {
                Text(hasActionableIncidents ? "Incidents actifs" : "Supervision stable")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(hasActionableIncidents ? AppTheme.critical : AppTheme.ok)
                    .textCase(.uppercase)

                Text(hasActionableIncidents ? actionHeadline(summary) : "Aucun incident non acquitté.")
                    .font(AppTheme.heroTitleFont)

                Text("\(summary.healthyTargetCount) cibles opérationnelles · \(summary.targetCount) supervisées")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            Spacer(minLength: 12)

            Image(systemName: "chevron.right")
                .font(.headline.weight(.semibold))
                .foregroundStyle(.tertiary)
        }
        .padding(20)
        .background(
            RoundedRectangle(cornerRadius: 22)
                .fill(heroBackground(hasActionableIncidents))
                .overlay(
                    RoundedRectangle(cornerRadius: 22)
                        .strokeBorder(
                            hasActionableIncidents
                                ? AppTheme.critical.opacity(0.18)
                                : AppTheme.ok.opacity(0.18)
                        )
                )
        )
    }

    private var realtimeLabel: String {
        switch model.realtimeState {
        case .offline:
            "Flux hors ligne"
        case .connecting:
            "Connexion en cours"
        case .online:
            "Flux en ligne"
        }
    }

    private var refreshButton: some View {
        AsyncButton {
            await model.refresh()
        } label: {
            RefreshGlyph()
        }
        .buttonStyle(.plain)
        .foregroundStyle(AppTheme.accent)
        .accessibilityLabel("Actualiser la projection")
    }

    private var verdictLine: String {
        "Dernière projection \(TimestampParser.relativeString(from: model.snapshot.lastRefreshAt))."
    }

    private func actionHeadline(_ summary: Summary) -> String {
        summary.unacknowledgedCount == 1
            ? "1 incident demande une action"
            : "\(summary.unacknowledgedCount) incidents demandent une action"
    }

    private func heroBackground(_ hasActionableIncidents: Bool) -> LinearGradient {
        LinearGradient(
            colors: [
                (hasActionableIncidents ? AppTheme.critical.opacity(0.10) : AppTheme.ok.opacity(0.08)),
                AppTheme.panel,
            ],
            startPoint: .topLeading,
            endPoint: .bottomTrailing
        )
    }

    @ViewBuilder
    private func header(_ title: String, count: Int) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Text(title)
                .font(AppTheme.sectionTitleFont)

            Spacer()

            Text(count == 1 ? "1 élément" : "\(count) éléments")
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    private func componentLabel(_ value: String) -> String {
        switch value {
        case "server":
            "Serveur"
        case "worker":
            "Worker"
        case "postgresql":
            "PostgreSQL"
        default:
            value.capitalized
        }
    }

    private func componentStatusText(_ value: String) -> String {
        switch value {
        case "operational":
            "Opérationnel"
        case "stale":
            "En retard"
        default:
            "Indisponible"
        }
    }
}
