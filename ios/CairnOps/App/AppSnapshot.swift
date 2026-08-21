import Foundation

struct AppSnapshot: Codable, Equatable, Sendable {
    enum TargetHealth: String, Codable, Sendable {
        case ok
        case degraded
        case down
        case maintenance
        case unknown
    }

    enum GlobalStatus: String, Codable, Sendable {
        case notConfigured
        case ongoingIncident
        case degradedServices
        case incompleteMonitoring
        case allOperational
    }

    /// Projections derivees de `targets`, `incidents` et `measures`.
    ///
    /// Ces valeurs etaient auparavant recalculees a chaque acces : la sante
    /// d'une cible reparcourait toute la liste d'incidents, et le tri appelait
    /// ce calcul dans son comparateur, soit un cout quadratique paye plusieurs
    /// fois par rendu. On les calcule desormais une seule fois par mise a jour.
    private struct Derived: Sendable {
        var incidentsByTarget: [String: [Incident]] = [:]
        var healthByTarget: [String: TargetHealth] = [:]
        var sortedTargets: [Target] = []
        var actionableIncidents: [Incident] = []
        var unacknowledgedIncidents: [Incident] = []
        var globalStatus: GlobalStatus = .notConfigured
        var freshestObservationAt: String?
    }

    var serverBaseURL = ""
    var targets: [Target] = [] { didSet { derived = nil } }
    var incidents: [Incident] = [] { didSet { derived = nil } }
    var measures: [String: TargetMeasures] = [:] { didSet { derived = nil } }
    var systemHealth: SystemHealth?
    var inbox: [InboxEntry] = []
    var unreadCount = 0
    var lastRefreshAt: String?
    var realtimeVersion: Int64?

    /// `nil` tant que l'index n'a pas ete reconstruit. Les accesseurs retombent
    /// alors sur un calcul direct : le resultat reste exact, seul le cout change.
    private var derived: Derived?

    init() {}

    // MARK: - Mise a jour

    /// Applique une projection complete puis reconstruit l'index une seule fois.
    mutating func applyProjection(
        targets: [Target],
        incidents: [Incident],
        measures: [String: TargetMeasures],
        inbox: [InboxEntry],
        unreadCount: Int
    ) {
        self.targets = targets
        self.incidents = incidents
        self.measures = measures
        self.inbox = inbox
        self.unreadCount = unreadCount
        rebuildDerived()
    }

    mutating func rebuildDerived() {
        derived = Self.makeDerived(targets: targets, incidents: incidents, measures: measures)
    }

    // MARK: - Lectures

    var hasProjection: Bool {
        !targets.isEmpty || !incidents.isEmpty || systemHealth != nil || !measures.isEmpty
    }

    var actionableIncidents: [Incident] {
        derived?.actionableIncidents ?? Self.makeActionableIncidents(from: incidents)
    }

    var unacknowledgedIncidents: [Incident] {
        derived?.unacknowledgedIncidents ?? Self.makeUnacknowledgedIncidents(from: actionableIncidents)
    }

    var freshestObservationAt: String? {
        if let derived {
            return derived.freshestObservationAt
        }
        return Self.makeFreshestObservationAt(from: measures)
    }

    var globalStatus: GlobalStatus {
        if let derived {
            return derived.globalStatus
        }
        return Self.makeGlobalStatus(states: targets.map(health(for:)))
    }

    var sortedTargets: [Target] {
        if let derived {
            return derived.sortedTargets
        }
        return Self.makeSortedTargets(targets, healthByTarget: healthLookup())
    }

    func health(for target: Target) -> TargetHealth {
        if let cached = derived?.healthByTarget[target.id] {
            return cached
        }

        return Self.makeHealth(
            for: target,
            incidents: incidents.filter { $0.targetID == target.id },
            measures: measures[target.id]
        )
    }

    // MARK: - Construction de l'index

    private func healthLookup() -> [String: TargetHealth] {
        if let derived {
            return derived.healthByTarget
        }

        let byTarget = Self.makeIncidentsByTarget(incidents)
        return Self.makeHealthByTarget(targets: targets, incidentsByTarget: byTarget, measures: measures)
    }

    private static func makeDerived(
        targets: [Target],
        incidents: [Incident],
        measures: [String: TargetMeasures]
    ) -> Derived {
        let incidentsByTarget = makeIncidentsByTarget(incidents)
        let healthByTarget = makeHealthByTarget(
            targets: targets,
            incidentsByTarget: incidentsByTarget,
            measures: measures
        )
        let actionable = makeActionableIncidents(from: incidents)

        return Derived(
            incidentsByTarget: incidentsByTarget,
            healthByTarget: healthByTarget,
            sortedTargets: makeSortedTargets(targets, healthByTarget: healthByTarget),
            actionableIncidents: actionable,
            unacknowledgedIncidents: makeUnacknowledgedIncidents(from: actionable),
            globalStatus: makeGlobalStatus(states: targets.map { healthByTarget[$0.id] ?? .unknown }),
            freshestObservationAt: makeFreshestObservationAt(from: measures)
        )
    }

    private static func makeIncidentsByTarget(_ incidents: [Incident]) -> [String: [Incident]] {
        var byTarget: [String: [Incident]] = [:]
        byTarget.reserveCapacity(incidents.count)

        for incident in incidents {
            byTarget[incident.targetID, default: []].append(incident)
        }

        return byTarget
    }

    private static func makeHealthByTarget(
        targets: [Target],
        incidentsByTarget: [String: [Incident]],
        measures: [String: TargetMeasures]
    ) -> [String: TargetHealth] {
        var healthByTarget: [String: TargetHealth] = [:]
        healthByTarget.reserveCapacity(targets.count)

        for target in targets {
            healthByTarget[target.id] = makeHealth(
                for: target,
                incidents: incidentsByTarget[target.id] ?? [],
                measures: measures[target.id]
            )
        }

        return healthByTarget
    }

    private static func makeHealth(
        for target: Target,
        incidents ownIncidents: [Incident],
        measures: TargetMeasures?
    ) -> TargetHealth {
        // Un seul parcours suffit : on agrege les drapeaux au lieu de derouler
        // plusieurs `filter`/`contains` successifs sur la meme liste.
        var hasActiveMaintenance = false
        var hasBlockingOperationalIncident = false
        var hasBlockingPostureIncident = false
        var hasWarning = false
        var hasInformation = false

        for incident in ownIncidents {
            if incident.maintenanceActive {
                hasActiveMaintenance = true
            }

            let severity = incident.effectiveSeverity
            let isBlocking = severity == .critical || severity == .major

            if incident.isPostureNature {
                if isBlocking {
                    hasBlockingPostureIncident = true
                }
            } else if isBlocking {
                hasBlockingOperationalIncident = true
            }

            if severity == .warning {
                hasWarning = true
            }
            if severity == .information {
                hasInformation = true
            }
        }

        if hasActiveMaintenance {
            return .maintenance
        }
        if hasBlockingOperationalIncident {
            return .down
        }
        if hasBlockingPostureIncident || hasWarning {
            return .degraded
        }
        if hasInformation {
            return .unknown
        }

        if target.sources.isEmpty,
           let sources = measures?.sources,
           !sources.contains(where: \.measuresAvailability) {
            return .unknown
        }
        if target.sources.isEmpty, target.externalSourceCount == 0 {
            return .unknown
        }

        return .ok
    }

    private static func makeSortedTargets(
        _ targets: [Target],
        healthByTarget: [String: TargetHealth]
    ) -> [Target] {
        targets.sorted { lhs, rhs in
            let lhsPriority = priority(for: healthByTarget[lhs.id] ?? .unknown)
            let rhsPriority = priority(for: healthByTarget[rhs.id] ?? .unknown)
            if lhsPriority != rhsPriority {
                return lhsPriority < rhsPriority
            }
            return lhs.name.localizedStandardCompare(rhs.name) == .orderedAscending
        }
    }

    private static func makeActionableIncidents(from incidents: [Incident]) -> [Incident] {
        incidents.filter { !$0.maintenanceActive }
    }

    private static func makeUnacknowledgedIncidents(from actionable: [Incident]) -> [Incident] {
        actionable.filter { !$0.isAcknowledged }
    }

    private static func makeGlobalStatus(states: [TargetHealth]) -> GlobalStatus {
        guard !states.isEmpty else {
            return .notConfigured
        }

        var hasDegraded = false
        var hasUnknown = false

        for state in states {
            switch state {
            case .down:
                return .ongoingIncident
            case .degraded, .maintenance:
                hasDegraded = true
            case .unknown:
                hasUnknown = true
            case .ok:
                break
            }
        }

        if hasDegraded {
            return .degradedServices
        }
        if hasUnknown {
            return .incompleteMonitoring
        }
        return .allOperational
    }

    private static func makeFreshestObservationAt(from measures: [String: TargetMeasures]) -> String? {
        // L'ancienne version reparsait les deux dates a chaque comparaison du
        // `max`. On parse une fois par valeur et on compare des `Date`.
        var freshestValue: String?
        var freshestDate = Date.distantPast

        for entry in measures.values {
            guard let value = entry.latestObservedAt,
                  let date = TimestampParser.date(from: value) else {
                continue
            }

            if freshestValue == nil || date > freshestDate {
                freshestValue = value
                freshestDate = date
            }
        }

        return freshestValue
    }

    private static func priority(for health: TargetHealth) -> Int {
        switch health {
        case .down:
            0
        case .degraded:
            1
        case .maintenance:
            2
        case .unknown:
            3
        case .ok:
            4
        }
    }

    // MARK: - Codable / Equatable

    private enum CodingKeys: String, CodingKey {
        case serverBaseURL
        case targets
        case incidents
        case measures
        case systemHealth
        case inbox
        case unreadCount
        case lastRefreshAt
        case realtimeVersion
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        serverBaseURL = try container.decodeIfPresent(String.self, forKey: .serverBaseURL) ?? ""
        targets = try container.decodeIfPresent([Target].self, forKey: .targets) ?? []
        incidents = try container.decodeIfPresent([Incident].self, forKey: .incidents) ?? []
        measures = try container.decodeIfPresent([String: TargetMeasures].self, forKey: .measures) ?? [:]
        systemHealth = try container.decodeIfPresent(SystemHealth.self, forKey: .systemHealth)
        inbox = try container.decodeIfPresent([InboxEntry].self, forKey: .inbox) ?? []
        unreadCount = try container.decodeIfPresent(Int.self, forKey: .unreadCount) ?? 0
        lastRefreshAt = try container.decodeIfPresent(String.self, forKey: .lastRefreshAt)
        realtimeVersion = try container.decodeIfPresent(Int64.self, forKey: .realtimeVersion)
        rebuildDerived()
    }

    /// L'index derive est exclu : il est entierement determine par les champs
    /// sources, et le comparer couterait cher pour un resultat identique.
    static func == (lhs: AppSnapshot, rhs: AppSnapshot) -> Bool {
        lhs.serverBaseURL == rhs.serverBaseURL
            && lhs.unreadCount == rhs.unreadCount
            && lhs.lastRefreshAt == rhs.lastRefreshAt
            && lhs.realtimeVersion == rhs.realtimeVersion
            && lhs.systemHealth == rhs.systemHealth
            && lhs.targets == rhs.targets
            && lhs.incidents == rhs.incidents
            && lhs.measures == rhs.measures
            && lhs.inbox == rhs.inbox
    }
}
