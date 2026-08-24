import SwiftUI

/// Fond de l'application.
///
/// La refonte « a nu » pose un fond plein et froid, sans halo ni degrade : le
/// relief venait auparavant de deux `RadialGradient` qui n'avaient plus de role
/// une fois les dalles supprimees, et qui coloraient le fond derriere chaque
/// ecran.
struct AppBackdrop: View {
    var body: some View {
        AppTheme.ground
            .ignoresSafeArea()
            .allowsHitTesting(false)
    }
}
