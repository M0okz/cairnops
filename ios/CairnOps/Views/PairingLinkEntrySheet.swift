import SwiftUI

struct PairingLinkEntrySheet: View {
    @Environment(\.dismiss) private var dismiss
    @FocusState private var isFocused: Bool
    @State private var link = ""
    @State private var errorMessage: String?

    let onSubmit: (String) -> Void

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField(
                        "cairnops://pair?instance=…",
                        text: $link,
                        axis: .vertical
                    )
                    .textInputAutocapitalization(.never)
                    .textContentType(.URL)
                    .keyboardType(.URL)
                    .autocorrectionDisabled()
                    .lineLimit(3...6)
                    .focused($isFocused)
                    .accessibilityIdentifier("pairing.link-field")
                } header: {
                    Text("Lien d’association")
                } footer: {
                    Text("Copiez le lien affiché avec le QR code dans les réglages Web de CairnOps.")
                }

                if let errorMessage {
                    Section {
                        Label(errorMessage, systemImage: "exclamationmark.triangle.fill")
                            .foregroundStyle(AppTheme.critical)
                    }
                }

                Section {
                    Button("Continuer") {
                        submit()
                    }
                    .frame(maxWidth: .infinity)
                    .disabled(link.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    .accessibilityIdentifier("pairing.link-submit")
                }
            }
            .navigationTitle("Associer l’iPhone")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Annuler") {
                        dismiss()
                    }
                }
            }
            .onAppear {
                isFocused = true
            }
        }
        .presentationDetents([.medium, .large])
    }

    private func submit() {
        do {
            _ = try DevicePairingLink(string: link)
            onSubmit(link)
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
