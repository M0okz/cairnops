import SwiftUI

/// Vue d'ensemble orientee exceptions.
///
/// L'ecran descend du plus urgent au plus calme : identite et fraicheur, les
/// deux compteurs de Gravite, la repartition de la flotte, les Incidents qui
/// demandent une action, puis le contexte.
struct OverviewView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.selectTab) private var selectTab

    /// Chiffres derives de la projection.
    ///
    /// Chaque valeur affichee declenchait auparavant son propre parcours de
    /// `targets` ou `incidents`, plusieurs fois par rendu. Un seul passage les
    /// calcule toutes.
    private struct Summary {
        var criticalCount = 0
        var criticalUnacknowledged = 0
        var warningCount = 0
        var warningUnacknowledged = 0
        var targetCount = 0
        var healthyCount = 0
        var degradedCount = 0
        var downCount = 0
        var availability: Double?
        var highlighted: [Incident] = []
        var loaded: [(target: Target, value: Double)] = []
    }

    private func makeSummary() -> Summary {
        let snapshot = model.snapshot
        var summary = Summary()
        summary.targetCount = snapshot.targets.count

        for incident in snapshot.actionableIncidents {
            switch incident.effectiveSeverity {
            case .critical, .major:
                summary.criticalCount += 1
                if !incident.isAcknowledged {
                    summary.criticalUnacknowledged += 1
                }
            case .warning:
                summary.warningCount += 1
                if !incident.isAcknowledged {
                    summary.warningUnacknowledged += 1
                }
            case .information:
                break
            }
        }

        var availabilitySum = 0.0
        var availabilityCount = 0

        for target in snapshot.sortedTargets {
            switch snapshot.health(for: target) {
            case .ok:
                summary.healthyCount += 1
            case .degraded, .maintenance:
                summary.degradedCount += 1
            case .down:
                summary.downCount += 1
            case .unknown:
                break
            }

            if let availability = snapshot.measures[target.id]?.last24Hours,
               availability.hasAvailabilitySignal,
               let value = availability.availability {
                availabilitySum += value
                availabilityCount += 1
            }

            // La charge n'existe que si un Connecteur fournit l'indicateur : on
            // ne fabrique pas une valeur pour completer la section.
            if let cpu = snapshot.indicatorTargets[target.id]?.indicators.first(
                where: { $0.semanticKey == "cpu.utilization" && $0.lastValue != nil }
            ), let value = cpu.lastValue {
                summary.loaded.append((target, value))
            }
        }

        if availabilityCount > 0 {
            summary.availability = availabilitySum / Double(availabilityCount)
        }

        summary.highlighted = Array(snapshot.unacknowledgedIncidents.prefix(4))
        summary.loaded = Array(
            summary.loaded.sorted { $0.value > $1.value }.prefix(4)
        )

        return summary
    }

    var body: some View {
        let summary = makeSummary()

        return BareScreen(hidesNavigationBar: true) {
            identity
            title

            if summary.targetCount == 0 && model.snapshot.incidents.isEmpty {
                emptyState
            } else {
                counters(summary)
                fleet(summary)
                incidents(summary)
                pinnedIndicators
                loaded(summary)
            }
        }
        .refreshable {
            await model.refresh()
        }
    }

    // MARK: - Haut de page

    private var identity: some View {
        HStack(spacing: 12) {
            NavigationLink {
                SettingsView()
            } label: {
                HStack(spacing: 12) {
                    AvatarBadge(name: model.user?.displayName ?? model.instanceLabel)

                    VStack(alignment: .leading, spacing: 1) {
                        Text(Self.identityDateStyle.format(.now))
                        .font(.caption2)
                        .foregroundStyle(AppTheme.inkMuted)

                        Text(model.instanceLabel)
                            .font(.subheadline.weight(.bold))
                            .tracking(-0.15)
                            .foregroundStyle(AppTheme.ink)
                    }
                }
                .contentShape(.rect)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Réglages de \(model.instanceLabel)")

            Spacer(minLength: 8)

            NavigationLink {
                NotificationSettingsView()
            } label: {
                Image(systemName: "bell")
                    .font(.system(size: 19, weight: .medium))
                    .foregroundStyle(AppTheme.inkStrong)
                    .overlay(alignment: .topTrailing) {
                        if model.snapshot.unreadCount > 0 {
                            Circle()
                                .fill(AppTheme.accentSolid)
                                .frame(width: 8, height: 8)
                                .overlay(
                                    Circle().strokeBorder(AppTheme.ground, lineWidth: 2)
                                )
                                .offset(x: 3, y: -2)
                        }
                    }
                    .frame(width: 32, height: 32)
                    .contentShape(.rect)
            }
            .buttonStyle(.plain)
            .accessibilityLabel(
                model.snapshot.unreadCount > 0
                    ? "Notifications, \(model.snapshot.unreadCount) non lues"
                    : "Notifications"
            )
        }
        .padding(.vertical, 4)
        .padding(.bottom, 10)
    }

    private var title: some View {
        PageTitle("Vue d’ensemble") {
            HStack(spacing: 6) {
                Text(TimestampParser.relativeString(from: model.snapshot.freshestObservationAt))
                    .font(.caption2)
                    .foregroundStyle(AppTheme.inkMuted)

                Image(systemName: realtimeSymbol)
                    .font(.system(size: 11, weight: .bold))
                    .foregroundStyle(realtimeTone)

                Text(realtimeLabel)
                    .font(.caption2)
                    .foregroundStyle(realtimeTone)
            }
            .lineLimit(1)
            .minimumScaleFactor(0.8)
        }
        .padding(.bottom, 20)
    }

    // MARK: - Compteurs

    private func counters(_ summary: Summary) -> some View {
        HStack(alignment: .top, spacing: 24) {
            counter(
                label: "Critiques",
                value: summary.criticalCount,
                detail: summary.criticalUnacknowledged == 0
                    ? "Tous acquittés"
                    : "\(summary.criticalUnacknowledged) non acquitté\(summary.criticalUnacknowledged > 1 ? "s" : "")",
                tone: AppTheme.accent,
                valueTone: summary.criticalCount > 0 ? AppTheme.criticalDisplay : AppTheme.ink,
                labelTone: AppTheme.accent
            )

            Rectangle()
                .fill(AppTheme.hairline)
                .frame(width: 1)

            counter(
                label: "Avertis.",
                value: summary.warningCount,
                detail: summary.warningUnacknowledged == 0
                    ? "Tous acquittés"
                    : "\(summary.warningUnacknowledged) non acquitté\(summary.warningUnacknowledged > 1 ? "s" : "")",
                tone: AppTheme.warning,
                valueTone: AppTheme.ink,
                labelTone: AppTheme.inkFaint
            )
        }
        .fixedSize(horizontal: false, vertical: true)
        .padding(.bottom, 24)
    }

    private func counter(
        label: String,
        value: Int,
        detail: String,
        tone: Color,
        valueTone: Color,
        labelTone: Color
    ) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 6) {
                Circle()
                    .fill(tone)
                    .frame(width: 7, height: 7)

                Text(label.uppercased())
                    .font(AppTheme.sectionLabelFont)
                    .tracking(AppTheme.labelTracking)
                    .foregroundStyle(labelTone)
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)
            }

            DisplayNumber(value: "\(value)", tone: valueTone)
                .padding(.top, 8)
                .padding(.bottom, 4)

            Text(detail)
                .font(AppTheme.metaFont)
                .foregroundStyle(AppTheme.inkMuted)
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }

    // MARK: - Flotte

    private func fleet(_ summary: Summary) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionLabel("Flotte", detail: fleetDetail(summary))

            FleetBar(segments: [
                .init(id: "ok", label: "saines", count: summary.healthyCount, tone: AppTheme.ok),
                .init(id: "degraded", label: "dégradées", count: summary.degradedCount, tone: AppTheme.warning),
                .init(id: "down", label: "injoignables", count: summary.downCount, tone: AppTheme.critical),
            ])
        }
        .padding(.top, 16)
        .padding(.bottom, 22)
        .hairlineTop()
    }

    private func fleetDetail(_ summary: Summary) -> String {
        let targets = summary.targetCount == 1 ? "1 cible" : "\(summary.targetCount) cibles"
        guard let availability = summary.availability else {
            return targets
        }
        return "\(targets) · \(availability.formatted(.percent.precision(.fractionLength(2)))) dispo."
    }

    // MARK: - Incidents

    @ViewBuilder
    private func incidents(_ summary: Summary) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionLabel("Incidents actifs") {
                Button {
                    selectTab(.incidents)
                } label: {
                    Text("Tout voir")
                        .font(AppTheme.metaFont.weight(.semibold))
                        .foregroundStyle(AppTheme.accent)
                }
                .buttonStyle(.plain)
            }
            .padding(.bottom, 4)

            if summary.highlighted.isEmpty {
                Text("Aucun incident ne demande d’action.")
                    .font(.subheadline)
                    .foregroundStyle(AppTheme.inkMuted)
                    .padding(.vertical, 14)
            } else {
                ForEach(summary.highlighted) { incident in
                    NavigationLink {
                        IncidentDetailView(incidentID: incident.id)
                    } label: {
                        IncidentRow(incident: incident)
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(.top, 16)
        .hairlineTop()
    }

    // MARK: - Contexte

    @ViewBuilder
    private var pinnedIndicators: some View {
        PinnedIndicatorsSection(
            projections: Array(model.snapshot.indicatorTargets.values),
            targetName: { targetID in
                model.target(withID: targetID)?.name ?? "Cible"
            }
        )
    }

    @ViewBuilder
    private func loaded(_ summary: Summary) -> some View {
        if !summary.loaded.isEmpty {
            VStack(alignment: .leading, spacing: 0) {
                SectionLabel("Cibles les plus chargées", detail: "processeur")
                    .padding(.bottom, 4)

                ForEach(summary.loaded, id: \.target.id) { entry in
                    NavigationLink {
                        TargetDetailView(targetID: entry.target.id)
                    } label: {
                        LoadRow(
                            title: entry.target.name,
                            ratio: entry.value / 100,
                            value: entry.value.formatted(.number.precision(.fractionLength(0))) + " %",
                            tone: loadTone(entry.value)
                        )
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.top, 16)
            .hairlineTop()
        }
    }

    private func loadTone(_ value: Double) -> Color {
        if value >= 90 {
            return AppTheme.critical
        }
        if value >= 75 {
            return AppTheme.warning
        }
        return AppTheme.ok
    }

    private var emptyState: some View {
        ContentUnavailableView(
            "Aucune cible active",
            systemImage: "dot.scope",
            description: Text("La projection mobile apparaîtra ici dès qu’une première cible sera supervisée.")
        )
        .frame(maxWidth: .infinity)
        .padding(.top, 48)
    }

    /// « lundi 24 août · 10:00 » : la maquette date la projection autant
    /// qu'elle l'heure.
    private static let identityDateStyle = Date.FormatStyle(
        date: .complete,
        time: .shortened
    )
    .locale(Locale(identifier: "fr_FR"))
    .year(.omitted)

    private var realtimeLabel: String {
        switch model.realtimeState {
        case .offline:
            "Flux hors ligne"
        case .connecting:
            "Connexion"
        case .online:
            "Flux en ligne"
        }
    }

    private var realtimeTone: Color {
        switch model.realtimeState {
        case .offline:
            AppTheme.inkMuted
        case .connecting:
            AppTheme.warningInk
        case .online:
            AppTheme.okInk
        }
    }

    private var realtimeSymbol: String {
        switch model.realtimeState {
        case .offline:
            "wifi.slash"
        case .connecting:
            "wifi.exclamationmark"
        case .online:
            "wifi"
        }
    }
}
