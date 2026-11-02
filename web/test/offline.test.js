import { test } from 'node:test';
import assert from 'node:assert/strict';
import { splitFlags, withFlags, withFlag, staleIds, removedIds, backoff } from '../src/lib/offline-core.js';

test('splitFlags stores records without the person’s flags', () => {
  const { items, flags } = splitFlags([
    { id: 'a', title: 'Dal', mine: { favorite: true, want_to_cook: false } },
    { id: 'b', title: 'Soup' },
  ]);
  assert.deepEqual(items, [
    { id: 'a', title: 'Dal' },
    { id: 'b', title: 'Soup' },
  ]);
  assert.deepEqual(flags, { a: { favorite: true, want_to_cook: false } });
});

test('withFlags puts a person’s flags back, and none for the rest', () => {
  const items = [{ id: 'a' }, { id: 'b' }];
  const got = withFlags(items, { a: { favorite: false, want_to_cook: true } });
  assert.deepEqual(got, [
    { id: 'a', mine: { favorite: false, want_to_cook: true } },
    { id: 'b', mine: { favorite: false, want_to_cook: false } },
  ]);
  assert.deepEqual(withFlags(items).map((r) => r.mine.favorite), [false, false]);
  assert.deepEqual(withFlag({ id: 'b' }, {}).mine, { favorite: false, want_to_cook: false });
});

test('staleIds lists recipes missing here or at another version', () => {
  const summaries = [
    { id: 'a', version: 3 },
    { id: 'b', version: 1 },
    { id: 'c', version: 2 },
  ];
  const stored = new Map([
    ['a', 3],
    ['b', 2],
  ]);
  assert.deepEqual(staleIds(summaries, stored), ['b', 'c']);
  assert.deepEqual(staleIds([], stored), []);
});

test('removedIds lists stored recipes no longer in the library', () => {
  assert.deepEqual(removedIds(['a', 'b', 'x'], [{ id: 'a' }, { id: 'b' }, { id: 'c' }]), ['x']);
});

test('backoff honours Retry-After and otherwise doubles, within a minute', () => {
  assert.equal(backoff('5', 0), 5000);
  assert.equal(backoff('0', 3), 0);
  assert.equal(backoff('600', 0), 60000);
  assert.equal(backoff(null, 0), 1000);
  assert.equal(backoff(null, 3), 8000);
  assert.equal(backoff('Wed, 21 Oct 2026 07:28:00 GMT', 1), 2000);
  assert.equal(backoff(null, 20), 60000);
});
