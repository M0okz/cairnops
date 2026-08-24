import SwiftUI

/// Jetons de la refonte « Homeblack ».
///
/// La direction visuelle impose un jeu de jetons unique : toute couleur, tout
/// espacement et toute forme viennent d'ici. Une valeur litterale dans une vue
/// est un defaut, pas un raccourci.
///
/// La maquette est composee en Archivo ; l'application reste en police systeme
/// conformement a `docs/DESIGN-DIRECTION.md` et a l'ADR 0020. Les graisses et
/// les chasses negatives reproduisent la densite de la maquette, et les styles
/// semantiques conservent le suivi de Dynamic Type.
enum AppTheme {

    // MARK: - Fond et encre

    /// Fond de page. Les deux themes echangent les roles sans changer la grille.
    static let ground = Color(light: 0xF6F4F1, dark: 0x0E0C10)

    /// Encre principale : ni noir ni blanc purs, comme la maquette.
    static let ink = Color(light: 0x201E1D, dark: 0xF5F3F2)

    /// Libelles de section en petites capitales.
    static let inkFaint = Color(light: 0x201E1D, dark: 0xF5F3F2, lightAlpha: 0.58, darkAlpha: 0.38)

    /// Metadonnees : durees, fraicheur, compteurs secondaires.
    static let inkMuted = Color(light: 0x201E1D, dark: 0xF5F3F2, lightAlpha: 0.62, darkAlpha: 0.45)

    /// Metadonnee mise en avant, comme le nom de Cible sur une ligne d'Incident.
    static let inkStrong = Color(light: 0x201E1D, dark: 0xF5F3F2, lightAlpha: 0.72, darkAlpha: 0.70)

    /// Filet de 1 px qui remplace les contours de dalle.
    static let hairline = Color(light: 0x201E1D, dark: 0xFFFFFF, lightAlpha: 0.12, darkAlpha: 0.07)

    // MARK: - Cuivre

    /// Accent unique : action principale et route courante, jamais decoratif.
    static let accent = Color(light: 0xA9631F, dark: 0xDDA15F)

    /// Cuivre plein : soulignement d'onglet et remplissages.
    static let accentSolid = Color(hex: 0xC07434)

    // MARK: - Etat de sante et Gravite
    //
    // Deux registres distincts : une teinte de remplissage, assez saturee pour
    // une pastille de 7 px, et une encre plus sombre en clair pour que le meme
    // etat reste lisible en texte.

    static let critical = Color(light: 0xC07434, dark: 0xC8813C)
    static let criticalInk = Color(light: 0x96551F, dark: 0xC8813C)

    /// Teinte des tres grands nombres. Le cuivre sature reste lisible a 11 px
    /// mais vibre a 56 px : la maquette y passe a une creme en sombre et a un
    /// cuivre profond en clair.
    static let criticalDisplay = Color(light: 0x96551F, dark: 0xF2DCBD)

    /// En sombre, la maquette oppose un avertissement pale a un critique
    /// sature. En clair elle rapproche les deux au point qu'une barre de flotte
    /// ne les distingue plus : le remplissage clair reprend donc le meme ecart
    /// pale/sature, la direction visuelle exigeant deux themes de qualite
    /// equivalente. L'encre de texte, elle, reste inchangee.
    static let warning = Color(light: 0xDDA96B, dark: 0xE7BD8E)
    static let warningInk = Color(light: 0x7A5A1E, dark: 0xE7BD8E)

    static let ok = Color(hex: 0x5EC99B)
    static let okInk = Color(light: 0x1B7C56, dark: 0x5EC99B)

    /// Etat neutralise : Incident acquitte, preuve invalidee.
    static let neutral = Color(light: 0x201E1D, dark: 0xF5F3F2, lightAlpha: 0.50, darkAlpha: 0.50)

    /// Le bleu signale l'information et la maintenance, jamais la Gravite.
    static let info = Color(light: 0x4B8AC9, dark: 0x6AA8E0)

    // MARK: - Verre

    /// Barre d'onglets et barre d'actions flottantes.
    static let glassFill = Color(light: 0xFFFFFF, dark: 0x1E1A20, lightAlpha: 0.72, darkAlpha: 0.62)
    static let glassStroke = Color(light: 0x201E1D, dark: 0xFFFFFF, lightAlpha: 0.10, darkAlpha: 0.14)
    static let glassShadow = Color(light: 0x201E1D, dark: 0x000000, lightAlpha: 0.14, darkAlpha: 0.45)

    // MARK: - Metriques

    /// Gouttiere laterale de tous les ecrans.
    static let screenPadding = 20.0

    /// Retrait lateral de la barre flottante.
    static let barInset = 14.0

    /// Respiration sous le contenu.
    ///
    /// La barre d'onglets systeme reserve deja sa propre zone sure : y ajouter
    /// sa hauteur rouvrirait la longue zone morte de fin de defilement deja
    /// corrigee une fois.
    static let tabBarScrollInset = 24.0

    /// Espace reserve sous le contenu pour degager la barre d'actions.
    static let actionBarScrollInset = 72.0

    /// Retrait supplementaire sous la zone sure haute. La maquette pose le
    /// contenu a 68 px du bord ; la zone sure en couvre l'essentiel.
    static let headerTopInset = 8.0

    static let sectionSpacing = 16.0
    static let actionCorner = 21.0

    // MARK: - Typographie
    //
    // Les styles semantiques suivent Dynamic Type. Les tailles fixes qui
    // faisaient se chevaucher le contenu aux grandes tailles de texte ont ete
    // corrigees une fois ; on ne les reintroduit pas.

    /// Titre d'ecran : 20 px, graisse maximale, chasse serree.
    static let pageTitleFont = Font.title3.weight(.heavy)

    /// Titre de detail d'Incident. La barre de navigation portant deja le nom
    /// de l'ecran, ce titre nomme le sujet sans avoir a crier.
    static let detailTitleFont = Font.title2.weight(.bold)

    /// Libelle de section en petites capitales.
    static let sectionLabelFont = Font.caption2.weight(.bold)

    /// Cle d'une paire cle/valeur, meme registre que le libelle de section.
    static let fieldLabelFont = Font.caption2.weight(.bold)

    /// Ligne principale d'une liste.
    static let rowTitleFont = Font.subheadline.weight(.semibold)

    /// Ligne principale mise en avant, comme l'Incident le plus recent.
    static let leadRowTitleFont = Font.callout.weight(.bold)

    /// Metadonnee d'une ligne.
    static let metaFont = Font.caption

    /// Valeur d'une paire cle/valeur.
    static let fieldValueFont = Font.subheadline.weight(.semibold)

    /// Onglet de filtre actif et inactif.
    static let filterFont = Font.subheadline.weight(.bold)
    static let filterIdleFont = Font.subheadline.weight(.medium)


    /// Chasse negative des grands titres, en points.
    static let titleTracking = -0.5
    static let displayTracking = -1.2

    /// Chasse positive des petites capitales.
    static let labelTracking = 1.1

    // MARK: - Correspondances metier

    static func severityColor(_ severity: IncidentSeverity) -> Color {
        switch severity {
        case .information:
            info
        case .warning:
            warning
        case .major, .critical:
            critical
        }
    }

    /// Encre du meme etat, pour un libelle plutot qu'une pastille.
    static func severityInk(_ severity: IncidentSeverity) -> Color {
        switch severity {
        case .information:
            info
        case .warning:
            warningInk
        case .major, .critical:
            criticalInk
        }
    }

    /// Libelle court affiche en fin de ligne, comme « CRIT » ou « AVERT ».
    static func severityShortLabel(_ severity: IncidentSeverity) -> String {
        switch severity {
        case .information:
            "INFO"
        case .warning:
            "AVERT"
        case .major:
            "MAJEUR"
        case .critical:
            "CRIT"
        }
    }

    static func severitySymbol(_ severity: IncidentSeverity) -> String {
        switch severity {
        case .information:
            "info.circle.fill"
        case .warning:
            "exclamationmark.triangle.fill"
        case .major:
            "bolt.horizontal.circle.fill"
        case .critical:
            "xmark.octagon.fill"
        }
    }

    static func targetHealthColor(_ health: AppSnapshot.TargetHealth) -> Color {
        switch health {
        case .ok:
            ok
        case .degraded:
            warning
        case .maintenance:
            info
        case .down:
            critical
        case .unknown:
            neutral
        }
    }

    static func targetHealthInk(_ health: AppSnapshot.TargetHealth) -> Color {
        switch health {
        case .ok:
            okInk
        case .degraded:
            warningInk
        case .maintenance:
            info
        case .down:
            criticalInk
        case .unknown:
            neutral
        }
    }

    static func targetHealthLabel(_ health: AppSnapshot.TargetHealth) -> String {
        switch health {
        case .ok:
            "Opérationnelle"
        case .degraded:
            "Dégradée"
        case .down:
            "Indisponible"
        case .maintenance:
            "Maintenance"
        case .unknown:
            "Inconnue"
        }
    }

    static func targetHealthShortLabel(_ health: AppSnapshot.TargetHealth) -> String {
        switch health {
        case .ok:
            "OK"
        case .degraded:
            "DÉGRADÉE"
        case .down:
            "HS"
        case .maintenance:
            "MAINT."
        case .unknown:
            "INCONNUE"
        }
    }

    static func targetHealthSymbol(_ health: AppSnapshot.TargetHealth) -> String {
        switch health {
        case .ok:
            "checkmark.circle.fill"
        case .degraded:
            "exclamationmark.triangle.fill"
        case .down:
            "xmark.octagon.fill"
        case .maintenance:
            "wrench.and.screwdriver.fill"
        case .unknown:
            "questionmark.circle.fill"
        }
    }

    static func globalStatusColor(_ status: AppSnapshot.GlobalStatus) -> Color {
        switch status {
        case .allOperational:
            ok
        case .ongoingIncident:
            critical
        case .degradedServices:
            warning
        case .incompleteMonitoring:
            neutral
        case .notConfigured:
            accent
        }
    }

    static func globalStatusLabel(_ status: AppSnapshot.GlobalStatus) -> String {
        switch status {
        case .allOperational:
            "Tout est opérationnel"
        case .ongoingIncident:
            "Incident en cours"
        case .degradedServices:
            "Services dégradés"
        case .incompleteMonitoring:
            "Supervision incomplète"
        case .notConfigured:
            "Supervision non configurée"
        }
    }

    static func globalStatusSymbol(_ status: AppSnapshot.GlobalStatus) -> String {
        switch status {
        case .allOperational:
            "checkmark.circle.fill"
        case .ongoingIncident:
            "xmark.octagon.fill"
        case .degradedServices:
            "exclamationmark.triangle.fill"
        case .incompleteMonitoring:
            "questionmark.circle.fill"
        case .notConfigured:
            "slider.horizontal.3"
        }
    }
}

extension Color {

    /// Couleur dynamique definie par ses deux valeurs de theme.
    ///
    /// Les jetons vivent ainsi dans un seul fichier lisible plutot que dans une
    /// vingtaine de dossiers du catalogue d'assets, et suivent la bascule de
    /// theme sans reconstruire la vue.
    init(light: UInt32, dark: UInt32, lightAlpha: Double = 1, darkAlpha: Double = 1) {
        self.init(uiColor: UIColor { traits in
            traits.userInterfaceStyle == .dark
                ? UIColor(hex: dark, alpha: darkAlpha)
                : UIColor(hex: light, alpha: lightAlpha)
        })
    }

    /// Couleur identique dans les deux themes.
    init(hex: UInt32, alpha: Double = 1) {
        self.init(uiColor: UIColor(hex: hex, alpha: alpha))
    }
}

extension UIColor {
    fileprivate convenience init(hex: UInt32, alpha: Double) {
        self.init(
            red: CGFloat((hex >> 16) & 0xFF) / 255,
            green: CGFloat((hex >> 8) & 0xFF) / 255,
            blue: CGFloat(hex & 0xFF) / 255,
            alpha: alpha
        )
    }
}
