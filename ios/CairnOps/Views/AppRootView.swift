import SwiftUI

struct AppRootView: View {
	@Environment(\.scenePhase) private var scenePhase
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
                } else if model.showsShell {
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
        .task {
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
            model.setScenePhaseActive(phase == .active)
        }
        .onOpenURL { url in
            model.acceptPairingURL(url)
        }
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
