// Keeps the whole library's text on this device, in the background, while
// Sous can be reached: the index, then every recipe that is new or changed
// since it was stored, fetched a hundred at a time. Photos are kept by the service worker as they are seen.

import { api } from './api.js';
import { isOffline } from './net.svelte.js';
import * as offline from './offline.js';
import { staleIds, removedIds, backoff, stampOf, chunks } from './offline-core.js';

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const BATCH = 100; // MaxBatch in internal/recipes

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

  // A hundred recipes a request (the most Sous gives at once), two requests
  // at a time: a whole library in a few requests rather than one per recipe.
  const queue = chunks(staleIds(summaries, upgraded ? new Map() : await offline.stamps(), stampOf), BATCH);
  let attempt = 0;
  let failed = false;
  const worker = async () => {
    while (queue.length && live() && !failed) {
      const ids = queue.shift();
      try {
        await offline.saveRecipes(await api.recipes(ids), username);
        attempt = 0;
      } catch (err) {
        if (err.status === 429 || err.status === 503) {
          // Busy: wait as asked and try the same batch again, a few times.
          if (attempt > 6) return void (failed = true);
          queue.unshift(ids);
          await sleep(backoff(err.retryAfter, attempt++));
        } else {
          // Offline, signed out, or anything unexpected: stop, and the next
          // sync carries on from what was stored. (A recipe trashed since the
          // index was read is simply left out of the answer.)
          failed = true;
        }
      }
    }
  };
  await Promise.all([worker(), worker()]);
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
