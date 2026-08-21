import SwiftUI

struct Panel<Content: View>: View {
    private let content: Content

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
    }

    var body: some View {
        content
            .padding(16)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(
                RoundedRectangle(cornerRadius: AppTheme.cardCorner)
                    .fill(AppTheme.panel)
                    .overlay(
                        RoundedRectangle(cornerRadius: AppTheme.cardCorner)
                            .strokeBorder(AppTheme.line)
                    )
            )
            .shadow(color: AppTheme.shadow, radius: 10, x: 0, y: 6)
    }
}
