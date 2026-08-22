import SwiftUI

struct SettingsView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: AppTheme.sectionSpacing) {
                Panel {
                    VStack(alignment: .leading, spacing: 14) {
                        Text("Notifications")
                            .font(AppTheme.sectionTitleFont)

                        NavigationLink {
                            NotificationSettingsView()
                        } label: {
                            HStack(spacing: 12) {
                                Image(systemName: "bell.badge")
                                    .font(.title3)
                                    .foregroundStyle(AppTheme.accent)
                                    .frame(width: 30)

                                VStack(alignment: .leading, spacing: 3) {
                                    Text("Alertes sur cet iPhone")
                                        .font(AppTheme.rowTitleFont)
                                    Text("Sons, alertes critiques et rappels")
                                        .font(.subheadline)
                                        .foregroundStyle(.secondary)
                                }

                                Spacer(minLength: 8)

                                Image(systemName: "chevron.right")
                                    .font(.footnote.weight(.semibold))
                                    .foregroundStyle(.tertiary)
                            }
                            .frame(minHeight: 44)
                            .contentShape(.rect)
                        }
                        .buttonStyle(.plain)
                        .accessibilityHint("Ouvre les réglages de notification")
                    }
                }

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
                            Text("Appareil associé")
                                .font(AppTheme.sectionTitleFont)

                            MetricGrid {
                                MetricTile(title: "Utilisateur", value: user.displayName, monospaced: false)
                                MetricTile(title: "Rôle", value: user.role.label, tone: AppTheme.info, monospaced: false)
                            }

                            HStack(spacing: 12) {
                                actionButton(title: "Actualiser", systemImage: "arrow.clockwise", tone: AppTheme.accent) {
                                    await model.refresh()
                                }
                                actionButton(title: "Dissocier", systemImage: "iphone.slash", tone: AppTheme.warning) {
                                    await model.logout()
                                }
                            }
                        }
                    }
                } else {
                    DevicePairingPanel()

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
        .navigationTitle("Réglages")
        .navigationBarTitleDisplayMode(.inline)
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
