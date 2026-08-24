import SwiftUI

/// Dalle de mesure : valeur, ecart et mini-tendance.
///
/// Elle remplace l'ancienne `MetricTile` a fond et contour : la maquette pose
/// la mesure directement sur la page, sous un filet.
struct MetricCell: View {
    @ScaledMetric(relativeTo: .title) private var valueScale = 1

    let label: String
    let value: String
    var unit: String?
    var delta: String?
    var tone: Color = AppTheme.ink
    var series: [Double] = []
    var lowerBound: Double?
    var upperBound: Double?

    /// Une valeur textuelle — « il y a 18 s » — composee dans la graisse des
    /// grands nombres devient illisible et deborde. Elle garde donc le corps
    /// d'un titre de section.
    var isNumeric = true

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Text(label.uppercased())
                    .font(AppTheme.fieldLabelFont)
                    .tracking(0.9)
                    .foregroundStyle(AppTheme.inkFaint)
                    .lineLimit(1)

                Spacer(minLength: 4)

                if let delta, !delta.isEmpty {
                    Text(delta)
                        .font(.caption2.weight(.bold))
                        .monospacedDigit()
                        .foregroundStyle(tone)
                }
            }

            HStack(alignment: .firstTextBaseline, spacing: 3) {
                Group {
                    if isNumeric {
                        Text(value)
                            .font(.system(size: 28 * min(valueScale, 1.4), weight: .heavy))
                            .monospacedDigit()
                            .tracking(-0.9)
                    } else {
                        Text(value)
                            .font(.title3.weight(.bold))
                            .tracking(-0.3)
                    }
                }
                .foregroundStyle(tone)
                .lineLimit(1)
                .minimumScaleFactor(0.6)

                if let unit, !unit.isEmpty {
                    Text(unit)
                        .font(AppTheme.metaFont)
                        .foregroundStyle(AppTheme.inkMuted)
                }
            }
            .padding(.top, 5)

            if series.count > 1 {
                Sparkline(
                    values: series,
                    tone: tone,
                    lowerBound: lowerBound,
                    upperBound: upperBound
                )
                .padding(.top, 8)
            }
        }
        .padding(.vertical, 14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .hairlineTop()
        .accessibilityElement(children: .combine)
        .accessibilityLabel(label)
        .accessibilityValue(unit.map { "\(value) \($0)" } ?? value)
    }
}
