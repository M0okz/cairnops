import SwiftUI

/// Coque de l'application : les quatre vues operationnelles.
///
/// La barre d'onglets est celle du systeme. Une barre dessinee a la main
/// reproduisait l'apparence de la maquette mais perdait tout ce qui va avec :
/// le verre natif, le repli au defilement, les tailles d'accessibilite et les
/// comportements attendus d'une barre iOS.
struct AppShellView: View {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(AppModel.self) private var model
    @State private var selectedTab: AppTab = Self.startingTab

    private static var startingTab: AppTab {
#if DEBUG
        ShellPreviewData.isEnabled ? ShellPreviewData.initialTab : .overview
#else
        .overview
#endif
    }

    var body: some View {
        TabView(selection: $selectedTab) {
            Tab(AppTab.overview.title, systemImage: AppTab.overview.symbolName, value: AppTab.overview) {
                NavigationStack {
                    OverviewView()
                }
            }

            Tab(AppTab.incidents.title, systemImage: AppTab.incidents.symbolName, value: AppTab.incidents) {
                NavigationStack {
                    IncidentsView()
                }
            }

            Tab(AppTab.targets.title, systemImage: AppTab.targets.symbolName, value: AppTab.targets) {
                NavigationStack {
                    TargetsView()
                }
            }

            Tab(AppTab.health.title, systemImage: AppTab.health.symbolName, value: AppTab.health) {
                NavigationStack {
                    HealthView()
                }
            }
        }
        .environment(\.selectTab) { tab in
            selectedTab = tab
        }
        // La barre de navigation etant a nouveau visible, la banniere se pose
        // sous elle plutot que de recouvrir le titre.
        .safeAreaInset(edge: .top, spacing: 0) {
            statusBanner
        }
    }

    @ViewBuilder
    private var statusBanner: some View {
        Group {
            if let statusMessage = model.statusMessage {
                Text(statusMessage)
                    .font(.footnote)
                    .multilineTextAlignment(.leading)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 16)
                    .padding(.vertical, 12)
                    .background(
                        RoundedRectangle(cornerRadius: 16, style: .continuous)
                            .fill(bannerColor)
                    )
                    .foregroundStyle(.white)
                    .padding(.horizontal, AppTheme.barInset)
                    .padding(.bottom, 8)
                    .transition(.move(edge: .top).combined(with: .opacity))
            }
        }
        // La banniere est informative : elle ne doit jamais voler le premier
        // geste de defilement ou masquer une commande.
        .allowsHitTesting(false)
        .animation(
            reduceMotion ? nil : .easeInOut(duration: 0.2),
            value: model.statusMessage
        )
    }

    private var bannerColor: Color {
        switch model.bannerTone {
        case .neutral:
            AppTheme.info
        case .caution:
            AppTheme.warning
        case .danger:
            AppTheme.critical
        }
    }
}
