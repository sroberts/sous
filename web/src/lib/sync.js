// Keeps the whole library's text on this device, in the background, while
// Sous can be reached: the index, then every recipe that is new or changed
// since it was stored. Photos are kept by the service worker as they are seen.

import { api } from './api.js';
import { isOffline } from './net.svelte.js';
import * as offline from './offline.js';
import { staleIds, removedIds, backoff } from './offline-core.js';

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

let generation = 0;
let running = null;

/** Sync the library read for username. One runs at a time: a new call stops
 *  the one running and starts over once it has. stopped() is asked between
 *  requests, and ends the sync early when true (signed out, say). */
export function syncLibrary(username, summaries, stopped = () => false) {
  const gen = ++generation;
  const live = () => gen === generation && !isOffline() && !stopped();
  const start = () => run(username, summaries, live).catch(() => {});
  running = (running ?? Promise.resolve()).then(start).finally(() => {
    if (gen === generation) running = null;
  });
  return running;
}

/** Stop the running sync after its current requests. */
export function cancelSync() {
  generation++;
}

async function run(username, summaries, live) {
  if (!live()) return;
  await offline.saveIndex(username, summaries);
  askToPersist();

  // A new version of Sous may store recipes differently: start them afresh.
  const { version } = await api.status();
  if (version !== (await offline.loadServerVersion())) {
    await offline.clearRecipes();
    await offline.saveServerVersion(version);
  }
  const have = await offline.versions();
  await offline.prune(removedIds([...have.keys()], summaries));

  const queue = staleIds(summaries, have);
  let attempt = 0;
  let failed = false;
  const worker = async () => {
    while (queue.length && live() && !failed) {
      const id = queue.shift();
      try {
        await offline.saveRecipe(await api.recipe(id), username);
        attempt = 0;
      } catch (err) {
        if (err.status === 429 || err.status === 503) {
          // Busy: wait as asked and try the same recipe again, a few times.
          if (attempt > 6) return void (failed = true);
          queue.unshift(id);
          await sleep(backoff(err.retryAfter, attempt++));
        } else if (err.code === 'offline' || err.code === 'signed_out') {
          failed = true;
        }
        // Anything else (trashed since the index was read): skip it.
      }
    }
  };
  await Promise.all([worker(), worker(), worker()]);
  if (live() && !queue.length && !failed) await offline.saveSyncedAt(Date.now());
}

// Asked once per device: without it the browser may clear the stored
// library when the device runs short of space.
let asked = false;
async function askToPersist() {
  if (asked || !navigator.storage?.persist) return;
  asked = true;
  try {
    if ((await navigator.storage.persisted()) || (await offline.loadAskedPersist())) return;
    await offline.saveAskedPersist();
    await navigator.storage.persist();
  } catch {}
}
