import Observation
import UIKit
import UserNotifications

@MainActor
@Observable
final class PushNotificationDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
	private(set) var deviceToken: String?
	private(set) var registrationError: String?

	func application(
		_ application: UIApplication,
		didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
	) -> Bool {
		let center = UNUserNotificationCenter.current()
		center.delegate = self
		center.setNotificationCategories([
			UNNotificationCategory(
				identifier: "CAIRNOPS_INCIDENT",
				actions: [],
				intentIdentifiers: [],
				options: []
			),
		])
		return true
	}

	func requestAuthorizationAndRegistration() async {
		let center = UNUserNotificationCenter.current()
		let settings = await center.notificationSettings()
		let allowed: Bool
		switch settings.authorizationStatus {
		case .notDetermined:
			do {
				allowed = try await center.requestAuthorization(options: [.alert, .sound, .badge])
			} catch {
				registrationError = error.localizedDescription
				return
			}
		case .authorized, .provisional, .ephemeral:
			allowed = true
		case .denied:
			allowed = false
		@unknown default:
			allowed = false
		}

		guard allowed else {
			return
		}
		UIApplication.shared.registerForRemoteNotifications()
	}

	func application(
		_ application: UIApplication,
		didRegisterForRemoteNotificationsWithDeviceToken token: Data
	) {
		deviceToken = token.map { String(format: "%02x", $0) }.joined()
		registrationError = nil
	}

	func application(
		_ application: UIApplication,
		didFailToRegisterForRemoteNotificationsWithError error: any Error
	) {
		registrationError = error.localizedDescription
	}

	func userNotificationCenter(
		_ center: UNUserNotificationCenter,
		willPresent notification: UNNotification
	) async -> UNNotificationPresentationOptions {
		[.banner, .list, .sound]
	}
}
