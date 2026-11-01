// A small history router. The Go service serves index.html for unknown
// paths (spec.md §4), so every route here is a real URL.

import { tick } from 'svelte';

const ROUTES = [
  ['library', /^\/$/],
  ['new', /^\/new$/],
  ['import', /^\/import$/],
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

export const route = $state({ ...match(location.pathname), search: new URLSearchParams(location.search) });

function sync() {
  const was = route.name + route.id;
  Object.assign(route, match(location.pathname), { search: new URLSearchParams(location.search) });
  if (route.name + route.id !== was) land();
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

function leave(path, replace) {
  if (!replace) pos += 1;
  history[replace ? 'replaceState' : 'pushState']({ pos }, '', path);
  sync();
  window.scrollTo({ top: 0 });
}

export function go(path, { replace = false } = {}) {
  if (path === location.pathname + location.search) return;
  if (guard && !guard(() => leave(path, replace))) return;
  leave(path, replace);
}

addEventListener('popstate', (e) => {
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
  sync();
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
