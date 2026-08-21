import Foundation
import Observation

@MainActor
@Observable
final class AppModel {
    enum RealtimeState {
        case offline
        case connecting
        case online
    }

    enum BannerTone {
        case neutral
        case caution
        case danger
    }

    /// Portions de la projection qu'un evenement temps reel invalide.
    ///
    /// Un evenement ne rend pas tout obsolete : un battement de coeur ne touche
    /// que la sante, une notification que la boite de reception. On ne recharge
    /// donc que ce qui a change au lieu des quatre appels systematiques.
    private struct SyncScope: OptionSet {
        let rawValue: Int

        static let health = SyncScope(rawValue: 1 << 0)
        static let inbox = SyncScope(rawValue: 1 << 1)
        static let projection = SyncScope(rawValue: 1 << 2)
    }

    /// Fenetre de regroupement des evenements temps reel. Une rafale de
    /// notifications rapprochees ne declenche qu'une seule synchronisation.
    private static let realtimeCoalescingWindow = Duration.milliseconds(400)

    private static let minimumReconnectDelay = Duration.seconds(2)
    private static let maximumReconnectDelay = Duration.seconds(60)

    var serverURLText = ""
    var usernameText = ""
    var passwordText = ""

    var user: User?
    var instanceName = ""
    var serverVersion = ""
    var snapshot = AppSnapshot()

    var isBootstrapping = true
    var isRefreshing = false
    var isAuthenticating = false
    var realtimeState: RealtimeState = .offline

    var statusMessage: String?
    var bannerTone: BannerTone = .neutral
    var loginError: String?

    @ObservationIgnored private let configurationStore = ServerConfigurationStore()
    @ObservationIgnored private let snapshotStore = SnapshotStore()
    @ObservationIgnored private var api: CairnOpsAPI?

    /// Passe a `false` des que la scene quitte le premier plan. La boucle temps
    /// reel s'arrete alors au lieu de maintenir une socket et des requetes
    /// pendant que l'app est en arriere-plan.
    @ObservationIgnored private var isSceneActive = true

    @ObservationIgnored private var pendingScopes: SyncScope = []
    @ObservationIgnored private var coalescingTask: Task<Void, Never>?
    @ObservationIgnored private var reconnectAttempt = 0
    @ObservationIgnored private var debugHooks = DebugHooks()

    private struct DebugHooks {
        var fetchProjection: (() async throws -> CairnOpsAPI.OperationalProjection)?
        var fetchInbox: (() async throws -> CairnOpsAPI.InboxPayload)?
        var fetchHealth: (() async throws -> SystemHealth?)?
        var fetchVersion: (() async throws -> String)?
    }

    var instanceLabel: String {
        instanceName.isEmpty ? "CairnOps" : instanceName
    }

    var showsShell: Bool {
        user != nil || snapshot.hasProjection
    }

    var isOfflineSnapshot: Bool {
        user == nil && snapshot.hasProjection
    }

    var canMutate: Bool {
        guard let user else {
            return false
        }
        return user.role.canAcknowledge && !isOfflineSnapshot
    }

    /// Identifie la session temps reel courante. Le `scenePhase` en fait partie
    /// pour que SwiftUI annule la tache lorsque l'app quitte le premier plan.
    var realtimeIdentity: String? {
        guard let user, isSceneActive else {
            return nil
        }
        return "\(serverURLText)#\(user.id)"
    }

    func bootstrap() async {
        guard isBootstrapping else {
            return
        }

        let storedConfiguration = configurationStore.load()
        serverURLText = storedConfiguration.baseURLString
        usernameText = storedConfiguration.username

        if let storedSnapshot = await snapshotStore.load(),
           storedSnapshot.serverBaseURL == storedConfiguration.baseURLString {
            snapshot = storedSnapshot
            instanceName = instanceName.isEmpty ? "CairnOps" : instanceName
        }

        guard !storedConfiguration.baseURLString.isEmpty else {
            isBootstrapping = false
            return
        }

        do {
            let configuration = try storedConfiguration.validated()
            prepareAPI(with: configuration)

            let setupStatus = try await currentAPI().getSetupStatus()
            instanceName = setupStatus.name
            serverVersion = (try? await currentAPI().getVersion()) ?? serverVersion

            let currentUser = try await currentAPI().getCurrentSession()
            user = currentUser
            bannerTone = .neutral
            statusMessage = nil
            await refreshProjection()
        } catch is CancellationError {
            realtimeState = .offline
        } catch let error as CairnOpsAPIError where error.statusCode == 401 {
            user = nil
            realtimeState = .offline
            if snapshot.hasProjection {
                statusMessage = "Dernier etat connu affiche. Reconnectez-vous pour reprendre les actions."
                bannerTone = .caution
            }
        } catch {
            realtimeState = .offline
            if snapshot.hasProjection {
                statusMessage = "Instance momentanement indisponible. Dernier etat connu affiche."
                bannerTone = .caution
            } else {
                loginError = userFacingMessage(from: error)
                bannerTone = .danger
            }
        }

        isBootstrapping = false
    }

    func login() async {
        guard !isAuthenticating else {
            return
        }

        isAuthenticating = true
        loginError = nil

        do {
            let configuration = try ServerConfiguration(
                baseURLString: serverURLText,
                username: usernameText
            ).validated()
            prepareAPI(with: configuration)

            let setupStatus = try await currentAPI().getSetupStatus()
            guard setupStatus.initialized else {
                throw ServerConfiguration.ConfigurationError.notInitialized
            }

            instanceName = setupStatus.name
            _ = try await currentAPI().login(
                username: configuration.username,
                password: passwordText
            )
            user = try await currentAPI().getCurrentSession()
            passwordText = ""
            serverVersion = (try? await currentAPI().getVersion()) ?? serverVersion
            configurationStore.save(configuration)
            statusMessage = nil
            bannerTone = .neutral
            await refreshProjection()
        } catch is CancellationError {
            loginError = nil
        } catch {
            loginError = userFacingMessage(from: error)
            bannerTone = .danger
        }

        isAuthenticating = false
    }

    func refresh() async {
        if user == nil {
            isBootstrapping = true
            await bootstrap()
            return
        }

        await refreshProjection()
    }

    /// Repercute le cycle de vie de la scene sur le flux temps reel.
    func setScenePhaseActive(_ isActive: Bool) {
        guard isSceneActive != isActive else {
            return
        }

        isSceneActive = isActive

        if isActive {
            // La projection a pu diverger pendant la mise en veille : on la
            // resynchronise au retour, la socket sera relancee par la `task`.
            reconnectAttempt = 0
            if user != nil {
                Task { await self.refreshProjection() }
            }
        } else {
            cancelCoalescing()
            realtimeState = .offline
            // L'ecriture du cache est differee : on la force avant la mise en
            // veille pour ne pas perdre le dernier etat connu.
            let store = snapshotStore
            Task { await store.flushNow() }
        }
    }

    func acknowledge(incidentID: Incident.ID) async {
        guard canMutate else {
            return
        }

        do {
            let updatedIncident = try await currentAPI().acknowledgeIncident(id: incidentID)
            upsert(incident: updatedIncident)
            snapshot.lastRefreshAt = Date.now.ISO8601Format()
            await snapshotStore.save(snapshot)
            statusMessage = "Incident acquitté."
            bannerTone = .neutral
        } catch is CancellationError {
            return
        } catch let error as CairnOpsAPIError where error.statusCode == 401 {
            await invalidateSession(message: "La session a expire. Reconnectez-vous pour continuer.")
        } catch {
            statusMessage = userFacingMessage(from: error)
            bannerTone = .danger
        }
    }

    func logout() async {
        do {
            try await currentAPI().logout()
        } catch is CancellationError {
            return
        } catch {
            // Une coupure de reseau ne doit pas empecher le nettoyage local.
        }

        await discardSessionData(keepConfiguration: true)
        statusMessage = nil
        loginError = nil
    }

    func clearOfflineSnapshot() async {
        snapshot = AppSnapshot()
        await snapshotStore.clear()
        statusMessage = nil
    }

    func runRealtimeLoop() async {
        guard user != nil, isSceneActive else {
            realtimeState = .offline
            return
        }

        defer {
            cancelCoalescing()
            realtimeState = .offline
        }

        realtimeState = .connecting

        while !Task.isCancelled, user != nil, isSceneActive {
            do {
                let socket = try currentAPI().makeRealtimeTask(after: snapshot.realtimeVersion)
                socket.resume()
                defer { socket.cancel(with: .goingAway, reason: nil) }

                while !Task.isCancelled, user != nil, isSceneActive {
                    let message = try await currentAPI().receiveRealtimeMessage(from: socket)

                    // Une trame recue prouve que le lien est sain : on remet le
                    // compteur de reconnexion a zero.
                    reconnectAttempt = 0
                    realtimeState = .online
                    snapshot.realtimeVersion = message.version

                    // On enregistre la portee a rafraichir et on repart lire la
                    // trame suivante. L'ancienne version attendait ici quatre
                    // requetes HTTP, ce qui laissait la socket s'accumuler.
                    scheduleSync(for: message)
                }
            } catch is CancellationError {
                return
            } catch let error as CairnOpsAPIError where error.statusCode == 401 {
                await invalidateSession(message: "La session mobile a ete revoquee.")
                return
            } catch {
                realtimeState = .offline

                guard !Task.isCancelled, user != nil, isSceneActive else {
                    return
                }

                statusMessage = "Flux temps reel interrompu. Nouvelle tentative en cours."
                bannerTone = .caution

                // Backoff exponentiel : une instance injoignable etait
                // auparavant sollicitee toutes les trois secondes sans fin.
                let delay = reconnectDelay()
                reconnectAttempt += 1

                do {
                    try await Task.sleep(for: delay)
                } catch {
                    return
                }
            }
        }
    }

    func incident(withID id: Incident.ID) -> Incident? {
        snapshot.incidents.first { $0.id == id }
    }

    func target(withID id: Target.ID) -> Target? {
        snapshot.targets.first { $0.id == id }
    }

    // MARK: - Synchronisation

    private func reconnectDelay() -> Duration {
        let exponent = min(reconnectAttempt, 5)
        let multiplier = 1 << exponent
        let seconds = Self.minimumReconnectDelay.components.seconds * Int64(multiplier)
        let capped = min(seconds, Self.maximumReconnectDelay.components.seconds)

        // Un peu de dispersion evite que plusieurs appareils se reconnectent
        // exactement au meme instant apres une coupure serveur.
        let jitter = Double.random(in: 0...0.3) * Double(capped)
        return .seconds(Double(capped) + jitter)
    }

    private func scheduleSync(for message: RealtimeMessage) {
        guard message.type == "event" else {
            return
        }

        let scope: SyncScope = switch message.kind {
        case "component.heartbeat":
            .health
        case "notification.changed":
            .inbox
        default:
            .projection
        }

        pendingScopes.insert(scope)

        guard coalescingTask == nil else {
            return
        }

        coalescingTask = Task { [weak self] in
            try? await Task.sleep(for: Self.realtimeCoalescingWindow)
            guard !Task.isCancelled else {
                return
            }
            await self?.flushPendingScopes()
        }
    }

    private func flushPendingScopes() async {
        coalescingTask = nil

        let scopes = pendingScopes
        pendingScopes = []

        guard !scopes.isEmpty, user != nil, isSceneActive else {
            return
        }

        do {
            // Une resynchronisation complete couvre deja la boite de reception :
            // inutile de la demander deux fois dans la meme fenetre.
            if scopes.contains(.projection) {
                let projection = try await loadOperationalProjection()
                applyProjection(projection)
            } else if scopes.contains(.inbox) {
                let inbox = try await loadInbox()
                snapshot.inbox = inbox.entries
                snapshot.unreadCount = inbox.unread
            }

            if scopes.contains(.health) {
                snapshot.systemHealth = try await loadSystemHealth()
            }

            snapshot.lastRefreshAt = Date.now.ISO8601Format()
            await snapshotStore.save(snapshot)

            if bannerTone != .danger {
                statusMessage = nil
            }
        } catch is CancellationError {
            return
        } catch let error as CairnOpsAPIError where error.statusCode == 401 {
            await invalidateSession(message: "La session a expire pendant la synchronisation.")
        } catch {
            statusMessage = "Synchronisation partielle ratee. Une nouvelle tentative suivra."
            bannerTone = .caution
        }
    }

    private func cancelCoalescing() {
        coalescingTask?.cancel()
        coalescingTask = nil
        pendingScopes = []
    }

    private func applyProjection(_ projection: CairnOpsAPI.OperationalProjection) {
        var measures: [String: TargetMeasures] = [:]
        measures.reserveCapacity(projection.measures.count)
        for entry in projection.measures {
            measures[entry.targetID] = entry
        }

        // Une seule mutation groupee : l'index derive n'est reconstruit qu'une
        // fois, et SwiftUI n'observe qu'une invalidation au lieu de cinq.
        snapshot.applyProjection(
            targets: projection.targets,
            incidents: projection.incidents,
            measures: measures,
            inbox: projection.inbox.entries,
            unreadCount: projection.inbox.unread
        )
    }

    private func refreshProjection() async {
        guard !isRefreshing else {
            return
        }

        isRefreshing = true
        defer { isRefreshing = false }

        do {
            // Les trois lectures sont independantes : on les mene de front au
            // lieu de les enchainer.
            async let projectionTask = loadOperationalProjection()
            async let healthTask = loadSystemHealth()
            async let versionTask = loadVersion()

            let projection = try await projectionTask
            let health = try await healthTask
            let version = try? await versionTask

            snapshot.serverBaseURL = serverURLText
            applyProjection(projection)
            snapshot.systemHealth = health
            snapshot.lastRefreshAt = Date.now.ISO8601Format()
            serverVersion = version ?? serverVersion

            await snapshotStore.save(snapshot)

            if isOfflineSnapshot {
                statusMessage = "Connexion retablie."
                bannerTone = .neutral
            } else {
                statusMessage = nil
                bannerTone = .neutral
            }
        } catch is CancellationError {
            return
        } catch let error as CairnOpsAPIError where error.statusCode == 401 {
            await invalidateSession(message: "La session a expire. Reconnectez-vous pour continuer.")
        } catch {
            statusMessage = snapshot.hasProjection
                ? "Lecture impossible pour le moment. Le dernier etat connu reste visible."
                : userFacingMessage(from: error)
            bannerTone = snapshot.hasProjection ? .caution : .danger
        }
    }

    private func prepareAPI(with configuration: ServerConfiguration) {
        serverURLText = configuration.baseURLString
        usernameText = configuration.username
        api = CairnOpsAPI(configuration: configuration)
    }

    private func currentAPI() throws -> CairnOpsAPI {
        guard let api else {
            throw ServerConfiguration.ConfigurationError.missingBaseURL
        }
        return api
    }

    private func loadOperationalProjection() async throws -> CairnOpsAPI.OperationalProjection {
        if let hook = debugHooks.fetchProjection {
            return try await hook()
        }

        return try await currentAPI().fetchOperationalProjection()
    }

    private func loadInbox() async throws -> CairnOpsAPI.InboxPayload {
        if let hook = debugHooks.fetchInbox {
            return try await hook()
        }

        return try await currentAPI().fetchInbox()
    }

    private func loadSystemHealth() async throws -> SystemHealth? {
        if let hook = debugHooks.fetchHealth {
            return try await hook()
        }

        return try await currentAPI().fetchSystemHealth()
    }

    private func loadVersion() async throws -> String {
        if let hook = debugHooks.fetchVersion {
            return try await hook()
        }

        return try await currentAPI().getVersion()
    }

    private func upsert(incident: Incident) {
        if let index = snapshot.incidents.firstIndex(where: { $0.id == incident.id }) {
            snapshot.incidents[index] = incident
        } else {
            snapshot.incidents.insert(incident, at: 0)
        }
        snapshot.rebuildDerived()
    }

    private func invalidateSession(message: String) async {
        await discardSessionData(keepConfiguration: true)
        loginError = message
        statusMessage = nil
        bannerTone = .danger
    }

    private func discardSessionData(keepConfiguration: Bool) async {
        cancelCoalescing()
        reconnectAttempt = 0
        api?.clearCookies()
        user = nil
        passwordText = ""
        realtimeState = .offline
        snapshot = AppSnapshot()
        await snapshotStore.clear()

        if keepConfiguration {
            let stored = configurationStore.load()
            serverURLText = stored.baseURLString
            usernameText = stored.username
            if let configuration = try? stored.validated() {
                prepareAPI(with: configuration)
            } else {
                api = nil
            }
        } else {
            configurationStore.clear()
            serverURLText = ""
            usernameText = ""
            api = nil
        }
    }

    private func userFacingMessage(from error: Error) -> String {
        if error is CancellationError {
            return ""
        }

        if let apiError = error as? CairnOpsAPIError {
            return apiError.message
        }

        if let urlError = error as? URLError {
            switch urlError.code {
            case .notConnectedToInternet, .networkConnectionLost:
                return "Connexion Internet indisponible."
            case .timedOut:
                return "La requete a expire. Réessayez."
            case .cannotFindHost, .cannotConnectToHost:
                return "L’instance CairnOps ne répond pas."
            case .secureConnectionFailed, .serverCertificateUntrusted:
                return "La connexion TLS à l’instance a échoué."
            case .cancelled:
                return ""
            default:
                break
            }
        }

        let localized = error.localizedDescription.trimmingCharacters(in: .whitespacesAndNewlines)
        if localized.folding(options: [.diacriticInsensitive, .caseInsensitive], locale: .current) == "cancelled" {
            return ""
        }

        return localized.isEmpty ? "Une erreur inattendue est survenue." : localized
    }
}

#if DEBUG
extension AppModel {
    @MainActor
    func debugInstallAPI(_ api: CairnOpsAPI, serverURL: String = "https://example.test", username: String = "ops") {
        serverURLText = serverURL
        usernameText = username
        self.api = api
    }

    @MainActor
    func debugInstallSyncHooks(
        projection: (() async throws -> CairnOpsAPI.OperationalProjection)? = nil,
        inbox: (() async throws -> CairnOpsAPI.InboxPayload)? = nil,
        health: (() async throws -> SystemHealth?)? = nil,
        version: (() async throws -> String)? = nil
    ) {
        debugHooks = DebugHooks(
            fetchProjection: projection,
            fetchInbox: inbox,
            fetchHealth: health,
            fetchVersion: version
        )
    }

    @MainActor
    func debugSetUser(_ user: User?) {
        self.user = user
    }

    @MainActor
    func debugQueueRealtimeEvent(kind: String?) {
        scheduleSync(
            for: RealtimeMessage(
                type: "event",
                version: snapshot.realtimeVersion ?? 0,
                kind: kind,
                entityType: nil,
                entityID: nil,
                occurredAt: nil
            )
        )
    }

    @MainActor
    func debugFlushPendingRealtimeScopes() async {
        await flushPendingScopes()
    }
}
#endif
