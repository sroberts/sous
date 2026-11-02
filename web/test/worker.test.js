import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { fillWorker } from '../build/pwa.js';

const template = readFileSync(new URL('../sw/sw.js', import.meta.url), 'utf8');
const BUILD = 'abcdef012345';

// A stand-in CacheStorage: named Maps, listed in the order they were made.
function fakeCaches(initial = {}) {
  const store = new Map(Object.entries(initial).map(([name, entries]) => [name, new Map(Object.entries(entries))]));
  const open = async (name) => {
    if (!store.has(name)) store.set(name, new Map());
    const c = store.get(name);
    return {
      match: async (key) => c.get(typeof key === 'string' ? key : new URL(key.url).pathname),
      put: async (key, res) => void c.set(key, res),
      delete: async (key) => c.delete(key),
      keys: async () => [...c.keys()].map((k) => ({ url: `https://sous.test${k}` })),
    };
  };
  return {
    store,
    api: {
      open,
      keys: async () => [...store.keys()],
      delete: async (name) => store.delete(name),
      match: async (key) => {
        for (const c of store.values()) if (c.has(key)) return c.get(key);
      },
    },
  };
}

// Node's Request wants an absolute URL; the worker's are relative to its origin.
class Request {
  constructor(url, init) {
    this.url = url;
    Object.assign(this, init);
  }
}

function worker({ precache, caches, fetch }) {
  const listeners = {};
  const self = {
    addEventListener: (type, fn) => (listeners[type] = fn),
    location: { origin: 'https://sous.test' },
    skipWaiting: async () => {},
    clients: { claim: async () => {}, matchAll: async () => [] },
  };
  const context = vm.createContext({ self, caches, fetch, URL, Request, Response, Headers, setTimeout, clearTimeout });
  vm.runInContext(fillWorker(template, BUILD, precache), context);
  const fire = async (type) => {
    let wait;
    listeners[type]({ waitUntil: (p) => (wait = p) });
    await wait;
  };
  return { context, fire };
}

const page = () => new Response(`<meta name="sous-build" content="${BUILD}" />`);
const ok = (body = 'x') => Object.defineProperty(new Response(body), 'type', { value: 'basic' });

test('keptShells skips a partial shell for the last complete one', () => {
  const { context } = worker({ precache: ['/'], caches: fakeCaches().api, fetch: async () => ok() });
  const shells = ['sous-shell-a', 'sous-shell-b', 'sous-shell-c'];
  assert.deepEqual([...context.keptShells(shells, 'sous-shell-c', ['sous-shell-a'])], ['sous-shell-a', 'sous-shell-c']);
  assert.deepEqual([...context.keptShells(shells, 'sous-shell-c', [])], ['sous-shell-c']);
});

test('a failed install leaves no shell behind', async () => {
  const fake = fakeCaches();
  const fetch = async (req) => (req.url === '/' ? Object.defineProperty(page(), 'type', { value: 'basic' }) : new Response('', { status: 500 }));
  const { fire } = worker({ precache: ['/', '/icons/icon.svg'], caches: fake.api, fetch });
  await assert.rejects(fire('install'), /precache \/icons\/icon.svg: 500/);
  assert.deepEqual([...fake.store.keys()], []);
});

test('install copies hashed assets an older shell holds, and fetches the rest', async () => {
  const held = ok('old asset');
  const fake = fakeCaches({ 'sous-shell-old': { '/': page(), '/assets/index-1.js': held } });
  const fetched = [];
  const fetch = async (req) => {
    const path = req.url;
    fetched.push(path);
    return path === '/' ? Object.defineProperty(page(), 'type', { value: 'basic' }) : ok();
  };
  const { fire } = worker({ precache: ['/', '/assets/index-1.js', '/assets/index-2.css', '/manifest.webmanifest'], caches: fake.api, fetch });
  await fire('install');
  assert.deepEqual(fetched.sort(), ['/', '/assets/index-2.css', '/manifest.webmanifest']);
  assert.equal(fake.store.get(`sous-shell-${BUILD}`).get('/assets/index-1.js'), held);
});

test('activate keeps the last complete shell and drops a partial one', async () => {
  const fake = fakeCaches({
    'sous-shell-old': { '/': page() },
    'sous-shell-partial': { '/assets/a.js': ok() },
    [`sous-shell-${BUILD}`]: { '/': page() },
    'sous-images-v1': {},
  });
  const { fire } = worker({ precache: ['/'], caches: fake.api, fetch: async () => ok() });
  await fire('activate');
  assert.deepEqual([...fake.store.keys()], ['sous-shell-old', `sous-shell-${BUILD}`, 'sous-images-v1']);
});

test('trim drops the oldest photos past the budget, reading sizes once', async () => {
  const MB = 1024 * 1024;
  const photo = (mb) => new Response('', { headers: { 'x-sous-bytes': String(mb * MB) } });
  const fake = fakeCaches({ 'sous-images-v1': { '/v1/images/a': photo(150), '/v1/images/b': photo(60), '/v1/images/c': photo(10) } });
  let matches = 0;
  const open = fake.api.open;
  fake.api.open = async (name) => {
    const c = await open(name);
    return { ...c, match: (k) => (matches++, c.match(k)) };
  };
  const { context } = worker({ precache: ['/'], caches: fake.api, fetch: async () => ok() });
  await context.trim();
  assert.deepEqual([...fake.store.get('sous-images-v1').keys()], ['/v1/images/b', '/v1/images/c']);
  assert.equal(matches, 3);
  await context.trim();
  assert.equal(matches, 3, 'a second trim reads no sizes from the store');
});
