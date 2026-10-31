<script>
  import { tick } from 'svelte';
  import { ArrowLeft, X, ImagePlus, Trash2, Link2 } from '@lucide/svelte';
  import { go, link } from '../lib/router.svelte.js';
  import { api } from '../lib/api.js';
  import { session, asPerson, refreshIndex, toast } from '../lib/session.svelte.js';
  import { amount, ago, personName } from '../lib/format.js';
  import Plate from '../components/Plate.svelte';

  // draft: an unsaved recipe from an import. With id and merge, the draft
  // updates that recipe instead (spec.md §14): tags, notes and photos stay.
  let { id, draft = null, merge = false } = $props();

  let loaded = $state(!id);
  let version = $state(null);
  let saving = $state(false);
  let dirty = $state(false);
  let problem = $state(null); // { message } or { conflict: recipe }
  let titleError = $state(false);
  let allTags = $state([]);

  let form = $state({
    title: '',
    source_url: '',
    description: '',
    yield_text: '',
    prep_minutes: '',
    cook_minutes: '',
    total_minutes: '',
    time_text: '',
    nutrition_text: '',
    notes: '',
    tags: [],
    images: [],
    ingredients: [''],
    steps: [''],
  });

  // In import mode, leaving goes back to the import page.
  const back = $derived(draft ? '/import' : id ? `/r/${id}` : '/');
  const backLabel = $derived(draft ? 'Import' : id ? 'Recipe' : 'Library');

  function fill(r) {
    const lines = (rows, field) => {
      const out = [];
      let section = null;
      for (const row of rows) {
        if (row.section !== section) {
          if (row.section) out.push(`# ${row.section}`);
          section = row.section;
        }
        out.push(row[field]);
      }
      return out.length ? out : [''];
    };
    form = {
      title: r.title,
      source_url: r.source_url ?? '',
      description: r.description ?? '',
      yield_text: r.yield_text ?? '',
      prep_minutes: r.prep_minutes ?? '',
      cook_minutes: r.cook_minutes ?? '',
      total_minutes: r.total_minutes ?? '',
      time_text: r.time_text ?? '',
      nutrition_text: r.nutrition_text ?? '',
      notes: r.notes ?? '',
      tags: [...r.tags],
      images: [...r.images],
      ingredients: lines(r.ingredients, 'raw'),
      steps: lines(r.steps, 'text'),
    };
    version = r.version;
    dirty = false;
  }

  function fromDraft(d) {
    const list = (a) => (a?.length ? [...a] : ['']);
    return {
      title: d.title ?? '',
      source_url: d.source_url ?? '',
      description: d.description ?? '',
      yield_text: d.yield_text ?? '',
      prep_minutes: d.prep_minutes ?? '',
      cook_minutes: d.cook_minutes ?? '',
      total_minutes: d.total_minutes ?? '',
      time_text: d.time_text ?? '',
      nutrition_text: d.nutrition_text ?? '',
      notes: d.notes ?? '',
      tags: [...(d.tags ?? [])],
      images: [...(d.images ?? [])],
      ingredients: list(d.ingredients),
      steps: list(d.steps),
    };
  }

  // Merge rules: the import replaces what the page says about the dish and
  // keeps what the household added. A new cover goes after the existing photos.
  // The imported version replaces what it has; anything the page didn't
  // carry (an empty yield, no description) keeps the household's own value.
  function mergeDraft(d) {
    const next = fromDraft(d);
    const blank = (v) => v == null || (typeof v === 'string' && !v.trim()) || (Array.isArray(v) && !v.some((x) => String(x).trim()));
    const kept = Object.fromEntries(Object.entries(next).map(([k, v]) => [k, blank(v) ? form[k] : v]));
    const cover = d.images?.[0];
    form = {
      ...kept,
      tags: form.tags,
      notes: form.notes,
      images: cover && !form.images.includes(cover) ? [...form.images, cover] : form.images,
    };
    dirty = true;
  }

  function startFromDraft() {
    if (!draft || id) return;
    form = fromDraft(draft);
    dirty = true;
    document.title = `Importing ${form.title || 'a recipe'} · Sous`;
  }
  startFromDraft();

  if (id) {
    api.recipe(id).then(
      (r) => {
        fill(r);
        if (merge && draft) mergeDraft(draft);
        loaded = true;
        document.title = `Editing ${r.title} · Sous`;
      },
      (err) => (problem = { message: err.message }),
    );
  }
  api.tags().then((t) => (allTags = t.items.map((x) => x.name)));

  // Parsed preview of each ingredient line, from the API's own parser.
  let parsed = $state([]);
  let parseTimer;
  $effect(() => {
    const lines = form.ingredients.map((l) => l);
    clearTimeout(parseTimer);
    parseTimer = setTimeout(() => {
      api.parseIngredients(lines.map((l) => l || ' ')).then((r) => (parsed = r.items)).catch(() => {});
    }, 180);
  });

  $effect(() => {
    const warn = (e) => dirty && e.preventDefault();
    addEventListener('beforeunload', warn);
    return () => removeEventListener('beforeunload', warn);
  });

  const touch = () => (dirty = true);

  // Line lists: Enter adds a line, Backspace on an empty line removes it,
  // and pasting several lines splits them.
  async function focusLine(list, i) {
    await tick();
    document.querySelector(`[data-list="${list}"][data-i="${i}"]`)?.focus();
  }
  function lineKey(list, i, e) {
    const lines = form[list];
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      lines.splice(i + 1, 0, '');
      touch();
      focusLine(list, i + 1);
    } else if (e.key === 'Backspace' && !lines[i] && lines.length > 1) {
      e.preventDefault();
      lines.splice(i, 1);
      touch();
      focusLine(list, Math.max(0, i - 1));
    }
  }
  function linePaste(list, i, e) {
    const text = e.clipboardData.getData('text');
    if (!text.includes('\n')) return;
    e.preventDefault();
    const pieces = text.split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
    const lines = form[list];
    lines.splice(i, lines[i] ? 0 : 1, ...pieces);
    touch();
    focusLine(list, i + pieces.length - 1);
  }
  function removeLine(list, i) {
    form[list].splice(i, 1);
    if (!form[list].length) form[list].push('');
    touch();
  }

  let tagDraft = $state('');
  function addTag(e) {
    e.preventDefault();
    const t = tagDraft.trim();
    if (t && !form.tags.some((x) => x.toLowerCase() === t.toLowerCase())) form.tags.push(t), touch();
    tagDraft = '';
  }

  let uploading = $state(false);
  async function addPhoto(e) {
    const file = e.currentTarget.files?.[0];
    if (!file) return;
    uploading = true;
    try {
      const { hash } = await asPerson(() => api.uploadImage(file));
      form.images = [hash, ...form.images.filter((h) => h !== hash)];
      touch();
    } catch (err) {
      problem = { message: err.message };
    } finally {
      uploading = false;
    }
  }

  const num = (v) => (v === '' || v == null ? null : Math.max(0, Math.round(+v)));

  function body() {
    return {
      title: form.title.trim(),
      source_url: form.source_url.trim() || null,
      description: form.description.trim() || null,
      yield_text: form.yield_text.trim() || null,
      prep_minutes: num(form.prep_minutes),
      cook_minutes: num(form.cook_minutes),
      total_minutes: num(form.total_minutes),
      time_text: form.time_text.trim() || null,
      nutrition_text: form.nutrition_text.trim() || null,
      notes: form.notes.trim() || null,
      tags: form.tags,
      images: form.images,
      ingredients: form.ingredients.map((l) => l.trim()).filter(Boolean),
      steps: form.steps.map((l) => l.trim()).filter(Boolean),
    };
  }

  async function save(e, overVersion) {
    e?.preventDefault();
    if (!form.title.trim()) {
      titleError = true;
      document.getElementById('title')?.focus();
      return;
    }
    saving = true;
    problem = null;
    try {
      const r = await asPerson(() => (id ? api.updateRecipe(id, overVersion ?? version, body()) : api.createRecipe(body())));
      if (!r) return;
      dirty = false;
      await refreshIndex();
      // An import's URL would fetch again, so it never stays in history.
      go(`/r/${r.id}`, { replace: !id || !!draft });
    } catch (err) {
      problem = err.status === 412 ? { conflict: err.body.current } : { message: err.message };
    } finally {
      saving = false;
    }
  }

  async function trash() {
    const ok = await asPerson(() => api.deleteRecipe(id, version).then(() => true));
    if (!ok) return;
    dirty = false;
    await refreshIndex();
    // Leaving the page clears the id prop, so Undo needs its own copy.
    const { title } = form;
    const trashed = id;
    go('/');
    toast(`${title} is in the trash.`, {
      label: 'Undo',
      run: async () => {
        await api.restoreRecipe(trashed);
        await refreshIndex();
      },
    });
  }

  const person = (u) => personName(u, session.people);
</script>

<form class="editor" onsubmit={save} oninput={touch} novalidate>
  <div class="toolbar">
    <a class="back" href={back} use:link><ArrowLeft size={16} aria-hidden="true" />{backLabel}</a>
    {#if !id && !draft}
      <a class="import-link" href="/import" use:link><Link2 size={16} aria-hidden="true" />Import from a link</a>
    {/if}
    <span class="state label" aria-live="polite">{saving ? 'Saving…' : dirty ? (draft && !merge ? 'Not saved yet' : 'Unsaved changes') : ''}</span>
    <a class="btn quiet" href={back} use:link>Cancel</a>
    <button class="btn primary" disabled={saving || !loaded}>{id ? 'Save recipe' : 'Add to the library'}</button>
  </div>

  {#if problem?.conflict}
    <div class="notice" role="alert">
      <p><strong>{person(problem.conflict.updated_by)} changed this recipe {ago(problem.conflict.updated_at)}</strong>, while you were editing. Saving now would replace their changes.</p>
      <div class="notice-acts">
        <button type="button" class="btn" onclick={() => save(null, problem.conflict.version)}>Save mine over theirs</button>
        <button type="button" class="btn quiet" onclick={() => (fill(problem.conflict), (problem = null))}>Discard mine, load theirs</button>
      </div>
    </div>
  {:else if problem}
    <div class="notice" role="alert"><p>{problem.message}</p></div>
  {/if}

  {#if loaded}
    <div class="sheet">
      <div class="main panel">
        <label class="visually-hidden" for="title">Title</label>
        <textarea id="title" class="title" rows="1" placeholder="Name the dish" bind:value={form.title} aria-invalid={titleError} oninput={() => (titleError = false)}></textarea>
        {#if titleError}<p class="field-error">A recipe needs a name.</p>{/if}

        <label class="field">
          <span class="label">Source</span>
          <input type="url" bind:value={form.source_url} placeholder="https://" />
        </label>
        <label class="field">
          <span class="label">Introduction</span>
          <textarea rows="2" bind:value={form.description} placeholder="A line or two about the dish (optional)"></textarea>
        </label>

        <section aria-labelledby="ing-h">
          <div class="col-head">
            <h2 id="ing-h" class="label">Ingredients</h2>
            <p class="hint">One per line. Start a line with # for a heading, like <em># For the sauce</em>.</p>
          </div>
          <ol class="lines">
            {#each form.ingredients as _, i}
              {@const p = parsed[i]}
              <li class:heading={form.ingredients[i].trim().startsWith('#')}>
                <input data-list="ingredients" data-i={i} bind:value={form.ingredients[i]} onkeydown={(e) => lineKey('ingredients', i, e)} onpaste={(e) => linePaste('ingredients', i, e)} placeholder={i === 0 ? '2 tablespoons Dijon mustard' : ''} aria-label="Ingredient {i + 1}" />
                <span class="parsed" aria-label="Read as">
                  {#if p && !p.section && form.ingredients[i].trim()}
                    <span class="p-amt fig" class:none={!amount(p)}>{amount(p) || 'no amount'}</span>
                    <span class="p-item">{p.item}</span>
                    {#if p.note}<span class="p-note">{p.note}</span>{/if}
                  {:else if p?.section}
                    <span class="p-note">heading</span>
                  {/if}
                </span>
                <button type="button" class="rm" onclick={() => removeLine('ingredients', i)} aria-label="Remove ingredient {i + 1}"><X size={16} /></button>
              </li>
            {/each}
          </ol>
          <button type="button" class="add" onclick={() => (form.ingredients.push(''), focusLine('ingredients', form.ingredients.length - 1))}>Add an ingredient</button>
        </section>

        <section aria-labelledby="method-h">
          <div class="col-head">
            <h2 id="method-h" class="label">Method</h2>
            <p class="hint">Enter starts a new step. Durations like <em>20 minutes</em> become timers when cooking.</p>
          </div>
          <ol class="lines steps">
            {#each form.steps as _, i}
              <li class:heading={form.steps[i].trim().startsWith('#')}>
                <span class="n fig" aria-hidden="true">{i + 1}</span>
                <textarea data-list="steps" data-i={i} rows="2" bind:value={form.steps[i]} onkeydown={(e) => lineKey('steps', i, e)} onpaste={(e) => linePaste('steps', i, e)} aria-label="Step {i + 1}"></textarea>
                <button type="button" class="rm" onclick={() => removeLine('steps', i)} aria-label="Remove step {i + 1}"><X size={16} /></button>
              </li>
            {/each}
          </ol>
          <button type="button" class="add" onclick={() => (form.steps.push(''), focusLine('steps', form.steps.length - 1))}>Add a step</button>
        </section>

        <label class="field">
          <span class="label">Notes</span>
          <textarea rows="4" bind:value={form.notes} placeholder="Substitutions, what worked, what to change next time"></textarea>
        </label>
      </div>

      <aside class="side panel">
        <div class="photo">
          <Plate hash={form.images[0]} title={form.title} />
          <label class="btn photo-btn">
            <ImagePlus aria-hidden="true" />{uploading ? 'Uploading…' : form.images.length ? 'Replace photo' : 'Add a photo'}
            <input type="file" accept="image/*" class="visually-hidden" onchange={addPhoto} />
          </label>
          {#if form.images.length}
            <button type="button" class="btn quiet" onclick={() => ((form.images = form.images.slice(1)), touch())}>Remove</button>
          {/if}
        </div>

        <label class="field">
          <span class="label">Yield</span>
          <input bind:value={form.yield_text} placeholder="Serves 4" />
        </label>
        <fieldset class="times">
          <legend class="label">Minutes</legend>
          <label><span>Prep</span><input type="number" min="0" inputmode="numeric" bind:value={form.prep_minutes} /></label>
          <label><span>Cook</span><input type="number" min="0" inputmode="numeric" bind:value={form.cook_minutes} /></label>
          <label><span>Total</span><input type="number" min="0" inputmode="numeric" bind:value={form.total_minutes} /></label>
        </fieldset>
        <label class="field">
          <span class="label">Time note</span>
          <input bind:value={form.time_text} placeholder="Plus overnight to chill" />
        </label>
        <label class="field">
          <span class="label">Nutrition</span>
          <textarea rows="2" bind:value={form.nutrition_text} placeholder="Per serving: 420 calories · 18 g protein"></textarea>
        </label>

        <div class="field">
          <label class="label" for="tag-input">Tags</label>
          <ul class="tags">
            {#each form.tags as t, i (t)}
              <li>{t}<button type="button" onclick={() => (form.tags.splice(i, 1), touch())} aria-label="Remove tag {t}"><X size={14} /></button></li>
            {/each}
          </ul>
          <input id="tag-input" list="all-tags" bind:value={tagDraft} placeholder="Add a tag" onkeydown={(e) => e.key === 'Enter' && addTag(e)} />
          <datalist id="all-tags">{#each allTags as t}<option value={t}></option>{/each}</datalist>
        </div>

        {#if id && !draft}
          <button type="button" class="trash" onclick={trash}><Trash2 size={16} aria-hidden="true" />Move to trash</button>
        {/if}
      </aside>
    </div>
  {/if}
</form>

<style>
  .editor {
    max-width: 80rem;
    margin: 0 auto;
    padding: 0 var(--gutter) var(--space-8);
  }
  .toolbar {
    position: sticky;
    top: var(--bar);
    z-index: 5;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) 0;
    background: var(--pass);
  }
  .back {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 40px;
    font-size: var(--ui);
    font-weight: 600;
    text-decoration: none;
    color: var(--ink-2);
  }
  .back:hover,
  .import-link:hover {
    color: var(--red);
  }
  .import-link {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 40px;
    margin-left: var(--space-3);
    padding-left: var(--space-4);
    border-left: 1.5px solid var(--perf);
    font-size: var(--ui);
    font-weight: 600;
    color: var(--ink-2);
    text-decoration: none;
  }
  .state {
    margin-left: auto;
    color: var(--ink-3);
  }
  .notice {
    margin-bottom: var(--space-4);
    padding: var(--space-4) var(--space-5);
    background: var(--ticket);
    border-top: 4px solid var(--red);
  }
  .notice-acts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }
  .sheet {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 20rem;
    gap: var(--space-4);
    align-items: start;
  }
  .panel {
    background: var(--ticket);
    padding: var(--space-5) var(--space-5) var(--space-6);
  }
  .main {
    border-top: 3px solid var(--ink);
  }
  .title {
    width: 100%;
    border: 0;
    border-bottom: 2px solid var(--ink);
    background: none;
    font-size: var(--title-xl);
    font-weight: 800;
    letter-spacing: -0.025em;
    line-height: 1.08;
    padding: var(--space-2) 0;
    resize: none;
    field-sizing: content;
    outline: none;
  }
  .title:focus,
  .title[aria-invalid='true'] {
    border-bottom-color: var(--red);
  }
  .field-error {
    color: var(--red);
    font-size: 0.875rem;
    font-weight: 600;
    margin-top: var(--space-2);
  }
  .field {
    display: grid;
    gap: var(--space-1);
    margin-top: var(--space-5);
  }
  .field > .label,
  .times legend {
    color: var(--ink-3);
  }
  .field input,
  .field textarea,
  .times input {
    width: 100%;
    border: 0;
    border-bottom: 1.5px solid var(--perf);
    background: none;
    padding: var(--space-2) 0;
    outline: none;
    font-size: var(--read);
  }
  .field textarea {
    resize: vertical;
    field-sizing: content;
    min-height: 3lh;
  }
  .field input:focus,
  .field textarea:focus,
  .times input:focus {
    border-bottom-color: var(--red);
  }

  section {
    margin-top: var(--space-7);
  }
  .col-head {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    align-items: baseline;
    gap: var(--space-1) var(--space-4);
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
  }
  .hint {
    font-size: 0.8125rem;
    color: var(--ink-3);
  }
  .hint em {
    font-style: normal;
    font-weight: 700;
    color: var(--ink-2);
  }
  .lines li {
    display: grid;
    grid-template-columns: minmax(0, 1.1fr) minmax(0, 1fr) 36px;
    gap: var(--space-4);
    align-items: center;
    border-bottom: 1.5px dashed var(--ticket-press);
  }
  .lines input,
  .lines textarea {
    width: 100%;
    border: 0;
    background: none;
    padding: var(--space-2) 0;
    font-size: var(--read);
    outline: none;
  }
  .lines li:focus-within {
    background: var(--ticket-press);
    box-shadow: -8px 0 0 var(--ticket-press), 8px 0 0 var(--ticket-press);
  }
  .lines li.heading input,
  .lines li.heading textarea {
    font-weight: 800;
  }
  .parsed {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 2px var(--space-2);
    font-size: 0.8125rem;
    color: var(--ink-3);
    min-height: 1.5em;
  }
  .p-amt {
    font-size: 1.0625rem;
    color: var(--ink);
  }
  .p-amt.none {
    font-stretch: 100%;
    font-weight: 500;
    font-size: 0.8125rem;
    color: var(--ink-3);
  }
  .p-item {
    color: var(--ink);
    font-weight: 600;
  }
  .rm {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border: 0;
    border-radius: 3px;
    background: none;
    color: var(--ink-3);
    opacity: 0;
  }
  .lines li:hover .rm,
  .lines li:focus-within .rm,
  .rm:focus-visible {
    opacity: 1;
  }
  .rm:hover {
    color: var(--red);
  }
  .steps li {
    grid-template-columns: 2.25rem minmax(0, 1fr) 36px;
    align-items: start;
  }
  .steps .n {
    font-size: 1.375rem;
    padding-top: var(--space-2);
  }
  .steps textarea {
    resize: none;
    field-sizing: content;
    line-height: var(--read-lh);
  }
  .steps .rm {
    margin-top: 4px;
  }
  .add {
    margin-top: var(--space-2);
    border: 0;
    background: none;
    min-height: 40px;
    padding: 0;
    font-size: 0.875rem;
    font-weight: 700;
    color: var(--red);
  }
  .add:hover {
    text-decoration: underline;
    text-underline-offset: 0.2em;
  }

  .side {
    position: sticky;
    top: calc(var(--bar) + 68px);
    display: grid;
    align-content: start;
  }
  .photo {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .photo-btn {
    flex: 1;
  }
  .times {
    border: 0;
    padding: 0;
    margin: var(--space-5) 0 0;
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: var(--space-4);
  }
  .times legend {
    padding: 0;
    margin-bottom: var(--space-1);
  }
  .times label span {
    font-size: 0.8125rem;
    font-weight: 600;
    color: var(--ink-2);
  }
  .times input {
    font-stretch: 62%;
    font-weight: 800;
    font-size: 1.5rem;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin: var(--space-1) 0;
  }
  .tags li {
    display: inline-flex;
    align-items: center;
    padding-left: var(--space-3);
    border: 2px solid var(--ink);
    border-radius: 3px;
    font-size: 0.875rem;
    font-weight: 700;
  }
  .tags button {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border: 0;
    background: none;
  }
  .tags button:hover {
    color: var(--red);
  }
  .trash {
    justify-self: start;
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 40px;
    margin-top: var(--space-6);
    border: 0;
    background: none;
    padding: 0;
    font-size: 0.875rem;
    font-weight: 700;
    color: var(--red);
  }
  .trash:hover {
    text-decoration: underline;
    text-underline-offset: 0.2em;
  }

  @media (max-width: 960px) {
    .sheet {
      grid-template-columns: 1fr;
    }
    .side {
      position: static;
      order: -1;
    }
    .main,
    .side {
      padding-left: var(--space-4);
      padding-right: var(--space-4);
    }
    .lines li {
      grid-template-columns: minmax(0, 1fr) 36px;
      gap: 0 var(--space-2);
    }
    .lines li .parsed {
      grid-column: 1;
      grid-row: 2;
      padding-bottom: var(--space-2);
    }
    .steps li {
      grid-template-columns: 2rem minmax(0, 1fr) 36px;
    }
    .rm {
      opacity: 1;
    }
  }
  @media (max-width: 760px) {
    .toolbar {
      top: 0;
      position: relative;
      flex-wrap: wrap;
    }
    .import-link {
      margin-left: 0;
      padding-left: var(--space-3);
    }
    .state:empty {
      display: none;
    }
    .state:empty + .btn {
      margin-left: auto;
    }
    .toolbar .primary {
      flex: 1 0 auto;
    }
  }
</style>
