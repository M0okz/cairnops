import Foundation

/// Cache local du dernier etat connu.
///
/// L'ecriture est volontairement paresseuse : elle sert a reafficher quelque
/// chose au lancement suivant, pas a garantir une durabilite transactionnelle.
/// La sauvegarde est donc groupee et n'interrompt jamais l'interface.
actor SnapshotStore {
    private static let writeDebounce = Duration.seconds(2)

    private let decoder = JSONDecoder()
    private let encoder = JSONEncoder()
    private let customFileURL: URL?

    private var pendingSnapshot: AppSnapshot?
    private var flushTask: Task<Void, Never>?

    init(fileURL: URL? = nil) {
        customFileURL = fileURL
    }

    func load() -> AppSnapshot? {
        let fileURL = snapshotFileURL()
        guard let data = try? Data(contentsOf: fileURL, options: .mappedIfSafe) else {
            return nil
        }
        return try? decoder.decode(AppSnapshot.self, from: data)
    }

    /// Enregistre l'etat a ecrire et programme un vidage differe. Une rafale
    /// d'evenements temps reel ne produit ainsi qu'une seule ecriture disque.
    func save(_ snapshot: AppSnapshot) {
        pendingSnapshot = snapshot

        guard flushTask == nil else {
            return
        }

        flushTask = Task { [weak self] in
            try? await Task.sleep(for: Self.writeDebounce)
            await self?.flush()
        }
    }

    /// Force l'ecriture immediate, par exemple avant une mise en veille.
    func flushNow() {
        flushTask?.cancel()
        flushTask = nil
        flush()
    }

    func clear() {
        flushTask?.cancel()
        flushTask = nil
        pendingSnapshot = nil
        try? FileManager.default.removeItem(at: snapshotFileURL())
    }

    private func flush() {
        flushTask = nil

        guard let snapshot = pendingSnapshot else {
            return
        }
        pendingSnapshot = nil

        let fileURL = snapshotFileURL()
        let directoryURL = fileURL.deletingLastPathComponent()

        do {
            try FileManager.default.createDirectory(at: directoryURL, withIntermediateDirectories: true)
            let data = try encoder.encode(snapshot)
            try data.write(to: fileURL, options: .atomic)
        } catch {
            // Le cache local est opportuniste : une erreur d'ecriture ne doit
            // pas bloquer la projection live.
        }
    }

    private func snapshotFileURL() -> URL {
        if let customFileURL {
            return customFileURL
        }
        return URL.applicationSupportDirectory
            .appending(path: "CairnOps")
            .appending(path: "snapshot.json")
    }
}
