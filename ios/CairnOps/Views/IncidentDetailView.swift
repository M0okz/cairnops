import SwiftUI

/// Detail d'un Incident.
///
/// L'ecran nomme d'abord la Gravite et l'etat d'Acquittement, puis la nature en
/// grand titre, ses references, la tendance de latence de la Cible, les preuves
/// et le journal. L'Acquittement reste accessible en permanence dans la barre
/// flottante.
struct IncidentDetailView: View {
    @Environment(AppModel.self) private var model

    let incidentID: Incident.ID

    var body: some View {
        Group {
            if let incident = model.incident(withID: incidentID) {
                content(incident)
            } else {
                ContentUnavailableView("Incident introuvable", systemImage: "exclamationmark.circle")
                    .background(AppTheme.ground.ignoresSafeArea())
            }
        }
        .refreshable {
            await model.refresh()
        }
    }

    private func content(_ incident: Incident) -> some View {
        BareScreen(bottomInset: AppTheme.actionBarScrollInset) {
            state(incident)
                .padding(.top, 4)
            title(incident)
            fields(incident)
            latency(incident)
            evidence(incident)
            activity(incident)
        }
        .navigationTitle("Incident")
        .navigationSubtitle(incident.id.prefix(8).uppercased())
        .navigationBarTitleDisplayMode(.inline)
        // La barre d'actions occupe le bas de l'ecran : y superposer la barre
        // d'onglets empilerait deux barres flottantes. La maquette montre
        // d'ailleurs les ecrans de detail sans onglets.
        .toolbar(.hidden, for: .tabBar)
        .overlay(alignment: .bottom) {
            actionBar(incident)
        }
    }

    // MARK: - Haut de page

    private func state(_ incident: Incident) -> some View {
        HStack(spacing: 10) {
            Text(incident.effectiveSeverity.label.uppercased())
                .font(.caption2.weight(.bold))
                .tracking(0.7)
                .foregroundStyle(AppTheme.severityInk(incident.effectiveSeverity))

            Circle()
                .fill(AppTheme.inkMuted)
                .frame(width: 3, height: 3)

            Text(acknowledgementLabel(incident))
                .font(.caption2.weight(.bold))
                .tracking(0.7)
                .foregroundStyle(AppTheme.inkMuted)

            Spacer(minLength: 8)

            Text(TimestampParser.elapsedString(since: incident.openedAt))
                .font(.footnote.weight(.bold))
                .monospacedDigit()
                .foregroundStyle(AppTheme.ink)
        }
        .lineLimit(1)
        .minimumScaleFactor(0.8)
        .padding(.bottom, 10)
    }

    private func title(_ incident: Incident) -> some View {
        Text(incident.natureLabel)
            .font(AppTheme.detailTitleFont)
            .tracking(-0.5)
            .foregroundStyle(AppTheme.ink)
            .fixedSize(horizontal: false, vertical: true)
            .accessibilityAddTraits(.isHeader)
            .padding(.bottom, 18)
    }

    private func fields(_ incident: Incident) -> some View {
        LazyVGrid(
            columns: [GridItem(.flexible(), spacing: 20, alignment: .topLeading),
                      GridItem(.flexible(), spacing: 20, alignment: .topLeading)],
            alignment: .leading,
            spacing: 14
        ) {
            FieldCell(label: "Cible", value: incident.targetName)
            FieldCell(label: "Début", value: TimestampParser.absoluteString(from: incident.openedAt))
            FieldCell(label: "Preuves", value: incident.visibleSignalCountLabel)
            FieldCell(label: "Origine", value: originLabel(incident))
        }
        .padding(.top, 16)
        .padding(.bottom, 20)
        .hairlineTop()
    }

    // MARK: - Tendance

    @ViewBuilder
    private func latency(_ incident: Incident) -> some View {
        let trend = model.snapshot.measures[incident.targetID]?.latencyTrend ?? []

        if trend.count > 1 {
            VStack(alignment: .leading, spacing: 10) {
                SectionLabel("Latence", detail: "tendance récente")

                ThresholdChart(
                    values: trend,
                    tone: AppTheme.accent
                )

                HStack {
                    Text("min \(latencyLabel(trend.min()))")
                    Spacer()
                    Text("max \(latencyLabel(trend.max()))")
                    Spacer()
                    Text("maintenant \(latencyLabel(trend.last))")
                }
                .font(.caption2)
                .foregroundStyle(AppTheme.inkMuted)
            }
            .padding(.top, 16)
            .padding(.bottom, 6)
            .hairlineTop()
        }
    }

    private func latencyLabel(_ value: Double?) -> String {
        guard let value else {
            return "—"
        }
        return value.formatted(.number.precision(.fractionLength(0))) + " ms"
    }

    // MARK: - Preuves et journal

    @ViewBuilder
    private func evidence(_ incident: Incident) -> some View {
        SectionLabel("Preuves", detail: incident.visibleSignalCountLabel)
            .padding(.top, 16)
            .padding(.bottom, 2)
            .hairlineTop()

        if incident.signals.isEmpty {
            Text("Aucune preuve rattachée à cet incident.")
                .font(.subheadline)
                .foregroundStyle(AppTheme.inkMuted)
                .padding(.vertical, 13)
        } else {
            ForEach(incident.signals) { signal in
                EvidenceRow(signal: signal)
            }
        }

        IncidentIndicatorsPanel(incidentID: incident.id)
    }

    @ViewBuilder
    private func activity(_ incident: Incident) -> some View {
        if !incident.activity.isEmpty {
            SectionLabel("Journal d’activité")
                .padding(.top, 16)
                .padding(.bottom, 2)
                .hairlineTop()

            // Le serveur livre deja le journal du plus recent au plus ancien :
            // aucun tri n'est requis pendant le rendu.
            ForEach(incident.activity) { entry in
                VStack(alignment: .leading, spacing: 3) {
                    Text(entry.message)
                        .font(AppTheme.fieldValueFont)
                        .foregroundStyle(AppTheme.ink)
                        .fixedSize(horizontal: false, vertical: true)

                    Text("\(entry.origin.capitalized) · \(TimestampParser.absoluteString(from: entry.occurredAt))")
                        .font(AppTheme.metaFont)
                        .foregroundStyle(AppTheme.inkMuted)
                }
                .padding(.vertical, 13)
                .frame(maxWidth: .infinity, alignment: .leading)
                .hairlineTop()
                .accessibilityElement(children: .combine)
            }
        }
    }

    // MARK: - Barre d'actions

    @ViewBuilder
    private func actionBar(_ incident: Incident) -> some View {
        GlassActionBar {
            if model.canMutate && !incident.isAcknowledged && !incident.isResolved {
                AsyncButton {
                    await model.acknowledge(incidentID: incident.id)
                } label: {
                    GlassActionLabel(
                        title: "Acquitter",
                        systemImage: "checkmark",
                        isProminent: true
                    )
                }
                .buttonStyle(.plain)
            } else {
                // Sans action possible, la barre garde sa place mais dit
                // pourquoi : un bouton grise n'expliquerait rien.
                GlassActionLabel(
                    title: acknowledgementLabel(incident).capitalized,
                    systemImage: incident.isResolved ? "checkmark.seal" : "checkmark.circle",
                    isProminent: false
                )
                .frame(maxWidth: .infinity)
            }
        } secondary: {
            NavigationLink {
                TargetDetailView(targetID: incident.targetID)
            } label: {
                GlassActionLabel(title: "Cible", systemImage: "dot.scope")
            }
            .buttonStyle(.plain)
        }
        .padding(.bottom, 4)
    }

    // MARK: - Libelles

    private func acknowledgementLabel(_ incident: Incident) -> String {
        if incident.isResolved {
            return "Résolu"
        }
        if incident.isAcknowledged {
            if let author = incident.acknowledgedBy, !author.isEmpty {
                return "Acquitté par \(author)"
            }
            return "Acquitté"
        }
        return "Non acquitté"
    }

    private func originLabel(_ incident: Incident) -> String {
        if let connector = incident.signals.compactMap(\.connectorName).first, !connector.isEmpty {
            return connector
        }
        if incident.signals.contains(where: { $0.origin == "native" }) {
            return "Contrôle natif"
        }
        return incident.signals.first?.origin.capitalized ?? "Inconnue"
    }
}
