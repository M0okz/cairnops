import SwiftUI

/// Dalle de mesure : valeur, ecart et mini-tendance.
///
/// Elle remplace l'ancienne `MetricTile` a fond et contour : la maquette pose
/// la mesure directement sur la page, sous un filet.
struct MetricCell: View {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @ScaledMetric(relativeTo: .title2) private var valueScale = 1

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
                        // La maquette pose ces valeurs a 28 px en graisse
                        // maximale. Sur quatre dalles empilees, la page devient
                        // un mur de chiffres : le corps redescend et la graisse
                        // s'allege, la tendance dessous portant la lecture.
                        Text(value)
                            .font(.system(size: 22 * min(valueScale, 1.4), weight: .bold))
                            .monospacedDigit()
                            .tracking(-0.5)
                            .contentTransition(.numericText())
                            .animation(
                                reduceMotion ? nil : .snappy(duration: 0.35),
                                value: value
                            )
                    } else {
                        Text(value)
                            .font(.subheadline.weight(.semibold))
                            .tracking(-0.2)
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
                    height: 22,
                    lowerBound: lowerBound,
                    upperBound: upperBound
                )
                .padding(.top, 7)
            }
        }
        .padding(.vertical, 12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .hairlineTop()
        .accessibilityElement(children: .combine)
        .accessibilityLabel(label)
        .accessibilityValue(unit.map { "\(value) \($0)" } ?? value)
    }
}
