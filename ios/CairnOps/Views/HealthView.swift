import SwiftUI

/// Sante de l'installation CairnOps elle-meme.
///
/// L'ecran ne parle pas des Cibles supervisees mais du produit : ses composants,
/// sa base de donnees et la couverture d'observation des dernieres 24 heures.
struct HealthView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Group {
            if let health = model.snapshot.systemHealth {
                content(health)
            } else {
                BareScreen {
                    PageTitle("Santé")
                        .padding(.bottom, 20)

                    ContentUnavailableView(
                        "Projection santé indisponible",
                        systemImage: "waveform.path.ecg",
                        description: Text("Le serveur n’a pas renvoyé de vue santé exploitable pour le moment.")
                    )
                    .frame(maxWidth: .infinity)
                    .padding(.top, 48)
                }
            }
        }
        .refreshable {
            await model.refresh()
        }
    }

    private func content(_ health: SystemHealth) -> some View {
        let isOperational = health.status == "operational"

        return BareScreen {
            PageTitle("Santé", detail: TimestampParser.relativeString(from: health.checkedAt)) {
                HStack(spacing: 6) {
                    Circle()
                        .fill(isOperational ? AppTheme.ok : AppTheme.warning)
                        .frame(width: 7, height: 7)

                    Text(isOperational ? "Opérationnelle" : "Dégradée")
                        .font(.caption2.weight(.bold))
                        .foregroundStyle(isOperational ? AppTheme.okInk : AppTheme.warningInk)
                }
                .lineLimit(1)
            }
            .padding(.bottom, 20)

            observations(health)
            components(health)
            database(health)
        }
    }

    // MARK: - Observations

    private func observations(_ health: SystemHealth) -> some View {
        let conclusive = health.hours.reduce(0) { $0 + $1.conclusiveObservations }
        let expected = health.hours.reduce(0) { $0 + $1.expectedObservations }
        let coverage = expected == 0 ? nil : Double(conclusive) / Double(expected)

        return LazyVGrid(
            columns: [GridItem(.flexible(), spacing: 20, alignment: .topLeading),
                      GridItem(.flexible(), spacing: 20, alignment: .topLeading)],
            alignment: .leading,
            spacing: 0
        ) {
            MetricCell(
                label: "Observations 24 h",
                value: conclusive.formatted(.number),
                unit: expected == 0 ? "aucune attendue" : "sur \(expected.formatted(.number))",
                tone: AppTheme.ink,
                series: health.hours.map { Double($0.conclusiveObservations) }
            )

            MetricCell(
                label: "Couverture",
                value: coverage.map { $0.formatted(.percent.precision(.fractionLength(0))) } ?? "—",
                tone: coverageTone(coverage),
                series: health.hours.compactMap { hour in
                    hour.expectedObservations == 0
                        ? nil
                        : Double(hour.conclusiveObservations) / Double(hour.expectedObservations)
                },
                lowerBound: 0,
                upperBound: 1
            )
        }
    }

    private func coverageTone(_ coverage: Double?) -> Color {
        guard let coverage else {
            return AppTheme.inkMuted
        }
        if coverage < 0.9 {
            return AppTheme.criticalInk
        }
        if coverage < 0.98 {
            return AppTheme.warningInk
        }
        return AppTheme.ink
    }

    // MARK: - Composants

    private func components(_ health: SystemHealth) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionLabel("Composants", detail: "\(health.components.count)")
                .padding(.top, 18)
                .padding(.bottom, 2)

            ForEach(health.components) { component in
                MetaRow(
                    title: componentLabel(component.name),
                    subtitle: component.instances == 1 ? "1 instance" : "\(component.instances) instances",
                    state: componentStatusText(component.status),
                    tone: componentTone(component.status),
                    stateInk: componentInk(component.status)
                )
            }
        }
        .padding(.top, 18)
        .hairlineTop()
    }

    // MARK: - Base de donnees

    private func database(_ health: SystemHealth) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionLabel("PostgreSQL", detail: "mesurée \(TimestampParser.relativeString(from: health.database.measuredSince))")
                .padding(.top, 18)
                .padding(.bottom, 2)

            LazyVGrid(
                columns: [GridItem(.flexible(), spacing: 20, alignment: .topLeading),
                          GridItem(.flexible(), spacing: 20, alignment: .topLeading)],
                alignment: .leading,
                spacing: 0
            ) {
                MetricCell(
                    label: "Latence",
                    value: health.database.latencyMilliseconds.formatted(.number.precision(.fractionLength(1))),
                    unit: "ms",
                    tone: AppTheme.ink
                )

                MetricCell(
                    label: "Maximum",
                    value: health.database.maximumLatencyMilliseconds.formatted(.number.precision(.fractionLength(1))),
                    unit: "ms",
                    tone: AppTheme.warningInk
                )
            }
        }
        .padding(.top, 18)
        .hairlineTop()
    }

    // MARK: - Libelles

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

    private func componentTone(_ value: String) -> Color {
        switch value {
        case "operational":
            AppTheme.ok
        case "stale":
            AppTheme.warning
        default:
            AppTheme.critical
        }
    }

    private func componentInk(_ value: String) -> Color {
        switch value {
        case "operational":
            AppTheme.okInk
        case "stale":
            AppTheme.warningInk
        default:
            AppTheme.criticalInk
        }
    }
}
