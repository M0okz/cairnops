import SwiftUI

struct HealthView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Group {
            if let health = model.snapshot.systemHealth {
                ScrollView {
                    VStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                ResponsiveStatusHeader(
                                    text: health.status == "operational" ? "Opérationnelle" : "Dégradée",
                                    color: health.status == "operational" ? AppTheme.ok : AppTheme.warning,
                                    systemImage: health.status == "operational" ? "heart.text.square.fill" : "waveform.path.ecg"
                                ) {
                                    Text("État global")
                                        .font(AppTheme.sectionTitleFont)
                                }
                                Text("Projection évaluée \(TimestampParser.relativeString(from: health.checkedAt)).")
                                    .foregroundStyle(.secondary)

                                MetricGrid {
                                    MetricTile(
                                        title: "Observations 24 h",
                                        value: "\(conclusiveObservations24h)",
                                        subtitle: expectedObservations24h == 0 ? "Aucune attendue" : "\(expectedObservations24h) attendues",
                                        tone: AppTheme.info
                                    )
                                    MetricTile(
                                        title: "Latence max",
                                        value: "\(health.database.maximumLatencyMilliseconds.formatted(.number.precision(.fractionLength(1)))) ms",
                                        subtitle: "Base de données",
                                        tone: AppTheme.warning
                                    )
                                }
                            }
                        }

                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                Text("Composants")
                                    .font(AppTheme.sectionTitleFont)

                                ForEach(health.components) { component in
                                    ComponentHealthRow(
                                        title: componentLabel(component.name),
                                        statusText: componentStatusText(component.status),
                                        statusColor: component.status == "operational" ? AppTheme.ok : AppTheme.warning,
                                        statusSymbol: component.status == "operational" ? "checkmark.circle.fill" : "exclamationmark.triangle.fill",
                                        detail: component.instances == 1 ? "1 instance" : "\(component.instances) instances"
                                    )
                                }
                            }
                        }

                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                Text("PostgreSQL")
                                    .font(AppTheme.sectionTitleFont)

                                MetricGrid {
                                    MetricTile(
                                        title: "Latence courante",
                                        value: "\(health.database.latencyMilliseconds.formatted(.number.precision(.fractionLength(1)))) ms",
                                        subtitle: "Mesurée \(TimestampParser.relativeString(from: health.database.measuredSince))",
                                        tone: AppTheme.ok
                                    )
                                    MetricTile(
                                        title: "Maximum observé",
                                        value: "\(health.database.maximumLatencyMilliseconds.formatted(.number.precision(.fractionLength(1)))) ms",
                                        subtitle: "Depuis la dernière fenêtre",
                                        tone: AppTheme.warning
                                    )
                                }
                            }
                        }
                    }
                    .padding(AppTheme.screenPadding)
                    .padding(.bottom, AppTheme.bottomScrollInset)
                }
            } else {
                ContentUnavailableView(
                    "Projection santé indisponible",
                    systemImage: "waveform.path.ecg",
                    description: Text("Le serveur n’a pas renvoyé de vue santé exploitable pour le moment.")
                )
            }
        }
        .background(AppBackdrop())
        .navigationTitle("Santé")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                refreshButton
            }
        }
        .refreshable {
            await model.refresh()
        }
    }

    private var expectedObservations24h: Int {
        model.snapshot.systemHealth?.hours.reduce(0) { $0 + $1.expectedObservations } ?? 0
    }

    private var conclusiveObservations24h: Int {
        model.snapshot.systemHealth?.hours.reduce(0) { $0 + $1.conclusiveObservations } ?? 0
    }

    private var refreshButton: some View {
        AsyncButton {
            await model.refresh()
        } label: {
            Image(systemName: "arrow.clockwise")
        }
        .accessibilityLabel("Actualiser la santé")
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
