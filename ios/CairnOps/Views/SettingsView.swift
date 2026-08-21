import SwiftUI

struct SettingsView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                ScreenHeader(
                    title: "Réglages",
                    subtitle: "Instance, session et cache local"
                )

                Panel {
                        VStack(alignment: .leading, spacing: 14) {
                            Text("Instance")
                                .font(AppTheme.sectionTitleFont)

                        VStack(alignment: .leading, spacing: 16) {
                            InfoRow(label: "Nom", value: model.instanceLabel)

                            Divider()

                            InfoRow(
                                label: "URL",
                                value: model.serverURLText.isEmpty ? "Non configurée" : model.serverURLText,
                                secondary: "Connexion directe à l’instance",
                                tone: .secondary,
                                allowsSelection: true
                            )

                            Divider()

                            InfoRow(
                                label: "Version serveur",
                                value: model.serverVersion.isEmpty ? "Inconnue" : model.serverVersion,
                                monospaced: true
                            )

                            Divider()

                            InfoRow(
                                label: "Dernière projection",
                                value: TimestampParser.relativeString(from: model.snapshot.lastRefreshAt),
                                secondary: TimestampParser.absoluteString(from: model.snapshot.lastRefreshAt)
                            )
                        }
                    }
                }

                if let user = model.user {
                    Panel {
                        VStack(alignment: .leading, spacing: 14) {
                            Text("Session")
                                .font(AppTheme.sectionTitleFont)

                            HStack(spacing: 12) {
                                MetricTile(title: "Utilisateur", value: user.displayName, monospaced: false)
                                MetricTile(title: "Rôle", value: user.role.label, tone: AppTheme.info, monospaced: false)
                            }

                            HStack(spacing: 12) {
                                actionButton(title: "Actualiser", systemImage: "arrow.clockwise", tone: AppTheme.accent) {
                                    await model.refresh()
                                }
                                actionButton(title: "Fermer", systemImage: "rectangle.portrait.and.arrow.right", tone: AppTheme.warning) {
                                    await model.logout()
                                }
                            }
                        }
                    }
                } else {
                    Panel {
                        VStack(alignment: .leading, spacing: 14) {
                            Text("Session")
                                .font(AppTheme.sectionTitleFont)
                            Text("Mode hors ligne ou session absente.")
                                .foregroundStyle(.secondary)

                            InstanceLoginForm(
                                title: "Rétablir la session",
                                subtitle: "Le cache local reste consultable, mais aucune mutation n’est envoyée tant que l’instance n’est pas rejointe."
                            )
                        }
                    }

                    if model.snapshot.hasProjection {
                        Panel {
                            VStack(alignment: .leading, spacing: 14) {
                                Text("Cache local")
                                    .font(AppTheme.sectionTitleFont)
                                Text("Efface la projection hors ligne conservée sur l’appareil.")
                                    .foregroundStyle(.secondary)

                                actionButton(title: "Effacer le cache", systemImage: "trash", tone: AppTheme.critical) {
                                    await model.clearOfflineSnapshot()
                                }
                            }
                        }
                    }
                }
            }
            .padding(AppTheme.screenPadding)
            .padding(.bottom, AppTheme.bottomScrollInset)
        }
        .background(AppBackdrop())
        .toolbar(.hidden, for: .navigationBar)
    }

    private func actionButton(
        title: String,
        systemImage: String,
        tone: Color,
        action: @escaping @Sendable () async -> Void
    ) -> some View {
        AsyncButton(action: action) {
            Label(title, systemImage: systemImage)
                .font(.headline)
                .frame(maxWidth: .infinity, minHeight: 26)
                .padding(.vertical, 14)
                .background(
                    RoundedRectangle(cornerRadius: 18)
                        .fill(tone.opacity(0.14))
                )
        }
        .buttonStyle(.plain)
        .foregroundStyle(tone)
    }
}
