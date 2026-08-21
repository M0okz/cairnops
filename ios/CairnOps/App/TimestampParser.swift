import Foundation
import Synchronization

enum TimestampParser {
    private static let interfaceLocale = Locale(identifier: "fr_FR")

    /// Les horodatages arrivent en ISO-8601 depuis l'API et sont reformates a
    /// chaque rendu de ligne. Les formatters Foundation sont couteux a allouer,
    /// on les conserve donc pour toute la duree de vie du processus et on
    /// memorise le resultat du parsing, qui est deterministe.
    private final class Formatters {
        let fractional: ISO8601DateFormatter = {
            let formatter = ISO8601DateFormatter()
            formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            return formatter
        }()

        let standard: ISO8601DateFormatter = {
            let formatter = ISO8601DateFormatter()
            formatter.formatOptions = [.withInternetDateTime]
            return formatter
        }()

        let relative: RelativeDateTimeFormatter = {
            let formatter = RelativeDateTimeFormatter()
            formatter.locale = TimestampParser.interfaceLocale
            formatter.unitsStyle = .short
            return formatter
        }()

        var parsedDates: [String: Date] = [:]
    }

    /// `Mutex` serialise les acces : les formatters ne s'echappent jamais de la
    /// section critique, ce qui les rend surs sous concurrence stricte.
    private static let formatters = Mutex(Formatters())

    private static let parseCacheLimit = 4_096

    private static let absoluteStyle = Date.FormatStyle(date: .abbreviated, time: .shortened)
        .locale(interfaceLocale)

    private static let dayScaleStyle = Duration.UnitsFormatStyle(
        allowedUnits: [.days, .hours],
        width: .abbreviated
    )
    .locale(interfaceLocale)

    private static let hourScaleStyle = Duration.UnitsFormatStyle(
        allowedUnits: [.hours, .minutes],
        width: .abbreviated
    )
    .locale(interfaceLocale)

    static func date(from value: String?) -> Date? {
        guard let value, !value.isEmpty else {
            return nil
        }

        return formatters.withLock { storage in
            if let cached = storage.parsedDates[value] {
                return cached
            }

            guard let parsed = storage.fractional.date(from: value)
                ?? storage.standard.date(from: value) else {
                return nil
            }

            // Le cache est borne : un flux temps reel de longue duree ne doit
            // pas faire croitre la memoire indefiniment.
            if storage.parsedDates.count >= parseCacheLimit {
                storage.parsedDates.removeAll(keepingCapacity: true)
            }
            storage.parsedDates[value] = parsed

            return parsed
        }
    }

    static func absoluteString(from value: String?) -> String {
        guard let date = date(from: value) else {
            return "Inconnue"
        }

        return absoluteStyle.format(date)
    }

    static func relativeString(from value: String?) -> String {
        guard let date = date(from: value) else {
            return "Jamais"
        }

        if abs(date.timeIntervalSinceNow) < 5 {
            return "à l’instant"
        }

        let reference = Date.now
        return formatters.withLock { storage in
            storage.relative.localizedString(for: date, relativeTo: reference)
        }
    }

    static func elapsedString(since value: String?) -> String {
        guard let date = date(from: value) else {
            return "Inconnue"
        }

        let interval = max(0, Date.now.timeIntervalSince(date))
        if interval < 60 {
            return "à l’instant"
        }

        let style = interval >= 86_400 ? dayScaleStyle : hourScaleStyle
        return Duration.seconds(interval).formatted(style)
    }
}
