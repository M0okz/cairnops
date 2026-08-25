import SwiftUI

/// Ligne d'Incident posee a nu, separee par un filet.
///
/// Elle porte la Gravite par sa pastille, la nature en tete, puis la Cible, une
/// metadonnee et la duree. Les Incidents acquittes sont attenues plutot que
/// masques : ils restent lisibles sans concurrencer ce qui demande une action.
struct IncidentRow: View {
    let incident: Incident

    /// Ligne de tete : l'Incident le plus recent porte un titre plus fort et
    /// l'action d'Acquittement directement dans la liste.
    var isLead = false
    var acknowledge: (@Sendable () async -> Void)?

    private var tone: Color {
        incident.isAcknowledged || incident.isResolved
            ? AppTheme.neutral
            : AppTheme.severityColor(incident.effectiveSeverity)
    }

    private var stateInk: Color {
        incident.isAcknowledged || incident.isResolved
            ? AppTheme.neutral
            : AppTheme.severityInk(incident.effectiveSeverity)
    }

    private var stateLabel: String {
        if incident.isResolved {
            return "RÉSOLU"
        }
        if incident.isAcknowledged {
            return "ACQ"
        }
        return AppTheme.severityShortLabel(incident.effectiveSeverity)
    }

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            StatusDot(tone: tone)
                .padding(.top, 5)

            VStack(alignment: .leading, spacing: 5) {
                title
                meta
            }
        }
        .padding(.vertical, 14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .opacity(incident.isAcknowledged && !isLead ? 0.6 : 1)
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var title: some View {
        if isLead {
            VStack(alignment: .leading, spacing: 5) {
                Text(incident.natureLabel)
                    .font(AppTheme.leadRowTitleFont)
                    .tracking(AppTheme.titleTracking)
                    .foregroundStyle(AppTheme.ink)
                    .fixedSize(horizontal: false, vertical: true)

                if let acknowledge, !incident.isAcknowledged, !incident.isResolved {
                    AsyncButton(action: acknowledge) {
                        HStack(spacing: 5) {
                            Image(systemName: "checkmark")
                                .font(.system(size: 14, weight: .bold))
                            Text("Acquitter")
                                .font(.footnote.weight(.bold))
                        }
                        .foregroundStyle(AppTheme.control)
                    }
                    .buttonStyle(.plain)
                }
            }
        } else {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(incident.natureLabel)
                    .font(AppTheme.rowTitleFont)
                    .tracking(-0.15)
                    .foregroundStyle(AppTheme.ink)
                    .fixedSize(horizontal: false, vertical: true)

                Spacer(minLength: 4)

                Text(TimestampParser.elapsedString(since: incident.openedAt))
                    .font(.caption2)
                    .monospacedDigit()
                    .foregroundStyle(AppTheme.inkMuted)
                    .layoutPriority(1)
            }
        }
    }

    private var meta: some View {
        // Le nom de Cible ne doit jamais disparaitre au profit de la nature :
        // il garde sa place et c'est la metadonnee qui se tronque.
        HStack(spacing: 7) {
            Text(incident.targetName)
                .font(AppTheme.metaFont.weight(.semibold))
                .foregroundStyle(AppTheme.inkStrong)
                .lineLimit(1)

            Circle()
                .fill(AppTheme.inkMuted)
                .frame(width: 3, height: 3)

            Text(detailLine)
                .font(AppTheme.metaFont)
                .foregroundStyle(AppTheme.inkMuted)
                .lineLimit(1)
                .truncationMode(.tail)

            Spacer(minLength: 6)

            Text(stateLabel)
                .font(.caption2.weight(.bold))
                .foregroundStyle(stateInk)
                .layoutPriority(1)
        }
    }

    private var detailLine: String {
        if isLead {
            return "\(incident.visibleSignalCountLabel) · \(TimestampParser.elapsedString(since: incident.openedAt))"
        }
        return incident.visibleSignalCountLabel
    }
}
