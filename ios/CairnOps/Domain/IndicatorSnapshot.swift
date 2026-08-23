import Foundation

struct IndicatorSnapshot: Codable, Equatable, Sendable, Identifiable {
    let indicatorID: String?
    let semanticKey: String
    let label: String
    let unit: IndicatorUnit
    let value: Double
    let observedAt: String

    var id: String { indicatorID ?? "\(semanticKey)#\(observedAt)" }

    private enum CodingKeys: String, CodingKey {
        case indicatorID = "indicator_id"
        case semanticKey = "semantic_key"
        case label
        case unit
        case value
        case observedAt = "observed_at"
    }
}
