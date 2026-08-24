import SwiftUI

/// Reglages de l'application et de l'appareil associe.
///
/// L'ecran est atteint depuis l'identite de la Vue d'ensemble : la barre
/// d'onglets reste consacree aux quatre vues operationnelles.
struct SettingsView: View {
    @Environment(AppModel.self) private var model
    @AppStorage(AppearancePreference.storageKey) private var appearance = AppearancePreference.system

    var body: some View {
        BareScreen {
            header
            account
            general
            notifications
            about
            device
        }
        .refreshable {
            await model.refresh()
        }
    }

    // MARK: - Haut de page

    private var header: some View {
        HStack {
            BackLink(title: "Vue d’ensemble")
            Spacer(minLength: 0)
        }
        .padding(.vertical, 2)
        .padding(.bottom, 18)
    }

    @ViewBuilder
    private var account: some View {
        if let user = model.user {
            HStack(spacing: 14) {
                AvatarBadge(name: user.displayName, size: 48)

                VStack(alignment: .leading, spacing: 2) {
                    Text(user.displayName)
                        .font(.title3.weight(.bold))
                        .tracking(-0.3)
                        .foregroundStyle(AppTheme.ink)

                    Text("\(user.role.label) · \(model.instanceLabel)")
                        .font(.footnote)
                        .foregroundStyle(AppTheme.inkMuted)
                }

                Spacer(minLength: 8)
            }
            .padding(.vertical, 16)
            .accessibilityElement(children: .combine)
        }
    }

    // MARK: - General

    private var general: some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionLabel("Général")
                .padding(.top, 22)
                .padding(.bottom, 4)

            HStack(spacing: 12) {
                Text("Apparence")
                    .font(AppTheme.fieldValueFont)
                    .foregroundStyle(AppTheme.ink)

                Spacer(minLength: 8)

                UnderlineTabs(
                    selection: $appearance,
                    items: AppearancePreference.allCases.map { .init($0, $0.title) },
                    isInline: true
                )
            }
            .padding(.vertical, 15)
            .frame(minHeight: 44)
        }
        .padding(.top, 10)
        .hairlineTop()
    }

    // MARK: - Notifications

    private var notifications: some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionLabel("Notifications")
                .padding(.top, 22)
                .padding(.bottom, 4)

            NavigationLink {
                NotificationSettingsView()
            } label: {
                SettingsRow(
                    title: "Alertes sur cet iPhone",
                    subtitle: "Sons, alertes critiques et rappels",
                    systemImage: "bell"
                )
            }
            .buttonStyle(.plain)
            .accessibilityHint("Ouvre les réglages de notification")
        }
        .padding(.top, 10)
        .hairlineTop()
    }

    // MARK: - A propos

    private var about: some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionLabel("À propos")
                .padding(.top, 22)
                .padding(.bottom, 4)

            FieldRow(label: "Nom", value: model.instanceLabel)

            FieldRow(
                label: "URL",
                value: model.serverURLText.isEmpty ? "Non configurée" : model.serverURLText,
                secondary: "Connexion directe à l’instance",
                allowsSelection: true
            )

            FieldRow(
                label: "Version serveur",
                value: model.serverVersion.isEmpty ? "Inconnue" : model.serverVersion
            )

            FieldRow(label: "Version app", value: Self.applicationVersion)

            FieldRow(
                label: "Dernière projection",
                value: TimestampParser.relativeString(from: model.snapshot.lastRefreshAt),
                secondary: TimestampParser.absoluteString(from: model.snapshot.lastRefreshAt)
            )
        }
        .padding(.top, 10)
        .hairlineTop()
    }

    private static var applicationVersion: String {
        let info = Bundle.main.infoDictionary
        let version = info?["CFBundleShortVersionString"] as? String ?? "—"
        guard let build = info?["CFBundleVersion"] as? String, !build.isEmpty else {
            return version
        }
        return "\(version) (\(build))"
    }

    // MARK: - Appareil

    @ViewBuilder
    private var device: some View {
        if let user = model.user {
            VStack(alignment: .leading, spacing: 0) {
                SectionLabel("Appareil associé")
                    .padding(.top, 22)
                    .padding(.bottom, 4)

                FieldRow(label: "Utilisateur", value: user.displayName)
                FieldRow(label: "Rôle", value: user.role.label, tone: AppTheme.accent)

                HStack(spacing: 26) {
                    inlineAction(title: "Actualiser", systemImage: "arrow.clockwise", tone: AppTheme.accent) {
                        await model.refresh()
                    }
                    inlineAction(title: "Dissocier", systemImage: "iphone.slash", tone: AppTheme.inkStrong) {
                        await model.logout()
                    }
                    Spacer(minLength: 0)
                }
                .padding(.top, 16)
            }
            .padding(.top, 10)
            .hairlineTop()
        } else {
            VStack(alignment: .leading, spacing: 0) {
                SectionLabel("Appareil")
                    .padding(.top, 22)
                    .padding(.bottom, 4)

                DevicePairingPanel()

                if model.snapshot.hasProjection {
                    Text("Le cache local conserve la dernière projection hors ligne sur l’appareil.")
                        .font(AppTheme.metaFont)
                        .foregroundStyle(AppTheme.inkMuted)
                        .padding(.top, 16)

                    inlineAction(title: "Effacer le cache", systemImage: "trash", tone: AppTheme.criticalInk) {
                        await model.clearOfflineSnapshot()
                    }
                    .padding(.top, 12)
                }
            }
            .padding(.top, 10)
            .hairlineTop()
        }
    }

    private func inlineAction(
        title: String,
        systemImage: String,
        tone: Color,
        action: @escaping @Sendable () async -> Void
    ) -> some View {
        AsyncButton(action: action) {
            HStack(spacing: 7) {
                Image(systemName: systemImage)
                    .font(.system(size: 15, weight: .bold))
                Text(title)
                    .font(AppTheme.fieldValueFont)
            }
            .foregroundStyle(tone)
            .frame(minHeight: 44)
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
    }
}
