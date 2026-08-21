import SwiftUI

struct MetricTile: View {
    let title: String
    let value: String
    var subtitle: String?
    var tone: Color = .primary
    var monospaced = true
    var systemImage: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                if let systemImage {
                    Image(systemName: systemImage)
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(tone)
                        .frame(width: 20, height: 20)
                        .background(
                            Circle()
                                .fill(tone.opacity(0.12))
                        )
                }

                Text(title)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .textCase(.uppercase)
            }

            Group {
                if monospaced {
                    Text(value)
                        .monospacedDigit()
                } else {
                    Text(value)
                }
            }
            .font(AppTheme.tileValueFont)
            .foregroundStyle(tone)

            if let subtitle, !subtitle.isEmpty {
                Text(subtitle)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(
            RoundedRectangle(cornerRadius: 14)
                .fill(AppTheme.subpanel)
                .overlay(
                    RoundedRectangle(cornerRadius: 14)
                        .strokeBorder(tone.opacity(0.12))
                )
        )
    }
}
