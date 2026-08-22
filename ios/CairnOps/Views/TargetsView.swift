import SwiftUI

struct TargetsView: View {
    @Environment(AppModel.self) private var model
    @State private var query = ""

    private func filteredTargets(in snapshot: AppSnapshot) -> [Target] {
        let sorted = snapshot.sortedTargets
        guard !query.isEmpty else {
            return sorted
        }

        return sorted.filter {
            $0.name.localizedStandardContains(query) ||
            $0.description.localizedStandardContains(query)
        }
    }

    var body: some View {
        let snapshot = model.snapshot
        let targets = filteredTargets(in: snapshot)

        return ScrollView {
            // Les lignes sont des enfants directs du `LazyVStack`. Les placer
            // dans un `Panel` contenant un `VStack` construisait les 1 000
            // cibles d'un coup et annulait entierement la paresse du conteneur.
            LazyVStack(alignment: .leading, spacing: 10) {
                Text("\(targets.count) visibles · \(snapshot.targets.count) au total")
                    .font(.footnote)
                    .foregroundStyle(.secondary)

                if targets.isEmpty {
                    Group {
                        if query.isEmpty {
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
                    .padding(.top, 64)
                } else {
                    ForEach(targets) { target in
                        NavigationLink {
                            TargetDetailView(targetID: target.id)
                        } label: {
                            TargetRow(
                                target: target,
                                health: snapshot.health(for: target),
                                measures: snapshot.measures[target.id],
                                isStandalone: true
                            )
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
            .padding(AppTheme.screenPadding)
            .padding(.bottom, AppTheme.bottomScrollInset)
        }
        .background(AppBackdrop())
        .navigationTitle("Cibles")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                refreshButton
            }
        }
        .searchable(text: $query, prompt: "Nom ou description")
        .scrollDismissesKeyboard(.interactively)
        .refreshable {
            await model.refresh()
        }
    }

    private var refreshButton: some View {
        AsyncButton {
            await model.refresh()
        } label: {
            Image(systemName: "arrow.clockwise")
        }
        .accessibilityLabel("Actualiser les cibles")
    }
}
