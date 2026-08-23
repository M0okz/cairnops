import Charts
import SwiftUI

struct IndicatorChart: View {
    let points: [IndicatorPoint]
    let unit: IndicatorUnit
    var compact = false

    private var datedPoints: [(date: Date, point: IndicatorPoint)] {
        points.compactMap { point in
            point.date.map { ($0, point) }
        }
    }

    private var rangeDescription: String {
        guard let minimum = points.map(\.value).min(),
              let maximum = points.map(\.value).max() else {
            return "Aucune donnée dans cette fenêtre."
        }
        return "De \(unit.format(minimum)) à \(unit.format(maximum)), sur \(points.count) relevés."
    }

    private var spansMultipleDays: Bool {
        guard let first = datedPoints.first?.date,
              let last = datedPoints.last?.date else { return false }
        return last.timeIntervalSince(first) > 36 * 60 * 60
    }

    private var xAxisDates: [Date] {
        guard let first = datedPoints.first?.date,
              let last = datedPoints.last?.date,
              last > first else { return [] }
        let span = last.timeIntervalSince(first)
        return (1...3).map { first.addingTimeInterval(span * Double($0) / 4) }
    }

    var body: some View {
        if datedPoints.isEmpty {
            ContentUnavailableView(
                "Pas encore de courbe",
                systemImage: "chart.xyaxis.line",
                description: Text("Les relevés expireront naturellement après leur fenêtre de conservation.")
            )
            .frame(minHeight: compact ? 76 : 130)
        } else {
            Chart(datedPoints, id: \.point.id) { entry in
                AreaMark(
                    x: .value("Date", entry.date),
                    y: .value("Valeur", entry.point.value)
                )
                .foregroundStyle(
                    LinearGradient(
                        colors: [AppTheme.accent.opacity(0.28), AppTheme.accent.opacity(0.02)],
                        startPoint: .top,
                        endPoint: .bottom
                    )
                )

                LineMark(
                    x: .value("Date", entry.date),
                    y: .value("Valeur", entry.point.value)
                )
                .foregroundStyle(AppTheme.accent)
                .lineStyle(StrokeStyle(lineWidth: compact ? 1.5 : 2, lineCap: .round, lineJoin: .round))
            }
            .chartYAxis {
                if !compact {
                    AxisMarks(position: .trailing, values: .automatic(desiredCount: 4)) { value in
                        AxisGridLine()
                        AxisTick()
                        AxisValueLabel {
                            if let number = value.as(Double.self) {
                                Text(unit.format(number))
                                    .font(.caption2)
                            }
                        }
                    }
                }
            }
            .chartXAxis {
                if !compact {
                    AxisMarks(values: xAxisDates) { value in
                        AxisGridLine()
                        AxisTick()
                        AxisValueLabel {
                            if let date = value.as(Date.self) {
                                Text(date.formatted(
                                    spansMultipleDays
                                        ? .dateTime.day().month(.abbreviated)
                                        : .dateTime.hour().minute()
                                ))
                                .font(.caption2)
                            }
                        }
                    }
                }
            }
            .chartPlotStyle { plot in
                plot.padding(.horizontal, compact ? 0 : 8)
            }
            .frame(height: compact ? 76 : 150)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Évolution de l’indicateur")
            .accessibilityValue(rangeDescription)
        }
    }
}
