import SwiftUI

/// Geometrie partagee des courbes de tendance.
///
/// La maquette lisse ses series en convertissant une spline Catmull-Rom en
/// courbes de Bezier cubiques. On reprend exactement la meme construction pour
/// que les sparklines et la courbe de latence aient le meme galbe.
enum TrendGeometry {

    /// Points de la serie ramenes au cadre, borne haute et basse comprises.
    static func points(
        values: [Double],
        in size: CGSize,
        padding: Double,
        lowerBound: Double?,
        upperBound: Double?
    ) -> [CGPoint] {
        guard values.count > 1 else {
            // Une valeur unique se pose au milieu : une courbe plate reste plus
            // honnete qu'un point colle en haut ou en bas du cadre.
            let middle = size.height / 2
            return values.isEmpty ? [] : [
                CGPoint(x: 0, y: middle),
                CGPoint(x: size.width, y: middle),
            ]
        }

        let minimum = lowerBound ?? (values.min() ?? 0)
        let maximum = upperBound ?? (values.max() ?? 1)
        let span = maximum - minimum

        return values.enumerated().map { index, value in
            let ratio = span == 0 ? 0.5 : (value - minimum) / span
            let clamped = min(max(ratio, 0), 1)
            return CGPoint(
                x: Double(index) / Double(values.count - 1) * size.width,
                y: size.height - padding - clamped * (size.height - padding * 2)
            )
        }
    }

    /// Chemin lisse passant par tous les points.
    static func curve(through points: [CGPoint]) -> Path {
        var path = Path()
        guard let first = points.first else {
            return path
        }

        path.move(to: first)
        guard points.count > 1 else {
            return path
        }

        for index in 0..<(points.count - 1) {
            let previous = index > 0 ? points[index - 1] : points[index]
            let start = points[index]
            let end = points[index + 1]
            let next = index + 2 < points.count ? points[index + 2] : end

            path.addCurve(
                to: end,
                control1: CGPoint(
                    x: start.x + (end.x - previous.x) / 6,
                    y: start.y + (end.y - previous.y) / 6
                ),
                control2: CGPoint(
                    x: end.x - (next.x - start.x) / 6,
                    y: end.y - (next.y - start.y) / 6
                )
            )
        }

        return path
    }

    /// Ordonnee d'une valeur donnee, pour poser un seuil dans le meme cadre.
    static func offset(
        of value: Double,
        in size: CGSize,
        padding: Double,
        lowerBound: Double,
        upperBound: Double
    ) -> Double {
        let span = upperBound - lowerBound
        let ratio = span == 0 ? 0.5 : (value - lowerBound) / span
        let clamped = min(max(ratio, 0), 1)
        return size.height - padding - clamped * (size.height - padding * 2)
    }
}

/// Trace de la serie.
struct TrendCurve: Shape {
    let values: [Double]
    var padding = 4.0
    var lowerBound: Double?
    var upperBound: Double?

    func path(in rect: CGRect) -> Path {
        TrendGeometry.curve(
            through: TrendGeometry.points(
                values: values,
                in: rect.size,
                padding: padding,
                lowerBound: lowerBound,
                upperBound: upperBound
            )
        )
    }
}

/// Aire sous la serie, fermee sur le bas du cadre.
struct TrendArea: Shape {
    let values: [Double]
    var padding = 4.0
    var lowerBound: Double?
    var upperBound: Double?

    func path(in rect: CGRect) -> Path {
        let points = TrendGeometry.points(
            values: values,
            in: rect.size,
            padding: padding,
            lowerBound: lowerBound,
            upperBound: upperBound
        )
        guard !points.isEmpty else {
            return Path()
        }

        var path = TrendGeometry.curve(through: points)
        path.addLine(to: CGPoint(x: rect.maxX, y: rect.maxY))
        path.addLine(to: CGPoint(x: rect.minX, y: rect.maxY))
        path.closeSubpath()
        return path
    }
}
