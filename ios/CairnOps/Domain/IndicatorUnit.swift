import Foundation

enum IndicatorUnit: String, Codable, Sendable {
    case percent
    case bytesPerSecond = "bytes_per_second"
    case milliseconds
    case days
    case boolean
    case count
    case seconds

    func format(_ value: Double) -> String {
        switch self {
        case .percent:
            return value.formatted(.number.precision(.fractionLength(0))) + " %"
        case .bytesPerSecond:
            if abs(value) < 0.5 {
                return "0 B/s"
            }
            return ByteCountFormatter.string(
                fromByteCount: Int64(value.rounded()),
                countStyle: .decimal
            ) + "/s"
        case .milliseconds:
            return value.formatted(.number.precision(.fractionLength(0))) + " ms"
        case .days:
            return value.formatted(.number.precision(.fractionLength(0)).locale(AppLanguage.currentLocale))
                + AppLanguage.localized("indicator.days.suffix")
        case .boolean:
            return AppLanguage.localized(value >= 0.5 ? "common.yes" : "common.no")
        case .count:
            return value.formatted(.number.precision(.fractionLength(0)))
        case .seconds:
            return Duration.seconds(value).formatted(
                .units(allowed: [.days, .hours, .minutes], width: .abbreviated)
                    .locale(AppLanguage.currentLocale)
            )
        }
    }
}
