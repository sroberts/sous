// The /v1 client. The UI uses only what any API client could (spec.md §4).

export class ApiError extends Error {
  constructor(status, body) {
    super(body?.error?.message || `Request failed (${status})`);
    this.status = status;
    this.code = body?.error?.code || 'error';
    this.body = body;
  }
}

let person = null;

/** Set who is cooking on this device; sent as X-Sous-Person (spec.md §5). */
export function setPerson(username) {
  person = username;
}

async function request(method, path, { body, version, raw } = {}) {
  const headers = { 'x-sous-via': 'ui' };
  if (person) headers['x-sous-person'] = person;
  if (version !== undefined) headers['if-match'] = `"${version}"`;
  let payload;
  if (raw) {
    payload = raw;
    headers['content-type'] = raw.type || 'application/octet-stream';
  } else if (body !== undefined) {
    payload = JSON.stringify(body);
    headers['content-type'] = 'application/json';
  }
  let res;
  try {
    res = await fetch(path, { method, headers, body: payload });
  } catch {
    throw new ApiError(0, { error: { code: 'offline', message: "Sous isn't answering. Check the connection and try again." } });
  }
  if (res.status === 204) return null;
  const data = await res.json().catch(() => null);
  if (!res.ok) throw new ApiError(res.status, data);
  return data;
}

export const api = {
  me: () => request('GET', '/v1/me'),
  people: () => request('GET', '/v1/people'),
  addPerson: (name) => request('POST', '/v1/people', { body: { name } }),

  /** Every recipe summary, paged through at the API's 200 limit. */
  async allRecipes() {
    const items = [];
    let cursor = '';
    do {
      const page = await request('GET', `/v1/recipes?limit=200${cursor ? `&cursor=${cursor}` : ''}`);
      items.push(...page.items);
      cursor = page.next_cursor;
    } while (cursor);
    return items;
  },
  recipe: (id) => request('GET', `/v1/recipes/${id}`),
  createRecipe: (body) => request('POST', '/v1/recipes', { body }),
  updateRecipe: (id, version, body) => request('PATCH', `/v1/recipes/${id}`, { body, version }),
  deleteRecipe: (id, version) => request('DELETE', `/v1/recipes/${id}`, { version }),
  restoreRecipe: (id) => request('POST', `/v1/recipes/${id}/restore`),
  setMine: (id, flags) => request('PUT', `/v1/recipes/${id}/mine`, { body: flags }),
  tags: () => request('GET', '/v1/tags'),
  parseIngredients: (lines) => request('POST', '/v1/ingredients/parse', { body: { lines } }),
  uploadImage: (file) => request('POST', '/v1/images', { raw: file }),
  /** Read a recipe from a link or pasted text into an unsaved draft. A failed
   * read throws an ApiError with status 422; err.body.page holds what the page
   * still offered. */
  importPreview: (body) => request('POST', '/v1/recipes/import/preview', { body }),
};

export const imageUrl = (hash) => `/v1/images/${hash}`;
