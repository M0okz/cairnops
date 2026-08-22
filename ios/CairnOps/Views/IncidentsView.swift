import SwiftUI

struct IncidentsView: View {
    @Environment(AppModel.self) private var model
    @State private var query = ""

    /// Le partitionnement etait auparavant reparti sur trois proprietes
    /// calculees qui refiltraient chacune la liste complete a chaque acces, soit
    /// plusieurs parcours par rendu. Un seul passage suffit.
    private struct Partition {
        var active: [Incident] = []
        var resolved: [Incident] = []

        var isEmpty: Bool { active.isEmpty && resolved.isEmpty }
    }

    private func makePartition(from snapshot: AppSnapshot) -> Partition {
        var result = Partition()

        for incident in snapshot.incidents {
            if !query.isEmpty,
               !incident.targetName.localizedStandardContains(query),
               !incident.natureLabel.localizedStandardContains(query) {
                continue
            }

            if incident.isResolved {
                result.resolved.append(incident)
            } else {
                result.active.append(incident)
            }
        }

        return result
    }

    var body: some View {
        let partition = makePartition(from: model.snapshot)

        return ScrollView {
            LazyVStack(alignment: .leading, spacing: 10) {
                Text("\(partition.active.count) actifs · \(partition.resolved.count) résolus")
                    .font(.footnote)
                    .foregroundStyle(.secondary)

                if partition.isEmpty {
                    Group {
                        if query.isEmpty {
                            ContentUnavailableView(
                                "Aucun incident",
                                systemImage: "checkmark.shield",
                                description: Text("Les incidents actifs et résolus s’afficheront ici.")
                            )
                        } else {
                            ContentUnavailableView.search
                        }
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.top, 64)
                } else {
                    if !partition.active.isEmpty {
                        sectionHeader("Actifs", count: partition.active.count)

                        ForEach(partition.active) { incident in
                            NavigationLink {
                                IncidentDetailView(incidentID: incident.id)
                            } label: {
                                IncidentRow(incident: incident, isStandalone: true)
                            }
                            .buttonStyle(.plain)
                        }
                    }

                    if !partition.resolved.isEmpty {
                        sectionHeader("Résolus", count: partition.resolved.count)
                            .padding(.top, 8)

                        ForEach(partition.resolved) { incident in
                            NavigationLink {
                                IncidentDetailView(incidentID: incident.id)
                            } label: {
                                IncidentRow(incident: incident, isStandalone: true)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
            }
            .padding(AppTheme.screenPadding)
            .padding(.bottom, AppTheme.bottomScrollInset)
        }
        .background(AppBackdrop())
        .navigationTitle("Incidents")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                refreshButton
            }
        }
        .searchable(text: $query, prompt: "Cible ou nature")
        .scrollDismissesKeyboard(.interactively)
        .refreshable {
            await model.refresh()
        }
    }

    @ViewBuilder
    private func sectionHeader(_ title: String, count: Int) -> some View {
        HStack(alignment: .firstTextBaseline) {
            Text(title)
                .font(AppTheme.sectionTitleFont)
            Spacer()
            Text(count == 1 ? "1 incident" : "\(count) incidents")
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
        .padding(.top, 4)
    }

    private var refreshButton: some View {
        AsyncButton {
            await model.refresh()
        } label: {
            Image(systemName: "arrow.clockwise")
        }
        .accessibilityLabel("Actualiser les incidents")
    }
}
