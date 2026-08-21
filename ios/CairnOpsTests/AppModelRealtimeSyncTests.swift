import Observation
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

    func testNotificationBurstCoalescesIntoOneInboxRead() async {
        let recorder = CallRecorder()
        let model = AppModel()
        model.debugInstallSyncHooks(
            inbox: {
                await recorder.recordInbox()
                return CairnOpsAPI.InboxPayload(entries: [], unread: 0)
            }
        )
        model.debugSetUser(Self.fixtureUser())

        for _ in 0..<25 {
            model.debugQueueRealtimeEvent(kind: "notification.changed")
        }
        await model.debugFlushPendingRealtimeScopes()

        let counts = await recorder.snapshot()
        XCTAssertEqual(counts.inbox, 1)
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

    func testUnknownEventFallsBackToFullProjection() async {
        let recorder = CallRecorder()
        let model = AppModel()
        model.debugInstallSyncHooks(
            projection: {
                await recorder.recordProjection()
                return Self.emptyProjection()
            }
        )
        model.debugSetUser(Self.fixtureUser())

        model.debugQueueRealtimeEvent(kind: "future.changed")
        await model.debugFlushPendingRealtimeScopes()

        let counts = await recorder.snapshot()
        XCTAssertEqual(counts.projection, 1)
        XCTAssertEqual(counts.inbox, 0)
        XCTAssertEqual(counts.health, 0)
    }

    func testIncidentEventRefreshesOnlyIncidents() async {
        let recorder = CallRecorder()
        let model = AppModel()
        model.debugInstallSyncHooks(
            projection: {
                await recorder.recordProjection()
                return Self.emptyProjection()
            },
            incidents: {
                await recorder.recordIncidents()
                return []
            },
            inbox: {
                await recorder.recordInbox()
                return CairnOpsAPI.InboxPayload(entries: [], unread: 0)
            }
        )
        model.debugSetUser(Self.fixtureUser())

        model.debugQueueRealtimeEvent(kind: "incident.changed")
        await model.debugFlushPendingRealtimeScopes()

        let counts = await recorder.snapshot()
        XCTAssertEqual(counts.incidents, 1)
        XCTAssertEqual(counts.projection, 0)
        XCTAssertEqual(counts.targets, 0)
        XCTAssertEqual(counts.measures, 0)
        XCTAssertEqual(counts.inbox, 0)
    }

    func testObservationEventRefreshesTargetsAndMeasuresOnly() async {
        let recorder = CallRecorder()
        let model = AppModel()
        model.debugInstallSyncHooks(
            targets: {
                await recorder.recordTargets()
                return []
            },
            incidents: {
                await recorder.recordIncidents()
                return []
            },
            measures: {
                await recorder.recordMeasures()
                return []
            }
        )
        model.debugSetUser(Self.fixtureUser())

        model.debugQueueRealtimeEvent(kind: "observation.created")
        await model.debugFlushPendingRealtimeScopes()

        let counts = await recorder.snapshot()
        XCTAssertEqual(counts.targets, 1)
        XCTAssertEqual(counts.measures, 1)
        XCTAssertEqual(counts.incidents, 0)
        XCTAssertEqual(counts.projection, 0)
        XCTAssertEqual(counts.inbox, 0)
        XCTAssertEqual(counts.health, 0)
    }

    func testRealtimeCursorAdvancesOnlyAfterProjectionWasApplied() async {
        let model = AppModel()
        model.debugInstallSyncHooks(projection: { Self.emptyProjection() })
        model.debugSetUser(Self.fixtureUser())

        model.debugReceiveRealtimeMessage(type: "event", kind: "future.changed", version: 42)

        XCTAssertNil(model.snapshot.realtimeVersion)

        await model.debugFlushPendingRealtimeScopes()

        XCTAssertEqual(model.snapshot.realtimeVersion, 42)
    }

    func testFailedRealtimeScopeIsRetriedInsteadOfBeingDropped() async {
        let loader = FailingOnceProjectionLoader()
        let model = AppModel()
        model.debugInstallSyncHooks(projection: { try await loader.load() })
        model.debugSetUser(Self.fixtureUser())

        model.debugQueueRealtimeEvent(kind: "future.changed")
        await model.debugFlushPendingRealtimeScopes()
        await model.debugFlushPendingRealtimeScopes()

        let calls = await loader.callCount()
        XCTAssertEqual(calls, 2)
        XCTAssertEqual(model.snapshot.realtimeVersion, 0)
    }

    func testRealtimeSyncNeverOverlapsAnotherSync() async {
        let loader = ControlledProjectionLoader()
        let model = AppModel()
        model.debugInstallSyncHooks(projection: { try await loader.load() })
        model.debugSetUser(Self.fixtureUser())

        model.debugQueueRealtimeEvent(kind: "future.changed")
        let firstFlush = Task { await model.debugFlushPendingRealtimeScopes() }
        await loader.waitUntilFirstCallStarts()

        model.debugQueueRealtimeEvent(kind: "future.changed")
        await model.debugFlushPendingRealtimeScopes()

        let overlapping = await loader.stats()
        XCTAssertEqual(overlapping.calls, 1)
        XCTAssertEqual(overlapping.maximumConcurrentCalls, 1)

        await loader.releaseFirstCall()
        await firstFlush.value
        await model.debugFlushPendingRealtimeScopes()

        let completed = await loader.stats()
        XCTAssertEqual(completed.calls, 2)
        XCTAssertEqual(completed.maximumConcurrentCalls, 1)
    }

    func testReadyFramePerformsCatchUpBeforeAdvancingCursor() async {
        let recorder = CallRecorder()
        let model = AppModel()
        model.debugInstallSyncHooks(
            projection: {
                await recorder.recordProjection()
                return Self.emptyProjection()
            }
        )
        model.debugSetUser(Self.fixtureUser())

        model.debugReceiveRealtimeMessage(type: "ready", kind: nil, version: 7)
        XCTAssertNil(model.snapshot.realtimeVersion)

        await model.debugFlushPendingRealtimeScopes()

        let counts = await recorder.snapshot()
        XCTAssertEqual(counts.projection, 1)
        XCTAssertEqual(model.snapshot.realtimeVersion, 7)
    }

    func testRealtimeFramesDoNotResetBackoffForAFlappingConnection() {
        let model = AppModel()
        model.debugSetUser(Self.fixtureUser())
        model.debugSetReconnectAttempt(4)

        model.debugReceiveRealtimeMessage(type: "event", kind: "device.changed", version: 1)

        XCTAssertEqual(model.debugReconnectAttempt, 4)
    }

    func testReconnectBackoffIsExponentialAndCapped() {
        let model = AppModel()

        for (attempt, expectedDelay) in [(0, 2), (1, 4), (2, 8), (4, 32), (5, 60), (10, 60)] {
            model.debugSetReconnectAttempt(attempt)
            XCTAssertEqual(
                model.debugReconnectDelayWithoutJitter,
                .seconds(expectedDelay),
                "unexpected delay for attempt \(attempt)"
            )
        }
    }

    func testDeviceEventAdvancesIgnoredCursorWithoutInvalidatingSnapshot() async {
        let model = AppModel()
        model.debugSetUser(Self.fixtureUser())

        model.debugReceiveRealtimeMessage(type: "event", kind: "device.changed", version: 9)
        await model.debugFlushPendingRealtimeScopes()

        XCTAssertEqual(model.debugRealtimeCursor, 9)
        XCTAssertNil(model.snapshot.realtimeVersion)
    }

    func testOnlineRealtimeStateIsNotInvalidatedForEveryFrame() async {
        let model = AppModel()
        model.debugSetUser(Self.fixtureUser())
        model.debugReceiveRealtimeMessage(type: "event", kind: "device.changed", version: 1)
        let invalidated = expectation(description: "online state remains stable")
        invalidated.isInverted = true

        withObservationTracking {
            _ = model.realtimeState
        } onChange: {
            invalidated.fulfill()
        }

        model.debugReceiveRealtimeMessage(type: "event", kind: "device.changed", version: 2)

        await fulfillment(of: [invalidated], timeout: 0.05)
        await model.debugFlushPendingRealtimeScopes()
    }

    func testScenePhaseChangeInvalidatesRealtimeTaskIdentity() async {
        let model = AppModel()
        model.debugSetUser(Self.fixtureUser())
        let invalidated = expectation(description: "realtime task identity invalidated")

        withObservationTracking {
            _ = model.realtimeIdentity
        } onChange: {
            invalidated.fulfill()
        }

        model.setScenePhaseActive(false)

        await fulfillment(of: [invalidated], timeout: 0.1)
        XCTAssertNil(model.realtimeIdentity)
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

    static func emptyProjection() -> CairnOpsAPI.OperationalProjection {
        CairnOpsAPI.OperationalProjection(
            targets: [],
            incidents: [],
            measures: [],
            inbox: CairnOpsAPI.InboxPayload(entries: [], unread: 0)
        )
    }
}

private actor CallRecorder {
    private var projection = 0
    private var targets = 0
    private var incidents = 0
    private var measures = 0
    private var inbox = 0
    private var health = 0

    func recordProjection() {
        projection += 1
    }

    func recordTargets() {
        targets += 1
    }

    func recordIncidents() {
        incidents += 1
    }

    func recordMeasures() {
        measures += 1
    }

    func recordInbox() {
        inbox += 1
    }

    func recordHealth() {
        health += 1
    }

    func snapshot() -> (
        projection: Int,
        targets: Int,
        incidents: Int,
        measures: Int,
        inbox: Int,
        health: Int
    ) {
        (projection, targets, incidents, measures, inbox, health)
    }
}

private actor FailingOnceProjectionLoader {
    private var calls = 0

    func load() throws -> CairnOpsAPI.OperationalProjection {
        calls += 1
        if calls == 1 {
            throw URLError(.timedOut)
        }
        return CairnOpsAPI.OperationalProjection(
            targets: [],
            incidents: [],
            measures: [],
            inbox: CairnOpsAPI.InboxPayload(entries: [], unread: 0)
        )
    }

    func callCount() -> Int {
        calls
    }
}

private actor ControlledProjectionLoader {
    private var calls = 0
    private var concurrentCalls = 0
    private var maximumConcurrentCalls = 0
    private var firstCallStarted = false
    private var firstCallStartWaiters: [CheckedContinuation<Void, Never>] = []
    private var firstCallRelease: CheckedContinuation<Void, Never>?

    func load() async throws -> CairnOpsAPI.OperationalProjection {
        calls += 1
        concurrentCalls += 1
        maximumConcurrentCalls = max(maximumConcurrentCalls, concurrentCalls)
        defer { concurrentCalls -= 1 }

        if calls == 1 {
            firstCallStarted = true
            let waiters = firstCallStartWaiters
            firstCallStartWaiters = []
            waiters.forEach { $0.resume() }
            await withCheckedContinuation { continuation in
                firstCallRelease = continuation
            }
        }

        return CairnOpsAPI.OperationalProjection(
            targets: [],
            incidents: [],
            measures: [],
            inbox: CairnOpsAPI.InboxPayload(entries: [], unread: 0)
        )
    }

    func waitUntilFirstCallStarts() async {
        guard !firstCallStarted else {
            return
        }
        await withCheckedContinuation { continuation in
            firstCallStartWaiters.append(continuation)
        }
    }

    func releaseFirstCall() {
        firstCallRelease?.resume()
        firstCallRelease = nil
    }

    func stats() -> (calls: Int, maximumConcurrentCalls: Int) {
        (calls, maximumConcurrentCalls)
    }
}
