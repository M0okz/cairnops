import SwiftUI

/// Ligne de Cible posee a nu.
///
/// La maquette montre l'adresse IP et deux colonnes de charge. L'API ne porte
/// pas d'adresse sur une Cible : la ligne affiche a la place ses Sources et son
/// Connecteur, qui existent bel et bien, et les deux colonnes de droite suivent
/// les indicateurs contextuels quand le Connecteur en fournit.
struct TargetRow: View {
    let target: Target
    let health: AppSnapshot.TargetHealth
    var measures: TargetMeasures?
    var indicators: TargetIndicators?

    private var tone: Color {
        AppTheme.targetHealthColor(health)
    }

    /// Premiere colonne : charge processeur, sinon disponibilite sur 24 h.
    private var leadingValue: (text: String, tone: Color)? {
        if let cpu = indicator(for: "cpu.utilization") {
            return (cpu.displayValue, load(cpu.lastValue))
        }
        if let availability = measures?.last24Hours?.availabilityDisplayValue {
            return (availability, AppTheme.ink)
        }
        return nil
    }

    /// Seconde colonne : memoire, sinon nombre de Sources.
    private var trailingValue: String? {
        if let memory = indicator(for: "memory.utilization") {
            return memory.displayValue
        }
        return nil
    }

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            StatusDot(tone: tone, size: 7, haloWidth: 3)

            VStack(alignment: .leading, spacing: 3) {
                Text(target.name)
                    .font(.subheadline.weight(.semibold))
                    .tracking(-0.15)
                    .foregroundStyle(AppTheme.ink)
                    .lineLimit(1)
                    .truncationMode(.middle)

                meta
            }

            Spacer(minLength: 8)

            // Les deux colonnes sont toujours posees, meme vides : la densite
            // constante veut que le bord droit s'aligne d'une ligne a l'autre.
            value(leadingValue?.text, tone: leadingValue?.tone ?? AppTheme.ink)
            value(trailingValue, tone: AppTheme.inkStrong)
        }
        .padding(.vertical, 12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(target.name), \(AppTheme.targetHealthLabel(health))")
    }

    private var meta: some View {
        HStack(spacing: 6) {
            Text(sourceCountLabel)

            if let connector = target.connectorName {
                separator
                Text(connector)
                    .fontWeight(.semibold)
                    .foregroundStyle(AppTheme.inkStrong)
            }

            separator
            Text(TimestampParser.relativeString(from: measures?.latestObservedAt))
                .lineLimit(1)
        }
        .font(.caption2)
        .foregroundStyle(AppTheme.inkMuted)
    }

    private var separator: some View {
        Circle()
            .fill(AppTheme.inkMuted)
            .frame(width: 3, height: 3)
    }

    private func value(_ text: String?, tone: Color) -> some View {
        Text(text ?? "")
            .font(.footnote.weight(.semibold))
            .monospacedDigit()
            .foregroundStyle(tone)
            .lineLimit(1)
            .frame(width: 48, alignment: .trailing)
    }

    private var sourceCountLabel: String {
        target.totalSourceCount == 1 ? "1 source" : "\(target.totalSourceCount) sources"
    }

    private func indicator(for semanticKey: String) -> ContextIndicator? {
        indicators?.indicators.first { $0.semanticKey == semanticKey && $0.lastValue != nil }
    }

    /// Une charge n'est alarmante qu'au-dela d'un seuil : la couleur ne sert ici
    /// qu'a signaler la saturation, jamais a decorer la colonne.
    private func load(_ value: Double?) -> Color {
        guard let value else {
            return AppTheme.ink
        }
        if value >= 90 {
            return AppTheme.criticalInk
        }
        if value >= 75 {
            return AppTheme.warningInk
        }
        return AppTheme.ink
    }
}
