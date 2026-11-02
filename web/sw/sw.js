// The Sous service worker. It keeps the app shell (index.html, this build's
// assets, the icons and manifest) so the app opens without a connection, and
// keeps photos as they are viewed. It never touches /v1 JSON, /mcp or /up:
// the library's text is stored by the page itself, in IndexedDB.
//
// A template: web/build/pwa.js fills in BUILD and PRECACHE at build time.

const BUILD = '__SOUS_BUILD__';
const PRECACHE = __SOUS_PRECACHE__;
const SHELL = `sous-shell-${BUILD}`;
const IMAGES = 'sous-images-v1';
const IMAGE_BUDGET = 200 * 1024 * 1024;
const IMAGE_MAX = 10 * 1024 * 1024;
const NAVIGATION_TIMEOUT = 3000;
const BYPASS = /^\/(v1|mcp|up)(\/|$)/;
const IMAGE = /^\/v1\/images\/[^/]+$/;

// Every file must come from this build: a redirect or a page from the Outpost
// gateway (its login, say) fails the install, and the old worker stays.
self.addEventListener('install', (e) => {
  e.waitUntil(
    (async () => {
      const cache = await caches.open(SHELL);
      for (const url of PRECACHE) {
        const res = await fetch(new Request(url, { cache: 'reload', redirect: 'manual' }));
        if (!res.ok || res.type !== 'basic') throw new Error(`precache ${url}: ${res.status || res.type}`);
        if (url === '/') {
          const body = await res.clone().text();
          if (!body.includes(`name="sous-build" content="${BUILD}"`)) throw new Error('precache /: not this build');
        }
        await cache.put(url, res);
      }
      await self.skipWaiting();
    })(),
  );
});

// The shell just before this one stays, so a page still open on the last
// build can load the chunks it has yet to ask for.
self.addEventListener('activate', (e) => {
  e.waitUntil(
    (async () => {
      const shells = (await caches.keys()).filter((k) => k.startsWith('sous-shell-'));
      const keep = keptShells(shells, SHELL);
      await Promise.all(shells.filter((k) => !keep.includes(k)).map((k) => caches.delete(k)));
      await self.clients.claim();
      for (const client of await self.clients.matchAll({ type: 'window' })) {
        client.postMessage({ type: 'sous:activated', build: BUILD });
      }
    })(),
  );
});

self.addEventListener('message', (e) => {
  if (e.data?.type === 'sous:build?') e.source?.postMessage({ type: 'sous:build', build: BUILD });
});

self.addEventListener('fetch', (e) => {
  const req = e.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (IMAGE.test(url.pathname)) {
    const got = image(req, url.pathname);
    e.respondWith(got.then((g) => g.res));
    e.waitUntil(got.then((g) => g.kept));
    return;
  }
  if (BYPASS.test(url.pathname)) return;
  if (req.mode === 'navigate') {
    e.respondWith(navigate(req));
  } else if (url.pathname.startsWith('/assets/') || PRECACHE.includes(url.pathname)) {
    e.respondWith(caches.match(url.pathname).then((hit) => hit ?? fetch(req)));
  }
});

// Pages come from the network, so a new build shows up on the next load. A
// gateway redirect goes through to the login; no answer within the timeout,
// or a server error, opens the cached shell. Pages are never cached here.
async function navigate(req) {
  let timer;
  try {
    const timeout = new Promise((_, reject) => (timer = setTimeout(() => reject(new Error('timeout')), NAVIGATION_TIMEOUT)));
    const res = await Promise.race([fetch(req), timeout]);
    if (res.status >= 500) return (await shell()) ?? res;
    return res;
  } catch {
    return (await shell()) ?? Response.error();
  } finally {
    clearTimeout(timer);
  }
}

async function shell() {
  return (await (await caches.open(SHELL)).match('/')) ?? (await caches.match('/'));
}

// Photos are immutable, so a stored one is always right. A new one is kept
// if it is an image of 10 MB or less, and the oldest go once the store
// passes its budget. Without a connection an unseen photo fails, and the
// page shows the dish's initial in its place.
async function image(req, key) {
  const cache = await caches.open(IMAGES);
  const hit = await cache.match(key, { ignoreVary: true });
  if (hit) return { res: hit, kept: null };
  let res;
  try {
    res = await fetch(req);
  } catch {
    return { res: Response.error(), kept: null };
  }
  const type = res.headers.get('content-type') ?? '';
  if (!res.ok || res.type !== 'basic' || !type.startsWith('image/')) return { res, kept: null };
  return { res, kept: keep(cache, key, res.clone()) };
}

async function keep(cache, key, res) {
  try {
    const blob = await res.blob();
    if (blob.size > IMAGE_MAX) return;
    const headers = new Headers(res.headers);
    headers.set('x-sous-bytes', String(blob.size));
    await cache.put(key, new Response(blob, { status: res.status, statusText: res.statusText, headers }));
    await trimSoon();
  } catch {
    // A full disk or a dropped body: the photo is fetched again next time.
  }
}

// Photos often arrive a screenful at a time, so trimming waits for a lull.
let trimming = null;
function trimSoon() {
  trimming ??= new Promise((resolve) => setTimeout(resolve, 2000)).then(trim).finally(() => (trimming = null));
  return trimming;
}

async function trim() {
  const cache = await caches.open(IMAGES);
  const entries = [];
  for (const req of await cache.keys()) {
    const res = await cache.match(req, { ignoreVary: true });
    entries.push({ key: req, bytes: Number(res?.headers.get('x-sous-bytes')) || 0 });
  }
  await Promise.all(evictions(entries, IMAGE_BUDGET).map((k) => cache.delete(k, { ignoreVary: true })));
}

/** The keys to delete, oldest first, to bring entries (oldest first) within budget bytes. */
function evictions(entries, budget) {
  let total = entries.reduce((sum, e) => sum + e.bytes, 0);
  const out = [];
  for (const e of entries) {
    if (total <= budget) break;
    out.push(e.key);
    total -= e.bytes;
  }
  return out;
}

/** The shell caches to keep: this one and the one made just before it.
 *  Cache names list in the order they were made. */
function keptShells(shells, current) {
  const i = shells.indexOf(current);
  return i > 0 ? [shells[i - 1], current] : [current];
}
