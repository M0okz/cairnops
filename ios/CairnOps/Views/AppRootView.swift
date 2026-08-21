import SwiftUI

struct AppRootView: View {
    @Environment(\.scenePhase) private var scenePhase
    @State private var model = AppModel()

    var body: some View {
        ZStack {
            AppBackdrop()

            Group {
                if model.showsShell {
                    AppShellView()
                } else if model.isBootstrapping {
                    ProgressView("Connexion a l'instance")
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
        // `realtimeIdentity` devient nil hors du premier plan : SwiftUI annule
        // alors la tache et ferme la socket au lieu de la laisser vivre.
        .task(id: model.realtimeIdentity) {
            await model.runRealtimeLoop()
        }
        .onChange(of: scenePhase, initial: true) { _, phase in
            model.setScenePhaseActive(phase == .active)
        }
    }
}
