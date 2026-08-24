import SwiftUI

/// Ligne generique a nu : pastille, intitule, fraicheur et etat.
///
/// Les Connecteurs d'une Cible, ses Incidents recents et les composants de
/// CairnOps partagent exactement cette forme ; les decliner separement
/// produisait trois lignes legerement differentes sans raison.
struct MetaRow: View {
    let title: String
    var subtitle: String?
    var detail: String?
    var state: String?
    var tone: Color = AppTheme.ink
    var stateInk: Color?

    var body: some View {
        HStack(alignment: .center, spacing: 11) {
            StatusDot(tone: tone, size: 7, haloWidth: 3)

            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.subheadline.weight(subtitle == nil ? .medium : .semibold))
                    .tracking(-0.15)
                    .foregroundStyle(AppTheme.ink)
                    .lineLimit(1)
                    .truncationMode(.tail)

                if let subtitle, !subtitle.isEmpty {
                    Text(subtitle)
                        .font(.caption2)
                        .foregroundStyle(AppTheme.inkMuted)
                        .lineLimit(1)
                }
            }

            Spacer(minLength: 8)

            if let detail, !detail.isEmpty {
                Text(detail)
                    .font(.caption2)
                    .foregroundStyle(AppTheme.inkMuted)
                    .lineLimit(1)
            }

            if let state, !state.isEmpty {
                Text(state)
                    .font(.caption2.weight(.bold))
                    .foregroundStyle(stateInk ?? tone)
                    .lineLimit(1)
            }
        }
        .padding(.vertical, 12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .hairlineTop()
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
    }
}

/// Ligne de charge : la barre situe la valeur sans reclamer une colonne.
struct LoadRow: View {
    let title: String
    let ratio: Double
    let value: String
    var tone: Color = AppTheme.ink

    var body: some View {
        HStack(spacing: 12) {
            StatusDot(tone: tone, size: 7, haloWidth: 3)

            Text(title)
                .font(.subheadline)
                .foregroundStyle(AppTheme.ink)
                .lineLimit(1)
                .truncationMode(.middle)

            Spacer(minLength: 8)

            ZStack(alignment: .leading) {
                Rectangle()
                    .fill(AppTheme.hairline)
                Rectangle()
                    .fill(tone)
                    .frame(width: 76 * min(max(ratio, 0), 1))
            }
            .frame(width: 76, height: 4)

            Text(value)
                .font(.footnote.weight(.bold))
                .monospacedDigit()
                .foregroundStyle(AppTheme.ink)
                .frame(width: 44, alignment: .trailing)
        }
        .padding(.vertical, 12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
        .accessibilityLabel(title)
        .accessibilityValue(value)
    }
}
