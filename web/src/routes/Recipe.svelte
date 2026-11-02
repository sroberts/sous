<script>
  import { tick } from 'svelte';
  import { ArrowLeft, Heart, Bookmark, Pencil, ExternalLink } from '@lucide/svelte';
  import { link } from '../lib/router.svelte.js';
  import { api } from '../lib/api.js';
  import { session, asPerson, patchSummary, toast } from '../lib/session.svelte.js';
  import { getRecipe } from '../lib/library.js';
  import { isOffline } from '../lib/net.svelte.js';
  import { amount, ago, personName, webLink } from '../lib/format.js';
  import Plate from '../components/Plate.svelte';
  import Facts from '../components/Facts.svelte';
  import { stickIfFits } from '../lib/stick.js';

  let { id } = $props();
  let recipe = $state(null);
  let error = $state(null);
  let asWritten = $state(false);

  getRecipe(id).then(
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
    let saved;
    try {
      saved = await asPerson(() => api.setMine(id, { [flag]: next[flag] }));
    } catch (err) {
      toast(err.message);
      return;
    }
    if (!saved) return;
    recipe.mine = saved;
    patchSummary(id, { mine: saved });
  }

  const person = (u) => personName(u, session.people);

  // Below 900px the columns stack, which puts the whole ingredient list
  // between you and the method. There they become two tabs that stick under
  // the masthead, each keeping its own place on the page.
  let narrow = $state(false);
  let tab = $state('ingredients');
  let tabs = $state();
  let tabsAt = $state(); // where the tabs sit before they stick
  const placeOf = { ingredients: null, method: null };
  $effect(() => {
    const mq = matchMedia('(max-width: 900px)');
    const set = () => (narrow = mq.matches);
    set();
    mq.addEventListener('change', set);
    return () => mq.removeEventListener('change', set);
  });

  async function show(next, focus = false) {
    if (next !== tab) {
      const stickTop = parseFloat(getComputedStyle(tabs).top) || 0;
      const home = tabsAt.getBoundingClientRect().top + scrollY - stickTop;
      const stuck = scrollY > home;
      placeOf[tab] = stuck ? scrollY : null;
      tab = next;
      await tick();
      // Scrolled into one tab: open the other where you left it, or at its top.
      if (stuck) scrollTo({ top: Math.max(placeOf[next] ?? home, home) });
    }
    if (focus) document.getElementById(`tab-${next}`)?.focus();
  }
  function tabKey(e) {
    const to = { ArrowLeft: 'ingredients', Home: 'ingredients', ArrowRight: 'method', End: 'method' }[e.key];
    if (!to) return;
    e.preventDefault();
    show(to, true);
  }
</script>

{#if error}
  <main class="missing">
    {#if error.code === 'not_on_device'}
      <h1>This recipe isn’t on this device yet.</h1>
      <p>It will be once you’re back online.</p>
    {:else}
      <h1>{error.status === 404 ? 'That recipe isn’t in the library.' : 'This recipe didn’t load.'}</h1>
      <p>{error.message}</p>
    {/if}
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

    <div class="rail" aria-hidden="true"></div>
    <header class="top ticket">
      <span class="clip" aria-hidden="true"></span>
      <div class="head">
        <h1>{recipe.title}</h1>
        {#if recipe.source_url || recipe.tags.length}
          <p class="src">
            {#if recipe.tags.length}<span>{recipe.tags.join(', ')}</span>{#if recipe.source_url}<span class="dot" aria-hidden="true">·</span>{/if}{/if}
            {#if webLink(recipe.source_url)}<a href={recipe.source_url.trim()} target="_blank" rel="noreferrer">{recipe.source_host ?? recipe.source_url}<ExternalLink size={13} aria-hidden="true" /></a>{:else if recipe.source_url}<span>{recipe.source_url}</span>{/if}
          </p>
        {/if}
        <div class="facts"><Facts {recipe} size="l" /></div>
        {#if recipe.description}<p class="desc">{recipe.description}</p>{/if}
        <div class="acts">
          <a class="btn primary cook" href="/r/{id}/cook" use:link>Begin cooking</a>
          <button class="btn" aria-pressed={recipe.mine.favorite} disabled={isOffline()} onclick={() => toggle('favorite')}>
            <Heart aria-hidden="true" fill={recipe.mine.favorite ? 'currentColor' : 'none'} />Favorite
          </button>
          <button class="btn" aria-pressed={recipe.mine.want_to_cook} disabled={isOffline()} onclick={() => toggle('want_to_cook')}>
            <Bookmark aria-hidden="true" fill={recipe.mine.want_to_cook ? 'currentColor' : 'none'} />Want to cook
          </button>
        </div>
      </div>
      <div class="photo" class:none={!recipe.images[0]}><Plate hash={recipe.images[0]} title={recipe.title} /></div>
    </header>

    <div class="body">
      {#if narrow}
        <div bind:this={tabsAt}></div>
        <div class="tabs" role="tablist" aria-label="Recipe" bind:this={tabs}>
          <button role="tab" id="tab-ingredients" aria-selected={tab === 'ingredients'} aria-controls="panel-ingredients" tabindex={tab === 'ingredients' ? 0 : -1} onkeydown={tabKey} onclick={() => show('ingredients')}>
            Ingredients<span class="count fig">{recipe.ingredients.length}</span>
          </button>
          <button role="tab" id="tab-method" aria-selected={tab === 'method'} aria-controls="panel-method" tabindex={tab === 'method' ? 0 : -1} onkeydown={tabKey} onclick={() => show('method')}>
            Method<span class="count fig">{recipe.steps.length}</span>
          </button>
        </div>
      {/if}
      <section id="panel-ingredients" class="ingredients sheet" role={narrow ? 'tabpanel' : undefined} aria-labelledby={narrow ? 'tab-ingredients' : 'ing-h'} hidden={narrow && tab !== 'ingredients'} use:stickIfFits={32}>
        <div class="col-head">
          <h2 id="ing-h" class="label" class:visually-hidden={narrow}>Ingredients</h2>
          <button class="as-written" aria-pressed={asWritten} onclick={() => (asWritten = !asWritten)}>Show as written</button>
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

      <section id="panel-method" class="method sheet" role={narrow ? 'tabpanel' : undefined} aria-labelledby={narrow ? 'tab-method' : 'method-h'} hidden={narrow && tab !== 'method'}>
        <div class="col-head"><h2 id="method-h" class="label" class:visually-hidden={narrow}>Method</h2><span class="label steps">{recipe.steps.length} {recipe.steps.length === 1 ? 'step' : 'steps'}</span></div>
        {#each groups(recipe.steps) as g}
          {#if g.section}<h3 class="sect">{g.section}</h3>{/if}
          <ol start={recipe.steps.indexOf(g.rows[0]) + 1}>
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

  .page > .rail {
    position: relative;
    z-index: 1;
  }
  .clip {
    position: absolute;
    top: 0;
    left: 50%;
    width: 44px;
    height: 10px;
    margin-left: -22px;
    border-radius: 0 0 3px 3px;
    background: linear-gradient(var(--steel), var(--steel-lo));
  }
  .top {
    position: relative;
    margin: -4px var(--space-4) 0;
    display: grid;
    grid-template-columns: minmax(0, 7fr) minmax(0, 5fr);
    gap: var(--space-6);
    padding: var(--space-7) var(--space-6) calc(var(--space-6) + var(--tear));
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
  .src {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--space-3);
  }
  .dot {
    color: var(--ink-3);
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
  .tabs {
    position: sticky;
    top: var(--bar-shown, 0px);
    z-index: 4;
    display: grid;
    grid-template-columns: 1fr 1fr;
    margin: 0 calc(var(--gutter) * -1) calc(var(--space-4) * -1);
    padding: 0 var(--gutter);
    background: var(--pass);
    border-bottom: 1px solid var(--perf);
    transition: top 240ms var(--ease-out);
  }
  .tabs button {
    display: inline-flex;
    justify-content: center;
    align-items: baseline;
    gap: var(--space-2);
    min-height: 48px;
    padding: var(--space-3) 0 0;
    border: 0;
    border-bottom: 3px solid transparent;
    background: none;
    font-size: var(--ui);
    font-weight: 700;
    color: var(--ink-2);
  }
  .tabs button[aria-selected='true'] {
    color: var(--ink);
    border-bottom-color: var(--ink);
  }
  .count {
    color: var(--ink-3);
  }
  .sheet {
    background: var(--ticket);
    padding: var(--space-4) var(--space-5) var(--space-6);
  }
  .ingredients:global(.stuck) {
    position: sticky;
    top: calc(var(--bar) + var(--space-4));
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
  .as-written:hover,
  .as-written[aria-pressed='true'] {
    color: var(--red);
  }
  .as-written[aria-pressed='true'] {
    text-decoration-color: var(--red);
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
    max-width: 65ch;
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
      margin: -4px var(--space-2) 0;
      padding: var(--space-5) var(--space-4) calc(var(--space-5) + var(--tear));
    }
    /* Title, facts and Begin cooking come first; the description and the
       photo follow, the photo as a short band, and a photo-less recipe shows
       no initial here. */
    .head {
      display: flex;
      flex-direction: column;
    }
    .desc {
      order: 1;
    }
    .photo :global(.plate) {
      max-height: 40vh;
    }
    .photo.none {
      display: none;
    }
    .body {
      grid-template-columns: 1fr;
    }
    .ingredients:global(.stuck) {
      position: static;
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
    /* Tags and source each take a line, so no separator is left hanging at
       the end of the tags when the source wraps. */
    .src {
      flex-direction: column;
      align-items: flex-start;
    }
    .dot {
      display: none;
    }
    .acts .btn:not(.cook) {
      flex: 1;
    }
    .cook {
      width: 100%;
    }
  }
</style>
