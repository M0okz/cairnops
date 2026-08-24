import SwiftUI

/// Paire cle/valeur empilee, pour la grille d'un detail d'Incident.
struct FieldCell: View {
    let label: String
    let value: String
    var tone: Color = AppTheme.ink

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(label.uppercased())
                .font(AppTheme.fieldLabelFont)
                .tracking(0.9)
                .foregroundStyle(AppTheme.inkFaint)

            Text(value)
                .font(AppTheme.fieldValueFont)
                .tracking(-0.15)
                .foregroundStyle(tone)
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }
}

/// Paire cle/valeur alignee, pour les sections « A propos » des Reglages.
struct FieldRow: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    let label: String
    let value: String
    var secondary: String?
    var tone: Color = AppTheme.ink
    var allowsSelection = false

    var body: some View {
        // La colonne fixe de 120 pt ne tient plus aux tailles d'accessibilite :
        // la paire s'empile alors au lieu d'ecraser la valeur.
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(alignment: .leading, spacing: 4) {
                    key
                    value(alignment: .leading)
                }
            } else {
                HStack(alignment: .firstTextBaseline, spacing: 12) {
                    key
                        .frame(width: 120, alignment: .leading)
                    value(alignment: .leading)
                }
            }
        }
        .padding(.vertical, 15)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }

    private var key: some View {
        Text(label.uppercased())
            .font(AppTheme.fieldLabelFont)
            .tracking(0.9)
            .foregroundStyle(AppTheme.inkFaint)
    }

    private func value(alignment: HorizontalAlignment) -> some View {
        VStack(alignment: alignment, spacing: 2) {
            Group {
                if allowsSelection {
                    Text(self.value).textSelection(.enabled)
                } else {
                    Text(self.value)
                }
            }
            .font(AppTheme.fieldValueFont)
            .foregroundStyle(tone)
            .fixedSize(horizontal: false, vertical: true)

            if let secondary, !secondary.isEmpty {
                Text(secondary)
                    .font(AppTheme.metaFont)
                    .foregroundStyle(AppTheme.inkMuted)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// Ligne de reglage : intitule, explication facultative, valeur et chevron.
struct SettingsRow: View {
    let title: String
    var subtitle: String?
    var value: String?
    var systemImage: String?
    var showsChevron = true

    var body: some View {
        HStack(spacing: 12) {
            if let systemImage {
                Image(systemName: systemImage)
                    .font(.system(size: 18, weight: .medium))
                    .foregroundStyle(AppTheme.accent)
                    .frame(width: 22)
            }

            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(AppTheme.fieldValueFont)
                    .foregroundStyle(AppTheme.ink)

                if let subtitle, !subtitle.isEmpty {
                    Text(subtitle)
                        .font(AppTheme.metaFont)
                        .foregroundStyle(AppTheme.inkMuted)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }

            Spacer(minLength: 8)

            if let value, !value.isEmpty {
                Text(value)
                    .font(.subheadline)
                    .foregroundStyle(AppTheme.inkMuted)
                    .lineLimit(1)
            }

            if showsChevron {
                Image(systemName: "chevron.right")
                    .font(.system(size: 15, weight: .bold))
                    .foregroundStyle(AppTheme.inkFaint)
            }
        }
        .padding(.vertical, 15)
        .frame(maxWidth: .infinity, alignment: .leading)
        // Une ligne de reglage reste une cible tactile : elle ne descend jamais
        // sous la hauteur minimale recommandee, meme sans sous-titre.
        .frame(minHeight: 44)
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
    }
}
