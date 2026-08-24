import SwiftUI

/// Inventaire des Cibles supervisees.
///
/// La maquette groupe les Cibles par zone d'infrastructure. L'API ne porte pas
/// cette notion : le regroupement suit ici l'Etat de sante, ce qui respecte la
/// hierarchie orientee exceptions et l'ordre de tri deja etabli par la
/// projection.
struct TargetsView: View {
    @Environment(AppModel.self) private var model

    enum Scope: Hashable {
        case all
        case degraded
        case down
    }

    @State private var scope: Scope = .all
    @State private var query = ""

    private struct HealthGroup: Identifiable {
        let health: AppSnapshot.TargetHealth
        var targets: [Target] = []

        var id: String { health.rawValue }

        var title: String {
            switch health {
            case .down:
                "Indisponibles"
            case .degraded:
                "Dégradées"
            case .maintenance:
                "En maintenance"
            case .unknown:
                "Sans mesure"
            case .ok:
                "Opérationnelles"
            }
        }
    }

    private struct Listing {
        var groups: [HealthGroup] = []
        var degradedCount = 0
        var downCount = 0
        var visibleCount = 0
    }

    /// L'ordre des sections reprend celui du tri de la projection : le plus
    /// grave en premier, jamais l'ordre alphabetique.
    private static let order: [AppSnapshot.TargetHealth] = [.down, .degraded, .maintenance, .unknown, .ok]

    private func makeListing() -> Listing {
        let snapshot = model.snapshot
        var listing = Listing()
        var byHealth: [AppSnapshot.TargetHealth: [Target]] = [:]

        for target in snapshot.sortedTargets {
            let health = snapshot.health(for: target)

            switch health {
            case .degraded, .maintenance:
                listing.degradedCount += 1
            case .down:
                listing.downCount += 1
            case .ok, .unknown:
                break
            }

            guard matchesScope(health) else {
                continue
            }
            if !query.isEmpty,
               !target.name.localizedStandardContains(query),
               !target.description.localizedStandardContains(query) {
                continue
            }

            byHealth[health, default: []].append(target)
            listing.visibleCount += 1
        }

        listing.groups = Self.order.compactMap { health in
            guard let targets = byHealth[health], !targets.isEmpty else {
                return nil
            }
            return HealthGroup(health: health, targets: targets)
        }

        return listing
    }

    private func matchesScope(_ health: AppSnapshot.TargetHealth) -> Bool {
        switch scope {
        case .all:
            true
        case .degraded:
            health == .degraded || health == .maintenance
        case .down:
            health == .down
        }
    }

    var body: some View {
        let listing = makeListing()

        return BareScreen {
            SearchField(text: $query, prompt: "Nom ou description")
                .padding(.top, 8)

            UnderlineTabs(
                selection: $scope,
                items: [
                    .init(Scope.all, "Toutes"),
                    .init(Scope.degraded, "Dégradées", count: listing.degradedCount),
                    .init(Scope.down, "HS", count: listing.downCount),
                ]
            )
            .padding(.top, 16)

            if listing.groups.isEmpty {
                emptyState
            } else {
                ForEach(listing.groups) { group in
                    section(group)
                }
            }
        }
        .navigationTitle("Cibles")
        .navigationSubtitle(supervisedLabel)
        .navigationBarTitleDisplayMode(.inline)
        .refreshable {
            await model.refresh()
        }
    }

    /// Un filet par groupe suffit.
    ///
    /// Un filet sous chaque ligne hachait la page en autant de bandes que de
    /// Cibles : sur une flotte de cent cibles, la separation devenait le motif
    /// dominant. Seul le changement d'Etat de sante merite une ligne.
    @ViewBuilder
    private func section(_ group: HealthGroup) -> some View {
        SectionLabel(group.title, detail: countLabel(group.targets.count))
            .padding(.top, 20)
            .padding(.bottom, 6)
            .hairlineTop()

        ForEach(group.targets) { target in
            NavigationLink {
                TargetDetailView(targetID: target.id)
            } label: {
                TargetRow(
                    target: target,
                    health: group.health,
                    measures: model.snapshot.measures[target.id],
                    indicators: model.snapshot.indicatorTargets[target.id]
                )
            }
            .buttonStyle(.plain)
        }
    }

    private func countLabel(_ count: Int) -> String {
        count == 1 ? "1 cible" : "\(count) cibles"
    }

    private var supervisedLabel: String {
        let count = model.snapshot.targets.count
        return count == 1 ? "1 supervisée" : "\(count) supervisées"
    }

    private var emptyState: some View {
        Group {
            if query.isEmpty && scope == .all {
                ContentUnavailableView(
                    "Aucune cible",
                    systemImage: "dot.scope",
                    description: Text("Les cibles supervisées apparaîtront ici avec leur santé et leur fraîcheur.")
                )
            } else {
                ContentUnavailableView.search
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.top, 56)
    }
}
