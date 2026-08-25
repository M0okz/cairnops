import SwiftUI

/// Courbe d'une serie avec son seuil, sur le detail d'un Incident.
///
/// Le seuil est trace en pointille pour rester une reference lisible sans
/// concurrencer la serie elle-meme.
struct ThresholdChart: View {
    @Environment(\.colorScheme) private var colorScheme

    let values: [Double]
    let tone: Color
    var threshold: Double?
    var height = 90.0

    private let padding = 7.0

    /// Bornes du cadre. Le seuil y est inclus : hors du cadre, il disparaitrait
    /// silencieusement au lieu de situer la serie.
    private var bounds: (lower: Double, upper: Double) {
        var lower = values.min() ?? 0
        var upper = values.max() ?? 1

        if let threshold {
            lower = min(lower, threshold)
            upper = max(upper, threshold)
        }

        if upper == lower {
            upper = lower + 1
        }

        // Une marge de 8 % evite que la courbe ne colle aux bords du cadre.
        let margin = (upper - lower) * 0.08
        return (lower - margin, upper + margin)
    }

    var body: some View {
        let bounds = self.bounds

        return ZStack(alignment: .topLeading) {
            if let threshold {
                GeometryReader { proxy in
                    let offset = TrendGeometry.offset(
                        of: threshold,
                        in: proxy.size,
                        padding: padding,
                        lowerBound: bounds.lower,
                        upperBound: bounds.upper
                    )

                    Path { path in
                        path.move(to: CGPoint(x: 0, y: offset))
                        path.addLine(to: CGPoint(x: proxy.size.width, y: offset))
                    }
                    .stroke(
                        AppTheme.inkMuted.opacity(colorScheme == .dark ? 0.55 : 0.45),
                        style: StrokeStyle(lineWidth: 1, dash: [3, 6])
                    )
                }
            }

            TrendArea(
                values: values,
                padding: padding,
                lowerBound: bounds.lower,
                upperBound: bounds.upper
            )
            .fill(
                LinearGradient(
                    colors: [
                        tone.opacity(colorScheme == .dark ? 0.22 : 0.16),
                        tone.opacity(0),
                    ],
                    startPoint: .top,
                    endPoint: .bottom
                )
            )

            TrendCurve(
                values: values,
                padding: padding,
                lowerBound: bounds.lower,
                upperBound: bounds.upper
            )
            .stroke(tone, style: StrokeStyle(lineWidth: 2, lineCap: .round, lineJoin: .round))

            endRule(bounds: bounds)
        }
        .frame(height: height)
    }

    /// Filet vertical sur le dernier releve : il ancre la lecture a maintenant.
    private func endRule(bounds: (lower: Double, upper: Double)) -> some View {
        GeometryReader { proxy in
            let points = TrendGeometry.points(
                values: values,
                in: proxy.size,
                padding: padding,
                lowerBound: bounds.lower,
                upperBound: bounds.upper
            )

            if let last = points.last {
                Path { path in
                    path.move(to: last)
                    path.addLine(to: CGPoint(x: last.x, y: proxy.size.height))
                }
                .stroke(tone.opacity(0.5), lineWidth: 1)
            }
        }
    }
}
