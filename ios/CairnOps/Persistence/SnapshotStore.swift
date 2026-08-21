import Foundation

/// Cache local du dernier etat connu.
///
/// L'ecriture est volontairement paresseuse : elle sert a reafficher quelque
/// chose au lancement suivant, pas a garantir une durabilite transactionnelle.
/// La sauvegarde est donc groupee et n'interrompt jamais l'interface.
actor SnapshotStore {
    private static let defaultWriteInterval = Duration.seconds(10)

    private let decoder = JSONDecoder()
    private let encoder = JSONEncoder()
    private let customFileURL: URL?
    private let writeInterval: Duration

    private var pendingSnapshot: AppSnapshot?
    private var flushTask: Task<Void, Never>?
    private var flushGeneration = 0

    init(
        fileURL: URL? = nil,
        writeInterval: Duration = SnapshotStore.defaultWriteInterval
    ) {
        customFileURL = fileURL
        self.writeInterval = writeInterval
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

        let generation = flushGeneration
        flushTask = Task { [weak self, writeInterval] in
            do {
                try await Task.sleep(for: writeInterval)
            } catch {
                return
            }
            await self?.flush(ifCurrentGeneration: generation)
        }
    }

    /// Force l'ecriture immediate, par exemple avant une mise en veille.
    func flushNow() {
        cancelScheduledFlush()
        flush()
    }

    func clear() {
        cancelScheduledFlush()
        pendingSnapshot = nil
        try? FileManager.default.removeItem(at: snapshotFileURL())
    }

    private func flush() {
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

    private func flush(ifCurrentGeneration generation: Int) {
        guard generation == flushGeneration else {
            return
        }
        flushTask = nil
        flush()
    }

    private func cancelScheduledFlush() {
        flushGeneration &+= 1
        flushTask?.cancel()
        flushTask = nil
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
