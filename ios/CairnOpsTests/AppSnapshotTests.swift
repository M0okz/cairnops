import XCTest
@testable import CairnOps

final class AppSnapshotTests: XCTestCase {
    func testCriticalIncidentMakesTargetUnavailable() {
        let target = makeTarget(name: "API", externalSourceCount: 1)
        let incident = makeIncident(targetID: target.id, severity: .critical)

        var snapshot = AppSnapshot()
        snapshot.targets = [target]
        snapshot.incidents = [incident]

        XCTAssertEqual(snapshot.health(for: target), .down)
        XCTAssertEqual(snapshot.globalStatus, .ongoingIncident)
    }

    func testTargetWithoutAnySourceStaysUnknown() {
        let target = makeTarget(name: "Worker", externalSourceCount: 0)

        var snapshot = AppSnapshot()
        snapshot.targets = [target]

        XCTAssertEqual(snapshot.health(for: target), .unknown)
        XCTAssertEqual(snapshot.globalStatus, .incompleteMonitoring)
    }

    func testResolvedIncidentDoesNotKeepTargetUnavailable() {
        let target = makeTarget(name: "API", externalSourceCount: 1)
        let incident = makeIncident(
            targetID: target.id,
            severity: .critical,
            status: "resolved"
        )

        var snapshot = AppSnapshot()
        snapshot.targets = [target]
        snapshot.incidents = [incident]

        XCTAssertEqual(snapshot.health(for: target), .ok)
        XCTAssertEqual(snapshot.globalStatus, .allOperational)
    }

    func testResolvedIncidentDoesNotRemainActionable() {
        let target = makeTarget(name: "API", externalSourceCount: 1)
        let incident = makeIncident(
            targetID: target.id,
            severity: .major,
            status: "resolved"
        )

        var snapshot = AppSnapshot()
        snapshot.targets = [target]
        snapshot.incidents = [incident]

        XCTAssertTrue(snapshot.actionableIncidents.isEmpty)
        XCTAssertTrue(snapshot.unacknowledgedIncidents.isEmpty)
    }

    private func makeTarget(name: String, externalSourceCount: Int) -> Target {
        Target(
            id: UUID().uuidString,
            name: name,
            description: "",
            createdAt: Date.now.ISO8601Format(),
            externalSourceCount: externalSourceCount,
            sources: []
        )
    }

    private func makeIncident(
        targetID: String,
        severity: IncidentSeverity,
        status: String = "active"
    ) -> Incident {
        let resolvedAt = status == "resolved" ? Date.now.ISO8601Format() : nil

        return Incident(
            id: UUID().uuidString,
            targetID: targetID,
            targetName: "API",
            natureKey: "availability",
            natureLabel: "Indisponibilite",
            status: status,
            sourceSeverity: severity,
            effectiveSeverity: severity,
            openedAt: Date.now.ISO8601Format(),
            resolvedAt: resolvedAt,
            acknowledgedAt: nil,
            acknowledgedBy: nil,
            acknowledgementOrigin: nil,
            acknowledgementSyncStatus: "not_applicable",
            acknowledgementSyncError: nil,
            maintenanceActive: false,
            maintenanceEndsAt: nil,
            signals: [],
            activity: [],
            createdAt: Date.now.ISO8601Format(),
            updatedAt: Date.now.ISO8601Format()
        )
    }
}
