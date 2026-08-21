import SwiftUI

struct ScreenHeader<ActionContent: View>: View {
    let title: String
    let subtitle: String?
    private let actionContent: ActionContent

    init(
        title: String,
        subtitle: String? = nil,
        @ViewBuilder action: () -> ActionContent = { EmptyView() }
    ) {
        self.title = title
        self.subtitle = subtitle
        self.actionContent = action()
    }

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            VStack(alignment: .leading, spacing: 4) {
                Text(title)
                    .font(AppTheme.pageTitleFont)

                if let subtitle, !subtitle.isEmpty {
                    Text(subtitle)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
            }

            Spacer(minLength: 16)

            actionContent
        }
    }
}
