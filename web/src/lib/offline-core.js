// The pure parts of the library kept on this device, apart from IndexedDB so
// node --test can check them (web/test/offline.test.js).

const NO_FLAGS = { favorite: false, want_to_cook: false };

/** Split each record's per-person flags off: records are stored for the
 *  whole household, flags under the person they belong to. */
export function splitFlags(items) {
  const flags = {};
  // A round trip through JSON drops Svelte's state proxies, which IndexedDB can't store.
  const rest = JSON.parse(JSON.stringify(items)).map(({ mine, ...r }) => {
    if (mine) flags[r.id] = { favorite: !!mine.favorite, want_to_cook: !!mine.want_to_cook };
    return r;
  });
  return { items: rest, flags };
}

/** Put one person's flags back on stored records. */
export const withFlags = (items, flags = {}) => items.map((r) => ({ ...r, mine: flags[r.id] ?? NO_FLAGS }));

export const withFlag = (r, flags = {}) => ({ ...r, mine: flags[r.id] ?? NO_FLAGS });

/** The ids of summaries whose full recipe is missing here or older than the
 *  server's. versions maps id to the stored recipe's version. */
export function staleIds(summaries, versions) {
  return summaries.filter((s) => versions.get(s.id) !== s.version).map((s) => s.id);
}

/** The stored ids no longer in the library (trashed or gone). */
export function removedIds(stored, summaries) {
  const live = new Set(summaries.map((s) => s.id));
  return stored.filter((id) => !live.has(id));
}

/** How long to wait before asking again after a 429 or 503: Retry-After in
 *  seconds when given, else a doubling backoff, at most a minute. */
export function backoff(retryAfter, attempt) {
  const s = Number(retryAfter);
  if (retryAfter != null && retryAfter !== '' && Number.isFinite(s) && s >= 0) return Math.min(s, 60) * 1000;
  return Math.min(1000 * 2 ** attempt, 60000);
}
