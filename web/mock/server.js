// A development stand-in for the Sous /v1 API (spec.md §8), served by
// Vite's dev server so the UI talks to real HTTP from day one. State is
// in memory and resets on restart. Loads web/mock-local/ (a gitignored
// sample of the household's Mela export, see scripts/mela-sample) when
// present, otherwise the committed fixtures, then the sample recipes a new
// library starts with (internal/samples/data), added by Sous itself.

import { createHash, randomUUID } from 'node:crypto';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { parseIngredientLines, parseStepLines, parseIngredient, toLines } from './parse.js';
import { splitText } from './importText.js';

const LOCAL = new URL('../mock-local/', import.meta.url).pathname;
const FIXTURES = new URL('./fixtures.json', import.meta.url).pathname;
const SAMPLES = new URL('../../internal/samples/data/', import.meta.url).pathname;
// Canned import results, keyed by normalizeSource(url). An entry is either
// { preview } or { error, page, delay? }.
const IMPORTS = new URL('./imports.json', import.meta.url).pathname;

function load() {
  const local = join(LOCAL, 'recipes.json');
  const raw = JSON.parse(readFileSync(existsSync(local) ? local : FIXTURES, 'utf8'));
  const images = new Map();
  if (existsSync(join(LOCAL, 'images'))) {
    for (const f of readdirSync(join(LOCAL, 'images'))) {
      const [hash, ext] = f.split('.');
      images.set(hash, { path: join(LOCAL, 'images', f), type: ext === 'png' ? 'image/png' : ext === 'webp' ? 'image/webp' : 'image/jpeg' });
    }
  }
  const people = new Map([['scott', { username: 'scott', name: 'Scott' }]]);
  const recipes = new Map();
  const mine = new Map(); // `${person}:${id}` → flags
  for (const f of readdirSync(SAMPLES).filter((f) => f.endsWith('.json'))) {
    const r = JSON.parse(readFileSync(join(SAMPLES, f), 'utf8'));
    r.created_by = '_sous';
    const photo = ['jpg', 'png'].map((ext) => join(SAMPLES, f.replace(/json$/, ext))).find(existsSync);
    if (photo) {
      const hash = createHash('sha256').update(readFileSync(photo)).digest('hex');
      images.set(hash, { path: photo, type: photo.endsWith('.png') ? 'image/png' : 'image/jpeg' });
      r.images = [hash];
    }
    raw.push(r);
  }
  for (const r of raw) {
    const by = r.created_by === '_sous' ? '_sous' : 'scott';
    // Stable across dev-server restarts, so deep links keep working.
    const h = createHash('sha1').update(`${r.title}\0${r.created_at ?? ''}\0${r.source_url ?? ''}`).digest('hex');
    const id = `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20, 32)}`;
    const at = r.created_at || new Date().toISOString();
    recipes.set(id, {
      id,
      title: r.title,
      source_url: r.source_url ?? null,
      description: r.description ?? null,
      notes: r.notes ?? null,
      yield_text: r.yield_text ?? null,
      servings: servingsFrom(r.yield_text),
      prep_minutes: r.prep_minutes ?? null,
      cook_minutes: r.cook_minutes ?? null,
      total_minutes: r.total_minutes ?? null,
      time_text: r.time_text ?? null,
      nutrition_text: r.nutrition_text ?? null,
      tags: r.tags ?? [],
      images: (r.images ?? []).filter((h) => images.has(h)),
      ingredients: parseIngredientLines(r.ingredients ?? []),
      steps: parseStepLines(r.steps ?? []),
      version: 1,
      deleted_at: null,
      created_at: at,
      created_by: by,
      updated_at: at,
      updated_by: by,
    });
    if (r.favorite || r.want_to_cook) mine.set(`scott:${id}`, { favorite: !!r.favorite, want_to_cook: !!r.want_to_cook });
  }
  return { recipes, images, people, mine, changes: [] };
}

function servingsFrom(text) {
  const m = (text || '').match(/(\d+)/);
  return m ? +m[1] : null;
}

function hostOf(url) {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return null;
  }
}

let db = null;

function summary(r, person) {
  return {
    id: r.id,
    title: r.title,
    source_host: hostOf(r.source_url),
    tags: r.tags,
    cover: r.images[0] ?? null,
    total_minutes: r.total_minutes ?? (r.prep_minutes || r.cook_minutes ? (r.prep_minutes ?? 0) + (r.cook_minutes ?? 0) : null),
    time_text: r.time_text,
    ingredient_items: r.ingredients.map((i) => i.item.toLowerCase()),
    mine: db.mine.get(`${person}:${r.id}`) ?? { favorite: false, want_to_cook: false },
    version: r.version,
    updated_at: r.updated_at,
  };
}

function full(r, person) {
  return { ...r, source_host: hostOf(r.source_url), mine: db.mine.get(`${person}:${r.id}`) ?? { favorite: false, want_to_cook: false } };
}

function send(res, status, body, headers = {}) {
  res.writeHead(status, { 'content-type': 'application/json', ...headers });
  res.end(body === undefined ? '' : JSON.stringify(body));
}

function fail(res, status, code, message) {
  send(res, status, { error: { code, message } });
}

async function readBody(req) {
  const chunks = [];
  for await (const c of req) chunks.push(c);
  return Buffer.concat(chunks);
}

// MOCK_OUTPOST=<username> acts as the Outpost gateway: every request is that
// person, signed in, whatever the device picked.
const outpost = process.env.MOCK_OUTPOST || null;

function personOf(req) {
  if (outpost && db.people.has(outpost)) return outpost;
  const header = req.headers['x-sous-person'];
  const cookie = /(?:^|;\s*)sous_person=([^;]+)/.exec(req.headers.cookie || '')?.[1];
  const name = header || (cookie && decodeURIComponent(cookie));
  return name && db.people.has(name) ? name : null;
}

function logChange(person, via, id, action, summaryText) {
  db.changes.unshift({ id: randomUUID(), at: new Date().toISOString(), person_id: person, via, entity: 'recipe', entity_id: id, action, summary: summaryText });
}

function applyPatch(r, body) {
  const fields = ['title', 'source_url', 'description', 'notes', 'yield_text', 'prep_minutes', 'cook_minutes', 'total_minutes', 'time_text', 'nutrition_text', 'tags', 'images'];
  for (const f of fields) if (f in body) r[f] = body[f];
  if ('yield_text' in body) r.servings = servingsFrom(body.yield_text);
  if ('ingredients' in body) r.ingredients = parseIngredientLines(body.ingredients);
  if ('steps' in body) r.steps = parseStepLines(body.steps);
}

const delay = (ms) => new Promise((r) => setTimeout(r, ms));

// The mock's NormalizeSourceURL: host without www., scheme, fragment,
// tracking params or trailing slash.
function normalizeSource(u) {
  try {
    const x = new URL(String(u).trim());
    if (x.protocol !== 'http:' && x.protocol !== 'https:') return null;
    for (const k of [...x.searchParams.keys()]) if (/^(utm_|mc_)/.test(k) || k === 'fbclid' || k === 'gclid') x.searchParams.delete(k);
    const q = x.searchParams.toString();
    return `${x.hostname.toLowerCase().replace(/^www\./, '')}${x.pathname.replace(/\/+$/, '')}${q ? `?${q}` : ''}`;
  } catch {
    return null;
  }
}

function duplicatesOf(urls) {
  const keys = new Set(urls.map(normalizeSource).filter(Boolean));
  return [...db.recipes.values()]
    .filter((r) => r.source_url && keys.has(normalizeSource(r.source_url)))
    .map((r) => ({ id: r.id, title: r.title, version: r.version, deleted: !!r.deleted_at }));
}

function titleFromSlug(url) {
  const slug = new URL(url).pathname.split('/').filter(Boolean).pop() ?? '';
  const words = slug.replace(/\.\w+$/, '').replace(/^\d+-/, '').split(/[-_]+/).filter(Boolean);
  return words.length ? words.map((w) => w[0].toUpperCase() + w.slice(1)).join(' ') : null;
}

function importFail(res, code, message, page) {
  send(res, 422, { error: { code, message }, page: page ?? null });
}

async function importPreview(req, res) {
  const body = JSON.parse((await readBody(req)).toString() || '{}');
  const url = String(body.url ?? '').trim();
  const text = String(body.text ?? '').trim();
  // Text may carry the link it came from; that link is credited, never fetched.
  if (!url && !text) return fail(res, 400, 'invalid', 'Send a url or some text.');
  const preview = (draft, report, images = []) =>
    send(res, 200, {
      draft,
      ingredients: parseIngredientLines(draft.ingredients ?? []),
      images,
      report: { fetched_url: null, archived: false, found: [], missing: [], warnings: [], ...report },
      duplicates: duplicatesOf([url, report.source_url, draft.source_url].filter(Boolean)),
    });

  if (text) {
    const split = splitText(text);
    const title = String(body.title ?? '').trim() || split.title || '';
    if (!split.ingredients.length && !split.steps.length) return importFail(res, 'no_recipe_found', 'Sous found no ingredients or steps in that text.');
    const draft = { title, ingredients: split.ingredients, steps: split.steps, ...(url ? { source_url: url } : {}) };
    const missing = [!title && 'title', !split.ingredients.length && 'ingredients', !split.steps.length && 'steps'].filter(Boolean);
    return preview(draft, { method: 'text', source_url: url, found: ['ingredients', 'steps'].filter((f) => !missing.includes(f)), missing: [...missing, 'times', 'photo'] });
  }

  const key = normalizeSource(url);
  if (!key) return importFail(res, 'unsupported_url', 'Sous reads web pages: links starting with http:// or https://.');
  const entry = JSON.parse(readFileSync(IMPORTS, 'utf8'))[key];
  if (entry?.delay) await delay(entry.delay);
  if (entry?.preview) {
    const p = structuredClone(entry.preview);
    return preview(p.draft, p.report, p.images);
  }
  if (entry?.error) return importFail(res, entry.error.code, entry.error.message, entry.page);
  return importFail(res, 'no_recipe_found', 'The page has no recipe data.', { source_url: url, title: titleFromSlug(url), image: null });
}

async function importSave(req, res, person, via) {
  const body = JSON.parse((await readBody(req)).toString() || '{}');
  const url = String(body.url ?? '').trim();
  // The sentences Siri reads, as internal/importer/classify.go has them.
  const said = {
    blocked: 'The site turned Sous away, so nothing was saved.',
    paywalled: "That recipe is only for the site's subscribers, so nothing was saved.",
    not_found: "The site says that page doesn't exist, so nothing was saved.",
    no_recipe_found: "Sous couldn't find a recipe on that page, so nothing was saved.",
    timeout: 'The site took too long to answer. Try again later.',
    too_large: 'That page is too large for Sous to read, so nothing was saved.',
    unsupported_url: 'Sous can only save recipes from web links, so nothing was saved.',
    site_error: 'The site had a problem serving that page. Try again later.',
  };
  const spoken = (status, code, message, page) => send(res, status, { error: { code, message }, ...(status === 422 ? { page: page ?? null } : {}), summary: (status === 422 && said[code]) || message });
  if (!url) return spoken(400, 'invalid', 'Send a url.');
  const key = normalizeSource(url);
  if (!key) return spoken(422, 'unsupported_url', 'Sous reads web pages: links starting with http:// or https://.');
  const reply = (r, summary, extra = {}) => send(res, 200, { summary, id: r.id, title: r.title, version: r.version, path: `/r/${r.id}`, url: outpost ? `http://localhost:5173/r/${r.id}` : null, duplicate: false, restored: false, missing: [], warnings: [], ...extra });
  const [dup] = duplicatesOf([url]);
  if (dup) {
    const r = db.recipes.get(dup.id);
    if (!r.deleted_at) return reply(r, `Already in Sous: ${r.title}.`, { duplicate: true });
    r.deleted_at = null;
    r.version += 1;
    logChange(person, via, r.id, 'restore', `Restored ${r.title}`);
    return reply(r, `Restored ${r.title} from the trash.`, { duplicate: true, restored: true });
  }
  const entry = JSON.parse(readFileSync(IMPORTS, 'utf8'))[key];
  if (entry?.error) return spoken(422, entry.error.code, entry.error.message, entry.page);
  if (!entry?.preview) return spoken(422, 'no_recipe_found', 'The page has no recipe data.');
  const { draft, report } = entry.preview;
  const now = new Date().toISOString();
  const r = { id: randomUUID(), images: [], tags: [], ingredients: [], steps: [], version: 1, deleted_at: null, created_at: now, created_by: person, updated_at: now, updated_by: person };
  applyPatch(r, { ...draft, source_url: draft.source_url ?? url });
  db.recipes.set(r.id, r);
  logChange(person, via, r.id, 'create', `Added ${r.title}`);
  const missing = (report?.missing ?? []).filter((f) => f === 'ingredients' || f === 'steps');
  const summary = missing.length ? `Saved ${r.title}, but no ${missing.join(' or ')} were found.` : `Saved ${r.title}.`;
  return reply(r, summary, { missing: report?.missing ?? [], warnings: report?.warnings ?? [] });
}

async function handle(req, res) {
  db ??= load();
  const url = new URL(req.url, 'http://x');
  const path = url.pathname;
  const person = personOf(req);
  const via = req.headers['x-sous-via'] || 'api';
  await delay(60 + Math.random() * 120); // feel the network a little
  const write = req.method !== 'GET';
  if (write && !person && path !== '/v1/people' && path !== '/v1/ingredients/parse') return fail(res, 401, 'who_are_you', 'Choose who is cooking before making changes.');

  let m;
  if (path === '/v1/status' && req.method === 'GET') {
    return send(res, 200, { version: 'mock', schema_version: 1, auth: outpost ? 'gateway' : 'none', recipes: db.recipes.size });
  }
  if (path === '/v1/me') {
    return person ? send(res, 200, { ...db.people.get(person), source: outpost ? 'outpost' : 'device', portal: outpost ? 'http://localhost:8443' : null }) : fail(res, 401, 'who_are_you', 'No one is selected on this device.');
  }
  if (path === '/v1/people') {
    if (req.method === 'GET') return send(res, 200, { items: [...db.people.values()] });
    const body = JSON.parse((await readBody(req)).toString() || '{}');
    const username = String(body.name || '').trim().toLowerCase().replace(/[^a-z0-9-]+/g, '-');
    if (!username) return fail(res, 400, 'invalid', 'A name is required.');
    if (db.people.has(username)) return fail(res, 409, 'exists', `${body.name} is already in the household.`);
    const p = { username, name: String(body.name).trim() };
    db.people.set(username, p);
    return send(res, 201, p);
  }
  if (path === '/v1/recipes' && req.method === 'GET') {
    const limit = Math.min(+url.searchParams.get('limit') || 50, 200);
    const start = +(url.searchParams.get('cursor') || 0);
    const all = [...db.recipes.values()].filter((r) => !r.deleted_at).sort((a, b) => a.title.localeCompare(b.title));
    const page = all.slice(start, start + limit).map((r) => summary(r, person));
    return send(res, 200, { items: page, total: all.length, next_cursor: start + limit < all.length ? String(start + limit) : null });
  }
  if (path === '/v1/recipes' && req.method === 'POST') {
    const body = JSON.parse((await readBody(req)).toString() || '{}');
    if (!body.title?.trim()) return fail(res, 400, 'invalid', 'A recipe needs a title.');
    const now = new Date().toISOString();
    const r = { id: randomUUID(), images: [], tags: [], ingredients: [], steps: [], version: 1, deleted_at: null, created_at: now, created_by: person, updated_at: now, updated_by: person };
    applyPatch(r, body);
    db.recipes.set(r.id, r);
    logChange(person, via, r.id, 'create', `Added ${r.title}`);
    return send(res, 201, full(r, person), { etag: `"${r.version}"` });
  }
  if (path === '/v1/recipes/import' && req.method === 'POST') return importSave(req, res, person, via);
  if (path === '/v1/recipes/import/preview' && req.method === 'POST') return importPreview(req, res);
  if ((m = path.match(/^\/v1\/recipes\/([\w-]+)$/))) {
    const r = db.recipes.get(m[1]);
    if (!r) return fail(res, 404, 'not_found', 'That recipe does not exist.');
    if (req.method === 'GET') return send(res, 200, full(r, person), { etag: `"${r.version}"` });
    const ifMatch = (req.headers['if-match'] || '').replace(/"/g, '');
    if (!ifMatch) return fail(res, 428, 'version_required', 'Send If-Match with the version you edited.');
    if (+ifMatch !== r.version) return send(res, 412, { error: { code: 'stale', message: 'Someone changed this recipe since you opened it.' }, current: full(r, person) });
    if (req.method === 'PATCH') {
      applyPatch(r, JSON.parse((await readBody(req)).toString() || '{}'));
      r.version++;
      r.updated_at = new Date().toISOString();
      r.updated_by = person;
      logChange(person, via, r.id, 'update', `Edited ${r.title}`);
      return send(res, 200, full(r, person), { etag: `"${r.version}"` });
    }
    if (req.method === 'DELETE') {
      r.deleted_at = new Date().toISOString();
      r.version++;
      logChange(person, via, r.id, 'delete', `Moved ${r.title} to the trash`);
      return send(res, 204);
    }
  }
  if ((m = path.match(/^\/v1\/recipes\/([\w-]+)\/restore$/)) && req.method === 'POST') {
    const r = db.recipes.get(m[1]);
    if (!r) return fail(res, 404, 'not_found', 'That recipe does not exist.');
    r.deleted_at = null;
    r.version++;
    logChange(person, via, r.id, 'restore', `Restored ${r.title}`);
    return send(res, 200, full(r, person));
  }
  if ((m = path.match(/^\/v1\/recipes\/([\w-]+)\/mine$/)) && req.method === 'PUT') {
    if (!db.recipes.has(m[1])) return fail(res, 404, 'not_found', 'That recipe does not exist.');
    const key = `${person}:${m[1]}`;
    const next = { ...(db.mine.get(key) ?? { favorite: false, want_to_cook: false }), ...JSON.parse((await readBody(req)).toString() || '{}') };
    db.mine.set(key, { favorite: !!next.favorite, want_to_cook: !!next.want_to_cook });
    return send(res, 200, db.mine.get(key));
  }
  if (path === '/v1/tags') {
    const counts = new Map();
    for (const r of db.recipes.values()) if (!r.deleted_at) for (const t of r.tags) counts.set(t, (counts.get(t) ?? 0) + 1);
    return send(res, 200, { items: [...counts].map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count) });
  }
  if (path === '/v1/ingredients/parse' && req.method === 'POST') {
    const body = JSON.parse((await readBody(req)).toString() || '{}');
    return send(res, 200, { items: (body.lines ?? []).map((l) => (l.trim().startsWith('#') ? { section: l.replace(/^#+\s*/, ''), raw: l } : parseIngredient(l))) });
  }
  if (path === '/v1/images' && req.method === 'POST') {
    const bytes = await readBody(req);
    const hash = createHash('sha256').update(bytes).digest('hex');
    db.images.set(hash, { bytes, type: req.headers['content-type'] || 'image/jpeg' });
    return send(res, 201, { hash });
  }
  if ((m = path.match(/^\/v1\/images\/([a-f0-9]{64})$/))) {
    const img = db.images.get(m[1]);
    if (!img) return fail(res, 404, 'not_found', 'No such image.');
    res.writeHead(200, { 'content-type': img.type, 'cache-control': 'public, max-age=31536000, immutable' });
    return res.end(img.bytes ?? readFileSync(img.path));
  }
  if (path === '/v1/changes') return send(res, 200, { items: db.changes.slice(0, 100) });
  return fail(res, 404, 'not_found', `No route for ${req.method} ${path}.`);
}

export function mockApi() {
  return {
    name: 'sous-mock-api',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        if (!req.url.startsWith('/v1/')) return next();
        handle(req, res).catch((err) => {
          console.error(err);
          fail(res, 500, 'internal', err.message);
        });
      });
    },
  };
}

export { toLines };
