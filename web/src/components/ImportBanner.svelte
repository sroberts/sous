<script>
  // What an import read and how, above the editor that holds the draft. With
  // an existing recipe from the same link, it asks what to do first.
  import { ExternalLink, CircleDashed, TriangleAlert, Archive } from '@lucide/svelte';
  import { link } from '../lib/router.svelte.js';

  // choice: { mode: 'new' } | { mode: 'merge', id } | null (not chosen yet)
  let { preview, choice = null, onchoose, onrestore } = $props();

  const report = $derived(preview.report);
  const draft = $derived(preview.draft);
  const host = $derived(hostOf(report.source_url || draft.source_url));
  const count = (lines) => (lines ?? []).filter((l) => l.trim() && !l.trim().startsWith('#')).length;
  const sections = $derived((draft.ingredients ?? []).filter((l) => l.trim().startsWith('#')).length);
  const photoProblem = $derived(!draft.images?.length ? preview.images.find((i) => i.problem)?.problem : null);
  const dups = $derived(preview.duplicates ?? []);
  // A photo that was found but couldn't be saved is its own note.
  const missing = $derived(report.missing.filter((m) => !(photoProblem && /photo|image/i.test(m))));

  const how = {
    'json-ld': 'Taken from the recipe data the page publishes.',
    microdata: 'Taken from the recipe markup in the page.',
    'h-recipe': 'Taken from the h-recipe markup in the page.',
    text: 'Split into ingredients and method from the text you pasted. Check where the lines fell.',
  };

  function hostOf(u) {
    try {
      return new URL(u).hostname.replace(/^www\./, '');
    } catch {
      return null;
    }
  }
  const list = (a) => (a.length < 2 ? a.join('') : `${a.slice(0, -1).join(', ')} and ${a.at(-1)}`);
</script>

<section class="banner" aria-labelledby="import-h">
  <div class="head">
    <div class="read">
      <h1 id="import-h">
        {#if report.method === 'text'}
          Read from your paste
        {:else}
          Read from <a href={report.source_url} target="_blank" rel="noreferrer">{host}<ExternalLink size={16} aria-hidden="true" /></a>
        {/if}
      </h1>
      <p>
        {how[report.method] ?? 'Read from the page.'}
        {#if !dups.length}Check it over, then add it to the library.{/if}
      </p>
      {#if report.archived}
        <p class="archived"><Archive size={16} aria-hidden="true" />From an archived copy on web.archive.org; the link points at the original.</p>
      {/if}
    </div>
    <dl class="counts">
      <div><dt class="label">Ingredients</dt><dd class="fig">{count(draft.ingredients)}</dd></div>
      <div><dt class="label">Steps</dt><dd class="fig">{count(draft.steps)}</dd></div>
      {#if sections}<div><dt class="label">Sections</dt><dd class="fig">{sections}</dd></div>{/if}
    </dl>
  </div>

  {#if missing.length || report.warnings.length || photoProblem}
    <ul class="notes">
      {#if missing.length}
        <li><CircleDashed size={18} aria-hidden="true" /><span>Not on the page: {list(missing)}. Fill in what you know below.</span></li>
      {/if}
      {#if photoProblem}
        <li><CircleDashed size={18} aria-hidden="true" /><span>The photo couldn't be saved: {photoProblem} Add one of your own below.</span></li>
      {/if}
      {#each report.warnings as w}
        <li><TriangleAlert size={18} aria-hidden="true" /><span>{w}</span></li>
      {/each}
    </ul>
  {/if}

  {#if dups.length}
    <div class="dups" role="group" aria-labelledby="dup-h">
      <h2 id="dup-h" class="visually-hidden">Already in Sous</h2>
      {#each dups as d (d.id)}
        <div class="dup">
          {#if d.deleted}
            <p><strong>{d.title}</strong> came from this link and is in the trash.</p>
            <div class="acts">
              <button type="button" class="btn" onclick={() => onrestore(d)}>Restore it</button>
            </div>
          {:else}
            <p><strong>{d.title}</strong> is already in the library, from this link.</p>
            <div class="acts">
              <a class="btn" href="/r/{d.id}" use:link>Open it</a>
              <button type="button" class="btn" aria-pressed={choice?.mode === 'merge' && choice.id === d.id} onclick={() => onchoose({ mode: 'merge', id: d.id })}>Update it with this version</button>
            </div>
          {/if}
        </div>
      {/each}
      <div class="dup new">
        <p>Or keep both and add this as a separate recipe.</p>
        <div class="acts">
          <button type="button" class="btn" aria-pressed={choice?.mode === 'new'} onclick={() => onchoose({ mode: 'new' })}>Save as new</button>
        </div>
      </div>
      {#if choice?.mode === 'merge'}
        <p class="merge-note" aria-live="polite">The title, introduction, yield, times, nutrition, ingredients, method and link are replaced below. Tags, notes and photos stay as they are; a new photo is added after them.</p>
      {:else if !choice}
        <p class="merge-note">Choose one to see the recipe.</p>
      {/if}
    </div>
  {/if}
</section>

<style>
  .banner {
    background: var(--ticket);
    border-top: 3px solid var(--ink);
    padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: var(--space-4) var(--space-6);
  }
  .read {
    display: grid;
    gap: var(--space-2);
    flex: 1 1 24rem;
  }
  h1 {
    font-size: var(--title-l);
    font-weight: 800;
    letter-spacing: -0.015em;
  }
  h1 a {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    text-decoration-line: underline;
  }
  h1 a :global(svg) {
    color: var(--ink-3);
  }
  .read p {
    color: var(--ink-2);
    max-width: 65ch;
  }
  .archived {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .counts {
    display: flex;
    gap: var(--space-5);
    margin: 0;
    align-self: start;
  }
  .counts div {
    min-width: 4.5rem;
    padding-top: var(--space-2);
    border-top: 3px solid var(--ink);
  }
  .counts dt {
    color: var(--ink-3);
  }
  .counts dd {
    margin: var(--space-1) 0 0;
    font-size: 2.25rem;
  }
  .notes {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-4);
    padding-top: var(--space-4);
    border-top: 1.5px dashed var(--perf);
  }
  .notes li {
    display: grid;
    grid-template-columns: 18px minmax(0, 1fr);
    gap: var(--space-3);
    align-items: start;
    max-width: 75ch;
  }
  .notes :global(svg) {
    margin-top: 3px;
    color: var(--ink-2);
  }
  .dups {
    display: grid;
    gap: var(--space-3);
    margin-top: var(--space-4);
    padding-top: var(--space-4);
    border-top: 1.5px dashed var(--perf);
  }
  .dup {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2) var(--space-4);
  }
  .dup strong {
    font-weight: 700;
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .merge-note {
    font-size: 0.875rem;
    color: var(--ink-3);
    max-width: 75ch;
  }
  @media (max-width: 760px) {
    .banner {
      padding: var(--space-4);
    }
    .acts,
    .acts .btn {
      width: 100%;
    }
    .acts .btn {
      white-space: normal;
      text-align: center;
    }
  }
</style>
