import SwiftUI

/// Ligne de preuve du detail d'Incident.
///
/// L'heure ancre la chronologie a gauche, le verdict porte la Gravite, et une
/// preuve invalidee reste visible, barree et attenuee : l'Invalidation est
/// motivee, elle n'efface pas l'historique.
struct EvidenceRow: View {
    let signal: Incident.Signal

    private var isInvalidated: Bool {
        signal.invalidatedAt != nil
    }

    private var tone: Color {
        if isInvalidated {
            return AppTheme.neutral
        }
        return signal.active ? AppTheme.severityInk(signal.severity) : AppTheme.okInk
    }

    private var time: String {
        guard let date = TimestampParser.date(from: signal.openedAt) else {
            return "--:--"
        }
        return date.formatted(.dateTime.hour().minute().locale(Locale(identifier: "fr_FR")))
    }

    private var detail: String {
        var parts: [String] = [originLabel]

        if let connector = signal.connectorName, !connector.isEmpty {
            parts.append(connector)
        }
        if let reason = signal.invalidationReason, !reason.isEmpty {
            parts.append("Invalidée : \(reason)")
        } else if let resolvedAt = signal.resolvedAt {
            parts.append("Résolue \(TimestampParser.relativeString(from: resolvedAt))")
        }

        return parts.joined(separator: " · ")
    }

    private var originLabel: String {
        switch signal.origin {
        case "native":
            "Contrôle natif"
        case "integration":
            "Intégration"
        case "webhook":
            "Signal entrant"
        default:
            signal.origin.capitalized
        }
    }

    private var stateLabel: String {
        if isInvalidated {
            return "Invalidée"
        }
        return signal.active ? "Active" : "Résolue"
    }

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Text(time)
                .font(AppTheme.metaFont.weight(.bold))
                .monospacedDigit()
                .foregroundStyle(AppTheme.inkMuted)
                .frame(width: 38, alignment: .leading)
                .padding(.top, 2)

            VStack(alignment: .leading, spacing: 3) {
                Text(signal.name)
                    .font(AppTheme.fieldValueFont)
                    .tracking(-0.15)
                    .foregroundStyle(tone)
                    .strikethrough(isInvalidated)
                    .fixedSize(horizontal: false, vertical: true)

                Text(detail)
                    .font(AppTheme.metaFont)
                    .foregroundStyle(AppTheme.inkMuted)
                    .fixedSize(horizontal: false, vertical: true)
            }

            Spacer(minLength: 8)

            Text(stateLabel)
                .font(AppTheme.metaFont.weight(.semibold))
                .foregroundStyle(isInvalidated ? AppTheme.inkMuted : AppTheme.inkStrong)
                .padding(.top, 2)
        }
        .padding(.vertical, 13)
        .opacity(isInvalidated ? 0.55 : 1)
        .frame(maxWidth: .infinity, alignment: .leading)
        .hairlineTop()
        .accessibilityElement(children: .combine)
    }
}
