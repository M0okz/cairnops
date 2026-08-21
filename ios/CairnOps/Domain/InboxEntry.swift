import Foundation

struct InboxEntry: Codable, Equatable, Identifiable, Sendable {
    let id: Int
    let incidentID: String
    let targetID: String?
    let eventKind: String
    let targetName: String
    let natureLabel: String
    let severity: IncidentSeverity
    let occurredAt: String
    let readAt: String?

    private enum CodingKeys: String, CodingKey {
        case id
        case incidentID = "incident_id"
        case targetID = "target_id"
        case eventKind = "event_kind"
        case targetName = "target_name"
        case natureLabel = "nature_label"
        case severity
        case occurredAt = "occurred_at"
        case readAt = "read_at"
    }
}
