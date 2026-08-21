import Foundation

struct Incident: Codable, Equatable, Identifiable, Sendable {
    struct Signal: Codable, Equatable, Identifiable, Sendable {
        let id: String
        let origin: String
        let connectorID: String?
        let connectorName: String?
        let externalEventID: String?
        let externalObjectID: String?
        let name: String
        let active: Bool
        let severity: IncidentSeverity
        let openedAt: String
        let resolvedAt: String?
        let upstreamAcknowledged: Bool
        let invalidatedAt: String?
        let invalidatedBy: String?
        let invalidationReason: String?
        let rearmedAt: String?

        private enum CodingKeys: String, CodingKey {
            case id
            case origin
            case connectorID = "connector_id"
            case connectorName = "connector_name"
            case externalEventID = "external_event_id"
            case externalObjectID = "external_object_id"
            case name
            case active
            case severity
            case openedAt = "opened_at"
            case resolvedAt = "resolved_at"
            case upstreamAcknowledged = "upstream_acknowledged"
            case invalidatedAt = "invalidated_at"
            case invalidatedBy = "invalidated_by"
            case invalidationReason = "invalidation_reason"
            case rearmedAt = "rearmed_at"
        }
    }

    struct Activity: Codable, Equatable, Identifiable, Sendable {
        let id: Int
        let kind: String
        let origin: String
        let actorName: String?
        let message: String
        let data: [String: JSONValue]
        let occurredAt: String

        private enum CodingKeys: String, CodingKey {
            case id
            case kind
            case origin
            case actorName = "actor_name"
            case message
            case data
            case occurredAt = "occurred_at"
        }
    }

    let id: String
    let targetID: String
    let targetName: String
    let natureKey: String
    let natureLabel: String
    let status: String
    let sourceSeverity: IncidentSeverity
    let effectiveSeverity: IncidentSeverity
    let openedAt: String
    let resolvedAt: String?
    let acknowledgedAt: String?
    let acknowledgedBy: String?
    let acknowledgementOrigin: String?
    let acknowledgementSyncStatus: String
    let acknowledgementSyncError: String?
    let maintenanceActive: Bool
    let maintenanceEndsAt: String?
    let signals: [Signal]
    let activity: [Activity]
    let createdAt: String
    let updatedAt: String

    var isAcknowledged: Bool {
        acknowledgedAt != nil
    }

    var isResolved: Bool {
        status == "resolved"
    }

    /// Les natures de posture decrivent un ecart de configuration a corriger,
    /// pas une indisponibilite : elles degradent la cible sans la rendre
    /// injoignable.
    var isPostureNature: Bool {
        natureKey == "security-patches-required" || natureKey == "reboot-required"
    }

    var primaryEvidenceLabel: String {
        guard !natureLabel.isEmpty else {
            return "Aucune description"
        }

        return natureLabel
    }

    var visibleSignalCountLabel: String {
        let count = signals.count
        return count == 1 ? "1 preuve" : "\(count) preuves"
    }

    private enum CodingKeys: String, CodingKey {
        case id
        case targetID = "target_id"
        case targetName = "target_name"
        case natureKey = "nature_key"
        case natureLabel = "nature_label"
        case status
        case sourceSeverity = "source_severity"
        case effectiveSeverity = "effective_severity"
        case openedAt = "opened_at"
        case resolvedAt = "resolved_at"
        case acknowledgedAt = "acknowledged_at"
        case acknowledgedBy = "acknowledged_by"
        case acknowledgementOrigin = "acknowledgement_origin"
        case acknowledgementSyncStatus = "acknowledgement_sync_status"
        case acknowledgementSyncError = "acknowledgement_sync_error"
        case maintenanceActive = "maintenance_active"
        case maintenanceEndsAt = "maintenance_ends_at"
        case signals
        case activity
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}
