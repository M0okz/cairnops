import Foundation
import Observation
import UIKit

@MainActor
@Observable
final class AppModel {
    enum RealtimeState: Equatable {
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
    private struct SyncScope: OptionSet, Sendable {
        let rawValue: Int

        static let targets = SyncScope(rawValue: 1 << 0)
        static let incidents = SyncScope(rawValue: 1 << 1)
        static let measures = SyncScope(rawValue: 1 << 2)
        static let inbox = SyncScope(rawValue: 1 << 3)
        static let health = SyncScope(rawValue: 1 << 4)
        static let version = SyncScope(rawValue: 1 << 5)

        static let projection: SyncScope = [.targets, .incidents, .measures, .inbox]
        static let fullRefresh: SyncScope = [.projection, .health, .version]
    }

    private struct SyncBatch {
        let scopes: SyncScope
        let realtimeVersion: Int64?
        let completions: [CheckedContinuation<Bool, Never>]

        var hasWork: Bool {
            !scopes.isEmpty || realtimeVersion != nil
        }
    }

    private struct SyncPayload {
        let projection: CairnOpsAPI.OperationalProjection?
        let targets: [Target]?
        let incidents: [Incident]?
        let measures: [TargetMeasures]?
        let inbox: CairnOpsAPI.InboxPayload?
        let health: SystemHealth?
        let version: String?
    }

    /// Fenetre de regroupement des evenements temps reel. Une rafale de
    /// notifications rapprochees ne declenche qu'une seule synchronisation.
    private static let realtimeCoalescingWindow = Duration.milliseconds(400)

    private static let minimumReconnectDelay = Duration.seconds(2)
    private static let maximumReconnectDelay = Duration.seconds(60)
    private static let stableConnectionResetThreshold = Duration.seconds(30)
    private static let minimumSyncRetryDelay = Duration.seconds(1)
    private static let maximumSyncRetryDelay = Duration.seconds(30)
    private static let realtimeReconnectMessage = "Flux temps réel interrompu. Nouvelle tentative en cours."

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
	@ObservationIgnored private let pushRelayFactory: () throws -> PushRelayClient
	@ObservationIgnored private let pairingPollInterval: Duration
    @ObservationIgnored private var api: CairnOpsAPI?
    @ObservationIgnored private var pendingPairing: PendingDevicePairing?
    @ObservationIgnored private var recoveredIdentity: DeviceIdentity?
    @ObservationIgnored private var activePairingAttemptID: UUID?
	@ObservationIgnored private var deferredPairingLink: String?
	@ObservationIgnored private var pushRegistrationInFlight: String?

    /// Passe a `false` des que la scene quitte le premier plan. La boucle temps
    /// reel s'arrete alors au lieu de maintenir une socket et des requetes
    /// pendant que l'app est en arriere-plan.
    private var isSceneActive = true

    @ObservationIgnored private var realtimeCursor: Int64?
    @ObservationIgnored private var pendingScopes: SyncScope = []
    @ObservationIgnored private var pendingRealtimeVersion: Int64?
    @ObservationIgnored private var pendingSyncCompletions: [CheckedContinuation<Bool, Never>] = []
    @ObservationIgnored private var synchronizationTask: Task<Void, Never>?
    @ObservationIgnored private var synchronizationGeneration = 0
    @ObservationIgnored private var synchronizationIsWaiting = false
    @ObservationIgnored private var synchronizationIsFetching = false
    @ObservationIgnored private var syncRetryAttempt = 0
    @ObservationIgnored private var activeRealtimeTask: URLSessionWebSocketTask?
    @ObservationIgnored private var realtimeLoopGeneration = 0
    @ObservationIgnored private var reconnectAttempt = 0
    @ObservationIgnored private var debugHooks = DebugHooks()

    private struct DebugHooks {
        var fetchProjection: (() async throws -> CairnOpsAPI.OperationalProjection)?
        var fetchTargets: (() async throws -> [Target])?
        var fetchIncidents: (() async throws -> [Incident])?
        var fetchMeasures: (() async throws -> [TargetMeasures])?
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
		},
		pushRelayFactory: @escaping () throws -> PushRelayClient = {
			try PushRelayClient.configured()
		}
	) {
        self.configurationStore = configurationStore
        self.credentialStore = credentialStore
        self.snapshotStore = snapshotStore
		self.pairingPollInterval = pairingPollInterval
		self.apiFactory = apiFactory
		self.pushRelayFactory = pushRelayFactory
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
            realtimeCursor = nil
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
            realtimeCursor = storedSnapshot.realtimeVersion
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
				pushRecipient: nil
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
						pushRegistration: nil
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

	func registerForPush(deviceToken: String) async {
		let normalizedToken = deviceToken.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
		guard !normalizedToken.isEmpty,
			pushRegistrationInFlight != normalizedToken,
			let identity = try? credentialStore.load().identity else {
			return
		}

		pushRegistrationInFlight = normalizedToken
		defer {
			if pushRegistrationInFlight == normalizedToken {
				pushRegistrationInFlight = nil
			}
		}

		do {
			let relay = try pushRelayFactory()
			let registration: PushRelayRegistration
			if let current = identity.pushRegistration {
				do {
					try await relay.rotate(current, deviceToken: normalizedToken)
					registration = current
				} catch let error as PushRelayError
					where error.statusCode == 401 || error.statusCode == 404 || error.statusCode == 410 {
					registration = try await relay.register(deviceToken: normalizedToken)
				}
			} else {
				registration = try await relay.register(deviceToken: normalizedToken)
			}

			let updatedIdentity = DeviceIdentity(
				serverBaseURL: identity.serverBaseURL,
				deviceID: identity.deviceID,
				deviceToken: identity.deviceToken,
				encryptionPrivateKey: identity.encryptionPrivateKey,
				pushRegistration: registration
			)
			// La capacité de gestion est durable avant le PATCH : une coupure entre
			// les deux appels reprend ainsi la même inscription au prochain essai.
			try credentialStore.save(identity: updatedIdentity)
			let configuration = try ServerConfiguration(
				baseURLString: identity.serverBaseURL
			).validated()
			try await apiFactory(configuration, identity.deviceToken).updatePushRecipient(
				deviceID: identity.deviceID,
				recipient: registration.recipient
			)
		} catch is CancellationError {
			return
		} catch {
			statusMessage = "Les notifications Push ne sont pas encore actives : \(userFacingMessage(from: error))"
			bannerTone = .caution
		}
	}

    /// Repercute le cycle de vie de la scene sur le flux temps reel.
    func setScenePhaseActive(_ isActive: Bool) {
        guard isSceneActive != isActive else {
            return
        }

        isSceneActive = isActive

        if isActive {
            reconnectAttempt = 0
            if user != nil {
                // La projection a pu diverger pendant la mise en veille. Elle
                // rejoint la file serialisee ; la reprise WebSocket la complete
                // sans pouvoir lancer une seconde synchronisation en parallele.
                queueSynchronization(scopes: .fullRefresh, immediate: true)
            } else if isPairingInFlight, activePairingAttemptID == nil {
                pairingTaskIdentity = UUID()
            }
        } else {
            realtimeLoopGeneration &+= 1
            stopRealtimeTransport()
            cancelSynchronization()
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
            await saveSnapshot()
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
		let identity = (try? credentialStore.load().identity) ?? recoveredIdentity
		if let registration = identity?.pushRegistration,
			let relay = try? pushRelayFactory() {
			try? await relay.remove(registration)
		}
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
        realtimeCursor = nil
        snapshot = AppSnapshot()
        await snapshotStore.clear()
        statusMessage = nil
    }

    func runRealtimeLoop() async {
        guard user != nil, isSceneActive else {
            realtimeState = .offline
            return
        }

        realtimeLoopGeneration &+= 1
        let loopGeneration = realtimeLoopGeneration
        let clock = ContinuousClock()

        defer {
            if realtimeLoopGeneration == loopGeneration {
                stopRealtimeTransport()
                cancelSynchronization()
                realtimeState = .offline
            }
        }

        realtimeState = .connecting

        while !Task.isCancelled,
              user != nil,
              isSceneActive,
              realtimeLoopGeneration == loopGeneration {
            let connectionStartedAt = clock.now
            do {
                let socket = try currentAPI().makeRealtimeTask(after: realtimeCursor)
                activeRealtimeTask = socket
                socket.resume()
                defer {
                    socket.cancel(with: .goingAway, reason: nil)
                    if activeRealtimeTask === socket {
                        activeRealtimeTask = nil
                    }
                }

                while !Task.isCancelled,
                      user != nil,
                      isSceneActive,
                      realtimeLoopGeneration == loopGeneration {
                    let message = try await currentAPI().receiveRealtimeMessage(from: socket)
                    handleRealtimeMessage(message)
                }
            } catch is CancellationError {
                return
            } catch let error as CairnOpsAPIError where error.statusCode == 401 {
                await invalidateSession(message: "La session mobile a ete revoquee.")
                return
            } catch {
                realtimeState = .offline

                guard !Task.isCancelled,
                      user != nil,
                      isSceneActive,
                      realtimeLoopGeneration == loopGeneration else {
                    return
                }

                statusMessage = Self.realtimeReconnectMessage
                bannerTone = .caution

                // Une simple trame `ready` ne suffit pas a declarer une socket
                // stable : une connexion qui s'ouvre puis se ferme aussitot doit
                // continuer son backoff au lieu de repartir de deux secondes.
                if connectionStartedAt.duration(to: clock.now) >= Self.stableConnectionResetThreshold {
                    reconnectAttempt = 0
                }
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

    private func reconnectDelay(
        jitterFraction: Double = Double.random(in: 0...0.3)
    ) -> Duration {
        let exponent = min(reconnectAttempt, 5)
        let multiplier = 1 << exponent
        let seconds = Self.minimumReconnectDelay.components.seconds * Int64(multiplier)
        let capped = min(seconds, Self.maximumReconnectDelay.components.seconds)

        // Un peu de dispersion evite que plusieurs appareils se reconnectent
        // exactement au meme instant apres une coupure serveur.
        let jitter = min(max(jitterFraction, 0), 0.3) * Double(capped)
        return .seconds(Double(capped) + jitter)
    }

    private func syncRetryDelay() -> Duration {
        let exponent = min(syncRetryAttempt, 5)
        let multiplier = 1 << exponent
        let seconds = Self.minimumSyncRetryDelay.components.seconds * Int64(multiplier)
        let capped = min(seconds, Self.maximumSyncRetryDelay.components.seconds)
        let jitter = Double.random(in: 0...0.2) * Double(capped)
        return .seconds(Double(capped) + jitter)
    }

    private func handleRealtimeMessage(_ message: RealtimeMessage) {
        guard message.version >= 0,
              message.type == "ready" || message.type == "event" else {
            return
        }

        if realtimeState != .online {
            realtimeState = .online
            if statusMessage == Self.realtimeReconnectMessage {
                statusMessage = nil
                bannerTone = .neutral
            }
        }

        let scopes: SyncScope
        if message.type == "ready" {
            // Sans curseur, le serveur se place directement a sa version la
            // plus recente. Une projection est donc necessaire avant de pouvoir
            // valider cette version. Lors d'une reprise, les evenements manques
            // sont rejoues et aucune lecture complete n'est necessaire ici.
            scopes = realtimeCursor == nil ? .projection : []
        } else {
            scopes = syncScope(for: message.kind)
        }

        queueSynchronization(scopes: scopes, realtimeVersion: message.version)
    }

    private func syncScope(for kind: String?) -> SyncScope {
        switch kind {
        case "target.changed":
            [.targets, .incidents, .measures]
        case "source.changed", "observation.created":
            [.targets, .measures]
        case "connector.changed", "incident.changed", "maintenance.changed":
            .incidents
        case "notification.changed":
            .inbox
        case "component.heartbeat":
            .health
        case "device.changed":
            []
        default:
            .projection
        }
    }

    private func scheduleSync(for message: RealtimeMessage) {
        handleRealtimeMessage(message)
    }

    private func queueSynchronization(
        scopes: SyncScope,
        realtimeVersion: Int64? = nil,
        completion: CheckedContinuation<Bool, Never>? = nil,
        immediate: Bool = false
    ) {
        guard user != nil, isSceneActive else {
            completion?.resume(returning: false)
            return
        }

        pendingScopes.formUnion(scopes)
        let latestKnownVersion = max(pendingRealtimeVersion ?? -1, realtimeCursor ?? -1)
        if let realtimeVersion,
           realtimeVersion > latestKnownVersion {
            pendingRealtimeVersion = realtimeVersion
        }
        if let completion {
            pendingSyncCompletions.append(completion)
        }

        guard hasPendingSynchronization else {
            let completions = pendingSyncCompletions
            pendingSyncCompletions = []
            resume(completions, success: true)
            return
        }

        if synchronizationTask == nil {
            startSynchronization(after: immediate ? .zero : Self.realtimeCoalescingWindow)
        } else if immediate {
            expediteWaitingSynchronization()
        }
    }

    private var hasPendingSynchronization: Bool {
        !pendingScopes.isEmpty || pendingRealtimeVersion != nil
    }

    private func startSynchronization(after delay: Duration) {
        guard synchronizationTask == nil,
              hasPendingSynchronization,
              user != nil,
              isSceneActive else {
            return
        }

        let generation = synchronizationGeneration
        synchronizationIsWaiting = delay > .zero
        synchronizationTask = Task { [weak self] in
            await self?.runSynchronizationDrain(
                generation: generation,
                initialDelay: delay
            )
        }
    }

    private func expediteWaitingSynchronization() {
        guard synchronizationTask != nil, synchronizationIsWaiting else {
            return
        }

        synchronizationGeneration &+= 1
        synchronizationTask?.cancel()
        synchronizationTask = nil
        synchronizationIsWaiting = false
        startSynchronization(after: .zero)
    }

    private func runSynchronizationDrain(
        generation: Int,
        initialDelay: Duration
    ) async {
        defer { finishSynchronization(generation: generation) }

        var delay = initialDelay

        while !Task.isCancelled,
              generation == synchronizationGeneration,
              user != nil,
              isSceneActive {
            if delay > .zero {
                synchronizationIsWaiting = true
                do {
                    try await Task.sleep(for: delay)
                } catch {
                    return
                }
                synchronizationIsWaiting = false
            }

            guard !Task.isCancelled,
                  generation == synchronizationGeneration,
                  user != nil,
                  isSceneActive else {
                return
            }

            let batch = takePendingSynchronization()
            guard batch.hasWork else {
                return
            }

            synchronizationIsFetching = true
            do {
                let payload = try await loadSyncPayload(for: batch.scopes)
                guard !Task.isCancelled,
                      generation == synchronizationGeneration,
                      user != nil,
                      isSceneActive else {
                    throw CancellationError()
                }

                applySyncPayload(payload, for: batch)
                await saveSnapshot()
                synchronizationIsFetching = false
                syncRetryAttempt = 0
                resume(batch.completions, success: true)

                if !batch.scopes.isEmpty {
                    statusMessage = nil
                    bannerTone = .neutral
                }

                guard hasPendingSynchronization else {
                    return
                }
                delay = pendingSyncCompletions.isEmpty ? Self.realtimeCoalescingWindow : .zero
            } catch is CancellationError {
                synchronizationIsFetching = false
                resume(batch.completions, success: false)
                return
            } catch let error as CairnOpsAPIError where error.statusCode == 401 {
                synchronizationIsFetching = false
                resume(batch.completions, success: false)
                guard !Task.isCancelled,
                      generation == synchronizationGeneration,
                      user != nil,
                      isSceneActive else {
                    return
                }
                await invalidateSession(
                    message: "L’identité de cet appareil a été révoquée pendant la synchronisation."
                )
                return
            } catch {
                synchronizationIsFetching = false
                guard !Task.isCancelled,
                      generation == synchronizationGeneration,
                      user != nil,
                      isSceneActive else {
                    resume(batch.completions, success: false)
                    return
                }
                restore(batch)
                resume(batch.completions, success: false)

                statusMessage = batch.completions.isEmpty
                    ? "Synchronisation partielle ratée. Une nouvelle tentative suivra."
                    : (snapshot.hasProjection
                        ? "Lecture impossible pour le moment. Le dernier état connu reste visible."
                        : userFacingMessage(from: error))
                bannerTone = snapshot.hasProjection ? .caution : .danger

                delay = syncRetryDelay()
                syncRetryAttempt += 1
            }
        }
    }

    private func takePendingSynchronization() -> SyncBatch {
        let batch = SyncBatch(
            scopes: pendingScopes,
            realtimeVersion: pendingRealtimeVersion,
            completions: pendingSyncCompletions
        )
        pendingScopes = []
        pendingRealtimeVersion = nil
        pendingSyncCompletions = []
        return batch
    }

    private func restore(_ batch: SyncBatch) {
        pendingScopes.formUnion(batch.scopes)
        let latestKnownVersion = max(pendingRealtimeVersion ?? -1, realtimeCursor ?? -1)
        if let version = batch.realtimeVersion,
           version > latestKnownVersion {
            pendingRealtimeVersion = version
        }
    }

    private func loadSyncPayload(for scopes: SyncScope) async throws -> SyncPayload {
        let loadsProjection = scopes.contains(.projection)

        async let projection = loadOperationalProjection(ifRequested: loadsProjection)
        async let targets = loadTargets(ifRequested: !loadsProjection && scopes.contains(.targets))
        async let incidents = loadIncidents(ifRequested: !loadsProjection && scopes.contains(.incidents))
        async let measures = loadMeasures(ifRequested: !loadsProjection && scopes.contains(.measures))
        async let inbox = loadInbox(ifRequested: !loadsProjection && scopes.contains(.inbox))
        async let health = loadSystemHealth(ifRequested: scopes.contains(.health))
        async let version = loadVersion(ifRequested: scopes.contains(.version))

        return try await SyncPayload(
            projection: projection,
            targets: targets,
            incidents: incidents,
            measures: measures,
            inbox: inbox,
            health: health,
            version: version
        )
    }

    private func applySyncPayload(_ payload: SyncPayload, for batch: SyncBatch) {
        var nextSnapshot = snapshot
        var rebuildsDerivedProjection = false
        var publishesSnapshot = false

        nextSnapshot.serverBaseURL = serverURLText

        if let projection = payload.projection {
            nextSnapshot.targets = projection.targets
            nextSnapshot.incidents = projection.incidents
            nextSnapshot.measures = measuresByTarget(projection.measures)
            nextSnapshot.inbox = projection.inbox.entries
            nextSnapshot.unreadCount = projection.inbox.unread
            rebuildsDerivedProjection = true
            publishesSnapshot = true
        } else {
            if let targets = payload.targets {
                nextSnapshot.targets = targets
                rebuildsDerivedProjection = true
                publishesSnapshot = true
            }
            if let incidents = payload.incidents {
                nextSnapshot.incidents = incidents
                rebuildsDerivedProjection = true
                publishesSnapshot = true
            }
            if let measures = payload.measures {
                nextSnapshot.measures = measuresByTarget(measures)
                rebuildsDerivedProjection = true
                publishesSnapshot = true
            }
            if let inbox = payload.inbox {
                nextSnapshot.inbox = inbox.entries
                nextSnapshot.unreadCount = inbox.unread
                publishesSnapshot = true
            }
        }

        if batch.scopes.contains(.health) {
            nextSnapshot.systemHealth = payload.health
            publishesSnapshot = true
        }

        if let version = payload.version {
            serverVersion = version
        }

        if let version = batch.realtimeVersion {
            realtimeCursor = max(realtimeCursor ?? version, version)
        }
        nextSnapshot.realtimeVersion = realtimeCursor

        if rebuildsDerivedProjection {
            nextSnapshot.rebuildDerived()
        }
        if publishesSnapshot {
            nextSnapshot.lastRefreshAt = Date.now.ISO8601Format()
            snapshot = nextSnapshot
        }
    }

    private func measuresByTarget(_ entries: [TargetMeasures]) -> [String: TargetMeasures] {
        var measures: [String: TargetMeasures] = [:]
        measures.reserveCapacity(entries.count)
        for entry in entries {
            measures[entry.targetID] = entry
        }
        return measures
    }

    private func finishSynchronization(generation: Int) {
        guard synchronizationGeneration == generation else {
            return
        }

        synchronizationTask = nil
        synchronizationIsWaiting = false
        synchronizationIsFetching = false

        if hasPendingSynchronization, user != nil, isSceneActive {
            let delay = pendingSyncCompletions.isEmpty ? Self.realtimeCoalescingWindow : .zero
            startSynchronization(after: delay)
        }
    }

    private func cancelSynchronization() {
        synchronizationTask?.cancel()
        pendingScopes = []
        pendingRealtimeVersion = nil
        syncRetryAttempt = 0

        let completions = pendingSyncCompletions
        pendingSyncCompletions = []
        resume(completions, success: false)

        if synchronizationTask == nil {
            synchronizationIsWaiting = false
            synchronizationIsFetching = false
        }
    }

    private func resume(
        _ completions: [CheckedContinuation<Bool, Never>],
        success: Bool
    ) {
        for completion in completions {
            completion.resume(returning: success)
        }
    }

    private func stopRealtimeTransport() {
        activeRealtimeTask?.cancel(with: .goingAway, reason: nil)
        activeRealtimeTask = nil
    }

    private func refreshProjection() async {
        guard !isRefreshing, user != nil, isSceneActive else {
            return
        }

        isRefreshing = true
        _ = await withCheckedContinuation { completion in
            queueSynchronization(
                scopes: .fullRefresh,
                completion: completion,
                immediate: true
            )
        }
        isRefreshing = false
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

    private func loadOperationalProjection(
        ifRequested isRequested: Bool
    ) async throws -> CairnOpsAPI.OperationalProjection? {
        guard isRequested else {
            return nil
        }
        return try await loadOperationalProjection()
    }

    private func loadTargets() async throws -> [Target] {
        if let hook = debugHooks.fetchTargets {
            return try await hook()
        }
        return try await currentAPI().fetchTargets()
    }

    private func loadTargets(ifRequested isRequested: Bool) async throws -> [Target]? {
        guard isRequested else {
            return nil
        }
        return try await loadTargets()
    }

    private func loadIncidents() async throws -> [Incident] {
        if let hook = debugHooks.fetchIncidents {
            return try await hook()
        }
        return try await currentAPI().fetchIncidents()
    }

    private func loadIncidents(ifRequested isRequested: Bool) async throws -> [Incident]? {
        guard isRequested else {
            return nil
        }
        return try await loadIncidents()
    }

    private func loadMeasures() async throws -> [TargetMeasures] {
        if let hook = debugHooks.fetchMeasures {
            return try await hook()
        }
        return try await currentAPI().fetchTargetMeasures()
    }

    private func loadMeasures(ifRequested isRequested: Bool) async throws -> [TargetMeasures]? {
        guard isRequested else {
            return nil
        }
        return try await loadMeasures()
    }

    private func loadInbox() async throws -> CairnOpsAPI.InboxPayload {
        if let hook = debugHooks.fetchInbox {
            return try await hook()
        }

        return try await currentAPI().fetchInbox()
    }

    private func loadInbox(
        ifRequested isRequested: Bool
    ) async throws -> CairnOpsAPI.InboxPayload? {
        guard isRequested else {
            return nil
        }
        return try await loadInbox()
    }

    private func loadSystemHealth() async throws -> SystemHealth? {
        if let hook = debugHooks.fetchHealth {
            return try await hook()
        }

        return try await currentAPI().fetchSystemHealth()
    }

    private func loadSystemHealth(ifRequested isRequested: Bool) async throws -> SystemHealth? {
        guard isRequested else {
            return nil
        }
        return try await loadSystemHealth()
    }

    private func loadVersion() async throws -> String {
        if let hook = debugHooks.fetchVersion {
            return try await hook()
        }

        return try await currentAPI().getVersion()
    }

    private func loadVersion(ifRequested isRequested: Bool) async -> String? {
        guard isRequested else {
            return nil
        }
        return try? await loadVersion()
    }

    private func saveSnapshot() async {
        var persistedSnapshot = snapshot
        persistedSnapshot.realtimeVersion = realtimeCursor
        await snapshotStore.save(persistedSnapshot)
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
        realtimeLoopGeneration &+= 1
        stopRealtimeTransport()
        cancelSynchronization()
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
        realtimeCursor = nil
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
        targets: (() async throws -> [Target])? = nil,
        incidents: (() async throws -> [Incident])? = nil,
        measures: (() async throws -> [TargetMeasures])? = nil,
        inbox: (() async throws -> CairnOpsAPI.InboxPayload)? = nil,
        health: (() async throws -> SystemHealth?)? = nil,
        version: (() async throws -> String)? = nil
    ) {
        debugHooks = DebugHooks(
            fetchProjection: projection,
            fetchTargets: targets,
            fetchIncidents: incidents,
            fetchMeasures: measures,
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
                version: realtimeCursor ?? 0,
                kind: kind,
                entityType: nil,
                entityID: nil,
                occurredAt: nil
            )
        )
    }

    @MainActor
    func debugReceiveRealtimeMessage(type: String, kind: String?, version: Int64) {
        scheduleSync(
            for: RealtimeMessage(
                type: type,
                version: version,
                kind: kind,
                entityType: nil,
                entityID: nil,
                occurredAt: nil
            )
        )
    }

    @MainActor
    func debugSetReconnectAttempt(_ attempt: Int) {
        reconnectAttempt = attempt
    }

    @MainActor
    var debugReconnectAttempt: Int {
        reconnectAttempt
    }

    @MainActor
    var debugReconnectDelayWithoutJitter: Duration {
        reconnectDelay(jitterFraction: 0)
    }

    @MainActor
    var debugRealtimeCursor: Int64? {
        realtimeCursor
    }

    @MainActor
    func debugFlushPendingRealtimeScopes() async {
        guard hasPendingSynchronization, !synchronizationIsFetching else {
            return
        }

        _ = await withCheckedContinuation { completion in
            queueSynchronization(
                scopes: [],
                completion: completion,
                immediate: true
            )
        }
    }
}
#endif
