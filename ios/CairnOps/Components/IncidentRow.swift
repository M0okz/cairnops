import SwiftUI

struct IncidentRow: View {
    let incident: Incident
    var isStandalone = false

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            RoundedRectangle(cornerRadius: 12)
                .fill(AppTheme.severityColor(incident.effectiveSeverity))
                .frame(width: 5)

            VStack(alignment: .leading, spacing: 8) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(incident.targetName)
                        .font(AppTheme.rowTitleFont)
                        .foregroundStyle(.primary)
                    Text(incident.primaryEvidenceLabel)
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }

                ViewThatFits(in: .horizontal) {
                    HStack(spacing: 10) {
                        metaLabel("clock", TimestampParser.relativeString(from: incident.openedAt))
                        metaLabel("waveform.path.ecg", incident.visibleSignalCountLabel)
                        if incident.isAcknowledged {
                            metaLabel("checkmark.circle", "Acquitté")
                        }
                    }

                    VStack(alignment: .leading, spacing: 6) {
                        HStack(spacing: 10) {
                            metaLabel("clock", TimestampParser.relativeString(from: incident.openedAt))
                            metaLabel("waveform.path.ecg", incident.visibleSignalCountLabel)
                        }
                        if incident.isAcknowledged {
                            metaLabel("checkmark.circle", "Acquitté")
                        }
                    }
                }

                if incident.acknowledgementSyncStatus == "failed" {
                    Text("Propagation distante en échec, reprise manuelle possible.")
                        .font(.footnote)
                        .foregroundStyle(AppTheme.warning)
                }
            }

            Spacer(minLength: 12)

            VStack(alignment: .trailing, spacing: 10) {
                StatusPill(
                    text: incident.effectiveSeverity.label,
                    color: AppTheme.severityColor(incident.effectiveSeverity),
                    systemImage: AppTheme.severitySymbol(incident.effectiveSeverity)
                )

                Image(systemName: "chevron.right")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.tertiary)
            }
        }
        .padding(12)
        .background(
            RoundedRectangle(cornerRadius: 16)
                .fill(isStandalone ? AppTheme.panel : AppTheme.subpanel)
                .overlay(
                    RoundedRectangle(cornerRadius: 16)
                        .strokeBorder(isStandalone ? AppTheme.line : .clear)
                )
        )
        .contentShape(.rect(cornerRadius: 16))
    }

    private func metaLabel(_ systemImage: String, _ text: String) -> some View {
        Label(text, systemImage: systemImage)
            .font(.footnote)
            .foregroundStyle(.secondary)
            .lineLimit(1)
            .minimumScaleFactor(0.85)
    }
}
