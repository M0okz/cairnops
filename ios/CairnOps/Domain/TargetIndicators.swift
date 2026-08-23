import Foundation

struct TargetIndicators: Codable, Equatable, Sendable, Identifiable {
    let targetID: String
    let generatedAt: String
    let indicators: [ContextIndicator]
    let series: [String: [IndicatorPoint]]?

    var id: String { targetID }

    private enum CodingKeys: String, CodingKey {
        case targetID = "target_id"
        case generatedAt = "generated_at"
        case indicators
        case series
    }
}
