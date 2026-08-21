import Foundation

struct RealtimeMessage: Codable, Equatable, Sendable {
    let type: String
    let version: Int64
    let kind: String?
    let entityType: String?
    let entityID: String?
    let occurredAt: String?

    private enum CodingKeys: String, CodingKey {
        case type
        case version
        case kind
        case entityType = "entity_type"
        case entityID = "entity_id"
        case occurredAt = "occurred_at"
    }
}
