// A small history router. The Go service serves index.html for unknown
// paths (spec.md §4), so every route here is a real URL.

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
  Object.assign(route, match(location.pathname), { search: new URLSearchParams(location.search) });
}

export function go(path, { replace = false } = {}) {
  if (path === location.pathname + location.search) return;
  history[replace ? 'replaceState' : 'pushState']({}, '', path);
  sync();
  window.scrollTo({ top: 0 });
}

addEventListener('popstate', sync);

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
