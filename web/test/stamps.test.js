import { test } from 'node:test';
import assert from 'node:assert/strict';
import { staleIds, stampOf } from '../src/lib/offline-core.js';

test('staleIds by stamp catches a version reused after a restore', () => {
  const summaries = [
    { id: 'a', version: 3, updated_at: '2026-10-01T10:00:00Z' },
    { id: 'b', version: 2, updated_at: '2026-10-02T09:00:00Z' },
    { id: 'c', version: 5, updated_at: '2026-10-03T08:00:00Z' },
  ];
  const stored = new Map([
    ['a', stampOf({ version: 3, updated_at: '2026-10-01T10:00:00Z' })],
    // Same version, but a different edit: a backup was restored, then edited.
    ['b', stampOf({ version: 2, updated_at: '2026-09-30T12:00:00Z' })],
  ]);
  assert.deepEqual(staleIds(summaries, stored, stampOf), ['b', 'c']);
});

test('after a server upgrade every recipe is stale', () => {
  const summaries = [
    { id: 'a', version: 1, updated_at: 't1' },
    { id: 'b', version: 1, updated_at: 't2' },
  ];
  assert.deepEqual(staleIds(summaries, new Map(), stampOf), ['a', 'b']);
});

test('a stamp is the version and the time of the edit', () => {
  assert.equal(stampOf({ version: 4, updated_at: '2026-10-01T10:00:00Z' }), '4:2026-10-01T10:00:00Z');
  assert.notEqual(stampOf({ version: 4, updated_at: 'x' }), stampOf({ version: 4, updated_at: 'y' }));
});
