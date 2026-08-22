import UserNotifications

final class NotificationService: UNNotificationServiceExtension {
	private var contentHandler: ((UNNotificationContent) -> Void)?
	private var bestAttemptContent: UNMutableNotificationContent?

	override func didReceive(
		_ request: UNNotificationRequest,
		withContentHandler contentHandler: @escaping (UNNotificationContent) -> Void
	) {
		self.contentHandler = contentHandler
		guard let content = request.content.mutableCopy() as? UNMutableNotificationContent else {
			contentHandler(request.content)
			return
		}
		bestAttemptContent = content

		do {
			guard let cairnops = request.content.userInfo["cairnops"] as? [String: Any],
				let envelopeValue = cairnops["envelope"],
				JSONSerialization.isValidJSONObject(envelopeValue),
				let privateKey = try SharedDeviceCredentials.encryptionPrivateKey() else {
				finish(with: content)
				return
			}
			let envelopeData = try JSONSerialization.data(withJSONObject: envelopeValue)
			let envelope = try JSONDecoder().decode(PushEnvelope.self, from: envelopeData)
			let message = try PushEnvelopeDecryptor.decrypt(envelope, privateKey: privateKey)
			content.title = message.presentation.title
			content.body = message.presentation.body
			content.categoryIdentifier = "CAIRNOPS_INCIDENT"
			var userInfo = content.userInfo
			userInfo["cairnops_incident_id"] = message.incidentID
			userInfo["cairnops_instance_url"] = message.instanceURL
			userInfo["cairnops_event_kind"] = message.eventKind
			userInfo["cairnops_severity"] = message.severity
			userInfo.removeValue(forKey: "cairnops")
			content.userInfo = userInfo

			let preferences = (try? NotificationPreferencesStore().load()) ?? NotificationPreferences()
			applySound(
				preferences.deliverySound(eventKind: message.eventKind, severity: message.severity),
				to: content
			)
			synchronizeReminders(for: message, content: content, preferences: preferences)
			finish(with: content)
		} catch {
			// Le texte APNs générique reste affichable sans jamais exposer le
			// contenu opérationnel si la clé ou l’enveloppe est invalide.
			finish(with: content)
		}
	}

	override func serviceExtensionTimeWillExpire() {
		guard let bestAttemptContent else {
			return
		}
		finish(with: bestAttemptContent)
	}

	private func synchronizeReminders(
		for message: PushMessage,
		content: UNMutableNotificationContent,
		preferences: NotificationPreferences
	) {
		let center = UNUserNotificationCenter.current()
		center.removePendingNotificationRequests(
			withIdentifiers: NotificationReminderIdentifier.identifiers(incidentID: message.incidentID)
		)
		guard message.eventKind == "firing" else {
			return
		}

		for stage in preferences.repeatPolicy.stages {
			guard let reminderContent = content.mutableCopy() as? UNMutableNotificationContent else {
				continue
			}
			var userInfo = reminderContent.userInfo
			userInfo["cairnops_is_reminder"] = true
			userInfo["cairnops_reminder_stage"] = stage.id
			reminderContent.userInfo = userInfo
			let trigger = UNTimeIntervalNotificationTrigger(
				timeInterval: stage.delay,
				repeats: stage.repeats
			)
			let request = UNNotificationRequest(
				identifier: NotificationReminderIdentifier.identifier(
					incidentID: message.incidentID,
					stageID: stage.id
				),
				content: reminderContent,
				trigger: trigger
			)
			center.add(request)
		}
	}

	private func applySound(
		_ deliverySound: NotificationDeliverySound,
		to content: UNMutableNotificationContent
	) {
		switch deliverySound {
		case .system:
			content.sound = .default
		case .silent:
			content.sound = nil
		case .critical:
			content.sound = .defaultCritical
			content.interruptionLevel = .critical
		}
	}

	private func finish(with content: UNNotificationContent) {
		guard let handler = contentHandler else {
			return
		}
		contentHandler = nil
		bestAttemptContent = nil
		handler(content)
	}
}
