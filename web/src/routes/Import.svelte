<script>
  // /import: read a recipe from a link (or pasted text) into the editor
  // for a person to check before it is saved (spec.md §14). ?url=, ?text= and
  // ?title= start an import straight away, for the bookmarklet, the Shortcut
  // and a future share target. App remounts this page when the query changes.
  import { ArrowLeft } from '@lucide/svelte';
  import { route, go, link } from '../lib/router.svelte.js';
  import { api } from '../lib/api.js';
  import { session, asPerson, refreshIndex, toast } from '../lib/session.svelte.js';
  import Editor from './Editor.svelte';
  import ImportBanner from '../components/ImportBanner.svelte';
  import SendToSous from '../components/SendToSous.svelte';

  const asked = (() => {
    const p = route.search;
    const url = p.get('url')?.trim() ?? '';
    const text = p.get('text')?.trim() ?? '';
    const title = p.get('title')?.trim() ?? '';
    // Share sheets often put the link in text.
    if (!url && /^https?:\/\/\S+$/.test(text)) return { url: text, text: '', title };
    return { url, text, title };
  })();

  let phase = $state(asked.url || asked.text ? 'loading' : 'form'); // form | loading | preview | error
  let preview = $state(null);
  let failure = $state(null); // { code, message, page, url }
  let choice = $state(null);
  let slow = $state(false);

  let linkDraft = $state(asked.url);
  let linkError = $state('');
  let pasting = $state(false); // the paste box on the blank form
  let paste = $state(asked.url ? '' : asked.text);
  let pasteTitle = $state(asked.title);
  let pasteError = $state('');
  let reading = $state(false);

  const hostOf = (u) => {
    try {
      return new URL(u).hostname.replace(/^www\./, '');
    } catch {
      return '';
    }
  };

  let alive = true;
  $effect(() => () => (alive = false));

  async function run(req) {
    phase = 'loading';
    slow = false;
    const timer = setTimeout(() => (slow = true), 5000);
    try {
      const p = await asPerson(() => api.importPreview(req));
      if (!alive) return;
      if (!p) {
        phase = 'form';
        return;
      }
      show(p);
    } catch (err) {
      if (!alive) return;
      failure = { code: err.status === 422 ? err.code : 'other', message: err.message, page: err.body?.page ?? null, url: req.url };
      pasteTitle ||= failure.page?.title ?? '';
      phase = 'error';
    } finally {
      clearTimeout(timer);
    }
  }

  function show(p) {
    preview = p;
    choice = p.duplicates?.length ? null : { mode: 'new' };
    phase = 'preview';
    document.title = `Importing ${p.draft.title || 'a recipe'} · Sous`;
  }

  // Wait for boot, so a saved cook is known before asking who's cooking.
  let started = false;
  $effect(() => {
    if (!session.loaded || started || phase !== 'loading') return;
    started = true;
    if (asked.url) run({ url: asked.url });
    else readPaste(null, asked.text, asked.title);
  });

  function fetchLink(e) {
    e.preventDefault();
    let u = linkDraft.trim();
    if (!u) {
      linkError = 'Paste a link to a recipe page.';
      return;
    }
    if (!/^[a-z][a-z0-9+.-]*:/i.test(u)) u = `https://${u}`;
    go(`/import?url=${encodeURIComponent(u)}`);
  }

  // Pasted text is split server-side. After a failed link, the link goes
  // along so the server credits it and checks for duplicates; it isn't
  // fetched again.
  async function readPaste(e, text = paste, title = pasteTitle) {
    e?.preventDefault();
    if (!text.trim()) {
      pasteError = 'Paste the ingredients and method first.';
      return;
    }
    reading = true;
    pasteError = '';
    try {
      const source = failure?.page?.source_url || failure?.url;
      const req = { text, ...(/^https?:\/\//i.test(source ?? '') ? { url: source } : {}), ...(title.trim() ? { title: title.trim() } : {}) };
      const p = await asPerson(() => api.importPreview(req));
      if (!alive || !p) {
        if (phase === 'loading') phase = 'form';
        return;
      }
      show(p);
    } catch (err) {
      if (!alive) return;
      pasteError = err.status === 422 ? "Sous couldn't find ingredients or steps in that text. Put each on its own line, under headings like Ingredients and Method." : err.message;
      if (phase === 'loading') ((phase = 'form'), (pasting = true), (paste = text));
    } finally {
      reading = false;
    }
  }

  async function restore(d) {
    let ok;
    try {
      ok = await asPerson(() => api.restoreRecipe(d.id).then(() => true));
    } catch (err) {
      toast(`${d.title} couldn’t be restored: ${err.message}`);
      return;
    }
    if (!ok) return;
    d.deleted = false;
    await refreshIndex();
    toast(`${d.title} is back in the library.`);
  }

  // Only http(s) links are ever made clickable; ?url= is outside input.
  const webLink = (u) => /^https?:\/\//i.test(u);

  const failHost = $derived(hostOf(failure?.page?.source_url ?? failure?.url ?? '') || 'The site');
  const copy = $derived.by(() => {
    const h = failHost;
    switch (failure?.code) {
      case 'blocked':
        return { title: `${h} turned Sous away.`, body: 'The site blocks automatic readers, so Sous can’t fetch the page. Open it in your browser, copy the recipe and paste it below.' };
      case 'paywalled':
        return { title: 'This one is for subscribers.', body: `${h} only shows the recipe to people signed in to it. If you subscribe, open the page, copy the ingredients and method, and paste them below.` };
      case 'not_found':
        return { title: 'That page isn’t there.', body: `${h} has nothing at that address, or the address doesn’t exist. Check the link for a typo; if the site is gone, an archived copy on web.archive.org often works.` };
      case 'no_recipe_found':
        return { title: 'No recipe on that page.', body: 'Sous read the page but found no recipe data in it. Many sites keep the recipe in a card further down: copy it from there and paste it below.' };
      case 'timeout':
        return { title: `${h} took too long to answer.`, body: 'The page didn’t arrive in time. Try again in a minute, or paste the recipe below.', retry: true };
      case 'too_large':
        return { title: 'That page is too big to read.', body: 'Sous stops reading pages over 5 MB. Copy the recipe from the page and paste it below.' };
      case 'unsupported_url':
        return { title: 'Sous can’t read that link.', body: failure.message || 'Links need to start with http:// or https://, and Sous won’t fetch addresses on your own network.' };
      case 'site_error':
        return { title: `${h} is having trouble.`, body: 'The site answered with an error of its own. That usually clears up soon; try again shortly, or paste the recipe below.', retry: true };
      default:
        return { title: 'The import didn’t work.', body: failure?.message ?? '', retry: true };
    }
  });
  let photoFailed = $state(false);
</script>

{#snippet pasteBox(intro, primary)}
  <form class="paste panel" onsubmit={readPaste} novalidate aria-labelledby="paste-h">
    <h2 id="paste-h">Paste the recipe</h2>
    <p class="intro">{intro}</p>
    <label class="field">
      <span class="label">Title</span>
      <input bind:value={pasteTitle} placeholder="Name the dish" />
    </label>
    <label class="field">
      <span class="label">Recipe text</span>
      <textarea rows="10" bind:value={paste} aria-invalid={!!pasteError} oninput={() => (pasteError = '')} placeholder={'Ingredients\n2 cups flour\n1 teaspoon salt\n\nMethod\nWhisk the flour and salt together…'}></textarea>
    </label>
    {#if pasteError}<p class="field-error" role="alert">{pasteError}</p>{/if}
    <div class="row-end">
      <button class="btn" class:primary disabled={reading}>{reading ? 'Reading…' : 'Read the recipe'}</button>
    </div>
  </form>
{/snippet}

{#if phase === 'preview' && preview}
  <div class="banner-wrap">
    <ImportBanner {preview} {choice} onchoose={(c) => (choice = c)} onrestore={restore} />
  </div>
  {#if choice}
    {#key `${choice.mode}:${choice.id ?? ''}`}
      {#if choice.mode === 'merge'}
        <Editor id={choice.id} draft={preview.draft} merge />
      {:else}
        <Editor id={null} draft={preview.draft} />
      {/if}
    {/key}
  {:else}
    <div class="choose-wrap"><a class="btn quiet" href="/import" use:link><ArrowLeft aria-hidden="true" />Import something else</a></div>
  {/if}
{:else}
  <main class="import">
    {#if phase === 'loading'}
      <div class="loading panel" aria-busy="true">
        <p class="reading" aria-live="polite">
          {#if asked.url}Reading <strong>{hostOf(asked.url) || asked.url}</strong>…{:else}Reading your text…{/if}
        </p>
        {#if asked.url}<p class="url">{asked.url}</p>{/if}
        <div class="progress" aria-hidden="true"><span></span></div>
        <p class="slow" aria-live="polite">{slow ? 'Still going. Some sites are slow, and photos take a moment; Sous gives up after 25 seconds.' : ''}</p>
        <a class="btn quiet" href="/import" use:link>Cancel</a>
      </div>
    {:else if phase === 'error'}
      <div class="failed panel" role="alert">
        <div class="why">
          <h1>{copy.title}</h1>
          <p>{copy.body}</p>
          {#if failure.url}<p class="url">{#if webLink(failure.url)}<a href={failure.url} target="_blank" rel="noreferrer">{failure.url}</a>{:else}{failure.url}{/if}</p>{/if}
          <div class="acts">
            {#if copy.retry && failure.url}<button class="btn" onclick={() => run({ url: failure.url })}>Try again</button>{/if}
            <a class="btn quiet" href="/import" use:link>Try another link</a>
          </div>
        </div>
        {#if failure.page?.title || (failure.page?.image && !photoFailed)}
          <div class="found">
            <p class="label">From the page</p>
            {#if failure.page.image && !photoFailed}
              <img src={failure.page.image.url} alt="" referrerpolicy="no-referrer" onerror={() => (photoFailed = true)} />
            {/if}
            {#if failure.page.title}<p class="found-title">{failure.page.title}</p>{/if}
          </div>
        {/if}
      </div>
      {@render pasteBox('Copy the ingredients and method from the page and paste them here. Sous splits them into lines, and you check the result before anything is saved.', true)}
    {:else}
      <h1>Import a recipe</h1>
      <p class="lede">Give Sous a link to a recipe page. It reads the recipe and its photo off the page, then opens it here so you can check it before it goes in the library.</p>
      <form class="fetch panel" onsubmit={fetchLink} novalidate>
        <label class="label" for="import-url">Recipe link</label>
        <div class="row">
          <input id="import-url" type="url" inputmode="url" autocomplete="url" bind:value={linkDraft} oninput={() => (linkError = '')} aria-invalid={!!linkError} placeholder="https://" />
          <button class="btn primary">Fetch</button>
        </div>
        {#if linkError}<p class="field-error" role="alert">{linkError}</p>{/if}
        {#if !pasting}
          <button type="button" class="text-btn" onclick={() => (pasting = true)}>No link? Paste the recipe text instead</button>
        {/if}
      </form>
      {#if pasting}
        {@render pasteBox('Ingredients and method, as copied from a page, an email or a note. Headings like Ingredients and Method help Sous split them.', false)}
      {/if}
      <SendToSous />
    {/if}
  </main>
{/if}

<style>
  .import {
    max-width: 56rem;
    margin: 0 auto;
    padding: var(--space-6) var(--gutter) var(--space-8);
  }
  h1 {
    font-size: var(--title-l);
    font-weight: 800;
  }
  .lede {
    margin-top: var(--space-2);
    color: var(--ink-2);
    max-width: 60ch;
  }
  .panel {
    background: var(--ticket);
    border-top: 3px solid var(--ink);
    padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .fetch {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-5);
  }
  .fetch > .label {
    color: var(--ink-3);
  }
  .row {
    display: flex;
    gap: var(--space-3);
    align-items: center;
  }
  .row input {
    flex: 1;
    min-width: 0;
    height: 44px;
    border: 0;
    border-bottom: 2px solid var(--ink);
    background: none;
    font-size: var(--read);
    outline: none;
  }
  .row input:focus,
  .row input[aria-invalid='true'] {
    border-bottom-color: var(--red);
  }
  .text-btn {
    justify-self: start;
    min-height: 40px;
    margin-top: var(--space-1);
    padding: 0;
    border: 0;
    background: none;
    font-size: 0.875rem;
    font-weight: 700;
    color: var(--ink-2);
    text-decoration: underline;
    text-decoration-color: var(--perf);
    text-underline-offset: 0.2em;
    text-decoration-thickness: 1.5px;
  }
  .text-btn:hover {
    color: var(--red);
    text-decoration-color: var(--red);
  }
  .field-error {
    color: var(--red);
    font-size: 0.875rem;
    font-weight: 600;
  }

  .loading {
    display: grid;
    gap: var(--space-3);
    justify-items: start;
  }
  .reading {
    font-size: var(--title-l);
    font-weight: 800;
    letter-spacing: -0.015em;
    line-height: 1.15;
  }
  .url {
    color: var(--ink-3);
    font-size: 0.875rem;
    overflow-wrap: anywhere;
  }
  .url a {
    color: inherit;
  }
  .progress {
    justify-self: stretch;
    height: 3px;
    overflow: hidden;
    background: var(--ticket-press);
  }
  .progress span {
    display: block;
    width: 30%;
    height: 100%;
    background: var(--ink);
    animation: slide 1.4s var(--ease-out) infinite;
  }
  @keyframes slide {
    from {
      transform: translateX(-100%);
    }
    to {
      transform: translateX(340%);
    }
  }
  .slow {
    color: var(--ink-2);
    min-height: 1.55em;
  }

  .failed {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-5);
    justify-content: space-between;
  }
  .why {
    display: grid;
    gap: var(--space-3);
    flex: 1 1 26rem;
    align-content: start;
  }
  .why > p:not(.url) {
    max-width: 60ch;
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }
  .acts > .quiet:first-child {
    margin-left: calc(var(--space-4) * -1);
  }
  .found {
    flex: 0 1 14rem;
    display: grid;
    gap: var(--space-2);
    align-content: start;
  }
  .found .label {
    color: var(--ink-3);
    padding-bottom: var(--space-1);
    border-bottom: 1.5px dashed var(--perf);
  }
  .found img {
    width: 100%;
    aspect-ratio: 4 / 3;
    object-fit: cover;
    background: var(--pass-deep);
  }
  .found-title {
    font-size: var(--title-m);
    font-weight: 700;
    line-height: 1.2;
  }

  .paste {
    display: grid;
    margin-top: var(--space-4);
  }
  .paste h2 {
    font-size: var(--title-m);
    font-weight: 800;
  }
  .intro {
    margin-top: var(--space-2);
    color: var(--ink-2);
    max-width: 60ch;
  }
  .field {
    display: grid;
    gap: var(--space-1);
    margin-top: var(--space-5);
  }
  .field > .label {
    color: var(--ink-3);
  }
  .field input,
  .field textarea {
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
    min-height: 12lh;
    line-height: var(--read-lh);
  }
  .field input:focus,
  .field textarea:focus,
  .field textarea[aria-invalid='true'] {
    border-bottom-color: var(--red);
  }
  .paste .field-error {
    margin-top: var(--space-2);
  }
  .row-end {
    display: flex;
    justify-content: flex-end;
    margin-top: var(--space-4);
  }

  .banner-wrap,
  .choose-wrap {
    max-width: 80rem;
    margin: 0 auto;
    padding: var(--space-4) var(--gutter) 0;
  }
  @media (max-width: 760px) {
    .import {
      padding-top: var(--space-5);
    }
    .panel {
      padding: var(--space-4);
    }
    .row {
      flex-direction: column;
      align-items: stretch;
    }
    .row-end .btn {
      width: 100%;
    }
  }
</style>
