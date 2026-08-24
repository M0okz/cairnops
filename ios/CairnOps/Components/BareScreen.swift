import SwiftUI

/// Ossature commune des ecrans « a nu ».
///
/// Elle pose le fond, la gouttiere laterale unique et la reserve basse qui
/// degage la barre flottante, puis masque la barre de navigation systeme : les
/// titres et les retours sont dessines dans le contenu.
///
/// Le contenu vit dans un `LazyVStack` : les listes de plusieurs centaines de
/// Cibles ne doivent pas etre construites d'un seul tenant.
struct BareScreen<Content: View>: View {
    var bottomInset = AppTheme.tabBarScrollInset
    private let content: Content

    init(
        bottomInset: Double = AppTheme.tabBarScrollInset,
        @ViewBuilder content: () -> Content
    ) {
        self.bottomInset = bottomInset
        self.content = content()
    }

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 0) {
                content
            }
            .padding(.horizontal, AppTheme.screenPadding)
            .padding(.top, AppTheme.headerTopInset)
            .padding(.bottom, bottomInset)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .background(AppTheme.ground.ignoresSafeArea())
        .toolbar(.hidden, for: .navigationBar)
        .scrollDismissesKeyboard(.interactively)
    }
}

/// Espace vertical entre deux sections, avec son filet de separation.
struct SectionBreak: View {
    var spacing = 16.0
    var showsRule = true

    var body: some View {
        VStack(spacing: 0) {
            Color.clear.frame(height: spacing)
            if showsRule {
                Hairline()
            }
            Color.clear.frame(height: spacing)
        }
    }
}
