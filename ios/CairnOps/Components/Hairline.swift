import SwiftUI

/// Filet de separation d'un point.
///
/// La maquette « a nu » n'a ni contour de dalle ni `Divider` systeme : les
/// sections et les lignes sont separees par ce seul filet, pleine largeur.
struct Hairline: View {
    var body: some View {
        Rectangle()
            .fill(AppTheme.hairline)
            .frame(height: 1)
            .accessibilityHidden(true)
    }
}

extension View {

    /// Pose le filet au-dessus du contenu, sans consommer de hauteur de ligne.
    func hairlineTop() -> some View {
        overlay(alignment: .top) {
            Hairline()
        }
    }
}
