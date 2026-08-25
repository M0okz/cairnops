import Foundation

enum AppLanguage: String, CaseIterable, Identifiable {
    case system
    case french
    case english

    static let storageKey = "fr.cairnops.interface-language"

    var id: Self { self }

    var title: String {
        switch self {
        case .system:
            Self.localized("language.system")
        case .french:
            Self.localized("language.french")
        case .english:
            Self.localized("language.english")
        }
    }

    var locale: Locale {
        switch self {
        case .system:
            .autoupdatingCurrent
        case .french:
            Locale(identifier: "fr_FR")
        case .english:
            Locale(identifier: "en_GB")
        }
    }

    var serverIdentifier: String {
        switch self {
        case .french:
            "fr"
        case .english:
            "en"
        case .system:
            Self.systemIdentifier
        }
    }

    static var current: AppLanguage {
        guard let rawValue = UserDefaults.standard.string(forKey: storageKey),
              let language = AppLanguage(rawValue: rawValue) else {
            return .system
        }
        return language
    }

    static var currentLocale: Locale {
        current.locale
    }

    static var currentServerIdentifier: String {
        current.serverIdentifier
    }

    static func localized(_ key: String) -> String {
        let language = currentServerIdentifier
        guard let path = Bundle.main.path(forResource: language, ofType: "lproj"),
              let bundle = Bundle(path: path) else {
            return Bundle.main.localizedString(forKey: key, value: key, table: nil)
        }
        return bundle.localizedString(forKey: key, value: key, table: nil)
    }

    private static var systemIdentifier: String {
        Locale.autoupdatingCurrent.language.languageCode?.identifier == "en" ? "en" : "fr"
    }
}
