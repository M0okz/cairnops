// @ts-nocheck -- exécuté directement par Node, hors du typage navigateur Svelte.
import assert from 'node:assert/strict';
import test from 'node:test';

import { absorbObservedVersion } from './version-state.ts';

test('adopts the first observed version as the running one', () => {
  assert.deepEqual(
    absorbObservedVersion(
      {
        currentVersion: 'dev',
        currentKnown: false,
        availableVersion: ''
      },
      '0.1.15'
    ),
    {
      currentVersion: '0.1.15',
      currentKnown: true,
      availableVersion: ''
    }
  );
});

test('keeps the running version and exposes a newer observed version', () => {
  assert.deepEqual(
    absorbObservedVersion(
      {
        currentVersion: '0.1.15',
        currentKnown: true,
        availableVersion: ''
      },
      '0.1.16'
    ),
    {
      currentVersion: '0.1.15',
      currentKnown: true,
      availableVersion: '0.1.16'
    }
  );
});

test('clears the update banner when the observed version matches again', () => {
  assert.deepEqual(
    absorbObservedVersion(
      {
        currentVersion: '0.1.15',
        currentKnown: true,
        availableVersion: '0.1.16'
      },
      '0.1.15'
    ),
    {
      currentVersion: '0.1.15',
      currentKnown: true,
      availableVersion: ''
    }
  );
});

test('ignores an empty observed version instead of dropping the known state', () => {
  assert.deepEqual(
    absorbObservedVersion(
      {
        currentVersion: '0.1.15',
        currentKnown: true,
        availableVersion: '0.1.16'
      },
      ''
    ),
    {
      currentVersion: '0.1.15',
      currentKnown: true,
      availableVersion: '0.1.16'
    }
  );
});
