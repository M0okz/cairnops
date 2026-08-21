import XCTest
@testable import CairnOps

@MainActor
final class AppModelRealtimeSyncTests: XCTestCase {
    func testNotificationEventRefreshesOnlyInbox() async {
        let recorder = CallRecorder()
        let model = AppModel()
        model.debugInstallSyncHooks(
            inbox: {
                await recorder.recordInbox()
                return CairnOpsAPI.InboxPayload(entries: [], unread: 0)
            }
        )
        model.debugSetUser(Self.fixtureUser())

        model.debugQueueRealtimeEvent(kind: "notification.changed")
        await model.debugFlushPendingRealtimeScopes()

        let counts = await recorder.snapshot()
        XCTAssertEqual(counts.inbox, 1)
        XCTAssertEqual(counts.health, 0)
        XCTAssertEqual(counts.projection, 0)
    }

    func testHeartbeatEventRefreshesOnlyHealth() async {
        let recorder = CallRecorder()
        let model = AppModel()
        model.debugInstallSyncHooks(
            health: {
                await recorder.recordHealth()
                return nil
            }
        )
        model.debugSetUser(Self.fixtureUser())

        model.debugQueueRealtimeEvent(kind: "component.heartbeat")
        await model.debugFlushPendingRealtimeScopes()

        let counts = await recorder.snapshot()
        XCTAssertEqual(counts.inbox, 0)
        XCTAssertEqual(counts.health, 1)
        XCTAssertEqual(counts.projection, 0)
    }

    func testProjectionEventRefreshesProjectionWithoutRedundantInboxRead() async {
        let recorder = CallRecorder()
        let model = AppModel()
        model.debugInstallSyncHooks(
            projection: {
                await recorder.recordProjection()
                return CairnOpsAPI.OperationalProjection(
                    targets: [],
                    incidents: [],
                    measures: [],
                    inbox: CairnOpsAPI.InboxPayload(entries: [], unread: 0)
                )
            }
        )
        model.debugSetUser(Self.fixtureUser())

        model.debugQueueRealtimeEvent(kind: "incident.changed")
        await model.debugFlushPendingRealtimeScopes()

        let counts = await recorder.snapshot()
        XCTAssertEqual(counts.projection, 1)
        XCTAssertEqual(counts.inbox, 0)
        XCTAssertEqual(counts.health, 0)
    }

    func testPendingRealtimeWorkStopsWhenSceneBecomesInactive() async {
        let recorder = CallRecorder()
        let model = AppModel()
        model.debugInstallSyncHooks(
            inbox: {
                await recorder.recordInbox()
                return CairnOpsAPI.InboxPayload(entries: [], unread: 0)
            }
        )
        model.debugSetUser(Self.fixtureUser())

        model.debugQueueRealtimeEvent(kind: "notification.changed")
        model.setScenePhaseActive(false)
        await model.debugFlushPendingRealtimeScopes()

        XCTAssertNil(model.realtimeIdentity)
        let counts = await recorder.snapshot()
        XCTAssertEqual(counts.projection, 0)
        XCTAssertEqual(counts.inbox, 0)
        XCTAssertEqual(counts.health, 0)
    }
}

private extension AppModelRealtimeSyncTests {
    static func fixtureUser() -> User {
        User(
            id: UUID().uuidString,
            username: "ops",
            displayName: "Ops",
            role: .administrator
        )
    }
}

private actor CallRecorder {
    private(set) var projection = 0
    private(set) var inbox = 0
    private(set) var health = 0

    func recordProjection() {
        projection += 1
    }

    func recordInbox() {
        inbox += 1
    }

    func recordHealth() {
        health += 1
    }

    func snapshot() -> (projection: Int, inbox: Int, health: Int) {
        (projection, inbox, health)
    }
}
