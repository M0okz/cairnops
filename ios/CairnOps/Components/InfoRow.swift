import SwiftUI

struct InfoRow: View {
    enum ValueTone {
        case normal
        case accent(Color)
        case secondary
    }

    let label: String
    let value: String
    var secondary: String?
    var tone: ValueTone = .normal
    var monospaced = false
    var allowsSelection = false

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label)
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .textCase(.uppercase)

            valueText
                .font(.body)
                .foregroundStyle(foregroundStyle)

            if let secondary, !secondary.isEmpty {
                Text(secondary)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var foregroundStyle: Color {
        switch tone {
        case .normal:
            .primary
        case .accent(let color):
            color
        case .secondary:
            .secondary
        }
    }

    @ViewBuilder
    private var valueText: some View {
        let baseText = monospaced
            ? Text(value).monospacedDigit()
            : Text(value)

        if allowsSelection {
            baseText.textSelection(.enabled)
        } else {
            baseText
        }
    }
}
