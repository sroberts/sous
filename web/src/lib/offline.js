// The library kept on this device, so Sous opens and reads without a
// connection. Records are stored once for the household; each person's
// favorites and want-to-cook flags are stored under their username.

import { tx, get, getAll, put, done } from './idb.js';
import { splitFlags, withFlags, withFlag, stampOf } from './offline-core.js';

const meta = async (key) => (await get('meta', key))?.value ?? null;
const setMeta = (key, value) => put('meta', { key, value });

/** Store the library index, and the flags of the person it was read for. */
export function saveIndex(username, summaries) {
  const { items, flags } = splitFlags(summaries);
  return tx(['summaries', 'mine'], 'readwrite', (s) => {
    s.summaries.clear();
    for (const r of items) s.summaries.put(r);
    if (username) s.mine.put({ username, flags, at: Date.now() });
  });
}

/** The stored index with username's flags, or null when nothing is stored. */
export async function loadIndex(username) {
  const [items, mine] = await Promise.all([getAll('summaries'), username ? get('mine', username) : null]);
  return items.length ? withFlags(items, mine?.flags) : null;
}

/** Store full recipes, replacing any older copies, and keep the person's
 *  flags for them in step. One transaction for the lot. */
export function saveRecipes(rs, username) {
  if (!rs.length) return Promise.resolve();
  const { items, flags } = splitFlags(rs);
  return tx(['recipes', 'stamps', 'mine'], 'readwrite', async (s) => {
    for (const r of items) {
      s.recipes.put(r);
      s.stamps.put({ id: r.id, stamp: stampOf(r) });
    }
    if (!username || !Object.keys(flags).length) return;
    const row = (await done(s.mine.get(username))) ?? { username, flags: {} };
    Object.assign(row.flags, flags);
    row.at = Date.now();
    s.mine.put(row);
  });
}

/** Store one full recipe, as saveRecipes does. */
export const saveRecipe = (r, username) => saveRecipes([r], username);

/** A stored recipe with username's flags, or null. */
export async function getRecipe(id, username) {
  const [r, mine] = await Promise.all([get('recipes', id), username ? get('mine', username) : null]);
  return r ? withFlag(r, mine?.flags) : null;
}

/** The stamp of every stored recipe, by id. */
export async function stamps() {
  const all = await getAll('stamps');
  return new Map(all.map((r) => [r.id, r.stamp]));
}

/** The ids of every stored recipe. */
export const storedIds = () => tx(['recipes'], 'readonly', (s) => done(s.recipes.getAllKeys()), []);

/** Drop stored recipes, by id. */
export const prune = (ids) =>
  tx(['recipes', 'stamps'], 'readwrite', (s) => {
    for (const id of ids) {
      s.recipes.delete(id);
      s.stamps.delete(id);
    }
  });

/** Forget every other person's flags: behind the gateway, the device belongs
 *  to whoever is signed in now. */
export const forgetOthers = (username) =>
  tx(['mine'], 'readwrite', async (s) => {
    for (const key of await done(s.mine.getAllKeys())) if (key !== username) s.mine.delete(key);
  });

export const saveMe = (me) => setMeta('me', me);
export const loadMe = () => meta('me');
export const savePeople = (people) => setMeta('people', people);
export const loadPeople = async () => (await meta('people')) ?? [];
export const loadServerVersion = () => meta('serverVersion');
export const saveServerVersion = (v) => setMeta('serverVersion', v);
export const loadSyncedAt = async () => (await meta('syncedAt')) ?? 0;
export const saveSyncedAt = (t) => setMeta('syncedAt', t);
export const loadAskedPersist = () => meta('askedPersist');
export const saveAskedPersist = () => setMeta('askedPersist', true);
