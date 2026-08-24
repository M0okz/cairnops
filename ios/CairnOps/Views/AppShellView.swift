import SwiftUI

/// Coque de l'application : les quatre vues operationnelles et leur barre.
///
/// La barre systeme est masquee au profit de la barre de verre de la maquette.
/// Le `TabView` est conserve dessous : il preserve la pile de navigation de
/// chaque onglet et n'en construit le contenu qu'a la premiere visite, ce
/// qu'un simple `ZStack` de vues cachees ne ferait pas.
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
            Tab(value: AppTab.overview) {
                NavigationStack {
                    OverviewView()
                }
                .toolbar(.hidden, for: .tabBar)
            }

            Tab(value: AppTab.incidents) {
                NavigationStack {
                    IncidentsView()
                }
                .toolbar(.hidden, for: .tabBar)
            }

            Tab(value: AppTab.targets) {
                NavigationStack {
                    TargetsView()
                }
                .toolbar(.hidden, for: .tabBar)
            }

            Tab(value: AppTab.health) {
                NavigationStack {
                    HealthView()
                }
                .toolbar(.hidden, for: .tabBar)
            }
        }
        .environment(\.selectTab) { tab in
            selectedTab = tab
        }
        .overlay(alignment: .bottom) {
            GlassTabBar(selection: $selectedTab)
        }
        // La barre de navigation etant masquee, le haut de page est du contenu :
        // une banniere posee en recouvrement cachait l'identite et le titre.
        // Elle reserve donc sa place au lieu de s'y superposer.
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
