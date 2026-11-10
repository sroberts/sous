<script>
  import { tick } from 'svelte';
  import { ArrowLeft, X, ImagePlus, Trash2, Link2 } from '@lucide/svelte';
  import { go, link, setLeaveGuard } from '../lib/router.svelte.js';
  import { api } from '../lib/api.js';
  import { session, asPerson, refreshIndex, toast } from '../lib/session.svelte.js';
  import { getRecipe } from '../lib/library.js';
  import { isOffline } from '../lib/net.svelte.js';
  import { amount, ago, personName, readTime, writeTime } from '../lib/format.js';
  import Plate from '../components/Plate.svelte';
  import { stickIfFits } from '../lib/stick.js';

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
  // The page's heading, for screen readers: the router focuses it on arrival.
  let loadedTitle = $state('');
  const heading = $derived(id ? loadedTitle && `Editing ${loadedTitle}` : 'New recipe');

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
      prep_minutes: writeTime(r.prep_minutes),
      cook_minutes: writeTime(r.cook_minutes),
      total_minutes: writeTime(r.total_minutes),
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
      prep_minutes: writeTime(d.prep_minutes),
      cook_minutes: writeTime(d.cook_minutes),
      total_minutes: writeTime(d.total_minutes),
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
    getRecipe(id).then(
      (r) => {
        fill(r);
        if (merge && draft) mergeDraft(draft);
        loaded = true;
        loadedTitle = r.title;
        document.title = `Editing ${r.title} · Sous`;
      },
      (err) => (problem = { message: err.message }),
    );
  }
  // Suggestions only: without them, tags are typed in full.
  api.tags().then((t) => (allTags = t.items.map((x) => x.name))).catch(() => {});

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

  // Leaving in the app (Cancel, a link, Back) with unsaved changes asks first,
  // in the toolbar. The guard reads dirty when asked, so a save or trash that
  // clears it and then navigates is never stopped.
  let leaving = $state(null); // the router's proceed, while asking
  let keepBtn = $state();
  let askedAt = 0;
  let askedFrom = null; // where focus was when the question took it
  $effect(() => {
    if (!dirty) return;
    return setLeaveGuard((proceed) => {
      if (!dirty) return true;
      leaving = proceed;
      askedAt = performance.now();
      askedFrom = document.activeElement;
      tick().then(() => keepBtn?.focus());
      return false;
    });
  });
  function discard() {
    // Discard sits near where Cancel was: a double click on Cancel must not
    // answer the question it just asked.
    if (performance.now() - askedAt < 500) return;
    const proceed = leaving;
    leaving = null;
    dirty = false;
    proceed?.();
  }
  // Back to where you were; if that was Cancel, which the question replaced,
  // to the Cancel that returns in its place.
  async function keepEditing() {
    leaving = null;
    await tick();
    const back = askedFrom?.isConnected && askedFrom !== document.body ? askedFrom : document.querySelector('.acts .cancel');
    back?.focus();
  }

  function onkey(e) {
    if ((e.metaKey || e.ctrlKey) && !e.altKey && e.key.toLowerCase() === 's') {
      e.preventDefault();
      if (!saving && loaded) save();
    } else if (e.key === 'Escape' && leaving) {
      keepEditing();
    }
  }

  // Step numbers count steps only; a # heading line takes no number, as on
  // the recipe page and in cook mode.
  const stepNumbers = $derived.by(() => {
    let n = 0;
    return form.steps.map((l) => (l.trim().startsWith('#') ? null : ++n));
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
    // An ingredient is one line, so Shift+Enter starts a new one too; a step
    // can hold a line break.
    if (e.key === 'Enter' && (!e.shiftKey || list === 'ingredients')) {
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

  // A time that doesn't read stays as typed, marked, and holds up the save.
  const TIMES = ['prep_minutes', 'cook_minutes', 'total_minutes'];
  const badTime = (k) => Number.isNaN(readTime(form[k]));
  // Said once the field is left (or a save is tried), not mid-typing "1:".
  let timesLeft = $state([]);
  const timesBad = $derived(TIMES.some((k) => timesLeft.includes(k) && badTime(k)));
  const tidyTime = (k) => {
    if (!timesLeft.includes(k)) timesLeft = [...timesLeft, k];
    const m = readTime(form[k]);
    if (m != null && !Number.isNaN(m) && writeTime(m) !== form[k]) form[k] = writeTime(m);
  };

  function body() {
    return {
      title: form.title.trim(),
      source_url: form.source_url.trim() || null,
      description: form.description.trim() || null,
      yield_text: form.yield_text.trim() || null,
      prep_minutes: readTime(form.prep_minutes),
      cook_minutes: readTime(form.cook_minutes),
      total_minutes: readTime(form.total_minutes),
      time_text: form.time_text.trim() || null,
      nutrition_text: form.nutrition_text.trim() || null,
      notes: form.notes.trim() || null,
      tags: form.tags,
      images: form.images,
      ingredients: form.ingredients.flatMap((l) => l.split(/\r?\n/)).map((l) => l.trim()).filter(Boolean),
      steps: form.steps.map((l) => l.trim()).filter(Boolean),
    };
  }

  async function save(e, overVersion) {
    e?.preventDefault();
    leaving = null;
    if (!form.title.trim()) {
      titleError = true;
      document.getElementById('title')?.focus();
      return;
    }
    const bad = TIMES.find(badTime);
    if (bad) {
      timesLeft = TIMES;
      document.getElementById(bad)?.focus();
      return;
    }
    saving = true;
    problem = null;
    try {
      const r = await asPerson(() => (id ? api.updateRecipe(id, overVersion ?? version, body()) : api.createRecipe(body())));
      if (!r) return;
      dirty = false;
      await refreshIndex();
      // Saved: leave even if a key was pressed while the index refreshed, so
      // the leave guard can't hold the page on a version (or, for a new
      // recipe, a create) that has already gone through.
      dirty = false;
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
    dirty = false; // trashed: leave even if a key was pressed meanwhile
    go('/');
    toast(`${title} is in the trash.`, {
      label: 'Undo',
      run: async () => {
        try {
          await api.restoreRecipe(trashed);
          await refreshIndex();
        } catch (err) {
          toast(`${title} couldn’t be restored: ${err.message}`);
        }
      },
    });
  }

  const person = (u) => personName(u, session.people);
</script>

<svelte:window onkeydown={onkey} />

<form class="editor" onsubmit={save} oninput={touch} novalidate>
  <!-- An import has its own heading in the banner above. -->
  {#if heading && !draft}<h1 class="visually-hidden">{heading}</h1>{/if}
  <div class="toolbar">
    <a class="back" href={back} use:link><ArrowLeft size={16} aria-hidden="true" />{backLabel}</a>
    {#if !id && !draft}
      <a class="import-link" href="/import" use:link><Link2 size={16} aria-hidden="true" />Import from a link</a>
    {/if}
    <!-- On phones these become a bar along the bottom, so Save stays in reach. -->
    <div class="acts">
      {#if leaving}
        <div class="leave" role="alertdialog" aria-labelledby="leave-q">
          <p id="leave-q">Discard your changes?</p>
          <!-- Keep editing takes Save's place, so a habitual click there is safe. -->
          <button type="button" class="btn quiet" onclick={discard}>Discard</button>
          <button type="button" class="btn keep" bind:this={keepBtn} onclick={keepEditing}>Keep editing</button>
        </div>
      {:else}
        <span class="state label" aria-live="polite">{saving ? 'Saving…' : dirty ? (draft && !merge ? 'Not saved yet' : 'Unsaved changes') : ''}</span>
        <!-- The form stays open offline; only saving waits for a connection. -->
        {#if isOffline()}<span class="offline" role="status">You’re offline. Saving needs a connection.</span>{/if}
        <a class="btn quiet cancel" href={back} use:link>Cancel</a>
        <button class="btn primary" disabled={saving || !loaded || isOffline()}>{id ? 'Save recipe' : 'Add to the library'}</button>
      {/if}
    </div>
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
        <textarea id="title" class="title" rows="1" placeholder="Name the dish" bind:value={form.title} aria-invalid={titleError} aria-describedby={titleError ? 'title-error' : undefined} oninput={() => (titleError = false)}></textarea>
        {#if titleError}<p id="title-error" class="field-error">A recipe needs a name.</p>{/if}

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
                <textarea data-list="ingredients" data-i={i} rows="1" bind:value={form.ingredients[i]} onkeydown={(e) => lineKey('ingredients', i, e)} onpaste={(e) => linePaste('ingredients', i, e)} placeholder={i === 0 ? 'e.g. an amount, then the ingredient' : ''} aria-label="Ingredient {i + 1}" aria-describedby="parsed-{i}"></textarea>
                <span class="parsed" id="parsed-{i}">
                  {#if p && !p.section && form.ingredients[i].trim()}
                    <span class="visually-hidden">Read as</span>
                    <span class="p-amt fig" class:none={!amount(p)}>{amount(p) || 'no amount'}</span>
                    <span class="p-item">{p.item}</span>
                    {#if p.note}<span class="p-note">{p.note}</span>{/if}
                  {:else if p?.section}
                    <span class="visually-hidden">Read as a heading</span><span class="p-note" aria-hidden="true">heading</span>
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
              {@const n = stepNumbers[i]}
              <li class:heading={n === null}>
                <span class="n fig" aria-hidden="true">{n ?? ''}</span>
                <textarea data-list="steps" data-i={i} rows="2" bind:value={form.steps[i]} onkeydown={(e) => lineKey('steps', i, e)} onpaste={(e) => linePaste('steps', i, e)} aria-label={n === null ? 'Heading' : `Step ${n}`}></textarea>
                <button type="button" class="rm" onclick={() => removeLine('steps', i)} aria-label={n === null ? 'Remove heading' : `Remove step ${n}`}><X size={16} /></button>
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

      <aside class="side panel" use:stickIfFits={84}>
        <div class="photo">
          <Plate hash={form.images[0]} title={form.title} />
          <label class="btn photo-btn" class:off={isOffline()}>
            <ImagePlus aria-hidden="true" />{uploading ? 'Uploading…' : form.images.length ? 'Replace photo' : 'Add a photo'}
            <input type="file" accept="image/*" class="visually-hidden" disabled={isOffline()} onchange={addPhoto} />
          </label>
          {#if form.images.length}
            <button type="button" class="btn quiet" onclick={() => ((form.images = form.images.slice(1)), touch())}>Remove</button>
          {/if}
        </div>

        <label class="field">
          <span class="label">Yield</span>
          <input bind:value={form.yield_text} placeholder="e.g. how many it serves or makes" />
        </label>
        <fieldset class="times">
          <legend class="label">Times</legend>
          {#each [['prep_minutes', 'Prep'], ['cook_minutes', 'Cook'], ['total_minutes', 'Total']] as [k, name] (k)}
            <label><span>{name}</span><input id={k} bind:value={form[k]} onblur={() => tidyTime(k)} placeholder="0:00" autocomplete="off" aria-invalid={timesLeft.includes(k) && badTime(k)} aria-describedby="times-hint" /></label>
          {/each}
          <p class="times-hint" class:hint={!timesBad} class:field-error={timesBad} id="times-hint">{timesBad ? 'Write a time as minutes (90) or hours and minutes (1:30).' : 'Minutes, or hours and minutes: 90 or 1:30.'}</p>
        </fieldset>
        <label class="field">
          <span class="label">Time note</span>
          <input bind:value={form.time_text} placeholder="e.g. resting, rising or chilling time" />
        </label>
        <label class="field">
          <span class="label">Nutrition</span>
          <textarea rows="2" bind:value={form.nutrition_text} placeholder="e.g. calories and protein per serving"></textarea>
        </label>

        <div class="field">
          <label class="label" for="tag-input">Tags</label>
          <ul class="tags">
            {#each form.tags as t, i (t)}
              <li>{t}<button type="button" onclick={() => (form.tags.splice(i, 1), touch())} aria-label="Remove tag {t}"><X size={14} /></button></li>
            {/each}
          </ul>
          <input id="tag-input" list="all-tags" bind:value={tagDraft} placeholder="Add a tag" aria-describedby="tag-hint" onkeydown={(e) => e.key === 'Enter' && addTag(e)} />
          <datalist id="all-tags">{#each allTags as t}<option value={t}></option>{/each}</datalist>
          <p class="hint tag-hint" id="tag-hint">The first tag is the recipe’s course in the library, like <em>Main</em> or <em>Dessert</em>.</p>
        </div>

        {#if id && !draft}
          <button type="button" class="trash" disabled={isOffline()} onclick={trash}><Trash2 size={16} aria-hidden="true" />Move to trash</button>
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
  .acts {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-left: auto;
  }
  .state {
    color: var(--ink-3);
  }
  .leave {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
  }
  .leave p {
    margin-right: var(--space-2);
    font-size: var(--ui);
    font-weight: 700;
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
    border-bottom: 1.5px solid var(--field);
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
  .tag-hint {
    margin-top: var(--space-2);
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
  .lines textarea {
    width: 100%;
    border: 0;
    background: none;
    padding: var(--space-2) 0;
    font-size: var(--read);
    line-height: var(--read-lh);
    outline: none;
    resize: none;
    field-sizing: content;
  }
  .lines li:focus-within {
    background: var(--ticket-press);
    box-shadow: -8px 0 0 var(--ticket-press), 8px 0 0 var(--ticket-press);
  }
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
  /* No hover on a touch screen: the remove buttons always show. */
  @media (hover: none) {
    .rm {
      opacity: 1;
    }
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

  /* The panel sticks beside the recipe only while all of it fits, so Move to
     trash at its foot is never out of reach. */
  .side {
    display: grid;
    align-content: start;
  }
  .side:global(.stuck) {
    position: sticky;
    top: calc(var(--bar) + 68px);
  }
  .photo {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .photo-btn {
    flex: 1;
  }
  .photo-btn.off {
    border-color: var(--perf);
    color: var(--ink-3);
    background: transparent;
    cursor: not-allowed;
  }
  .offline {
    font-size: var(--ui);
    font-weight: 600;
    color: var(--ink-2);
  }
  .times {
    border: 0;
    padding: 0;
    margin: var(--space-5) 0 0;
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: var(--space-4);
  }
  .times input[aria-invalid='true'] {
    border-bottom-color: var(--red);
  }
  .times-hint {
    grid-column: 1 / -1;
    margin-top: calc(var(--space-2) * -1);
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
    color: var(--ink-2);
  }
  .trash:disabled {
    color: var(--ink-3);
    cursor: not-allowed;
  }
  .trash:not(:disabled):hover,
  .trash:focus-visible {
    color: var(--red);
    text-decoration: underline;
    text-underline-offset: 0.2em;
  }

  @media (max-width: 960px) {
    .sheet {
      grid-template-columns: 1fr;
    }
    .side:global(.stuck) {
      position: static;
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
    .editor {
      padding-bottom: calc(var(--space-8) + 68px);
    }
    .toolbar {
      top: 0;
      position: relative;
      flex-wrap: wrap;
    }
    .import-link {
      margin-left: 0;
      padding-left: var(--space-3);
    }
    .acts {
      position: fixed;
      left: 0;
      right: 0;
      bottom: 0;
      z-index: 10;
      flex-wrap: wrap;
      padding: var(--space-3) var(--gutter) max(var(--space-3), env(safe-area-inset-bottom));
      background: var(--ticket);
      border-top: 1px solid var(--perf);
    }
    .state:empty {
      display: none;
    }
    .state:empty + .btn {
      margin-left: auto;
    }
    .acts .primary {
      flex: 1 0 auto;
    }
    .offline {
      flex: 1 0 100%;
    }
    .leave {
      flex: 1;
    }
    /* As Cancel and Save were: Discard beside the question, Keep editing
       across the full width where Save was. */
    .leave p {
      flex: 1 1 auto;
    }
    .leave .keep {
      flex: 1 0 100%;
    }
  }
</style>
