import Foundation

struct SystemHealth: Codable, Equatable, Sendable {
    struct Component: Codable, Equatable, Identifiable, Sendable {
        let name: String
        let status: String
        let instances: Int
        let lastSeenAt: String?

        var id: String { name }

        private enum CodingKeys: String, CodingKey {
            case name
            case status
            case instances
            case lastSeenAt = "last_seen_at"
        }
    }

    struct Database: Codable, Equatable, Sendable {
        let latencyMilliseconds: Double
        let maximumLatencyMilliseconds: Double
        let samples: [Double]
        let measuredSince: String

        private enum CodingKeys: String, CodingKey {
            case latencyMilliseconds = "latency_milliseconds"
            case maximumLatencyMilliseconds = "maximum_latency_milliseconds"
            case samples
            case measuredSince = "measured_since"
        }
    }

    struct ActivityHour: Codable, Equatable, Identifiable, Sendable {
        let hour: String
        let expectedObservations: Int
        let conclusiveObservations: Int
        let healthyObservations: Int
        let averageLatencyMilliseconds: Double?

        var id: String { hour }

        private enum CodingKeys: String, CodingKey {
            case hour
            case expectedObservations = "expected_observations"
            case conclusiveObservations = "conclusive_observations"
            case healthyObservations = "healthy_observations"
            case averageLatencyMilliseconds = "average_latency_milliseconds"
        }
    }

    let status: String
    let checkedAt: String
    let components: [Component]
    let database: Database
    let hours: [ActivityHour]

    private enum CodingKeys: String, CodingKey {
        case status
        case checkedAt = "checked_at"
        case components
        case database
        case hours
    }
}
