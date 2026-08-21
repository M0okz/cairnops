import Foundation

struct ServerConfigurationStore {
    private let defaults = UserDefaults.standard
    private let storageKey = "cairnops.mobile.configuration"

    func load() -> ServerConfiguration {
        guard let data = defaults.data(forKey: storageKey),
              let configuration = try? JSONDecoder().decode(ServerConfiguration.self, from: data) else {
            return ServerConfiguration()
        }

        return configuration
    }

    func save(_ configuration: ServerConfiguration) {
        guard let data = try? JSONEncoder().encode(configuration) else {
            return
        }

        defaults.set(data, forKey: storageKey)
    }

    func clear() {
        defaults.removeObject(forKey: storageKey)
    }
}
