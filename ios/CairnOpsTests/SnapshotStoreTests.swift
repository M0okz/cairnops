import Foundation
import XCTest
@testable import CairnOps

final class SnapshotStoreTests: XCTestCase {
    func testCancelledFlushCannotConsumeANewerPendingSnapshot() async throws {
        let directory = FileManager.default.temporaryDirectory
            .appending(path: UUID().uuidString, directoryHint: .isDirectory)
        let fileURL = directory.appending(path: "snapshot.json")
        defer { try? FileManager.default.removeItem(at: directory) }

        let store = SnapshotStore(fileURL: fileURL, writeInterval: .seconds(60))
        var first = AppSnapshot()
        first.lastRefreshAt = "first"
        await store.save(first)
        await store.flushNow()

        var second = AppSnapshot()
        second.lastRefreshAt = "second"
        await store.save(second)

        // L'ancienne tache vient d'etre annulee par flushNow. Elle peut encore
        // reprendre son execution, mais ne doit ni ecrire ni vider `second`.
        for _ in 0..<20 {
            await Task.yield()
        }

        let persistedBeforeFlush = await store.load()
        XCTAssertEqual(persistedBeforeFlush?.lastRefreshAt, "first")

        await store.flushNow()
        let persistedAfterFlush = await store.load()
        XCTAssertEqual(persistedAfterFlush?.lastRefreshAt, "second")
    }

    func testBurstPersistsOnlyItsLatestSnapshot() async throws {
        let directory = FileManager.default.temporaryDirectory
            .appending(path: UUID().uuidString, directoryHint: .isDirectory)
        let fileURL = directory.appending(path: "snapshot.json")
        defer { try? FileManager.default.removeItem(at: directory) }

        let store = SnapshotStore(fileURL: fileURL, writeInterval: .seconds(60))
        for marker in ["one", "two", "three"] {
            var snapshot = AppSnapshot()
            snapshot.lastRefreshAt = marker
            await store.save(snapshot)
        }

        let persistedBeforeFlush = await store.load()
        XCTAssertNil(persistedBeforeFlush)

        await store.flushNow()
        let persisted = await store.load()
        XCTAssertEqual(persisted?.lastRefreshAt, "three")
    }
}
