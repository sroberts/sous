// Reading a recipe: from Sous when it answers, from this device when it
// doesn't. Every recipe read online is kept, so one opened once opens again
// offline even before the background sync has reached it.

import { api, ApiError } from './api.js';
import { isOffline, NOT_ON_DEVICE } from './net.svelte.js';
import { session } from './session.svelte.js';
import * as offline from './offline.js';

// Without a connection, or behind a gateway whose sign-in has lapsed, the
// stored copy stands in.
const kept = (err) => err.code === 'offline' || err.code === 'signed_out';

export async function getRecipe(id) {
  const username = session.person?.username ?? null;
  if (isOffline()) {
    const r = await offline.getRecipe(id, username);
    if (r) return r;
  }
  try {
    const r = await api.recipe(id);
    offline.saveRecipe(r, username);
    return r;
  } catch (err) {
    if (!kept(err)) throw err;
    const r = await offline.getRecipe(id, username);
    if (r) return r;
    throw new ApiError(0, { error: { code: 'not_on_device', message: NOT_ON_DEVICE } });
  }
}
