// Fixture de conception uniquement. Aucun appel à une instance CairnOps.
export const names = [
  'API publique', 'Sauvegarde nocturne', 'Homybudget', 'Passerelle réseau',
  'Stockage principal', 'Authentification', 'GitLab', 'DNS interne',
  ...Array.from({ length: 40 }, (_, i) => `Service ${String(i + 9).padStart(2, '0')}`)
];
export const targets = names.map((name, i) => ({
  id: i, name,
  category: i === 1 ? 'Tâche planifiée' : i === 3 ? 'Réseau' : i === 4 ? 'Stockage' : 'Service',
  state: i === 0 ? 'Dégradée' : i === 1 ? 'Indisponible' : 'Opérationnelle',
  tone: i === 0 ? 'warn' : i === 1 ? 'crit' : 'ok',
  latency: i === 1 ? null : i === 0 ? 184 : 18 + (i * 7) % 59,
  availability: i === 0 ? '100' : i === 1 ? '97,22' : '100',
  coverage: '100',
  source: i % 3 === 0 ? 'Uptime Kuma' : i % 3 === 1 ? 'CairnOps' : 'Zabbix',
  sourceCount: i < 16 ? 2 : 1
}));
export type Target = (typeof targets)[number];
export const incidents = [
  { id: 1, target: 'Sauvegarde nocturne', title: 'Heartbeat absent', severity: 'Majeur', tone: 'crit', since: 'Depuis 13:52', duration: '40 min', source: 'CairnOps', proof: 'Aucun signal reçu depuis 13:52. La tolérance de 30 minutes est dépassée.', assurance: 'Signalée' },
  { id: 0, target: 'API publique', title: 'Temps de réponse élevé', severity: 'Avertissement', tone: 'warn', since: 'Depuis 14:18', duration: '14 min', source: 'Uptime Kuma', proof: 'Temps de réponse de 184 ms, au-dessus du seuil de 150 ms défini dans Uptime Kuma.', assurance: 'Signalée' }
];
export const chartSeries = (range: string, seed = 0) => {
  const n = range === '7j' ? 56 : range === '1h' ? 36 : 64;
  return Array.from({ length: n }, (_, i) => {
    const base = 42 + 9 * Math.sin(i * 0.39 + seed) + 6 * Math.cos(i * 0.91);
    const burst = Math.max(0, 1 - Math.abs(i - n * 0.62) / (n * 0.07)) * 98;
    const current = i > n * 0.91 ? (i - n * 0.91) * 23 : 0;
    return i === n-1 ? 184 : Math.round(base + burst + current);
  });
};
export const cities = [
  { name: 'Paris', lat: 48.8566, lon: 2.3522 },
  { name: 'Lyon', lat: 45.764, lon: 4.8357 },
  { name: 'Marseille', lat: 43.2965, lon: 5.3698 },
  { name: 'Bordeaux', lat: 44.8378, lon: -0.5792 },
  { name: 'Lille', lat: 50.6292, lon: 3.0573 },
  { name: 'Bruxelles', lat: 50.8503, lon: 4.3517 },
  { name: 'Genève', lat: 46.2044, lon: 6.1432 },
  { name: 'Montréal', lat: 45.5019, lon: -73.5674 },
  { name: 'Tromsø', lat: 69.6492, lon: 18.9553 }
];
