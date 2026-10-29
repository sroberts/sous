<script>
  import { ArrowLeft, Heart, Bookmark, Pencil, ExternalLink } from '@lucide/svelte';
  import { link } from '../lib/router.svelte.js';
  import { api } from '../lib/api.js';
  import { session, asPerson, patchSummary } from '../lib/session.svelte.js';
  import { amount, ago } from '../lib/format.js';
  import Plate from '../components/Plate.svelte';
  import Facts from '../components/Facts.svelte';

  let { id } = $props();
  let recipe = $state(null);
  let error = $state(null);
  let asWritten = $state(false);

  api.recipe(id).then(
    (r) => {
      recipe = r;
      document.title = `${r.title} · Sous`;
    },
    (err) => (error = err),
  );

  const groups = (rows) => {
    const out = [];
    for (const row of rows) {
      if (!out.length || out.at(-1).section !== row.section) out.push({ section: row.section, rows: [] });
      out.at(-1).rows.push(row);
    }
    return out;
  };

  async function toggle(flag) {
    const next = { ...recipe.mine, [flag]: !recipe.mine[flag] };
    const saved = await asPerson(() => api.setMine(id, { [flag]: next[flag] }));
    if (!saved) return;
    recipe.mine = saved;
    patchSummary(id, { mine: saved });
  }

  const person = (u) => session.people.find((p) => p.username === u)?.name ?? u;
</script>

{#if error}
  <main class="missing">
    <h1>{error.status === 404 ? 'That recipe isn’t in the library.' : 'This recipe didn’t load.'}</h1>
    <p>{error.message}</p>
    <a class="btn" href="/" use:link>Back to the library</a>
  </main>
{:else if !recipe}
  <main class="page loading" aria-busy="true"><span class="visually-hidden">Loading recipe</span></main>
{:else}
  <main class="page">
    <div class="crumbs">
      <a class="back" href="/" use:link><ArrowLeft size={16} aria-hidden="true" />Library</a>
      <a class="back" href="/r/{id}/edit" use:link><Pencil size={16} aria-hidden="true" />Edit recipe</a>
    </div>

    <header class="top ticket">
      <div class="head">
        {#if recipe.tags.length}<p class="tags label">{recipe.tags.join(' · ')}</p>{/if}
        <h1>{recipe.title}</h1>
        {#if recipe.source_url}
          <p class="src"><a href={recipe.source_url} target="_blank" rel="noreferrer">{recipe.source_host ?? recipe.source_url}<ExternalLink size={13} aria-hidden="true" /></a></p>
        {/if}
        <div class="facts"><Facts {recipe} size="l" /></div>
        {#if recipe.description}<p class="desc">{recipe.description}</p>{/if}
        <div class="acts">
          <a class="btn primary cook" href="/r/{id}/cook" use:link>Begin cooking</a>
          <button class="btn" aria-pressed={recipe.mine.favorite} onclick={() => toggle('favorite')}>
            <Heart aria-hidden="true" fill={recipe.mine.favorite ? 'currentColor' : 'none'} />Favorite
          </button>
          <button class="btn" aria-pressed={recipe.mine.want_to_cook} onclick={() => toggle('want_to_cook')}>
            <Bookmark aria-hidden="true" fill={recipe.mine.want_to_cook ? 'currentColor' : 'none'} />Want to cook
          </button>
        </div>
      </div>
      <div class="photo"><Plate hash={recipe.images[0]} title={recipe.title} /></div>
    </header>

    <div class="body">
      <section class="ingredients ticket" aria-labelledby="ing-h">
        <div class="col-head">
          <h2 id="ing-h" class="label">Ingredients</h2>
          <button class="as-written" aria-pressed={asWritten} onclick={() => (asWritten = !asWritten)}>
            {asWritten ? 'Show measured' : 'Show as written'}
          </button>
        </div>
        {#each groups(recipe.ingredients) as g}
          {#if g.section}<h3 class="sect">{g.section}</h3>{/if}
          <ul>
            {#each g.rows as i}
              {#if asWritten}
                <li class="raw">{i.raw}</li>
              {:else}
                <li>
                  <span class="amt fig">{amount(i)}</span>
                  <span class="item">{i.item}{#if i.note}<span class="note">, {i.note}</span>{/if}</span>
                </li>
              {/if}
            {/each}
          </ul>
        {/each}
      </section>

      <section class="method ticket" aria-labelledby="method-h">
        <div class="col-head"><h2 id="method-h" class="label">Method</h2><span class="label steps">{recipe.steps.length} steps</span></div>
        {#each groups(recipe.steps) as g}
          {#if g.section}<h3 class="sect">{g.section}</h3>{/if}
          <ol>
            {#each g.rows as s}
              <li><span class="n fig" aria-hidden="true">{recipe.steps.indexOf(s) + 1}</span><p>{s.text}</p></li>
            {/each}
          </ol>
        {/each}

        {#if recipe.notes}
          <h2 class="label aside-h">Notes</h2>
          <p class="notes">{recipe.notes}</p>
        {/if}
        {#if recipe.nutrition_text}
          <h2 class="label aside-h">Nutrition</h2>
          <p class="notes">{recipe.nutrition_text}</p>
        {/if}

        <p class="colophon">
          Added by {person(recipe.created_by)} {ago(recipe.created_at)}{#if recipe.updated_at !== recipe.created_at}, last edited by {person(recipe.updated_by)} {ago(recipe.updated_at)}{/if}.
        </p>
      </section>
    </div>
  </main>
{/if}

<style>
  .page {
    max-width: 80rem;
    margin: 0 auto;
    padding: var(--space-3) var(--gutter) var(--space-8);
  }
  .loading {
    min-height: 60vh;
  }
  .crumbs {
    display: flex;
    justify-content: space-between;
    margin-bottom: var(--space-3);
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
  .back:hover {
    color: var(--red);
  }

  .top {
    display: grid;
    grid-template-columns: minmax(0, 7fr) minmax(0, 5fr);
    gap: var(--space-6);
    padding: var(--space-6) var(--space-6) calc(var(--space-6) + var(--tear));
    border-top: 3px solid var(--ink);
  }
  .tags {
    color: var(--ink-3);
    margin-bottom: var(--space-2);
  }
  h1 {
    font-size: var(--title-xl);
    font-weight: 800;
    line-height: 1.04;
    letter-spacing: -0.025em;
  }
  .src {
    margin-top: var(--space-2);
    font-size: var(--ui);
    color: var(--ink-2);
  }
  .src a {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .facts {
    margin-top: var(--space-5);
  }
  .desc {
    margin-top: var(--space-4);
    max-width: 65ch;
    color: var(--ink-2);
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-top: var(--space-5);
  }
  .cook {
    min-height: 52px;
    padding: 0 var(--space-6);
    font-size: 1.0625rem;
  }
  .acts .btn:not(.cook) {
    min-height: 52px;
  }
  .photo {
    align-self: start;
  }

  .body {
    display: grid;
    grid-template-columns: minmax(17rem, 23rem) minmax(0, 1fr);
    gap: var(--space-4);
    margin-top: var(--space-4);
    align-items: start;
  }
  .ingredients,
  .method {
    padding: var(--space-4) var(--space-5) calc(var(--space-5) + var(--tear));
  }
  .ingredients {
    position: sticky;
    top: calc(var(--bar) + var(--space-4));
    max-height: calc(100dvh - var(--bar) - var(--space-6));
    overflow-y: auto;
    scrollbar-width: thin;
  }
  .col-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    min-height: 2.25rem;
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
  }
  .steps {
    color: var(--ink-3);
  }
  .as-written {
    border: 0;
    background: none;
    font-size: 0.8125rem;
    font-weight: 600;
    color: var(--ink-2);
    text-decoration: underline;
    text-decoration-color: var(--perf);
    text-underline-offset: 0.2em;
    padding: var(--space-1) 0;
  }
  .as-written:hover {
    color: var(--red);
  }
  .sect {
    font-size: var(--ui);
    font-weight: 800;
    margin: var(--space-5) 0 0;
    padding-bottom: var(--space-1);
  }
  .ingredients li {
    display: grid;
    grid-template-columns: 5.25rem 1fr;
    gap: var(--space-3);
    padding: var(--space-2) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
    line-height: 1.4;
  }
  .ingredients li.raw {
    display: block;
  }
  .amt {
    font-size: 1.1875rem;
    padding-top: 1px;
  }
  .note {
    color: var(--ink-3);
  }

  .method li {
    display: grid;
    grid-template-columns: 3rem minmax(0, 65ch);
    gap: var(--space-3);
    padding: var(--space-4) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
  }
  .n {
    font-size: 2rem;
  }
  .aside-h {
    margin-top: var(--space-7);
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
    max-width: calc(65ch + 3rem + var(--space-3));
  }
  .notes {
    margin-top: var(--space-3);
    max-width: calc(65ch + 3rem + var(--space-3));
    white-space: pre-line;
  }
  .colophon {
    margin-top: var(--space-7);
    font-size: 0.875rem;
    color: var(--ink-3);
  }
  .missing {
    max-width: 36rem;
    margin: var(--space-8) auto;
    padding: 0 var(--gutter);
    display: grid;
    gap: var(--space-4);
    justify-items: start;
  }
  .missing h1 {
    font-size: var(--title-l);
  }

  @media (max-width: 900px) {
    .top {
      grid-template-columns: 1fr;
      padding: var(--space-4) var(--space-4) calc(var(--space-5) + var(--tear));
    }
    .photo {
      order: -1;
    }
    .body {
      grid-template-columns: 1fr;
    }
    .ingredients {
      position: static;
      max-height: none;
    }
    .ingredients,
    .method {
      padding-left: var(--space-4);
      padding-right: var(--space-4);
    }
    .method li {
      grid-template-columns: 2.25rem minmax(0, 1fr);
    }
  }
  @media (max-width: 760px) {
    .acts .btn:not(.cook) {
      flex: 1;
    }
    .cook {
      width: 100%;
    }
  }
</style>
