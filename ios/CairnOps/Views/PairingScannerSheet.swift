import AVFoundation
import SwiftUI
import Vision
import VisionKit

struct PairingScannerSheet: View {
    private enum CameraState {
        case checking
        case ready
        case unavailable(String)
    }

    @Environment(\.dismiss) private var dismiss
    @State private var cameraState: CameraState = .checking
    @State private var scanMessage: String?

    let onScan: (String) -> Void

    var body: some View {
        NavigationStack {
            Group {
                switch cameraState {
                case .checking:
                    ProgressView("Préparation de la caméra")
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                case .ready:
                    LiveQRCodeScanner(
                        onPayload: { payload in
                            onScan(payload)
                            dismiss()
                        },
                        onInvalidPayload: {
                            scanMessage = "Ce QR code ne contient pas une invitation CairnOps valide."
                        },
                        onUnavailable: { message in
                            cameraState = .unavailable(message)
                        }
                    )
                    .overlay(alignment: .bottom) {
                        Text(
                            scanMessage
                                ?? "Placez le QR code CairnOps dans le cadre. Le scan démarre automatiquement."
                        )
                            .font(.subheadline)
                            .multilineTextAlignment(.center)
                            .foregroundStyle(scanMessage == nil ? Color.primary : AppTheme.critical)
                            .padding(14)
                            .frame(maxWidth: .infinity)
                            .background(.ultraThinMaterial)
                    }
                    .accessibilityIdentifier("pairing.scanner")
                case .unavailable(let message):
                    ContentUnavailableView(
                        "Caméra indisponible",
                        systemImage: "camera.fill",
                        description: Text(message)
                    )
                }
            }
            .navigationTitle("Scanner le QR code")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Annuler") {
                        dismiss()
                    }
                }
            }
        }
        .task {
            await prepareCamera()
        }
    }

    private func prepareCamera() async {
        guard DataScannerViewController.isSupported else {
            cameraState = .unavailable(
                "Ce modèle ne prend pas en charge le scanner. Utilisez la saisie du lien d’association."
            )
            return
        }

        let authorized: Bool = switch AVCaptureDevice.authorizationStatus(for: .video) {
        case .authorized:
            true
        case .notDetermined:
            await AVCaptureDevice.requestAccess(for: .video)
        case .denied, .restricted:
            false
        @unknown default:
            false
        }

        guard authorized, DataScannerViewController.isAvailable else {
            cameraState = .unavailable(
                "Autorisez l’accès à la caméra dans Réglages, ou utilisez la saisie du lien d’association."
            )
            return
        }
        cameraState = .ready
    }
}

private struct LiveQRCodeScanner: UIViewControllerRepresentable {
    let onPayload: (String) -> Void
    let onInvalidPayload: () -> Void
    let onUnavailable: (String) -> Void

    func makeCoordinator() -> Coordinator {
        Coordinator(
            onPayload: onPayload,
            onInvalidPayload: onInvalidPayload,
            onUnavailable: onUnavailable
        )
    }

    func makeUIViewController(context: Context) -> DataScannerViewController {
        let scanner = DataScannerViewController(
            recognizedDataTypes: [.barcode(symbologies: [.qr])],
            qualityLevel: .balanced,
            recognizesMultipleItems: false,
            isHighFrameRateTrackingEnabled: false,
            isPinchToZoomEnabled: true,
            isGuidanceEnabled: true,
            isHighlightingEnabled: true
        )
        scanner.delegate = context.coordinator

        do {
            try scanner.startScanning()
        } catch {
            let message = error.localizedDescription
            let coordinator = context.coordinator
            Task { @MainActor in
                coordinator.reportUnavailable(message)
            }
        }
        return scanner
    }

    func updateUIViewController(
        _ uiViewController: DataScannerViewController,
        context: Context
    ) {}

    static func dismantleUIViewController(
        _ uiViewController: DataScannerViewController,
        coordinator: Coordinator
    ) {
        uiViewController.stopScanning()
    }

    @MainActor
    final class Coordinator: NSObject, DataScannerViewControllerDelegate {
        private let onPayload: (String) -> Void
        private let onInvalidPayload: () -> Void
        private let onUnavailable: (String) -> Void
        private var deliveredPayload = false

        init(
            onPayload: @escaping (String) -> Void,
            onInvalidPayload: @escaping () -> Void,
            onUnavailable: @escaping (String) -> Void
        ) {
            self.onPayload = onPayload
            self.onInvalidPayload = onInvalidPayload
            self.onUnavailable = onUnavailable
        }

        func dataScanner(
            _ dataScanner: DataScannerViewController,
            didAdd addedItems: [RecognizedItem],
            allItems: [RecognizedItem]
        ) {
            guard let item = addedItems.first else {
                return
            }
            deliver(item, scanner: dataScanner)
        }

        func dataScanner(
            _ dataScanner: DataScannerViewController,
            didTapOn item: RecognizedItem
        ) {
            deliver(item, scanner: dataScanner)
        }

        func dataScanner(
            _ dataScanner: DataScannerViewController,
            becameUnavailableWithError error: DataScannerViewController.ScanningUnavailable
        ) {
            reportUnavailable(error.localizedDescription)
        }

        func reportUnavailable(_ message: String) {
            onUnavailable(message)
        }

        private func deliver(
            _ item: RecognizedItem,
            scanner: DataScannerViewController
        ) {
            guard !deliveredPayload,
                  case .barcode(let barcode) = item,
                  let payload = barcode.payloadStringValue else {
                return
            }
            guard (try? DevicePairingLink(string: payload)) != nil else {
                onInvalidPayload()
                return
            }
            deliveredPayload = true
            scanner.stopScanning()
            onPayload(payload)
        }
    }
}
