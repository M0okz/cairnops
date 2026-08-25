import SwiftUI

struct DevicePairingPanel: View {
    @Environment(AppModel.self) private var model
    @State private var destination: PairingDestination?

    private enum PairingDestination: String, Identifiable {
        case scanner
        case link

        var id: String { rawValue }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            VStack(alignment: .leading, spacing: 6) {
                Text("Associer cet iPhone")
                    .font(.title3.weight(.bold))
                    .tracking(-0.3)
                    .foregroundStyle(AppTheme.ink)
                Text("L’identité reste propre à cet appareil et peut être révoquée sans fermer vos autres accès.")
                    .font(.subheadline)
                    .foregroundStyle(AppTheme.inkMuted)
            }

            stateContent

            Hairline()

            VStack(alignment: .leading, spacing: 12) {
                PairingStep(
                    number: 1,
                    title: "Scanner",
                    detail: "Le QR code contient l’adresse de l’instance et un secret valable dix minutes."
                )
                PairingStep(
                    number: 2,
                    title: "Vérifier sur le Web",
                    detail: "Le navigateur affiche le nom de cet iPhone avant de créer son identité."
                )
                PairingStep(
                    number: 3,
                    title: "Confirmer",
                    detail: "Le jeton révocable est remis une seule fois et conservé dans Keychain."
                )
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .sheet(item: $destination) { destination in
            switch destination {
            case .scanner:
                PairingScannerSheet { payload in
                    model.acceptPairingLink(payload)
                }
            case .link:
                PairingLinkEntrySheet { link in
                    model.acceptPairingLink(link)
                }
            }
        }
    }

    @ViewBuilder
    private var stateContent: some View {
        switch model.pairingState {
        case .idle:
            pairingActions
        case .claiming(let instance):
            progress(
                title: "Présentation à \(instance)",
                detail: "CairnOps prépare l’identité cryptographique de cet iPhone.",
                allowsCancellation: true
            )
        case .awaitingConfirmation(let instance):
            progress(
                title: "Confirmation attendue sur le Web",
                detail: "Vérifiez le nom de l’appareil dans \(instance), puis confirmez l’association. Pour arrêter, annulez l’invitation depuis le Web.",
                allowsCancellation: false
            )
        case .finalizing(let instance):
            progress(
                title: "Ouverture de \(instance)",
                detail: "L’identité est enregistrée. La première projection opérationnelle se synchronise.",
                allowsCancellation: false
            )
        case .failed(let message):
            VStack(alignment: .leading, spacing: 12) {
                Label(message, systemImage: "exclamationmark.triangle.fill")
                    .font(.subheadline)
                    .foregroundStyle(AppTheme.criticalInk)

                if model.canRetryPairing {
                    Button("Réessayer") {
                        model.retryPairing()
                    }
                    .buttonStyle(.borderedProminent)
                    .accessibilityIdentifier("pairing.retry")
                }

                if model.canRetryPairing, !model.hasDeviceIdentity {
                    Text("Si cet iPhone apparaît déjà dans le navigateur, annulez aussi l’invitation sur le Web avant d’en utiliser une autre.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }

                Button(
                    model.hasDeviceIdentity
                        ? "Dissocier et scanner un nouveau QR code"
                        : model.canRetryPairing
                            ? "Abandonner et scanner un nouveau QR code"
                            : "Scanner un nouveau QR code"
                ) {
                    beginNewPairing(at: .scanner)
                }
                .buttonStyle(.bordered)
                .accessibilityIdentifier("pairing.scan-again")

                Button(
                    model.hasDeviceIdentity
                        ? "Dissocier et saisir un nouveau lien"
                        : model.canRetryPairing
                            ? "Abandonner et saisir un nouveau lien"
                            : "Saisir le lien d’association"
                ) {
                    beginNewPairing(at: .link)
                }
                .buttonStyle(.plain)
                .foregroundStyle(AppTheme.control)
                .accessibilityIdentifier("pairing.link-again")
            }
        }
    }

    private var pairingActions: some View {
        VStack(alignment: .leading, spacing: 12) {
            Button {
                destination = .scanner
            } label: {
                Label("Scanner le QR code", systemImage: "qrcode.viewfinder")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.large)
            .accessibilityIdentifier("pairing.scan")

            Button {
                destination = .link
            } label: {
                Label("Saisir le lien d’association", systemImage: "link")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
            .controlSize(.large)
            .accessibilityIdentifier("pairing.link")
        }
    }

    private func progress(
        title: String,
        detail: String,
        allowsCancellation: Bool
    ) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .top, spacing: 12) {
                ProgressView()
                    .controlSize(.large)
                VStack(alignment: .leading, spacing: 4) {
                    Text(title)
                        .font(AppTheme.fieldValueFont)
                    Text(detail)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
            }

            if allowsCancellation {
                Button("Annuler l’appairage", role: .cancel) {
                    model.cancelPairing()
                }
                .buttonStyle(.bordered)
                .accessibilityIdentifier("pairing.cancel")
            }
        }
        .accessibilityElement(children: .contain)
    }

    private func beginNewPairing(at destination: PairingDestination) {
        Task { @MainActor in
            if model.hasDeviceIdentity {
                await model.logout()
            } else {
                model.cancelPairing()
            }
            self.destination = destination
        }
    }
}

private struct PairingStep: View {
    let number: Int
    let title: String
    let detail: String

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Text(String(number))
                .font(.caption.weight(.semibold))
                .monospacedDigit()
                .frame(width: 26, height: 26)
                .foregroundStyle(AppTheme.control)
                .background(Circle().fill(AppTheme.controlFill))
                .accessibilityHidden(true)

            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.subheadline.weight(.semibold))
                Text(detail)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("Étape \(number), \(title). \(detail)")
    }
}
