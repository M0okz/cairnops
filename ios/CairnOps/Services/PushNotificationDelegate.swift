import Observation
import UIKit
import UserNotifications

@MainActor
@Observable
final class PushNotificationDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
	private(set) var deviceToken: String?
	private(set) var registrationError: String?
	private(set) var authorizationStatus: UNAuthorizationStatus = .notDetermined
	private(set) var criticalAlertSetting: UNNotificationSetting = .notSupported
	private(set) var criticalAlertError: String?

	var notificationsEnabled: Bool {
		switch authorizationStatus {
		case .authorized, .provisional, .ephemeral:
			true
		case .notDetermined, .denied:
			false
		@unknown default:
			false
		}
	}

	var criticalAlertsAvailable: Bool {
		authorizationStatus != .denied && criticalAlertSetting != .notSupported
	}

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
		update(from: settings)
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
		await refreshNotificationSettings()
		UIApplication.shared.registerForRemoteNotifications()
	}

	func refreshNotificationSettings() async {
		update(from: await UNUserNotificationCenter.current().notificationSettings())
	}

	func requestCriticalAlertAuthorization() async -> Bool {
		guard criticalAlertsAvailable else {
			criticalAlertError = "Cette version n’est pas autorisée par Apple à émettre des alertes critiques."
			return false
		}
		do {
			_ = try await UNUserNotificationCenter.current().requestAuthorization(
				options: [.alert, .sound, .badge, .criticalAlert]
			)
			criticalAlertError = nil
		} catch {
			criticalAlertError = error.localizedDescription
		}
		await refreshNotificationSettings()
		return criticalAlertSetting == .enabled
	}

	func cancelAllIncidentReminders() async {
		let center = UNUserNotificationCenter.current()
		let identifiers = await center.pendingNotificationRequests()
			.map(\.identifier)
			.filter(NotificationReminderIdentifier.isReminder)
		guard !identifiers.isEmpty else {
			return
		}
		center.removePendingNotificationRequests(withIdentifiers: identifiers)
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

	private func update(from settings: UNNotificationSettings) {
		authorizationStatus = settings.authorizationStatus
		criticalAlertSetting = settings.criticalAlertSetting
	}
}
