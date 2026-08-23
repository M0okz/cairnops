import Foundation

struct IncidentIndicators: Codable, Equatable, Sendable, Identifiable {
    let incidentID: String
    let targetID: String
    let openedAt: String
    let snapshots: [IndicatorSnapshot]
    let indicators: [ContextIndicator]
    let series: [String: [IndicatorPoint]]
    let disclaimer: String

    var id: String { incidentID }

    private enum CodingKeys: String, CodingKey {
        case incidentID = "incident_id"
        case targetID = "target_id"
        case openedAt = "opened_at"
        case snapshots
        case indicators
        case series
        case disclaimer
    }
}
