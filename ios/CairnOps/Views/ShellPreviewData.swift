#if DEBUG
import Foundation

/// Projection d'exemple pour la verification visuelle de la coque.
///
/// Elle reprend la flotte de la maquette afin que les ecrans puissent etre
/// controles en clair et en sombre sans instance appairee. Le jeu passe par le
/// decodeur reel : une fixture qui contournerait `Codable` ne prouverait rien
/// sur la forme effective du contrat.
enum ShellPreviewData {

    static let launchArgument = "-CairnOpsShellPreview"

    static var isEnabled: Bool {
        ProcessInfo.processInfo.arguments.contains(launchArgument)
    }

    /// Onglet ouvert au lancement, pour capturer un écran sans le parcourir.
    static var initialTab: AppTab {
        value(for: "-CairnOpsShellTab").flatMap(AppTab.init(rawValue:)) ?? .overview
    }

    /// Écran de détail présenté directement, hors de la coque.
    enum Detail: String {
        case incident
        case target
        case settings
    }

    static var detail: Detail? {
        value(for: "-CairnOpsShellDetail").flatMap(Detail.init(rawValue:))
    }

    static let previewIncidentID = "inc-4821"
    static let previewTargetID = "api-gw-02"

    private static func value(for flag: String) -> String? {
        let arguments = ProcessInfo.processInfo.arguments
        guard let index = arguments.firstIndex(of: flag),
              arguments.index(after: index) < arguments.endIndex else {
            return nil
        }
        return arguments[arguments.index(after: index)]
    }

    static func makeSnapshot() -> AppSnapshot {
        var snapshot = AppSnapshot()
        snapshot.serverBaseURL = "https://cairnops.int.homeblack.fr"
        snapshot.lastRefreshAt = iso(-12)
        snapshot.systemHealth = decode(SystemHealth.self, from: healthJSON)
        snapshot.applyProjection(
            targets: decode([Target].self, from: targetsJSON) ?? [],
            incidents: decode([Incident].self, from: incidentsJSON) ?? [],
            measures: makeMeasures(),
            indicatorTargets: makeIndicators(),
            inbox: [],
            unreadCount: 3
        )
        return snapshot
    }

    static func makeUser() -> User {
        User(
            id: "00000000-0000-4000-8000-000000000001",
            username: "gregory",
            displayName: "Grégory",
            role: .administrator
        )
    }

    // MARK: - Fabrication

    private static func makeMeasures() -> [String: TargetMeasures] {
        let plans: [(id: String, availability: Double, latency: Double, seed: UInt64)] = [
            ("api-gw-02", 0.918, 1_240, 91),
            ("db-primary-01", 0.964, 840, 23),
            ("log-store-04", 0.995, 61, 37),
            ("edge-lb-01", 0.981, 318, 59),
            ("worker-07", 0.998, 48, 71),
            ("api-gw-01", 0.9997, 112, 13),
            ("db-replica-02", 0.9992, 96, 17),
            ("cache-redis-01", 1, 4, 29),
            ("edge-lb-02", 0.9994, 104, 41),
            ("cdn-origin-03", 0.9998, 87, 53),
            ("ci-runner-11", 0.9989, 39, 67),
        ]

        return plans.reduce(into: [:]) { result, plan in
            result[plan.id] = TargetMeasures(
                targetID: plan.id,
                measures: [
                    TargetMeasures.Measure(
                        window: .last24Hours,
                        availability: plan.availability,
                        coverage: 0.99,
                        averageLatencyMilliseconds: plan.latency,
                        maximumLatencyMilliseconds: plan.latency * 1.8,
                        conclusiveObservations: 1_420,
                        unknownObservations: 12,
                        expectedObservations: 1_440
                    ),
                ],
                trend: series(seed: plan.seed, count: 24, start: plan.availability, spread: 0.015, minimum: 0.82, maximum: 1),
                latencyTrend: series(seed: plan.seed &+ 7, count: 44, start: plan.latency, spread: plan.latency * 0.12, minimum: plan.latency * 0.55, maximum: plan.latency * 1.6),
                latestObservedAt: iso(-18),
                sources: sourceMeasures(for: plan.id)
            )
        }
    }

    /// Les Sources telles que la projection des mesures les decrit : natives
    /// et importees melees, chacune nommee.
    private static func sourceMeasures(for targetID: String) -> [TargetMeasures.SourceMeasures] {
        let plans: [(suffix: String, name: String, kind: String, origin: String, outcome: String)]

        switch targetID {
        case "api-gw-02":
            plans = [
                ("http-a", "Sonde HTTP eu-west-1a", "http", "native", "unhealthy"),
                ("http-b", "Sonde HTTP eu-west-1b", "http", "native", "unhealthy"),
                ("prom", "Prometheus · probe_success", "metric", "integration", "unhealthy"),
            ]
        case "db-primary-01":
            plans = [
                ("tcp", "Sonde TCP 5432", "tcp", "native", "healthy"),
                ("agent", "Agent système", "metric", "integration", "unhealthy"),
            ]
        case "edge-lb-01":
            plans = [
                ("http", "Sonde HTTP publique", "http", "native", "healthy"),
                ("tls", "Contrôle certificat TLS", "http", "native", "unhealthy"),
            ]
        default:
            plans = [("http", "Sonde HTTP", "http", "native", "healthy")]
        }

        return plans.map { plan in
            TargetMeasures.SourceMeasures(
                sourceID: "\(targetID)-src-\(plan.suffix)",
                name: plan.name,
                kind: plan.kind,
                origin: plan.origin,
                measuresAvailability: plan.origin == "native",
                latestOutcome: plan.outcome,
                latestObservedAt: iso(-24),
                measures: []
            )
        }
    }

    private static func makeIndicators() -> [String: TargetIndicators] {
        let plans: [(id: String, cpu: Double, memory: Double, pinned: Bool)] = [
            ("api-gw-02", 96, 71, true),
            ("worker-07", 78, 66, true),
            ("db-primary-01", 64, 88, false),
            ("ci-runner-11", 55, 43, false),
        ]

        return plans.reduce(into: [:]) { result, plan in
            let cpuID = "\(plan.id)#cpu"
            let memoryID = "\(plan.id)#memory"

            result[plan.id] = TargetIndicators(
                targetID: plan.id,
                generatedAt: iso(-20),
                indicators: [
                    indicator(id: cpuID, target: plan.id, key: "cpu.utilization", label: "Utilisation CPU", value: plan.cpu, pinned: plan.pinned, position: 0),
                    indicator(id: memoryID, target: plan.id, key: "memory.utilization", label: "Mémoire", value: plan.memory, pinned: false, position: nil),
                ],
                series: [
                    cpuID: points(series(seed: UInt64(plan.cpu), count: 26, start: plan.cpu, spread: 9, minimum: 4, maximum: 99)),
                    memoryID: points(series(seed: UInt64(plan.memory) &+ 3, count: 26, start: plan.memory, spread: 5, minimum: 4, maximum: 99)),
                ]
            )
        }
    }

    private static func indicator(
        id: String,
        target: String,
        key: String,
        label: String,
        value: Double,
        pinned: Bool,
        position: Int?
    ) -> ContextIndicator {
        ContextIndicator(
            id: id,
            connectorID: "connector-prometheus",
            bindingID: "binding-\(target)",
            targetID: target,
            semanticKey: key,
            label: label,
            externalID: "item-\(key)",
            dimension: nil,
            unit: .percent,
            enabled: true,
            lastValue: value,
            lastObservedAt: iso(-15),
            lastError: nil,
            pinned: pinned,
            pinPosition: position
        )
    }

    private static func points(_ values: [Double]) -> [IndicatorPoint] {
        values.enumerated().map { index, value in
            IndicatorPoint(
                at: iso(Double(index - values.count + 1) * 3_600),
                value: value,
                minimum: nil,
                maximum: nil,
                samples: 6
            )
        }
    }

    /// Marche aleatoire deterministe : la capture d'un ecran doit etre
    /// reproductible d'un lancement a l'autre.
    private static func series(
        seed: UInt64,
        count: Int,
        start: Double,
        spread: Double,
        minimum: Double,
        maximum: Double
    ) -> [Double] {
        var state = seed == 0 ? 1 : seed
        var value = start
        var output: [Double] = []

        for _ in 0..<count {
            state = state &* 6_364_136_223_846_793_005 &+ 1_442_695_040_888_963_407
            let unit = Double(state >> 33) / Double(UInt32.max)
            value += (unit - 0.5) * spread
            value = Swift.min(Swift.max(value, minimum), maximum)
            output.append(value)
        }

        return output
    }

    private static func decode<T: Decodable>(_ type: T.Type, from json: String) -> T? {
        guard let data = json.data(using: .utf8) else {
            return nil
        }
        return try? JSONDecoder().decode(type, from: data)
    }

    private static func iso(_ offsetSeconds: Double) -> String {
        Date.now.addingTimeInterval(offsetSeconds).ISO8601Format()
    }

    // MARK: - Fixtures

    private static var targetsJSON: String {
        let entries: [(String, String, Int, Int)] = [
            ("api-gw-02", "Passerelle API — découvert par Prometheus", 2, 1),
            ("db-primary-01", "Base primaire PostgreSQL", 2, 0),
            ("log-store-04", "Collecte des journaux", 1, 0),
            ("edge-lb-01", "Répartiteur de charge edge", 2, 0),
            ("worker-07", "Traitement asynchrone — découvert par Prometheus", 1, 1),
            ("api-gw-01", "Passerelle API", 1, 0),
            ("db-replica-02", "Réplique de lecture", 1, 0),
            ("cache-redis-01", "Cache mémoire", 1, 0),
            ("edge-lb-02", "Répartiteur de charge edge", 1, 0),
            ("cdn-origin-03", "Origine CDN", 1, 0),
            ("ci-runner-11", "Exécuteur d’intégration continue", 1, 0),
        ]

        let body = entries.map { entry in
            let sources = (0..<entry.2).map { index in
                """
                {"id":"\(entry.0)-src-\(index)","target_id":"\(entry.0)","name":"Sonde HTTP \(index + 1)","kind":"http","enabled":true,"interval_seconds":30,"timeout_milliseconds":5000,"failure_threshold":3,"recovery_threshold":2,"severity":"critical","last_signal_at":"\(iso(-30))","last_observed_at":"\(iso(-30))","latest_outcome":"healthy"}
                """
            }.joined(separator: ",")

            return """
            {"id":"\(entry.0)","name":"\(entry.0)","description":"\(entry.1)","created_at":"\(iso(-864_000))","external_source_count":\(entry.3),"sources":[\(sources)]}
            """
        }.joined(separator: ",")

        return "[\(body)]"
    }

    private static var incidentsJSON: String {
        let entries: [(id: String, target: String, nature: String, label: String, severity: String, age: Double, acknowledged: Bool)] = [
            ("inc-4821", "api-gw-02", "service-unreachable", "Service injoignable — 3 sondes consécutives", "critical", -480, false),
            ("inc-4822", "db-primary-01", "latency-threshold", "Latence p95 au-dessus du seuil", "critical", -1_320, false),
            ("inc-4823", "log-store-04", "disk-usage", "Disque à 91 %", "warning", -4_440, false),
            ("inc-4824", "edge-lb-01", "certificate-expiry", "Certificat expire dans 6 jours", "warning", -10_800, false),
            ("inc-4825", "worker-07", "queue-backlog", "File de traitement en retard", "warning", -14_700, true),
            ("inc-4826", "db-replica-02", "backup-failed", "Sauvegarde nocturne échouée", "major", -32_400, true),
        ]

        let body = entries.map { entry in
            let acknowledged = entry.acknowledged
                ? "\"\(iso(entry.age + 600))\""
                : "null"
            let acknowledgedBy = entry.acknowledged ? "\"M. Lefèvre\"" : "null"

            return """
            {"id":"\(entry.id)","target_id":"\(entry.target)","target_name":"\(entry.target)","nature_key":"\(entry.nature)","nature_label":"\(entry.label)","status":"active","source_severity":"\(entry.severity)","effective_severity":"\(entry.severity)","opened_at":"\(iso(entry.age))","resolved_at":null,"acknowledged_at":\(acknowledged),"acknowledged_by":\(acknowledgedBy),"acknowledgement_origin":null,"acknowledgement_sync_status":"none","acknowledgement_sync_error":null,"maintenance_active":false,"maintenance_ends_at":null,"signals":[{"id":"\(entry.id)-s1","origin":"native","connector_id":null,"connector_name":"Sonde HTTP","external_event_id":null,"external_object_id":null,"name":"HTTP 502 — 3 essais","active":true,"severity":"\(entry.severity)","opened_at":"\(iso(entry.age))","resolved_at":null,"upstream_acknowledged":false,"invalidated_at":null,"invalidated_by":null,"invalidation_reason":null,"rearmed_at":null},{"id":"\(entry.id)-s2","origin":"integration","connector_id":"c1","connector_name":"Prometheus","external_event_id":null,"external_object_id":null,"name":"Pic CPU 96 %","active":false,"severity":"warning","opened_at":"\(iso(entry.age + 60))","resolved_at":null,"upstream_acknowledged":false,"invalidated_at":"\(iso(entry.age + 300))","invalidated_by":"M. Lefèvre","invalidation_reason":"pic de déploiement attendu","rearmed_at":null}],"activity":[{"id":4,"kind":"invalidated","origin":"human","actor_name":"M. Lefèvre","message":"Preuve « Pic CPU 96 % » invalidée : pic de déploiement attendu","data":{},"occurred_at":"\(iso(entry.age + 300))"},{"id":3,"kind":"signal_added","origin":"connector","actor_name":null,"message":"Nouvelle preuve rattachée : Prometheus · probe_success","data":{},"occurred_at":"\(iso(entry.age + 120))"},{"id":2,"kind":"signal_added","origin":"cairnops","actor_name":null,"message":"Sonde HTTP eu-west-1b en échec","data":{},"occurred_at":"\(iso(entry.age + 60))"},{"id":1,"kind":"opened","origin":"cairnops","actor_name":null,"message":"Incident ouvert après 3 observations non concluantes","data":{},"occurred_at":"\(iso(entry.age))"}],"created_at":"\(iso(entry.age))","updated_at":"\(iso(-60))"}
            """
        }.joined(separator: ",")

        return "[\(body)]"
    }

    private static var healthJSON: String {
        let hours = (0..<24).map { index in
            """
            {"hour":"\(iso(Double(index - 23) * 3_600))","expected_observations":60,"conclusive_observations":\(54 + index % 6),"healthy_observations":\(50 + index % 8),"average_latency_milliseconds":\(90 + index * 3)}
            """
        }.joined(separator: ",")

        return """
        {"status":"operational","checked_at":"\(iso(-45))","components":[{"name":"server","status":"operational","instances":2,"last_seen_at":"\(iso(-20))"},{"name":"worker","status":"operational","instances":1,"last_seen_at":"\(iso(-25))"},{"name":"postgresql","status":"stale","instances":1,"last_seen_at":"\(iso(-300))"}],"database":{"latency_milliseconds":3.4,"maximum_latency_milliseconds":18.7,"samples":[2.9,3.1,3.4,4.0,3.6],"measured_since":"\(iso(-3_600))"},"hours":[\(hours)]}
        """
    }
}
#endif
