import SwiftUI

struct AppRootView: View {
	@Environment(\.scenePhase) private var scenePhase
	@AppStorage(AppearancePreference.storageKey) private var appearance = AppearancePreference.system
	@State private var model = AppModel()
	let pushNotifications: PushNotificationDelegate

    var body: some View {
        ZStack {
            AppBackdrop()

            Group {
                if showsIndicatorsPreview {
                    NavigationStack {
                        IndicatorsPreviewView()
                    }
                } else if showsSettingsPreview {
                    NavigationStack {
                        SettingsView()
                    }
                } else if showsNotificationSettingsPreview {
                    NavigationStack {
                        NotificationSettingsView()
                    }
                } else if let previewDetail {
                    NavigationStack {
                        switch previewDetail {
                        case .incident:
                            IncidentDetailView(incidentID: ShellPreviewData.previewIncidentID)
                        case .target:
                            TargetDetailView(targetID: ShellPreviewData.previewTargetID)
                        case .settings:
                            SettingsView()
                        }
                    }
                } else if showsShellPreview || model.showsShell {
                    AppShellView()
                } else if model.isBootstrapping {
                    ProgressView("Connexion à l’instance")
                        .controlSize(.large)
                } else {
                    LoginView()
                }
            }
        }
        .environment(model)
        .tint(AppTheme.accent)
        .preferredColorScheme(appearance.colorScheme)
        .task {
            // La prévisualisation de coque installe sa propre projection :
            // laisser l'amorçage s'exécuter l'écraserait aussitôt.
            guard !showsShellPreview else {
                installShellPreview()
                return
            }
            await model.bootstrap()
        }
        .task(id: model.pairingTaskIdentity) {
            guard model.pairingTaskIdentity != nil else {
                return
            }
            await model.runPairingAttempt()
        }
        // `realtimeIdentity` devient nil hors du premier plan : SwiftUI annule
        // alors la tache et ferme la socket au lieu de la laisser vivre.
		.task(id: model.realtimeIdentity) {
			// La prévisualisation n'a pas d'instance en face : ouvrir la socket
			// ne ferait qu'afficher une bannière d'échec par-dessus l'écran
			// que l'on cherche justement à contrôler.
			guard !showsShellPreview else {
				return
			}
			await model.runRealtimeLoop()
		}
		.task(id: model.hasDeviceIdentity) {
			guard model.hasDeviceIdentity else {
				return
			}
			await pushNotifications.requestAuthorizationAndRegistration()
			if let deviceToken = pushNotifications.deviceToken {
				await model.registerForPush(deviceToken: deviceToken)
			}
		}
		.task(id: pushNotifications.deviceToken) {
			guard model.hasDeviceIdentity,
				let deviceToken = pushNotifications.deviceToken else {
				return
			}
			await model.registerForPush(deviceToken: deviceToken)
		}
        .onChange(of: scenePhase, initial: true) { _, phase in
            // Le passage au premier plan declenche une synchronisation
            // complete. En prévisualisation elle n'a aucune instance en face
            // et ne produirait qu'une bannière d'échec.
            guard !showsShellPreview else {
                return
            }
            model.setScenePhaseActive(phase == .active)
        }
        .onOpenURL { url in
            model.acceptPairingURL(url)
        }
    }

    private var previewDetail: ShellPreviewData.Detail? {
#if DEBUG
        showsShellPreview ? ShellPreviewData.detail : nil
#else
        nil
#endif
    }

    private var showsShellPreview: Bool {
#if DEBUG
        ShellPreviewData.isEnabled
#else
        false
#endif
    }

    private func installShellPreview() {
#if DEBUG
        model.isBootstrapping = false
        model.instanceName = "Homeblack"
        model.serverURLText = "https://cairnops.int.homeblack.fr"
        model.serverVersion = "0.1.35"
        model.debugSetUser(ShellPreviewData.makeUser())
        model.snapshot = ShellPreviewData.makeSnapshot()
#endif
    }

    private var showsNotificationSettingsPreview: Bool {
#if DEBUG
        ProcessInfo.processInfo.arguments.contains("-CairnOpsNotificationSettingsPreview")
#else
        false
#endif
    }

    private var showsIndicatorsPreview: Bool {
#if DEBUG
        ProcessInfo.processInfo.arguments.contains("-CairnOpsIndicatorsPreview")
#else
        false
#endif
    }

    private var showsSettingsPreview: Bool {
#if DEBUG
        ProcessInfo.processInfo.arguments.contains("-CairnOpsSettingsPreview")
#else
        false
#endif
    }
}
