// A small history router. The Go service serves index.html for unknown
// paths (spec.md §4), so every route here is a real URL.

import { flushSync, tick } from 'svelte';

const ROUTES = [
  ['library', /^\/$/],
  ['new', /^\/new$/],
  ['import', /^\/import$/],
  ['assistants', /^\/assistants$/],
  ['recipe', /^\/r\/([\w-]+)$/],
  ['cook', /^\/r\/([\w-]+)\/cook$/],
  ['edit', /^\/r\/([\w-]+)\/edit$/],
];

function match(pathname) {
  for (const [name, re] of ROUTES) {
    const m = pathname.match(re);
    if (m) return { name, id: m[1] ?? null };
  }
  return { name: 'library', id: null };
}

// `back` is true when the page was reached by Back or Forward, so a page can
// put back where the reader was (the library's scroll, its selection).
export const route = $state({ ...match(location.pathname), search: new URLSearchParams(location.search), back: false });

// `then` runs once the new page is drawn (the scroll to its top, say).
function sync(back = false, { then, animate = true } = {}) {
  const next = { ...match(location.pathname), search: new URLSearchParams(location.search), back };
  const page = next.name + next.id !== route.name + route.id;
  const update = () => {
    Object.assign(route, next);
    if (page) land();
    then?.();
  };
  if (!page || !animate || !crossfade()) return update();
  // The old page fades into the new one, as an app's screens change. The
  // page is drawn and scrolled inside the update, so the fade is between
  // finished screens, and the library's own scroll is put back before the
  // new picture is taken.
  document.startViewTransition(async () => {
    update();
    flushSync();
    await tick();
  });
}

// Not under reduced motion, and never while someone types: the fade shows a
// picture of the page, so letters typed during it would appear only after.
function crossfade() {
  return (
    !!document.startViewTransition &&
    !matchMedia('(prefers-reduced-motion: reduce)').matches &&
    !document.activeElement?.matches('input, textarea, select, [contenteditable]')
  );
}

// Arriving on another page in the app moves focus to its heading, as a page
// load would, so a screen reader starts there. A page that fetches first gets
// it when its heading appears; if none does, the page title is announced.
// The first load, a change within one page (a search, a filter) and focus in
// a field are left alone, and pressing a key or the pointer while a page loads
// cancels it.
let landing = null;

function land() {
  landing?.();
  let timer;
  let observer;
  const stop = () => {
    clearTimeout(timer);
    observer?.disconnect();
    removeEventListener('keydown', stop, true);
    removeEventListener('pointerdown', stop, true);
    if (landing === stop) landing = null;
  };
  landing = stop;
  const typing = () => document.activeElement?.matches('input, textarea, select, [contenteditable]');
  const settle = () => {
    const h = [...document.querySelectorAll('h1')].find((el) => el.textContent.trim());
    if (!h) return false;
    stop();
    if (typing()) return true;
    if (!h.hasAttribute('tabindex')) h.setAttribute('tabindex', '-1');
    h.focus({ preventScroll: true });
    return true;
  };
  tick().then(() => {
    if (landing !== stop || settle()) return;
    observer = new MutationObserver(settle);
    observer.observe(document.body, { childList: true, subtree: true, characterData: true });
    timer = setTimeout(() => {
      stop();
      if (!typing()) announce(document.title);
    }, 4000);
  });
  addEventListener('keydown', stop, true);
  addEventListener('pointerdown', stop, true);
}

let live = null;
function announce(text) {
  if (!live) {
    live = document.createElement('div');
    live.className = 'visually-hidden';
    live.setAttribute('aria-live', 'polite');
    document.body.append(live);
  }
  live.textContent = '';
  setTimeout(() => (live.textContent = text), 50);
}

// A page can guard leaving it (the editor, with unsaved changes). The guard
// is called with a proceed function and returns true to let the leave happen
// now; false keeps the page, and calling proceed later leaves after all.
let guard = null;

export function setLeaveGuard(fn) {
  guard = fn;
  return () => {
    if (guard === fn) guard = null;
  };
}

// Each entry carries its position in the session history, so a refused Back
// or Forward can be undone by moving the same distance the other way.
let pos = history.state?.pos ?? 0;
history.replaceState({ ...history.state, pos }, '');
let quiet = null; // 'undo' or 'pass': the next popstate is the router's own

// A search being typed changes the page's query without touching history:
// on an iPhone home-screen app, changing the URL (or anything else) under the
// focused field as each letter lands left the keyboard typing into nothing.
// The URL catches up once, when the field is left or the page is.
let staged = null;

export function stage(search) {
  staged = `${location.pathname}${search.size ? `?${search}` : ''}`;
  route.search = search;
  // New results start at the top, as a new page would; at the top already
  // (where the search box is, on a phone) nothing moves.
  if (scrollY > 0) window.scrollTo({ top: 0 });
}

export function commit() {
  if (staged === null) return;
  const path = staged;
  staged = null;
  if (path !== location.pathname + location.search) history.replaceState(history.state, '', path);
}

addEventListener('pagehide', commit);
document.addEventListener('visibilitychange', () => document.visibilityState === 'hidden' && commit());

function leave(path, replace) {
  if (!replace) pos += 1;
  history[replace ? 'replaceState' : 'pushState']({ pos }, '', path);
  sync(false, { then: () => window.scrollTo({ top: 0 }) });
}

export function go(path, { replace = false } = {}) {
  commit();
  if (path === location.pathname + location.search) return;
  if (guard && !guard(() => leave(path, replace))) return;
  leave(path, replace);
}

addEventListener('popstate', (e) => {
  staged = null; // the entry it was for is no longer the current one
  const to = e.state?.pos ?? null;
  const mode = quiet;
  quiet = null;
  if (mode === 'undo') return;
  if (mode !== 'pass' && guard && to !== null && to !== pos) {
    const delta = to - pos;
    const proceed = () => {
      quiet = 'pass';
      history.go(delta);
    };
    if (!guard(proceed)) {
      quiet = 'undo';
      history.go(-delta);
      return;
    }
  }
  if (to !== null) pos = to;
  // A swipe back on iOS has already animated the change; don't do it twice.
  sync(true, { animate: !e.hasUAVisualTransition });
});

/** Use on <a href> so links work as links and navigate in place. */
export function link(node) {
  const onclick = (e) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    const href = node.getAttribute('href');
    if (!href?.startsWith('/')) return;
    e.preventDefault();
    go(href);
  };
  node.addEventListener('click', onclick);
  return { destroy: () => node.removeEventListener('click', onclick) };
}
