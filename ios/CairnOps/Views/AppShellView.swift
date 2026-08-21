import SwiftUI

struct AppShellView: View {
    @Environment(AppModel.self) private var model
    @State private var selectedTab: AppTab = .overview

    var body: some View {
        TabView(selection: $selectedTab) {
            NavigationStack {
                OverviewView()
            }
            .tabItem { Label(AppTab.overview.title, systemImage: AppTab.overview.symbolName) }
            .tag(AppTab.overview)

            NavigationStack {
                TargetsView()
            }
            .tabItem { Label(AppTab.targets.title, systemImage: AppTab.targets.symbolName) }
            .tag(AppTab.targets)

            NavigationStack {
                IncidentsView()
            }
            .tabItem { Label(AppTab.incidents.title, systemImage: AppTab.incidents.symbolName) }
            .tag(AppTab.incidents)

            NavigationStack {
                HealthView()
            }
            .tabItem { Label(AppTab.health.title, systemImage: AppTab.health.symbolName) }
            .tag(AppTab.health)

            NavigationStack {
                SettingsView()
            }
            .tabItem { Label(AppTab.settings.title, systemImage: AppTab.settings.symbolName) }
            .tag(AppTab.settings)
        }
        .overlay(alignment: .top) {
            if let statusMessage = model.statusMessage {
                Text(statusMessage)
                    .font(.footnote)
                    .multilineTextAlignment(.leading)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 16)
                    .padding(.vertical, 12)
                    .background(
                        RoundedRectangle(cornerRadius: 16)
                            .fill(bannerColor)
                    )
                    .foregroundStyle(.white)
                    .padding(.horizontal, 16)
                    .padding(.top, 8)
            }
        }
        .animation(.easeInOut(duration: 0.2), value: model.statusMessage)
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
