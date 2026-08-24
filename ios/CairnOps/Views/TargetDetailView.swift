import SwiftUI

/// Detail d'une Cible.
///
/// L'identite et l'Etat de sante ouvrent l'ecran, les mesures suivent en dalles
/// chiffrees avec leur tendance, puis les Sources qui l'observent et son
/// historique d'Incidents.
struct TargetDetailView: View {
    @Environment(AppModel.self) private var model

    let targetID: Target.ID

    private var measures: TargetMeasures? {
        model.snapshot.measures[targetID]
    }

    var body: some View {
        Group {
            if let target = model.target(withID: targetID) {
                content(target)
            } else {
                ContentUnavailableView("Cible introuvable", systemImage: "questionmark.circle")
                    .background(AppTheme.ground.ignoresSafeArea())
            }
        }
        .refreshable {
            await model.refresh()
        }
    }

    private func content(_ target: Target) -> some View {
        // Un seul calcul de sante au lieu de plusieurs appels identiques
        // pendant le rendu.
        let health = model.snapshot.health(for: target)

        return BareScreen(bottomInset: AppTheme.actionBarScrollInset) {
            identity(target, health: health)
                .padding(.top, 4)
            state(target, health: health)
            measureGrid(target)
            TargetIndicatorsPanel(targetID: target.id)
            sources(target)
            incidents(target)
        }
        .navigationTitle("Cible")
        .navigationSubtitle(AppTheme.targetHealthLabel(health))
        .navigationBarTitleDisplayMode(.inline)
        .toolbar(.hidden, for: .tabBar)
        .overlay(alignment: .bottom) {
            actionBar
        }
    }

    private func identity(_ target: Target, health: AppSnapshot.TargetHealth) -> some View {
        HStack(alignment: .center, spacing: 11) {
            StatusDot(tone: AppTheme.targetHealthColor(health), size: 12, haloWidth: 5)

            VStack(alignment: .leading, spacing: 4) {
                // La barre de navigation nomme deja l'ecran : le nom de la
                // Cible n'a plus besoin du corps le plus grand pour se lire
                // comme le sujet de la page.
                Text(target.name)
                    .font(.title.weight(.bold))
                    .tracking(-0.6)
                    .foregroundStyle(AppTheme.ink)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityAddTraits(.isHeader)

                if let description = target.displayDescription ?? target.sourceOriginSummary {
                    Text(description)
                        .font(AppTheme.metaFont)
                        .foregroundStyle(AppTheme.inkMuted)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
        .padding(.bottom, 8)
    }

    private func state(_ target: Target, health: AppSnapshot.TargetHealth) -> some View {
        HStack(spacing: 9) {
            Text(AppTheme.targetHealthShortLabel(health))
                .font(AppTheme.metaFont.weight(.bold))
                .tracking(0.6)
                .foregroundStyle(AppTheme.targetHealthInk(health))

            if let connector = target.connectorName {
                Text(connector)
                    .font(AppTheme.metaFont)
                    .foregroundStyle(AppTheme.inkMuted)
            }

            Spacer(minLength: 8)

            Text(sourceCountLabel(target))
                .font(AppTheme.metaFont)
                .foregroundStyle(AppTheme.inkMuted)
        }
        .lineLimit(1)
        .minimumScaleFactor(0.8)
        .padding(.bottom, 6)
    }

    // MARK: - Mesures

    private func measureGrid(_ target: Target) -> some View {
        LazyVGrid(
            columns: [GridItem(.flexible(), spacing: 20, alignment: .topLeading),
                      GridItem(.flexible(), spacing: 20, alignment: .topLeading)],
            alignment: .leading,
            spacing: 0
        ) {
            MetricCell(
                label: "Dispo. 24 h",
                value: measures?.last24Hours?.availabilityDisplayValue ?? "—",
                unit: measures?.last24Hours?.coverageDisplayValue.map { "couv. \($0)" },
                tone: availabilityTone,
                // Cadrage automatique : borner a 0-1 tasserait toute la
                // variation utile dans le dernier pour cent du cadre.
                series: measures?.trend ?? []
            )

            MetricCell(
                label: "Latence moy.",
                value: latencyValue,
                unit: latencyValue == "—" ? nil : "ms",
                tone: AppTheme.ink,
                series: measures?.latencyTrend ?? []
            )

            MetricCell(
                label: "Sources",
                value: "\(target.totalSourceCount)",
                unit: target.totalSourceCount == 1 ? "active" : "actives",
                tone: AppTheme.ink
            )

            MetricCell(
                label: "Observation",
                value: TimestampParser.relativeString(from: measures?.latestObservedAt),
                tone: AppTheme.ink,
                isNumeric: false
            )
        }
        .padding(.top, 4)
    }

    private var latencyValue: String {
        guard let latency = measures?.last24Hours?.averageLatencyMilliseconds else {
            return "—"
        }
        return latency.formatted(.number.precision(.fractionLength(0)))
    }

    private var availabilityTone: Color {
        guard let availability = measures?.last24Hours?.availability,
              measures?.last24Hours?.hasAvailabilitySignal == true else {
            return AppTheme.inkMuted
        }
        if availability < 0.95 {
            return AppTheme.criticalInk
        }
        if availability < 0.99 {
            return AppTheme.warningInk
        }
        return AppTheme.ink
    }

    // MARK: - Sources et Incidents

    /// Une Source telle qu'affichee, quelle que soit son origine.
    private struct SourceEntry: Identifiable {
        let id: String
        let name: String
        let origin: String
        let kind: String
        let outcome: String?
        let observedAt: String?
        let native: Target.Source?
    }

    /// Toutes les Sources de la Cible, une ligne chacune.
    ///
    /// `Target.sources` ne porte que les Controles natifs ; les Sources
    /// importees n'y figurent que par leur nombre. Les replier en une seule
    /// ligne « Integration » revenait a masquer la moitie de ce qui observe la
    /// Cible. La projection des mesures, elle, decrit chaque Source quelle que
    /// soit son origine : c'est donc elle qui fait foi ici.
    private func sourceEntries(_ target: Target) -> [SourceEntry] {
        let nativeByID = Dictionary(
            target.sources.map { ($0.id, $0) },
            uniquingKeysWith: { first, _ in first }
        )

        if let measured = measures?.sources, !measured.isEmpty {
            return measured.map { entry in
                SourceEntry(
                    id: entry.sourceID,
                    name: entry.name,
                    origin: entry.origin,
                    kind: entry.kind,
                    outcome: entry.latestOutcome,
                    observedAt: entry.latestObservedAt,
                    native: nativeByID[entry.sourceID]
                )
            }
        }

        return target.sources.map { source in
            SourceEntry(
                id: source.id,
                name: source.name,
                origin: "native",
                kind: source.kind.rawValue,
                outcome: source.latestOutcome?.rawValue,
                observedAt: source.lastObservedAt,
                native: source
            )
        }
    }

    @ViewBuilder
    private func sources(_ target: Target) -> some View {
        let entries = sourceEntries(target)
        let unlisted = max(target.totalSourceCount - entries.count, 0)

        if !entries.isEmpty || unlisted > 0 {
            SectionLabel("Sources de signal", detail: sourceCountLabel(target))
                .padding(.top, 18)
                .padding(.bottom, 2)
                .hairlineTop()

            ForEach(entries) { entry in
                MetaRow(
                    title: entry.name,
                    subtitle: sourceSubtitle(entry),
                    detail: TimestampParser.relativeString(from: entry.observedAt),
                    state: sourceState(entry),
                    tone: sourceTone(entry),
                    stateInk: entry.native?.enabled == false ? AppTheme.warningInk : AppTheme.inkStrong
                )
            }

            // Sans projection de mesures, les Sources importees ne sont connues
            // que par leur nombre : on le dit plutot que de les taire.
            if unlisted > 0 {
                MetaRow(
                    title: target.connectorName ?? "Sources importées",
                    subtitle: "Détail non projeté pour le moment",
                    state: "\(unlisted)",
                    tone: AppTheme.info,
                    stateInk: AppTheme.inkStrong
                )
            }
        }
    }

    private func sourceSubtitle(_ entry: SourceEntry) -> String {
        if let native = entry.native {
            return "Intervalle \(native.intervalSeconds) s · seuil \(native.failureThreshold)/\(native.recoveryThreshold)"
        }
        return sourceOriginLabel(entry.origin)
    }

    private func sourceOriginLabel(_ origin: String) -> String {
        switch origin {
        case "native":
            "Contrôle natif"
        case "integration":
            "Importée par une Intégration"
        case "webhook":
            "Signal entrant"
        default:
            origin.capitalized
        }
    }

    private func sourceState(_ entry: SourceEntry) -> String {
        if entry.native?.enabled == false {
            return "SUSPENDUE"
        }
        return entry.kind.uppercased()
    }

    private func sourceTone(_ entry: SourceEntry) -> Color {
        if entry.native?.enabled == false {
            return AppTheme.neutral
        }
        switch entry.outcome {
        case "healthy":
            return AppTheme.ok
        case "unhealthy":
            return entry.native.map { AppTheme.severityColor($0.severity) } ?? AppTheme.critical
        default:
            return AppTheme.neutral
        }
    }

    @ViewBuilder
    private func incidents(_ target: Target) -> some View {
        let incidents = model.snapshot.incidents(forTargetID: target.id)

        SectionLabel("Incidents", detail: incidents.isEmpty ? "aucun" : "\(incidents.count)")
            .padding(.top, 18)
            .padding(.bottom, 2)
            .hairlineTop()

        if incidents.isEmpty {
            Text("Aucun incident actif ou résolu pour cette cible.")
                .font(.subheadline)
                .foregroundStyle(AppTheme.inkMuted)
                .padding(.vertical, 13)
        } else {
            ForEach(incidents) { incident in
                NavigationLink {
                    IncidentDetailView(incidentID: incident.id)
                } label: {
                    IncidentRow(incident: incident)
                        .hairlineTop()
                }
                .buttonStyle(.plain)
            }
        }
    }

    // MARK: - Barre d'actions

    /// La maquette propose « Sonder maintenant » et « Maintenance ». Aucune des
    /// deux n'existe dans le contrat client : la barre ne porte donc que
    /// l'actualisation, plutot qu'un bouton sans effet.
    private var actionBar: some View {
        GlassActionBar {
            AsyncButton {
                await model.refresh()
            } label: {
                GlassActionLabel(
                    title: "Actualiser",
                    systemImage: "arrow.clockwise",
                    isProminent: true
                )
            }
            .buttonStyle(.plain)
        }
        .padding(.bottom, 4)
    }

    private func sourceCountLabel(_ target: Target) -> String {
        target.totalSourceCount == 1 ? "1 source" : "\(target.totalSourceCount) sources"
    }
}
