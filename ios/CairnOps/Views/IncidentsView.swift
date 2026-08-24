import SwiftUI

/// Journal des Incidents.
///
/// Le titre partage sa ligne avec le compteur, les filtres d'etat suivent
/// immediatement, et les Incidents sont regroupes par age : le haut de page
/// devient utile au lieu d'etre decoratif.
struct IncidentsView: View {
    @Environment(AppModel.self) private var model

    enum Scope: Hashable {
        case active
        case acknowledged
        case all
    }

    /// Tranches d'age de la liste, du plus recent au plus ancien.
    private enum Bucket: String, CaseIterable, Identifiable {
        case lastHour
        case today
        case earlier

        var id: String { rawValue }

        var title: String {
            switch self {
            case .lastHour:
                "Moins d’une heure"
            case .today:
                "Moins d’un jour"
            case .earlier:
                "Plus tôt"
            }
        }
    }

    @State private var scope: Scope = .active
    @State private var query = ""
    @State private var severityFilter: IncidentSeverity?

    private struct Listing {
        var buckets: [Bucket: [Incident]] = [:]
        var activeCount = 0
        var acknowledgedCount = 0
        var totalCount = 0
        var isEmpty = true
    }

    /// Un seul parcours produit les compteurs d'onglets et le classement par
    /// age. Les repartir sur plusieurs proprietes calculees refiltrait la liste
    /// complete a chaque acces, plusieurs fois par rendu.
    private func makeListing() -> Listing {
        var listing = Listing()
        let now = Date.now

        for incident in model.snapshot.incidents {
            if !incident.isResolved {
                if incident.isAcknowledged {
                    listing.acknowledgedCount += 1
                } else {
                    listing.activeCount += 1
                }
            }
            listing.totalCount += 1

            guard matchesScope(incident) else {
                continue
            }
            if let severityFilter, incident.effectiveSeverity != severityFilter {
                continue
            }
            if !query.isEmpty,
               !incident.targetName.localizedStandardContains(query),
               !incident.natureLabel.localizedStandardContains(query) {
                continue
            }

            listing.buckets[bucket(for: incident, now: now), default: []].append(incident)
            listing.isEmpty = false
        }

        return listing
    }

    private func matchesScope(_ incident: Incident) -> Bool {
        switch scope {
        case .active:
            !incident.isResolved && !incident.isAcknowledged
        case .acknowledged:
            !incident.isResolved && incident.isAcknowledged
        case .all:
            true
        }
    }

    private func bucket(for incident: Incident, now: Date) -> Bucket {
        guard let opened = TimestampParser.date(from: incident.openedAt) else {
            return .earlier
        }
        let age = now.timeIntervalSince(opened)
        if age < 3_600 {
            return .lastHour
        }
        if age < 86_400 {
            return .today
        }
        return .earlier
    }

    var body: some View {
        let listing = makeListing()

        return BareScreen {
            UnderlineTabs(
                selection: $scope,
                items: [
                    .init(Scope.active, "Actifs", count: listing.activeCount),
                    .init(Scope.acknowledged, "Acquittés", count: listing.acknowledgedCount),
                    .init(Scope.all, "Tous", count: listing.totalCount),
                ]
            )
            .padding(.top, 4)

            severityFilters
                .padding(.vertical, 14)

            SearchField(text: $query, prompt: "Cible ou nature")
                .padding(.bottom, 4)

            if listing.isEmpty {
                emptyState
            } else {
                ForEach(Bucket.allCases) { bucket in
                    section(bucket, incidents: listing.buckets[bucket] ?? [])
                }
            }
        }
        .navigationTitle("Incidents")
        .navigationSubtitle(headerDetail(listing))
        .navigationBarTitleDisplayMode(.inline)
        .refreshable {
            await model.refresh()
        }
    }

    @ViewBuilder
    private func section(_ bucket: Bucket, incidents: [Incident]) -> some View {
        if !incidents.isEmpty {
            // Le decoupage par age reste marque d'un filet : c'est lui qui
            // structure la lecture. Un filet sous chaque Incident en plus
            // saturait la page sans rien separer d'utile.
            SectionLabel(bucket.title)
                .padding(.top, 20)
                .padding(.bottom, 4)
                .hairlineTop()

            ForEach(Array(incidents.enumerated()), id: \.element.id) { index, incident in
                NavigationLink {
                    IncidentDetailView(incidentID: incident.id)
                } label: {
                    IncidentRow(
                        incident: incident,
                        // Le premier Incident de la tranche la plus recente
                        // porte le titre fort et l'Acquittement en ligne : c'est
                        // celui qui appelle une action immediate.
                        isLead: isLead(bucket: bucket, index: index, incident: incident),
                        acknowledge: acknowledgement(for: incident)
                    )
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func isLead(bucket: Bucket, index: Int, incident: Incident) -> Bool {
        bucket == .lastHour
            && index == 0
            && !incident.isAcknowledged
            && !incident.isResolved
    }

    private func acknowledgement(for incident: Incident) -> (@Sendable () async -> Void)? {
        guard model.canMutate else {
            return nil
        }
        let identifier = incident.id
        return { await model.acknowledge(incidentID: identifier) }
    }

    private var severityFilters: some View {
        HStack(spacing: 14) {
            ForEach([IncidentSeverity.critical, .major, .warning], id: \.self) { severity in
                FilterChip(
                    title: severity.label,
                    isActive: severityFilter == severity
                ) {
                    severityFilter = severityFilter == severity ? nil : severity
                }
            }

            Spacer(minLength: 0)
        }
    }

    private func headerDetail(_ listing: Listing) -> String {
        let total = listing.activeCount + listing.acknowledgedCount
        let subject = total == 1 ? "1 en cours" : "\(total) en cours"
        return "\(subject) · \(listing.acknowledgedCount) acquittés"
    }

    private var emptyState: some View {
        Group {
            if query.isEmpty && severityFilter == nil {
                ContentUnavailableView(
                    "Aucun incident",
                    systemImage: "checkmark.shield",
                    description: Text("Les incidents actifs et acquittés s’afficheront ici.")
                )
            } else {
                ContentUnavailableView.search
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.top, 56)
    }
}
