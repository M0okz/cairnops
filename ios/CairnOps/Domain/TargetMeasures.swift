import Foundation

struct TargetMeasures: Codable, Equatable, Sendable {
    struct Measure: Codable, Equatable, Sendable {
        enum Window: String, Codable, Sendable {
            case last24Hours = "24h"
            case last7Days = "7d"
            case last30Days = "30d"
        }

        let window: Window
        let availability: Double?
        let coverage: Double?
        let averageLatencyMilliseconds: Double?
        let maximumLatencyMilliseconds: Double?
        let conclusiveObservations: Int
        let unknownObservations: Int
        let expectedObservations: Int

        var hasAvailabilitySignal: Bool {
            availability != nil && conclusiveObservations > 0
        }

        var hasCoverageSignal: Bool {
            coverage != nil && expectedObservations > 0
        }

        var availabilityDisplayValue: String? {
            guard hasAvailabilitySignal, let availability else {
                return nil
            }

            return availability.formatted(.percent.precision(.fractionLength(0)))
        }

        var coverageDisplayValue: String? {
            guard hasCoverageSignal, let coverage else {
                return nil
            }

            return coverage.formatted(.percent.precision(.fractionLength(0)))
        }

        private enum CodingKeys: String, CodingKey {
            case window
            case availability
            case coverage
            case averageLatencyMilliseconds = "average_latency_milliseconds"
            case maximumLatencyMilliseconds = "maximum_latency_milliseconds"
            case conclusiveObservations = "conclusive_observations"
            case unknownObservations = "unknown_observations"
            case expectedObservations = "expected_observations"
        }
    }

    struct SourceMeasures: Codable, Equatable, Sendable {
        let sourceID: String
        let name: String
        let kind: String
        let origin: String
        let measuresAvailability: Bool
        let latestOutcome: String?
        let latestObservedAt: String?
        let measures: [Measure]

        private enum CodingKeys: String, CodingKey {
            case sourceID = "source_id"
            case name
            case kind
            case origin
            case measuresAvailability = "measures_availability"
            case latestOutcome = "latest_outcome"
            case latestObservedAt = "latest_observed_at"
            case measures
        }
    }

    let targetID: String
    let measures: [Measure]
    let trend: [Double]
    let latencyTrend: [Double]
    let latestObservedAt: String?
    let sources: [SourceMeasures]

    var last24Hours: Measure? {
        measures.first { $0.window == .last24Hours }
    }

    var hasImportedOnlySignals: Bool {
        sources.contains(where: { $0.origin == "integration" }) && !sources.contains(where: { $0.origin == "native" })
    }

    private enum CodingKeys: String, CodingKey {
        case targetID = "target_id"
        case measures
        case trend
        case latencyTrend = "latency_trend"
        case latestObservedAt = "latest_observed_at"
        case sources
    }
}
