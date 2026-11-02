// Keeps the whole library's text on this device, in the background, while
// Sous can be reached: the index, then every recipe that is new or changed
// since it was stored. Photos are kept by the service worker as they are seen.

import { api } from './api.js';
import { isOffline } from './net.svelte.js';
import * as offline from './offline.js';
import { staleIds, removedIds, backoff, stampOf } from './offline-core.js';

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

let generation = 0;
let running = null;

/** Sync the library read for username. One runs at a time: a new call stops
 *  the one running and starts over once it has. summaries() is read as the
 *  sync starts, so it stores the index as it is then, never an older one.
 *  stopped() is asked between requests, and ends the sync early when true
 *  (signed out, say). */
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

async function run(username, read, live) {
  if (!live()) return;
  const summaries = read();
  await offline.saveIndex(username, summaries);
  askToPersist();

  // A new version of Sous may shape recipes differently: fetch every one
  // again. Each replaces its old copy as it arrives, so an interrupted pass
  // leaves the rest readable, and the next pass carries on.
  const { version } = await api.status();
  const upgraded = version !== (await offline.loadServerVersion());
  await offline.prune(removedIds(await offline.storedIds(), summaries));

  const queue = staleIds(summaries, upgraded ? new Map() : await offline.stamps(), stampOf);
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
  if (live() && !queue.length && !failed) {
    if (upgraded) await offline.saveServerVersion(version);
    await offline.saveSyncedAt(Date.now());
  }
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
