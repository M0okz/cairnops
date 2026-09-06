// @ts-nocheck -- Node exécute directement les modules TypeScript.
import assert from 'node:assert/strict';
import test from 'node:test';
import { resolvedTheme, solarSchedule, themeMode } from './appearance.ts';

test('keeps existing explicit choices and defaults missing or invalid choices to the system', () => {
  assert.equal(themeMode('light'), 'light');
  assert.equal(themeMode('dark'), 'dark');
  assert.equal(themeMode(null), 'system');
  assert.equal(themeMode('unexpected'), 'system');
  assert.equal(resolvedTheme('system', true, '', new Date()), 'dark');
  assert.equal(resolvedTheme('system', false, '', new Date()), 'light');
});

test('solar mode follows the system until a valid city is chosen', () => {
  for (const city of ['', 'Invalid']) {
    assert.equal(resolvedTheme('solar', false, city, new Date()), 'light');
    assert.equal(resolvedTheme('solar', true, city, new Date()), 'dark');
  }
});

test('switches at the actual sunrise and sunset, including daylight saving dates', () => {
  for (const date of ['2026-03-29T12:00:00Z', '2026-09-06T12:00:00Z', '2026-10-25T12:00:00Z']) {
    for (const city of ['Paris', 'Montréal']) {
      const sun = solarSchedule(city, new Date(date));
      assert.equal(resolvedTheme('solar', true, city, new Date(+sun.sunrise - 1)), 'dark');
      assert.equal(resolvedTheme('solar', true, city, sun.sunrise), 'light');
      assert.equal(resolvedTheme('solar', false, city, new Date(+sun.sunset - 1)), 'light');
      assert.equal(resolvedTheme('solar', false, city, sun.sunset), 'dark');
    }
  }
});

test('handles polar day and night without invalid dates or an inverted appearance', () => {
  assert.equal(resolvedTheme('solar', true, 'Tromsø', new Date('2026-06-21T12:00:00Z')), 'light');
  assert.equal(resolvedTheme('solar', false, 'Tromsø', new Date('2026-12-21T12:00:00Z')), 'dark');
});
