// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';

import { formatCalendarDay } from './calendar-day.ts';

test('calendarDay preserves the UTC day supplied by the server', () => {
  assert.equal(formatCalendarDay('2026-08-20T00:00:00Z', 'fr-FR'), 'jeudi 20 août');
});
