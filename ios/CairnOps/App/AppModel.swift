import Foundation
import Observation
import UIKit

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

    enum PairingState: Equatable {
        case idle
        case claiming(instance: String)
        case awaitingConfirmation(instance: String)
        case finalizing(instance: String)
        case failed(message: String)
    }

    private enum PairingFlowError: LocalizedError {
        case incompleteCredential

        var errorDescription: String? {
            "L’instance a confirmé l’appareil sans remettre une identité complète. Relancez un nouvel appairage."
        }
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

    var user: User?
    var instanceName = ""
    var serverVersion = ""
    var snapshot = AppSnapshot()

    var isBootstrapping = true
    var isRefreshing = false
    var realtimeState: RealtimeState = .offline

    var statusMessage: String?
    var bannerTone: BannerTone = .neutral
    var loginError: String?
    var pairingState: PairingState = .idle
    var pairingTaskIdentity: UUID?
    var canRetryPairing = false
    var hasDeviceIdentity = false

    @ObservationIgnored private let configurationStore: ServerConfigurationStore
    @ObservationIgnored private let credentialStore: DeviceCredentialStore
    @ObservationIgnored private let snapshotStore: SnapshotStore
    @ObservationIgnored private let apiFactory: (ServerConfiguration, String?) -> CairnOpsAPI
    @ObservationIgnored private let pairingPollInterval: Duration
    @ObservationIgnored private var api: CairnOpsAPI?
    @ObservationIgnored private var pendingPairing: PendingDevicePairing?
    @ObservationIgnored private var recoveredIdentity: DeviceIdentity?
    @ObservationIgnored private var activePairingAttemptID: UUID?
    @ObservationIgnored private var deferredPairingLink: String?

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

    init(
        configurationStore: ServerConfigurationStore = ServerConfigurationStore(),
        credentialStore: DeviceCredentialStore = DeviceCredentialStore(),
        snapshotStore: SnapshotStore = SnapshotStore(),
        pairingPollInterval: Duration = .seconds(2),
        apiFactory: @escaping (ServerConfiguration, String?) -> CairnOpsAPI = {
            CairnOpsAPI(configuration: $0, deviceToken: $1)
        }
    ) {
        self.configurationStore = configurationStore
        self.credentialStore = credentialStore
        self.snapshotStore = snapshotStore
        self.pairingPollInterval = pairingPollInterval
        self.apiFactory = apiFactory
    }

    var instanceLabel: String {
        instanceName.isEmpty ? "CairnOps" : instanceName
    }

    var isPairingInFlight: Bool {
        switch pairingState {
        case .claiming, .awaitingConfirmation, .finalizing:
            true
        case .idle, .failed:
            false
        }
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
        defer {
            isBootstrapping = false
            if let link = deferredPairingLink {
                deferredPairingLink = nil
                acceptPairingLink(link)
            }
        }

        let storedConfiguration = configurationStore.load()
        serverURLText = storedConfiguration.baseURLString

        let credentialState: DeviceCredentialState
        do {
            credentialState = try credentialStore.load()
            hasDeviceIdentity = credentialState.identity != nil
        } catch {
            try? credentialStore.clear()
            hasDeviceIdentity = false
            snapshot = AppSnapshot()
            await snapshotStore.clear()
            loginError = userFacingMessage(from: error)
            pairingState = .failed(message: loginError ?? "L’identité locale est invalide.")
            canRetryPairing = false
            return
        }

        let credentialBaseURL = credentialState.identity?.serverBaseURL
            ?? credentialState.pendingPairing?.serverBaseURL
            ?? storedConfiguration.baseURLString

        if let storedSnapshot = await snapshotStore.load(),
           storedSnapshot.serverBaseURL == credentialBaseURL {
            snapshot = storedSnapshot
            instanceName = instanceName.isEmpty ? "CairnOps" : instanceName
        }

        if let pending = credentialState.pendingPairing {
            pendingPairing = pending
            hasDeviceIdentity = false
            serverURLText = pending.serverBaseURL
            configurationStore.save(ServerConfiguration(baseURLString: pending.serverBaseURL))
            pairingState = .claiming(instance: pairingInstanceLabel(pending.serverBaseURL))
            canRetryPairing = true
            showPairingBanner("Association de cet iPhone en cours. Confirmez-la depuis votre session Web.")
            pairingTaskIdentity = UUID()
            return
        }

        guard let identity = credentialState.identity else {
            // Un lien profond peut arriver pendant la lecture asynchrone du
            // cache. On relit alors Keychain avant de conclure qu’il n’y a rien
            // à reprendre.
            if let pending = try? credentialStore.load().pendingPairing {
                pendingPairing = pending
                hasDeviceIdentity = false
                serverURLText = pending.serverBaseURL
                configurationStore.save(ServerConfiguration(baseURLString: pending.serverBaseURL))
                pairingState = .claiming(instance: pairingInstanceLabel(pending.serverBaseURL))
                canRetryPairing = true
                showPairingBanner("Association de cet iPhone en cours. Confirmez-la depuis votre session Web.")
                pairingTaskIdentity = UUID()
                return
            }

            // Les anciennes versions ouvraient une session par mot de passe et
            // cookie. Une app sans identité d’appareil doit désormais repasser
            // par la confirmation Web, sans réutiliser silencieusement ce cookie.
            if let configuration = try? storedConfiguration.validated() {
                CairnOpsAPI(configuration: configuration).clearCookies()
            }
            if snapshot.hasProjection {
                statusMessage = "Dernier état connu affiché. Associez cet iPhone pour reprendre les actions."
                bannerTone = .caution
            }
            return
        }

        do {
            try await activateDeviceIdentity(identity)
        } catch is CancellationError {
            realtimeState = .offline
        } catch let error as CairnOpsAPIError where error.statusCode == 401 {
            await discardSessionData(keepConfiguration: true)
            loginError = "Cette identité d’appareil a été révoquée. Associez de nouveau cet iPhone."
            pairingState = .failed(message: loginError ?? "Identité révoquée.")
            canRetryPairing = false
        } catch {
            realtimeState = .offline
            if snapshot.hasProjection {
                statusMessage = "Instance momentanément indisponible. Dernier état connu affiché."
                bannerTone = .caution
                pairingState = .failed(
                    message: "Connexion à l’instance indisponible. L’identité de cet iPhone reste enregistrée."
                )
                canRetryPairing = true
            } else {
                loginError = userFacingMessage(from: error)
                pairingState = .failed(message: loginError ?? "Connexion impossible.")
                canRetryPairing = true
                bannerTone = .danger
            }
        }

    }

    func acceptPairingURL(_ url: URL) {
        acceptPairingLink(url.absoluteString)
    }

    func acceptPairingLink(_ rawValue: String) {
        guard !isBootstrapping else {
            deferredPairingLink = rawValue
            return
        }
        guard user == nil else {
            statusMessage = "Déconnectez cet appareil avant d’en associer un autre."
            bannerTone = .caution
            return
        }
        // Un second lien ne doit pas annuler une lecture de `/result` en vol :
        // le jeton d’appareil est retiré du serveur dès cette réponse. L’écran
        // courant permet d’annuler explicitement avant de scanner à nouveau.
        guard !isPairingInFlight, !canRetryPairing else {
            return
        }

        do {
            let link = try DevicePairingLink(string: rawValue)
            let pending = PendingDevicePairing.make(
                from: link,
                deviceName: UIDevice.current.name,
                appVersion: Self.appVersionLabel,
                locale: Locale.current.language.languageCode?.identifier ?? "fr"
            )
            try credentialStore.save(pendingPairing: pending)

            let configuration = ServerConfiguration(baseURLString: pending.serverBaseURL)
            configurationStore.save(configuration)
            pendingPairing = pending
            recoveredIdentity = nil
            hasDeviceIdentity = false
            serverURLText = pending.serverBaseURL
            loginError = nil
            pairingState = .claiming(instance: pairingInstanceLabel(pending.serverBaseURL))
            canRetryPairing = true
            showPairingBanner("Association de cet iPhone en cours. Confirmez-la depuis votre session Web.")
            pairingTaskIdentity = UUID()
        } catch {
            loginError = userFacingMessage(from: error)
            pairingState = .failed(message: loginError ?? "Invitation invalide.")
            canRetryPairing = false
            bannerTone = .danger
        }
    }

    func retryPairing() {
        do {
            let credentials = try credentialStore.load()
            guard credentials.pendingPairing != nil
                    || credentials.identity != nil
                    || recoveredIdentity != nil else {
                pairingState = .idle
                return
            }
            if let pending = credentials.pendingPairing {
                pendingPairing = pending
                pairingState = .claiming(instance: pairingInstanceLabel(pending.serverBaseURL))
            } else {
                let baseURL = credentials.identity?.serverBaseURL
                    ?? recoveredIdentity?.serverBaseURL
                    ?? serverURLText
                pairingState = .finalizing(instance: pairingInstanceLabel(baseURL))
            }
            loginError = nil
            canRetryPairing = true
            showPairingBanner("Nouvelle tentative d’association en cours.")
            pairingTaskIdentity = UUID()
        } catch {
            let message = userFacingMessage(from: error)
            pairingState = .failed(message: message)
        }
    }

    func cancelPairing() {
        try? credentialStore.clear()
        pendingPairing = nil
        recoveredIdentity = nil
        hasDeviceIdentity = false
        pairingTaskIdentity = nil
        pairingState = .idle
        canRetryPairing = false
        loginError = nil
        api = nil
        if snapshot.hasProjection {
            statusMessage = "Dernier état connu affiché. Associez cet iPhone pour reprendre les actions."
            bannerTone = .caution
        }
    }

    func runPairingAttempt() async {
        guard let attemptID = pairingTaskIdentity,
              user == nil,
              !isBootstrapping,
              isSceneActive else {
            return
        }

        activePairingAttemptID = attemptID
        defer {
            if activePairingAttemptID == attemptID {
                activePairingAttemptID = nil
            }
        }

        do {
            if let recoveredIdentity {
                try credentialStore.save(identity: recoveredIdentity)
                hasDeviceIdentity = true
                self.recoveredIdentity = nil
                try await activateDeviceIdentity(
                    recoveredIdentity,
                    pairingAttemptID: attemptID
                )
                pairingState = .idle
                canRetryPairing = false
                return
            }

            let storedCredentials = try credentialStore.load()
            if let identity = storedCredentials.identity {
                hasDeviceIdentity = true
                try await activateDeviceIdentity(
                    identity,
                    pairingAttemptID: attemptID
                )
                pairingState = .idle
                canRetryPairing = false
                return
            }

            guard let pending = storedCredentials.pendingPairing ?? pendingPairing else {
                pairingState = .idle
                return
            }
            pendingPairing = pending

            let configuration = try ServerConfiguration(
                baseURLString: pending.serverBaseURL
            ).validated()
            prepareAPI(with: configuration, deviceToken: nil)

            let setupStatus = try await currentAPI().getSetupStatus()
            try ensureCurrentPairingAttempt(attemptID)
            guard setupStatus.initialized else {
                throw ServerConfiguration.ConfigurationError.notInitialized
            }
            instanceName = setupStatus.name

            let instance = pairingInstanceLabel(pending.serverBaseURL)
            pairingState = .claiming(instance: instance)
            try await waitUntilPairingCanContactServer(attemptID)
            let claim = CairnOpsAPI.DevicePairingClaim(
                name: pending.deviceName,
                platform: "ios",
                appVersion: pending.appVersion,
                locale: pending.locale,
                notificationContent: pending.notificationContent,
                encryptionPublicKey: try pending.encryptionPublicKey(),
                pushRecipient: pending.pushRecipient
            )

            do {
                _ = try await currentAPI().claimDevicePairing(
                    pairingToken: pending.pairingToken,
                    claim: claim
                )
            } catch let error as CairnOpsAPIError where error.statusCode == 409 {
                // Une réponse perdue après le POST laisse le serveur déjà
                // revendiqué. Le même secret peut alors reprendre sur /result.
            }
            try ensureCurrentPairingAttempt(attemptID)

            pairingState = .awaitingConfirmation(instance: instance)
            showPairingBanner("Confirmation de cet iPhone attendue depuis votre session Web.")
            while !Task.isCancelled {
                try await waitUntilPairingCanContactServer(attemptID)
                let result = try await currentAPI().getDevicePairingResult(
                    pairingToken: pending.pairingToken
                )
                try ensureCurrentPairingAttempt(attemptID)
                if result.status == .confirmed {
                    guard let deviceID = result.deviceID,
                          !deviceID.isEmpty,
                          let deviceToken = result.deviceToken,
                          !deviceToken.isEmpty else {
                        throw PairingFlowError.incompleteCredential
                    }

                    let identity = DeviceIdentity(
                        serverBaseURL: pending.serverBaseURL,
                        deviceID: deviceID,
                        deviceToken: deviceToken,
                        encryptionPrivateKey: pending.encryptionPrivateKey,
                        pushRecipient: pending.pushRecipient
                    )
                    recoveredIdentity = identity
                    hasDeviceIdentity = true
                    try credentialStore.save(identity: identity)
                    recoveredIdentity = nil
                    pendingPairing = nil
                    pairingState = .finalizing(instance: instance)
                    showPairingBanner("Identité enregistrée. Synchronisation de la projection en cours.")
                    try await activateDeviceIdentity(
                        identity,
                        pairingAttemptID: attemptID
                    )
                    pairingState = .idle
                    canRetryPairing = false
                    return
                }

                try await Task.sleep(for: pairingPollInterval)
            }
        } catch is CancellationError {
            return
        } catch {
            guard pairingTaskIdentity == attemptID, !Task.isCancelled else {
                return
            }
            await handlePairingFailure(error)
        }
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
            } else if isPairingInFlight, activePairingAttemptID == nil {
                pairingTaskIdentity = UUID()
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
            await invalidateSession(
                message: "L’identité de cet appareil a été révoquée. Associez de nouveau cet iPhone."
            )
        } catch {
            statusMessage = userFacingMessage(from: error)
            bannerTone = .danger
        }
    }

    func logout() async {
        if let recoveredIdentity,
           let configuration = try? ServerConfiguration(
               baseURLString: recoveredIdentity.serverBaseURL
           ).validated() {
            prepareAPI(with: configuration, deviceToken: recoveredIdentity.deviceToken)
        }
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
            await invalidateSession(
                message: "L’identité de cet appareil a été révoquée pendant la synchronisation."
            )
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
            await invalidateSession(
                message: "L’identité de cet appareil a été révoquée. Associez de nouveau cet iPhone."
            )
        } catch {
            statusMessage = snapshot.hasProjection
                ? "Lecture impossible pour le moment. Le dernier etat connu reste visible."
                : userFacingMessage(from: error)
            bannerTone = snapshot.hasProjection ? .caution : .danger
        }
    }

    private static var appVersionLabel: String {
        let version = Bundle.main.object(
            forInfoDictionaryKey: "CFBundleShortVersionString"
        ) as? String ?? "0.1.0"
        let build = Bundle.main.object(
            forInfoDictionaryKey: "CFBundleVersion"
        ) as? String ?? "1"
        return "\(version) (\(build))"
    }

    private func activateDeviceIdentity(
        _ identity: DeviceIdentity,
        pairingAttemptID: UUID? = nil
    ) async throws {
        let configuration = try ServerConfiguration(
            baseURLString: identity.serverBaseURL
        ).validated()
        prepareAPI(with: configuration, deviceToken: identity.deviceToken)

        let setupStatus = try await currentAPI().getSetupStatus()
        try ensureCurrentPairingAttempt(pairingAttemptID)
        guard setupStatus.initialized else {
            throw ServerConfiguration.ConfigurationError.notInitialized
        }

        let authenticatedUser = try await currentAPI().getCurrentSession()
        try ensureCurrentPairingAttempt(pairingAttemptID)
        let version = try? await currentAPI().getVersion()
        try ensureCurrentPairingAttempt(pairingAttemptID)

        instanceName = setupStatus.name
        user = authenticatedUser
        serverVersion = version ?? serverVersion
        configurationStore.save(configuration)
        loginError = nil
        statusMessage = nil
        bannerTone = .neutral
        await refreshProjection()
    }

    private func ensureCurrentPairingAttempt(_ expectedID: UUID?) throws {
        guard let expectedID else {
            return
        }
        guard !Task.isCancelled, pairingTaskIdentity == expectedID else {
            throw CancellationError()
        }
    }

    /// N’entame pas une nouvelle requête de résultat en arrière-plan, mais ne
    /// coupe jamais une requête déjà partie : une réponse confirmée contient un
    /// secret à usage unique qu’il faut enregistrer même si la scène se ferme.
    private func waitUntilPairingCanContactServer(_ attemptID: UUID) async throws {
        while !isSceneActive {
            try ensureCurrentPairingAttempt(attemptID)
            try await Task.sleep(for: .seconds(1))
        }
        try ensureCurrentPairingAttempt(attemptID)
    }

    private func handlePairingFailure(_ error: Error) async {
        let terminalFailure: Bool
        if let apiError = error as? CairnOpsAPIError {
            terminalFailure = [400, 401, 404, 409, 410].contains(apiError.statusCode)
        } else {
            terminalFailure = error is PairingFlowError
                || error is ServerConfiguration.ConfigurationError
        }

        if terminalFailure {
            try? credentialStore.clear()
            pendingPairing = nil
            recoveredIdentity = nil
            hasDeviceIdentity = false
        }
        canRetryPairing = !terminalFailure

        let message: String
        if let apiError = error as? CairnOpsAPIError {
            message = switch apiError.statusCode {
            case 400:
                "L’instance a refusé l’identité de cet iPhone. Relancez un nouvel appairage."
            case 401, 404:
                "Cette invitation d’appairage n’est plus valide. Générez un nouveau QR code sur le Web."
            case 409:
                "Cet appairage a été annulé. Générez un nouveau QR code sur le Web."
            case 410:
                "Cette invitation a expiré. Générez un nouveau QR code sur le Web."
            default:
                userFacingMessage(from: apiError)
            }
        } else {
            message = userFacingMessage(from: error)
        }

        loginError = message
        pairingState = .failed(message: message)
        bannerTone = .danger
        statusMessage = snapshot.hasProjection ? message : nil
        realtimeState = .offline
    }

    private func showPairingBanner(_ message: String) {
        guard snapshot.hasProjection else {
            statusMessage = nil
            return
        }
        statusMessage = message
        bannerTone = .caution
    }

    private func pairingInstanceLabel(_ baseURL: String) -> String {
        guard let url = URL(string: baseURL), let host = url.host() else {
            return baseURL
        }
        let path = url.path == "/" ? "" : url.path
        return host + path
    }

    private func prepareAPI(
        with configuration: ServerConfiguration,
        deviceToken: String?
    ) {
        serverURLText = configuration.baseURLString
        api = apiFactory(configuration, deviceToken)
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
        try? credentialStore.clear()
        pendingPairing = nil
        recoveredIdentity = nil
        hasDeviceIdentity = false
        pairingTaskIdentity = nil
        pairingState = .idle
        canRetryPairing = false
        user = nil
        realtimeState = .offline
        snapshot = AppSnapshot()
        await snapshotStore.clear()

        if keepConfiguration {
            let stored = configurationStore.load()
            serverURLText = stored.baseURLString
            api = nil
        } else {
            configurationStore.clear()
            serverURLText = ""
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
    func debugInstallAPI(_ api: CairnOpsAPI, serverURL: String = "https://example.test") {
        serverURLText = serverURL
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
