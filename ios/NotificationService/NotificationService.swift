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
				contentHandler(content)
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
			content.userInfo = userInfo
			contentHandler(content)
		} catch {
			// Le texte APNs générique reste affichable sans jamais exposer le
			// contenu opérationnel si la clé ou l'enveloppe est invalide.
			contentHandler(content)
		}
	}

	override func serviceExtensionTimeWillExpire() {
		guard let contentHandler, let bestAttemptContent else {
			return
		}
		contentHandler(bestAttemptContent)
	}
}
