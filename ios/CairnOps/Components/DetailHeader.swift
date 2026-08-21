import SwiftUI

struct DetailHeader: View {
    @Environment(\.dismiss) private var dismiss

    let title: String
    let subtitle: String?

    init(title: String, subtitle: String? = nil) {
        self.title = title
        self.subtitle = subtitle
    }

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Button {
                dismiss()
            } label: {
                Image(systemName: "chevron.left")
                    .font(.headline.weight(.semibold))
                    .frame(width: 40, height: 40)
                    .background(
                        Circle()
                            .fill(AppTheme.panel)
                            .overlay(
                                Circle()
                                    .strokeBorder(AppTheme.line)
                            )
                    )
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Revenir")

            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .font(AppTheme.detailTitleFont)

                if let subtitle, !subtitle.isEmpty {
                    Text(subtitle)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
            }

            Spacer(minLength: 0)
        }
    }
}
