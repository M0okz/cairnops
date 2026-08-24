import SwiftUI

/// Ossature commune des ecrans « a nu ».
///
/// Elle pose le fond, la gouttiere laterale unique et la reserve basse
/// eventuelle.
///
/// La barre de navigation systeme est conservee : la masquer supprimait aussi
/// le geste de retour par balayage, et le titre centre qu'elle affiche marque
/// bien mieux l'ecran courant qu'un titre noye dans le contenu. Seule la Vue
/// d'ensemble la masque, n'ayant ni retour ni parent.
///
/// Le contenu vit dans un `LazyVStack` : les listes de plusieurs centaines de
/// Cibles ne doivent pas etre construites d'un seul tenant.
struct BareScreen<Content: View>: View {
    var bottomInset: Double
    var hidesNavigationBar: Bool
    private let content: Content

    init(
        bottomInset: Double = AppTheme.tabBarScrollInset,
        hidesNavigationBar: Bool = false,
        @ViewBuilder content: () -> Content
    ) {
        self.bottomInset = bottomInset
        self.hidesNavigationBar = hidesNavigationBar
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
        .toolbar(hidesNavigationBar ? .hidden : .automatic, for: .navigationBar)
        .scrollDismissesKeyboard(.interactively)
    }
}
