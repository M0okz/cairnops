import SwiftUI

struct StatusPill: View {
    let text: String
    let color: Color
    var systemImage: String?

    var body: some View {
        Group {
            if let systemImage {
                Label(text, systemImage: systemImage)
            } else {
                Text(text)
            }
        }
        .font(.caption.weight(.medium))
        .lineLimit(1)
        .minimumScaleFactor(0.85)
        .foregroundStyle(color)
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(
            Capsule()
                .fill(color.opacity(0.14))
                .overlay(
                    Capsule()
                        .strokeBorder(color.opacity(0.08))
                )
        )
    }
}
