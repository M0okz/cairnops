import Foundation

struct IndicatorPoint: Codable, Equatable, Sendable, Identifiable {
    let at: String
    let value: Double
    let minimum: Double?
    let maximum: Double?
    let samples: Int?

    var id: String { at }
    var date: Date? { TimestampParser.date(from: at) }
}
