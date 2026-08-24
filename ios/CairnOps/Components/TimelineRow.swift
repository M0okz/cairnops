import SwiftUI

/// Entree de chronologie alignee sur un filet vertical.
///
/// La direction visuelle decrit ainsi la chronologie d'un Incident : « un filet
/// vertical discret le long duquel les entrees s'alignent ». Le filet relie les
/// entrees entre elles, le noeud marque l'instant, et l'heure reste a droite
/// pour que les intitules s'alignent sur une seule colonne.
struct TimelineRow<Content: View>: View {
    let systemImage: String
    let tone: Color
    var time: String?
    var isFirst = false
    var isLast = false
    var isMuted = false
    private let content: Content

    init(
        systemImage: String,
        tone: Color,
        time: String? = nil,
        isFirst: Bool = false,
        isLast: Bool = false,
        isMuted: Bool = false,
        @ViewBuilder content: () -> Content
    ) {
        self.systemImage = systemImage
        self.tone = tone
        self.time = time
        self.isFirst = isFirst
        self.isLast = isLast
        self.isMuted = isMuted
        self.content = content()
    }

    private let nodeSize = 22.0
    private let contentInset = 10.0

    private var nodeCenter: Double {
        contentInset + nodeSize / 2
    }

    var body: some View {
        // Le remplissage vertical vit dans les colonnes de contenu, jamais sur
        // la rangee : le filet doit courir d'un bord a l'autre de la ligne pour
        // rejoindre l'entree suivante.
        HStack(alignment: .top, spacing: 12) {
            rail

            VStack(alignment: .leading, spacing: 3) {
                content
            }
            .padding(.vertical, contentInset)
            .frame(maxWidth: .infinity, alignment: .leading)

            if let time, !time.isEmpty {
                Text(time)
                    .font(.caption2)
                    .monospacedDigit()
                    .foregroundStyle(AppTheme.inkMuted)
                    .padding(.top, contentInset + 2)
            }
        }
        .opacity(isMuted ? 0.55 : 1)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }

    private var rail: some View {
        ZStack(alignment: .top) {
            GeometryReader { proxy in
                Path { path in
                    let x = nodeSize / 2
                    // La premiere entree n'a rien au-dessus d'elle, la derniere
                    // rien en dessous : le filet s'arrete au noeud plutot que
                    // de pendre dans le vide.
                    let top = isFirst ? nodeCenter : 0
                    let bottom = isLast ? nodeCenter : proxy.size.height
                    guard bottom > top else {
                        return
                    }
                    path.move(to: CGPoint(x: x, y: top))
                    path.addLine(to: CGPoint(x: x, y: bottom))
                }
                .stroke(AppTheme.hairline, lineWidth: 1)
            }

            node
                .padding(.top, contentInset)
        }
        .frame(width: nodeSize)
    }

    private var node: some View {
        Image(systemName: systemImage)
            .font(.system(size: 10, weight: .bold))
            .foregroundStyle(tone)
            .frame(width: nodeSize, height: nodeSize)
            .background(Circle().fill(tone.opacity(0.16)))
            // Fond opaque : sans lui, le filet traverserait le noeud.
            .background(Circle().fill(AppTheme.ground))
            .accessibilityHidden(true)
    }
}
