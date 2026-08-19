---
status: accepted
---

# Livrer les notifications mobiles par un relais officiel minimal

Une instance CairnOps chiffre les notifications pour chaque appareil puis confie leur livraison à un Relais Push officiel extérieur, seul détenteur des capacités éditeur nécessaires à APNs et FCM. Le relais ne reçoit qu'un destinataire opaque, le message chiffré et les métadonnées indispensables, sans accès à l'instance ni à son état. Après la V1, un Bail de présence facultatif lui permettra aussi de livrer un message d'absence préchiffré si l'instance cesse de le renouveler. Le relais reste désactivable, au prix de perdre la garantie de notifications mobiles immédiates puis de détection externe de l'absence, tandis que la synchronisation complète s'effectue toujours directement entre l'appareil et l'instance.
