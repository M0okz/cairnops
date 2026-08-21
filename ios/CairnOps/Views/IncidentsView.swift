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

    private var partition: Partition {
        var result = Partition()

        for incident in model.snapshot.incidents {
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
        let partition = partition

        return Group {
            if partition.isEmpty {
                if query.isEmpty {
                    ContentUnavailableView(
                        "Aucun incident",
                        systemImage: "checkmark.shield",
                        description: Text("Les incidents actifs et résolus s’afficheront ici.")
                    )
                } else {
                    ContentUnavailableView.search
                }
            } else {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                        ScreenHeader(
                            title: "Incidents",
                            subtitle: "\(partition.active.count) actifs · \(partition.resolved.count) résolus"
                        ) {
                            refreshButton
                        }

                        if !partition.active.isEmpty {
                            Panel {
                                incidentSection("Actifs", incidents: partition.active)
                            }
                        }

                        if !partition.resolved.isEmpty {
                            Panel {
                                incidentSection("Résolus", incidents: partition.resolved)
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
        .searchable(text: $query, prompt: "Cible ou nature")
    }

    @ViewBuilder
    private func incidentSection(_ title: String, incidents: [Incident]) -> some View {
        VStack(alignment: .leading, spacing: 14) {
            Text(title)
                .font(.title3.bold())

            ForEach(incidents) { incident in
                NavigationLink {
                    IncidentDetailView(incidentID: incident.id)
                } label: {
                    IncidentRow(incident: incident)
                }
                .buttonStyle(.plain)

                if incident.id != incidents.last?.id {
                    Divider()
                }
            }
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
        .accessibilityLabel("Actualiser les incidents")
    }
}
