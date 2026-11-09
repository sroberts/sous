import { test } from 'node:test';
import assert from 'node:assert/strict';
import { chunks } from '../src/lib/offline-core.js';

test('chunks splits ids into batches of at most n, in order', () => {
  const ids = Array.from({ length: 250 }, (_, i) => `r${i}`);
  const out = chunks(ids, 100);
  assert.deepEqual(out.map((c) => c.length), [100, 100, 50]);
  assert.deepEqual(out.flat(), ids);
  assert.deepEqual(chunks([], 100), []);
  assert.deepEqual(chunks(['a'], 100), [['a']]);
});
