import SwiftUI

struct TargetsView: View {
    @Environment(AppModel.self) private var model
    @State private var query = ""

    private var filteredTargets: [Target] {
        let sorted = model.snapshot.sortedTargets
        guard !query.isEmpty else {
            return sorted
        }

        return sorted.filter {
            $0.name.localizedStandardContains(query) ||
            $0.description.localizedStandardContains(query)
        }
    }

    var body: some View {
        let targets = filteredTargets

        return Group {
            if targets.isEmpty {
                if query.isEmpty {
                    ContentUnavailableView(
                        "Aucune cible",
                        systemImage: "dot.scope",
                        description: Text("Les cibles supervisées apparaîtront ici avec leur santé et leur fraîcheur.")
                    )
                } else {
                    ContentUnavailableView.search
                }
            } else {
                ScrollView {
                    // `LazyVStack` ne construit que les lignes visibles : au-dela
                    // de quelques dizaines de cibles, `VStack` instanciait toute
                    // la liste a chaque rendu.
                    LazyVStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                        ScreenHeader(
                            title: "Cibles",
                            subtitle: "\(targets.count) visibles · \(model.snapshot.targets.count) au total"
                        ) {
                            refreshButton
                        }

                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                ForEach(targets) { target in
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

                                    if target.id != targets.last?.id {
                                        Divider()
                                    }
                                }
                            }
                        }
                    }
                    .padding(AppTheme.screenPadding)
                    .padding(.bottom, AppTheme.bottomScrollInset)
                }
            }
        }
        .background(AppBackdrop())
        .toolbar(.hidden, for: .navigationBar)
        .searchable(text: $query, prompt: "Nom ou description")
    }

    private var refreshButton: some View {
        AsyncButton {
            await model.refresh()
        } label: {
            RefreshGlyph()
        }
        .buttonStyle(.plain)
        .foregroundStyle(AppTheme.accent)
        .accessibilityLabel("Actualiser les cibles")
    }
}
