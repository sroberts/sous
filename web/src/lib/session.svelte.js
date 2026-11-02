// Shared app state: who is cooking, and the library index every screen
// searches against. Behind the Outpost gateway the login says who is
// cooking; otherwise this device picks a person and remembers them. Without
// a connection, the library stored on this device stands in (offline.js).

import { untrack } from 'svelte';
import { api, ApiError, setPerson, onSignedOut } from './api.js';
import { isOffline, OFFLINE_SAVE } from './net.svelte.js';
import * as offline from './offline.js';
import { syncLibrary, cancelSync } from './sync.js';

const KEY = 'sous.person';

function stored() {
  try {
    return localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

export const session = $state({
  person: null, // { username, name }
  outpost: false, // signed in through Outpost, so the person is fixed
  portal: null, // the Outpost portal, for the account menu
  signedOut: false, // the Outpost sign-in ended while the page was open
  people: [],
  recipes: [], // summaries from /v1/recipes
  loaded: false,
  fromCache: false, // opened from this device's stored library, offline
  error: null,
  asking: false, // the who's-cooking prompt is open
  toast: null, // { id, message, action? }
});

/** A short note at the foot of the page, with an optional action such as Undo.
 *  One with an action stays longer, so there is time to reach it. */
export function toast(message, action = null) {
  const id = crypto.randomUUID();
  session.toast = { id, message, action };
  setTimeout(() => {
    if (session.toast?.id === id) session.toast = null;
  }, action ? 15000 : 8000);
}

onSignedOut(() => {
  session.signedOut = true;
  cancelSync();
});

export async function boot() {
  try {
    const [me, { items: people }, recipes] = await Promise.all([signedIn(), api.people(), api.allRecipes()]);
    session.people = people;
    session.recipes = recipes;
    session.fromCache = false;
    if (me) {
      session.person = { username: me.username, name: me.name };
      session.outpost = true;
      session.portal = me.portal;
      // Per-person flags in the index were read with the login already applied.
      // The device is this person's now: no one else's flags stay on it.
      offline.forgetOthers(me.username);
    } else {
      const saved = people.find((p) => p.username === stored());
      if (saved) choosePerson(saved);
    }
    offline.saveMe(me && { username: me.username, name: me.name, portal: me.portal });
    offline.savePeople(people);
    session.loaded = true;
    session.error = null;
    if (me || !session.person) sync(); // choosePerson syncs for a saved person
  } catch (err) {
    if ((err.code === 'offline' || err.code === 'signed_out') && (await bootFromCache())) return;
    session.error = err;
  }
}

/** Open with the library stored on this device. False when there is none. */
async function bootFromCache() {
  const [me, people] = await Promise.all([offline.loadMe(), offline.loadPeople()]);
  const username = me?.username ?? stored();
  const recipes = await offline.loadIndex(username);
  if (!recipes) return false;
  session.people = people;
  session.recipes = recipes;
  if (me) {
    session.person = { username: me.username, name: me.name };
    session.outpost = true;
    session.portal = me.portal;
  } else {
    const saved = people.find((p) => p.username === username);
    if (saved) {
      session.person = saved;
      setPerson(saved.username);
    }
  }
  session.fromCache = true;
  session.loaded = true;
  session.error = null;
  return true;
}

/** Bring the stored library up to date, in the background. */
function sync() {
  syncLibrary(session.person?.username ?? null, $state.snapshot(session.recipes), () => session.signedOut);
}

// Back online: a page opened from the stored library boots again, quietly;
// one that was already up to date catches up on what changed meanwhile.
$effect.root(() => {
  $effect(() => {
    if (isOffline()) return;
    untrack(() => {
      if (session.fromCache) boot();
      else if (session.loaded) sync();
    });
  });
});

// Coming back to a page left open a while.
document.addEventListener('visibilitychange', async () => {
  if (document.visibilityState !== 'visible' || isOffline() || !session.loaded || session.fromCache) return;
  if (Date.now() - (await offline.loadSyncedAt()) > 10 * 60 * 1000) sync();
});

/** The Outpost login, or null when this device has to pick someone. */
async function signedIn() {
  try {
    const me = await api.me();
    return me.source === 'outpost' ? me : null;
  } catch {
    // 401 who_are_you is the usual answer without the gateway. A failed
    // gateway token also fails the other boot requests, which report it.
    return null;
  }
}

export function choosePerson(p) {
  if (session.outpost) return;
  session.person = p;
  setPerson(p.username);
  document.cookie = `sous_person=${encodeURIComponent(p.username)}; path=/; max-age=31536000; samesite=lax`;
  try {
    localStorage.setItem(KEY, p.username);
  } catch {}
  // Per-person flags differ, so the index is refetched for the new cook, or
  // read with their stored flags without a connection.
  const fromDevice = () => offline.loadIndex(p.username).then((r) => r && (session.recipes = r));
  if (isOffline()) return void fromDevice();
  api.allRecipes().then(
    (r) => {
      session.recipes = r;
      sync();
    },
    fromDevice,
  );
}

/** Run a write; if no one is cooking yet, ask first and retry. */
export async function asPerson(fn) {
  // Picking someone leads to a change, which can't be saved offline.
  if (!session.person && isOffline()) throw new ApiError(0, { error: { code: 'offline', message: OFFLINE_SAVE } });
  if (!session.person) {
    session.asking = true;
    await new Promise((resolve) => {
      const stop = $effect.root(() => {
        $effect(() => {
          if (!session.asking) {
            stop();
            resolve();
          }
        });
      });
    });
    if (!session.person) return null;
  }
  return fn();
}

/** Keep the index row in step after a recipe changes. */
export function patchSummary(id, fields) {
  const i = session.recipes.findIndex((r) => r.id === id);
  if (i >= 0) session.recipes[i] = { ...session.recipes[i], ...fields };
  offline.saveIndex(session.person?.username ?? null, $state.snapshot(session.recipes));
}

export async function refreshIndex() {
  session.recipes = await api.allRecipes();
  sync();
}
