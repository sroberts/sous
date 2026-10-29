// Shared app state: who is cooking on this device, and the library index
// every screen searches against.

import { api, setPerson } from './api.js';

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

export async function boot() {
  try {
    const [{ items: people }, recipes] = await Promise.all([api.people(), api.allRecipes()]);
    session.people = people;
    session.recipes = recipes;
    const saved = people.find((p) => p.username === stored());
    if (saved) choosePerson(saved);
    session.loaded = true;
    session.error = null;
  } catch (err) {
    session.error = err;
  }
}

export function choosePerson(p) {
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
