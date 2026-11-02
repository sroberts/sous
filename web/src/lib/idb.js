// A small promise wrapper over IndexedDB, for the library kept on this
// device. Where IndexedDB isn't there (some private windows) or fails,
// every call quietly does nothing and Sous works online only.

const STORES = {
  summaries: 'id',
  recipes: 'id',
  stamps: 'id', // { id, stamp }: which version of each stored recipe (offline-core.js)
  mine: 'username', // { username, flags: { [id]: { favorite, want_to_cook } }, at }
  meta: 'key', // { key, value }: me, people, person, serverVersion, syncedAt
};

let opening = null;

function open() {
  opening ??= new Promise((resolve) => {
    try {
      // Version 2 added stamps; recipes stored before it are fetched once more.
      const req = indexedDB.open('sous', 2);
      req.onupgradeneeded = () => {
        for (const [name, keyPath] of Object.entries(STORES)) {
          if (!req.result.objectStoreNames.contains(name)) req.result.createObjectStore(name, { keyPath });
        }
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => resolve(null);
      req.onblocked = () => resolve(null);
    } catch {
      resolve(null);
    }
  });
  return opening;
}

const done = (req) =>
  new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });

/** Run fn(stores) in one transaction over the named stores, resolving with
 *  what fn returns once the transaction commits. Resolves with fallback when
 *  there is no database or the transaction fails. */
export async function tx(names, mode, fn, fallback = null) {
  const db = await open();
  if (!db) return fallback;
  try {
    return await new Promise((resolve, reject) => {
      const t = db.transaction(names, mode);
      const stores = Object.fromEntries(names.map((n) => [n, t.objectStore(n)]));
      let result;
      Promise.resolve(fn(stores)).then((r) => (result = r), reject);
      t.oncomplete = () => resolve(result);
      t.onerror = () => reject(t.error);
      t.onabort = () => reject(t.error);
    });
  } catch {
    return fallback;
  }
}

export const get = (name, key) => tx([name], 'readonly', (s) => done(s[name].get(key)));
export const getAll = (name) => tx([name], 'readonly', (s) => done(s[name].getAll()), []);
export const put = (name, value) => tx([name], 'readwrite', (s) => void s[name].put(value));
export { done };
