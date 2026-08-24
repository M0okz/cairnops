import SwiftUI

/// Mini-tendance d'une metrique, avec son marqueur de fin.
///
/// Elle accompagne une valeur chiffree et ne porte ni axe ni graduation : la
/// lecture precise appartient au detail, pas a la dalle de mesure.
struct Sparkline: View {
    @Environment(\.colorScheme) private var colorScheme

    let values: [Double]
    let tone: Color
    var height = 30.0
    var lowerBound: Double?
    var upperBound: Double?

    /// L'aire situe la courbe dans une dalle de mesure. Sous une vingtaine de
    /// points de haut elle se lit comme un aplat plein, pas comme une tendance :
    /// les mini-tendances en ligne s'en passent.
    var showsArea = true

    var body: some View {
        if values.count < 2 {
            // Sans au moins deux relevés, une courbe donnerait l'illusion d'une
            // tendance mesurée. Le filet dit franchement qu'il n'y en a pas.
            Hairline()
                .frame(height: height, alignment: .center)
        } else {
            ZStack(alignment: .topLeading) {
                if showsArea {
                    TrendArea(values: values, lowerBound: lowerBound, upperBound: upperBound)
                        .fill(
                            LinearGradient(
                                colors: [
                                    tone.opacity(colorScheme == .dark ? 0.15 : 0.11),
                                    tone.opacity(0),
                                ],
                                startPoint: .top,
                                endPoint: .bottom
                            )
                        )
                }

                TrendCurve(values: values, lowerBound: lowerBound, upperBound: upperBound)
                    .stroke(tone, style: StrokeStyle(lineWidth: 1.4, lineCap: .round, lineJoin: .round))

                endMarker
            }
            .frame(height: height)
            .accessibilityHidden(true)
        }
    }

    /// Petit rectangle plein sur le dernier releve, comme dans la maquette.
    private var endMarker: some View {
        GeometryReader { proxy in
            let points = TrendGeometry.points(
                values: values,
                in: proxy.size,
                padding: 4,
                lowerBound: lowerBound,
                upperBound: upperBound
            )

            if let last = points.last {
                Circle()
                    .fill(tone)
                    .frame(width: 3.2, height: 3.2)
                    .position(x: last.x - 1.6, y: last.y)
            }
        }
    }
}
