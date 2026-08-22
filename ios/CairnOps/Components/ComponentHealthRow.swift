import SwiftUI

struct ComponentHealthRow: View {
    let title: String
    let statusText: String
    let statusColor: Color
    let statusSymbol: String
    let detail: String?

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(alignment: .top, spacing: 16) {
                identity
                Spacer(minLength: 16)
                status
            }

            VStack(alignment: .leading, spacing: 8) {
                identity
                status
            }
        }
    }

    private var identity: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title)
                .font(AppTheme.rowTitleFont)

            if let detail, !detail.isEmpty {
                Text(detail)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var status: some View {
        StatusPill(
            text: statusText,
            color: statusColor,
            systemImage: statusSymbol
        )
    }
}
