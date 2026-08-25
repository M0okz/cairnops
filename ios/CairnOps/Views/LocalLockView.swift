import SwiftUI

struct LocalLockView: View {
    @Environment(LocalAppLock.self) private var localLock

    var body: some View {
        VStack(spacing: 18) {
            Image(systemName: "lock.shield.fill")
                .font(.largeTitle)
                .foregroundStyle(AppTheme.control)
                .accessibilityHidden(true)

            VStack(spacing: 6) {
                Text("lock.title")
                    .font(AppTheme.detailTitleFont)
                    .foregroundStyle(AppTheme.ink)

                Text("lock.detail")
                    .font(.body)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(AppTheme.inkMuted)
            }

            AsyncButton {
                await localLock.unlock()
            } label: {
                Label("lock.unlock", systemImage: "faceid")
                    .font(AppTheme.fieldValueFont)
                    .foregroundStyle(AppTheme.ground)
                    .padding(.horizontal, 18)
                    .frame(minHeight: 44)
                    .background(
                        Capsule(style: .continuous)
                            .fill(AppTheme.control)
                    )
            }
            .buttonStyle(.plain)

            if let errorMessage = localLock.errorMessage {
                Text(errorMessage)
                    .font(.footnote)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(AppTheme.criticalInk)
            }
        }
        .padding(32)
        .frame(maxWidth: 420)
        .task {
            await localLock.unlock()
        }
        .accessibilityElement(children: .contain)
    }
}
