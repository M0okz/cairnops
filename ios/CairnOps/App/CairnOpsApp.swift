import SwiftUI

@main
struct CairnOpsApp: App {
	@UIApplicationDelegateAdaptor(PushNotificationDelegate.self) private var pushNotifications

	var body: some Scene {
		WindowGroup {
			AppRootView(pushNotifications: pushNotifications)
		}
	}
}
