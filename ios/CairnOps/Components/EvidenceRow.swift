import SwiftUI

/// Preuve d'un Incident, posee sur la chronologie.
///
/// Une preuve invalidee reste visible, barree et attenuee : l'Invalidation est
/// motivee, elle n'efface pas l'historique.
struct EvidenceRow: View {
    let signal: Incident.Signal
    var isFirst = false
    var isLast = false

    private var isInvalidated: Bool {
        signal.invalidatedAt != nil
    }

    private var tone: Color {
        if isInvalidated {
            return AppTheme.neutral
        }
        return signal.active ? AppTheme.severityInk(signal.severity) : AppTheme.okInk
    }

    private var symbol: String {
        if isInvalidated {
            return "xmark"
        }
        return signal.active ? "exclamationmark" : "checkmark"
    }

    private var time: String {
        guard let date = TimestampParser.date(from: signal.openedAt) else {
            return "--:--"
        }
        return date.formatted(.dateTime.hour().minute().locale(AppLanguage.currentLocale))
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

    var body: some View {
        TimelineRow(
            systemImage: symbol,
            tone: tone,
            time: time,
            isFirst: isFirst,
            isLast: isLast,
            isMuted: isInvalidated
        ) {
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
    }
}

/// Entree du journal d'activite, posee sur la meme chronologie.
struct ActivityRow: View {
    let activity: Incident.Activity
    var isFirst = false
    var isLast = false

    /// Chaque transition significative porte son propre repere : le journal se
    /// parcourt alors du regard sans lire chaque intitule.
    private var symbol: String {
        switch activity.kind {
        case "opened":
            "exclamationmark"
        case "acknowledged", "upstream_acknowledged":
            "checkmark"
        case "resolved":
            "checkmark.seal"
        case "invalidated":
            "xmark"
        case "signal_added":
            "plus"
        case "signal_resolved":
            "minus"
        case "target_reconciled", "source_reassigned", "source_moved", "reconciled":
            "arrow.triangle.merge"
        default:
            "circle"
        }
    }

    private var tone: Color {
        switch activity.kind {
        case "opened", "signal_added":
            AppTheme.criticalInk
        case "acknowledged", "upstream_acknowledged", "resolved", "signal_resolved":
            AppTheme.okInk
        case "invalidated":
            AppTheme.neutral
        case "target_reconciled", "source_reassigned", "source_moved", "reconciled":
            AppTheme.info
        default:
            AppTheme.inkMuted
        }
    }

    /// L'origine d'une entree est toujours explicite : CairnOps, un Connecteur
    /// ou une personne.
    private var origin: String {
        if let actor = activity.actorName, !actor.isEmpty {
            return actor
        }
        switch activity.origin {
        case "cairnops":
            return "CairnOps"
        case "connector", "integration":
            return "Connecteur"
        case "human":
            return "Opérateur"
        default:
            return activity.origin.capitalized
        }
    }

    var body: some View {
        TimelineRow(
            systemImage: symbol,
            tone: tone,
            time: TimestampParser.relativeString(from: activity.occurredAt),
            isFirst: isFirst,
            isLast: isLast
        ) {
            Text(activity.message)
                .font(AppTheme.fieldValueFont)
                .foregroundStyle(AppTheme.ink)
                .fixedSize(horizontal: false, vertical: true)

            Text("\(origin) · \(TimestampParser.absoluteString(from: activity.occurredAt))")
                .font(AppTheme.metaFont)
                .foregroundStyle(AppTheme.inkMuted)
                .fixedSize(horizontal: false, vertical: true)
        }
    }
}
