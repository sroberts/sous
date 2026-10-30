// Shared app state: who is cooking, and the library index every screen
// searches against. Behind the Outpost gateway the login says who is
// cooking; otherwise this device picks a person and remembers them.

import { api, setPerson, onSignedOut } from './api.js';

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
  error: null,
  asking: false, // the who's-cooking prompt is open
  toast: null, // { id, message, action? }
});

/** A short note at the foot of the page, with an optional action such as Undo. */
export function toast(message, action = null) {
  const id = crypto.randomUUID();
  session.toast = { id, message, action };
  setTimeout(() => {
    if (session.toast?.id === id) session.toast = null;
  }, 8000);
}

onSignedOut(() => (session.signedOut = true));

export async function boot() {
  try {
    const [me, { items: people }, recipes] = await Promise.all([signedIn(), api.people(), api.allRecipes()]);
    session.people = people;
    session.recipes = recipes;
    if (me) {
      session.person = { username: me.username, name: me.name };
      session.outpost = true;
      session.portal = me.portal;
      // Per-person flags in the index were read with the login already applied.
    } else {
      const saved = people.find((p) => p.username === stored());
      if (saved) choosePerson(saved);
    }
    session.loaded = true;
    session.error = null;
  } catch (err) {
    session.error = err;
  }
}

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
  // Per-person flags differ, so the index is refetched for the new cook.
  api.allRecipes().then((r) => (session.recipes = r)).catch(() => {});
}

/** Run a write; if no one is cooking yet, ask first and retry. */
export async function asPerson(fn) {
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
}

export async function refreshIndex() {
  session.recipes = await api.allRecipes();
}
