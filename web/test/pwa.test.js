import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { buildId, precacheList, stampPage, fillWorker } from '../build/pwa.js';

const template = readFileSync(new URL('../sw/sw.js', import.meta.url), 'utf8');

test('buildId is 12 hex, ignores order, and changes with any file', () => {
  const files = [
    ['assets/a.js', 'one'],
    ['index.html', 'two'],
  ];
  const id = buildId(files);
  assert.match(id, /^[0-9a-f]{12}$/);
  assert.equal(buildId([...files].reverse()), id);
  assert.notEqual(buildId([['assets/a.js', 'one!'], files[1]]), id);
  assert.notEqual(buildId([['assets/b.js', 'one'], files[1]]), id);
});

test('precacheList is the page, then every other file, without index.html', () => {
  const list = precacheList(['index.html', 'assets/index-1.js', 'assets/index-2.css'], ['manifest.webmanifest', 'icons/icon-192.png']);
  assert.deepEqual(list, ['/', '/assets/index-1.js', '/assets/index-2.css', '/icons/icon-192.png', '/manifest.webmanifest']);
  assert.deepEqual(precacheList(['index.html'], []), ['/']);
});

test('stampPage and fillWorker write the same build into both', () => {
  const page = stampPage('<meta name="sous-build" content="%SOUS_BUILD%" />', 'abcdef012345');
  assert.equal(page, '<meta name="sous-build" content="abcdef012345" />');
  const sw = fillWorker(template, 'abcdef012345', ['/', '/assets/a.js']);
  assert.match(sw, /const BUILD = 'abcdef012345';/);
  // The Go test reads PRECACHE as JSON (internal/ui/pwa_test.go).
  const list = sw.match(/const PRECACHE = (\[.*?\]);/s);
  assert.deepEqual(JSON.parse(list[1]), ['/', '/assets/a.js']);
  assert.ok(!sw.includes('__SOUS_'));
});

// The worker is a plain script; run it with a stub self to reach its helpers.
function worker() {
  const listeners = {};
  const self = { addEventListener: (type, fn) => (listeners[type] = fn), location: { origin: 'https://sous.test' } };
  const context = vm.createContext({ self, URL, Response, Request, Headers, setTimeout, clearTimeout });
  vm.runInContext(fillWorker(template, 'abcdef012345', ['/']), context);
  return { context, listeners };
}

test('the worker listens for install, activate, fetch and message', () => {
  const { listeners } = worker();
  assert.deepEqual(Object.keys(listeners).sort(), ['activate', 'fetch', 'install', 'message']);
});

test('evictions drops the oldest photos until the store fits its budget', () => {
  const { context } = worker();
  const entries = [
    { key: 'old', bytes: 50 },
    { key: 'mid', bytes: 30 },
    { key: 'new', bytes: 40 },
  ];
  assert.deepEqual([...context.evictions(entries, 200)], []);
  assert.deepEqual([...context.evictions(entries, 120)], []);
  assert.deepEqual([...context.evictions(entries, 100)], ['old']);
  assert.deepEqual([...context.evictions(entries, 40)], ['old', 'mid']);
  assert.deepEqual([...context.evictions(entries, 0)], ['old', 'mid', 'new']);
});

test('keptShells keeps this build’s shell and the one before it', () => {
  const { context } = worker();
  assert.deepEqual([...context.keptShells(['sous-shell-a', 'sous-shell-b', 'sous-shell-c'], 'sous-shell-c')], ['sous-shell-b', 'sous-shell-c']);
  assert.deepEqual([...context.keptShells(['sous-shell-c'], 'sous-shell-c')], ['sous-shell-c']);
  assert.deepEqual([...context.keptShells(['sous-shell-a', 'sous-shell-c', 'sous-shell-b'], 'sous-shell-c')], ['sous-shell-a', 'sous-shell-c']);
});
