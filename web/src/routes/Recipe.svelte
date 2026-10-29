<script>
  import { ArrowLeft, Heart, Bookmark, Pencil, ExternalLink } from '@lucide/svelte';
  import { link } from '../lib/router.svelte.js';
  import { api } from '../lib/api.js';
  import { session, asPerson, patchSummary } from '../lib/session.svelte.js';
  import { amount, duration, ago } from '../lib/format.js';
  import Plate from '../components/Plate.svelte';

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
    <h1>{error.status === 404 ? 'That recipe isn’t on the menu.' : 'This recipe didn’t load.'}</h1>
    <p>{error.message}</p>
    <a class="btn" href="/" use:link>Back to the menu</a>
  </main>
{:else if !recipe}
  <main class="page loading" aria-busy="true"><span class="visually-hidden">Loading recipe</span></main>
{:else}
  <main class="page">
    <div class="crumbs">
      <a class="back caps" href="/" use:link><ArrowLeft size={14} aria-hidden="true" />The menu</a>
      <a class="back caps" href="/r/{id}/edit" use:link><Pencil size={14} aria-hidden="true" />Edit recipe</a>
    </div>

    <header class="top">
      <div class="plate"><Plate hash={recipe.images[0]} title={recipe.title} /></div>
      <div class="title">
        <h1>{recipe.title}</h1>
        {#if recipe.source_url}
          <p class="src">from <a href={recipe.source_url} target="_blank" rel="noreferrer">{recipe.source_host}<ExternalLink size={12} aria-hidden="true" /></a></p>
        {/if}
        {#if recipe.description}<p class="desc">{recipe.description}</p>{/if}
        <dl class="facts">
          {#if recipe.yield_text}<div><dt class="caps">Yield</dt><dd>{recipe.yield_text.replace(/^yield:?\s*/i, '')}</dd></div>{/if}
          {#if recipe.prep_minutes}<div><dt class="caps">Prep</dt><dd>{duration(recipe.prep_minutes)}</dd></div>{/if}
          {#if recipe.cook_minutes}<div><dt class="caps">Cook</dt><dd>{duration(recipe.cook_minutes)}</dd></div>{/if}
          {#if recipe.total_minutes}<div><dt class="caps">Total</dt><dd>{duration(recipe.total_minutes)}</dd></div>{/if}
          {#if recipe.time_text && !recipe.total_minutes}<div><dt class="caps">Time</dt><dd>{recipe.time_text}</dd></div>{/if}
        </dl>
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
    </header>

    <hr class="double-rule" />

    <div class="body">
      <section class="ingredients" aria-labelledby="ing-h">
        <div class="col-head">
          <h2 id="ing-h" class="caps">Ingredients</h2>
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
                  <span class="amt">{amount(i)}</span>
                  <span class="item">{i.item}{#if i.note}<span class="note">, {i.note}</span>{/if}</span>
                </li>
              {/if}
            {/each}
          </ul>
        {/each}
      </section>

      <section class="method" aria-labelledby="method-h">
        <div class="col-head"><h2 id="method-h" class="caps">Method</h2></div>
        {#each groups(recipe.steps) as g}
          {#if g.section}<h3 class="sect">{g.section}</h3>{/if}
          <ol>
            {#each g.rows as s}
              <li><span class="n" aria-hidden="true">{recipe.steps.indexOf(s) + 1}</span><p>{s.text}</p></li>
            {/each}
          </ol>
        {/each}

        {#if recipe.notes}
          <h2 class="caps aside-h">Notes</h2>
          <p class="notes">{recipe.notes}</p>
        {/if}
        {#if recipe.nutrition_text}
          <h2 class="caps aside-h">Nutrition</h2>
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
    max-width: 76rem;
    margin: 0 auto;
    padding: var(--space-5) var(--gutter) var(--space-8);
  }
  .loading {
    min-height: 60vh;
  }
  .crumbs {
    display: flex;
    justify-content: space-between;
  }
  .back {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    text-decoration: none;
    color: var(--ink-soft);
    padding: var(--space-2) 0;
  }
  .back:hover {
    color: var(--brick);
  }
  .top {
    display: grid;
    grid-template-columns: minmax(0, 5fr) minmax(0, 7fr);
    gap: var(--space-7);
    align-items: start;
    margin: var(--space-4) 0 var(--space-6);
  }
  h1 {
    font-size: var(--title-xl);
    line-height: 1.08;
  }
  .src {
    margin-top: var(--space-3);
    font-style: italic;
    color: var(--ink-soft);
  }
  .src a {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .desc {
    margin-top: var(--space-4);
    max-width: 60ch;
  }
  .facts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3) var(--space-6);
    margin: var(--space-5) 0 0;
  }
  .facts dt {
    color: var(--ink-soft);
    font-size: 0.6875rem;
  }
  .facts dd {
    margin: 2px 0 0;
    font-family: var(--sans);
    font-size: 1.0625rem;
    font-weight: 500;
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-top: var(--space-6);
  }
  .cook {
    min-height: 52px;
    padding: 0 var(--space-6);
    font-size: 0.875rem;
  }
  .acts button[aria-pressed='true'] {
    color: var(--brick);
    border-color: var(--brick);
  }

  .body {
    display: grid;
    grid-template-columns: minmax(16rem, 22rem) minmax(0, 1fr);
    gap: var(--space-8);
    margin-top: var(--space-6);
  }
  .ingredients {
    position: sticky;
    top: calc(var(--masthead) + var(--space-5));
    align-self: start;
    max-height: calc(100dvh - var(--masthead) - var(--space-6));
    overflow-y: auto;
    scrollbar-width: thin;
  }
  /* Both column heads share one height, so their rules sit level. */
  .col-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    min-height: 2.25rem;
    padding-bottom: var(--space-2);
    border-bottom: 1px solid var(--ink);
  }
  .col-head h2 {
    line-height: 1.2;
  }
  .as-written {
    line-height: 1.2;
    border: 0;
    background: none;
    font-family: var(--sans);
    font-size: 0.8125rem;
    color: var(--ink-soft);
    text-decoration: underline;
    text-decoration-color: var(--ink-faint);
    text-underline-offset: 0.2em;
    padding: var(--space-1) 0;
  }
  .as-written:hover {
    color: var(--ink);
  }
  .sect {
    font-family: var(--serif);
    font-style: italic;
    font-size: 1.0625rem;
    margin: var(--space-5) 0 var(--space-1);
  }
  .ingredients li {
    display: grid;
    grid-template-columns: 5.5em 1fr;
    gap: var(--space-3);
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--rule);
    font-size: 1.0625rem;
    line-height: 1.4;
  }
  .ingredients li.raw {
    display: block;
  }
  .amt {
    font-family: var(--sans);
    font-size: 0.9375rem;
    font-weight: 500;
    padding-top: 1px;
  }
  .note {
    font-style: italic;
    color: var(--ink-soft);
  }

  .method ol {
    margin-top: var(--space-2);
  }
  .method li {
    display: grid;
    grid-template-columns: 2.5rem minmax(0, 65ch);
    gap: var(--space-3);
    padding: var(--space-4) 0;
    border-bottom: 1px solid var(--rule);
  }
  .n {
    font-family: var(--sans);
    font-weight: 650;
    font-size: 1.25rem;
    line-height: 1.3;
  }
  .aside-h {
    margin-top: var(--space-7);
    padding-bottom: var(--space-2);
    border-bottom: 1px solid var(--ink);
    max-width: calc(65ch + 2.5rem + var(--space-3));
  }
  .notes {
    margin-top: var(--space-3);
    max-width: calc(65ch + 2.5rem + var(--space-3));
    white-space: pre-line;
  }
  .colophon {
    margin-top: var(--space-7);
    font-style: italic;
    font-size: 0.9375rem;
    color: var(--ink-soft);
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
      gap: var(--space-5);
    }
    .body {
      grid-template-columns: 1fr;
      gap: var(--space-7);
    }
    .ingredients {
      position: static;
      max-height: none;
    }
    .method li {
      grid-template-columns: 2rem minmax(0, 1fr);
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
