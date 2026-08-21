import SwiftUI

struct AsyncButton<Label: View>: View {
    let role: ButtonRole?
    let action: @Sendable () async -> Void
    private let label: () -> Label

    @State private var task: Task<Void, Never>?
    @State private var isRunning = false

    init(
        role: ButtonRole? = nil,
        action: @escaping @Sendable () async -> Void,
        @ViewBuilder label: @escaping () -> Label
    ) {
        self.role = role
        self.action = action
        self.label = label
    }

    var body: some View {
        Button(role: role) {
            task?.cancel()
            isRunning = true
            task = Task {
                await action()
                await MainActor.run {
                    isRunning = false
                    task = nil
                }
            }
        } label: {
            ZStack {
                label()
                    .opacity(isRunning ? 0 : 1)
                if isRunning {
                    ProgressView()
                }
            }
            .frame(minHeight: 24)
        }
        .disabled(isRunning)
        .onDisappear {
            task?.cancel()
        }
    }
}
