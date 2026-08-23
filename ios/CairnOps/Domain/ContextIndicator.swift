import Foundation

struct ContextIndicator: Codable, Equatable, Sendable, Identifiable {
    let id: String
    let connectorID: String
    let bindingID: String
    let targetID: String
    let semanticKey: String
    let label: String
    let externalID: String
    let dimension: String?
    let unit: IndicatorUnit
    let enabled: Bool
    let lastValue: Double?
    let lastObservedAt: String?
    let lastError: String?
    let pinned: Bool
    let pinPosition: Int?

    var displayLabel: String {
        guard let dimension, !dimension.isEmpty else { return label }
        return "\(label) · \(dimension)"
    }

    var displayValue: String {
        lastValue.map(unit.format) ?? "—"
    }

    private enum CodingKeys: String, CodingKey {
        case id
        case connectorID = "connector_id"
        case bindingID = "binding_id"
        case targetID = "target_id"
        case semanticKey = "semantic_key"
        case label
        case externalID = "external_id"
        case dimension
        case unit
        case enabled
        case lastValue = "last_value"
        case lastObservedAt = "last_observed_at"
        case lastError = "last_error"
        case pinned
        case pinPosition = "pin_position"
    }
}
