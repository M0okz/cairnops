import Foundation

struct Target: Codable, Equatable, Identifiable, Sendable {
    struct Source: Codable, Equatable, Identifiable, Sendable {
        enum SourceKind: String, Codable, Sendable {
            case http
            case tcp
            case dns
            case icmp
            case heartbeat
        }

        enum Outcome: String, Codable, Sendable {
            case healthy
            case unhealthy
            case unknown
        }

        let id: String
        let targetID: String
        let name: String
        let kind: SourceKind
        let enabled: Bool
        let intervalSeconds: Int
        let timeoutMilliseconds: Int
        let failureThreshold: Int
        let recoveryThreshold: Int
        let severity: IncidentSeverity
        let lastSignalAt: String?
        let lastObservedAt: String?
        let latestOutcome: Outcome?

        private enum CodingKeys: String, CodingKey {
            case id
            case targetID = "target_id"
            case name
            case kind
            case enabled
            case intervalSeconds = "interval_seconds"
            case timeoutMilliseconds = "timeout_milliseconds"
            case failureThreshold = "failure_threshold"
            case recoveryThreshold = "recovery_threshold"
            case severity
            case lastSignalAt = "last_signal_at"
            case lastObservedAt = "last_observed_at"
            case latestOutcome = "latest_outcome"
        }
    }

    let id: String
    let name: String
    let description: String
    let createdAt: String
    let externalSourceCount: Int
    let sources: [Source]

    var totalSourceCount: Int {
        sources.count + externalSourceCount
    }

    var displayDescription: String? {
        let trimmed = description.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            return nil
        }

        if connectorName != nil, trimmed.folding(options: [.diacriticInsensitive, .caseInsensitive], locale: .current).contains("decouvert par") {
            return nil
        }

        return trimmed
    }

    var connectorName: String? {
        let normalized = description.folding(options: [.diacriticInsensitive, .caseInsensitive], locale: .current)

        if normalized.contains("zabbix") {
            return "Zabbix"
        }
        if normalized.contains("uptime kuma") {
            return "Uptime Kuma"
        }
        if normalized.contains("patchmon") {
            return "PatchMon"
        }
        if normalized.contains("webhook") {
            return "Webhook"
        }

        return nil
    }

    var sourceOriginSummary: String? {
        guard let connectorName else {
            return nil
        }

        return "Supervision importée via \(connectorName)"
    }

    private enum CodingKeys: String, CodingKey {
        case id
        case name
        case description
        case createdAt = "created_at"
        case externalSourceCount = "external_source_count"
        case sources
    }
}
