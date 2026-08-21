import SwiftUI

struct ComponentHealthRow: View {
    let title: String
    let statusText: String
    let statusColor: Color
    let statusSymbol: String
    let detail: String?

    var body: some View {
        HStack(alignment: .top, spacing: 16) {
            VStack(alignment: .leading, spacing: 4) {
                Text(title)
                    .font(AppTheme.rowTitleFont)

                if let detail, !detail.isEmpty {
                    Text(detail)
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }

            Spacer(minLength: 16)

            StatusPill(
                text: statusText,
                color: statusColor,
                systemImage: statusSymbol
            )
        }
    }
}
