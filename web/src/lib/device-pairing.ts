import type { DevicePairingStatus } from './api';

const liveStatuses = new Set<DevicePairingStatus>([
  'awaiting_scan',
  'awaiting_confirmation',
  'confirmed'
]);

const cancellableStatuses = new Set<DevicePairingStatus>([
  'awaiting_scan',
  'awaiting_confirmation'
]);

export function pairingSecondsRemaining(expiresAt: string, now: Date = new Date()): number {
  const expires = new Date(expiresAt).getTime();
  if (!Number.isFinite(expires)) return 0;
  return Math.max(0, Math.ceil((expires - now.getTime()) / 1000));
}

export function pairingShouldPoll(status: DevicePairingStatus): boolean {
  return liveStatuses.has(status);
}

export function pairingCanCancel(status: DevicePairingStatus): boolean {
  return cancellableStatuses.has(status);
}

export function pairingStep(status: DevicePairingStatus): 1 | 2 | 3 {
  if (status === 'awaiting_scan') return 1;
  if (status === 'awaiting_confirmation') return 2;
  return 3;
}

export async function pairingQRCode(payload: string): Promise<string> {
  /* Le moteur QR ne charge qu'au geste d'association, pas à chaque ouverture
   * des Réglages : son code dépasse à lui seul celui de cette section. */
  const { default: QRCode } = await import('qrcode');
  return QRCode.toDataURL(payload, {
    errorCorrectionLevel: 'M',
    margin: 2,
    width: 512
  });
}
