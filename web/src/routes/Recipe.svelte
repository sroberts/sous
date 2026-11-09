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
  import Markdown from '../components/Markdown.svelte';
  import { scrollQuietly } from '../lib/stick.js';
  import { labelledFigures } from '../lib/markdown.js';

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
      toast(`Couldn’t save ${flag === 'favorite' ? 'Favorite' : 'Want to cook'}. ${err.message}`);
      return;
    }
    if (!saved) return;
    recipe.mine = saved;
    patchSummary(id, { mine: saved });
  }

  const person = (u) => personName(u, session.people);

  // A long description is held to three lines so the recipe itself starts in
  // the first screen; More opens the rest.
  let descEl = $state();
  let descOpen = $state(false);
  let descLong = $state(false);
  $effect(() => {
    if (!descEl) return;
    const measure = () => (descLong = descOpen || descEl.scrollHeight > descEl.clientHeight + 1);
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(descEl);
    return () => ro.disconnect();
  });

  // Notes open the method, but read once and returned to never: collapsed,
  // only the first paragraph shows, held to two lines; More opens them all.
  let notesEl = $state();
  let notesOpen = $state(false);
  let notesLong = $state(false);
  const notesParas = $derived(recipe?.notes ? recipe.notes.split(/\n\s*\n/).map((p) => p.trim()).filter(Boolean) : []);
  $effect(() => {
    if (!notesEl) return;
    const measure = () => (notesLong = notesOpen || notesParas.length > 1 || notesEl.scrollHeight > notesEl.clientHeight + 1);
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(notesEl);
    return () => ro.disconnect();
  });

  // Below 900px the columns stack. A short recipe stays that way, both lists
  // in one scroll. A long one would put the whole ingredient list between
  // you and the method, so there they become two tabs that stick under the
  // masthead, each keeping its own place on the page.
  let narrow = $state(false);
  let fits = $state(null); // both lists in about a screen and a half (null: not measured yet)
  let tabbed = $derived(narrow && fits === false);
  let tab = $state('ingredients');
  let tabs = $state();
  let tabsAt = $state(); // where the tabs sit before they stick
  let ingPanel = $state();
  let methodPanel = $state();
  const placeOf = { ingredients: null, method: null };
  $effect(() => {
    const mq = matchMedia('(max-width: 900px)');
    const set = () => {
      narrow = mq.matches;
      fits = null;
    };
    set();
    mq.addEventListener('change', set);
    return () => mq.removeEventListener('change', set);
  });
  // Measured once, stacked, before the first paint; not again on scroll or
  // Show as written, so the layout doesn't change under you.
  $effect(() => {
    if (!narrow || fits !== null || !ingPanel || !methodPanel) return;
    fits = ingPanel.offsetHeight + methodPanel.offsetHeight <= innerHeight * 1.5;
  });

  // Where the tabs stick: the masthead's height when it's showing. Read from
  // --bar-shown, not the strip's own top, which is mid-transition as the
  // masthead slides.
  const stickTop = () => parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--bar-shown')) || 0;

  async function show(next, focus = false) {
    if (next !== tab) {
      const home = tabsAt.getBoundingClientRect().top + scrollY - stickTop();
      const stuck = scrollY > home;
      placeOf[tab] = stuck ? scrollY : null;
      tab = next;
      await tick();
      // Scrolled into one tab: open the other where you left it, or at its top.
      if (stuck) scrollQuietly(Math.max(placeOf[next] ?? home, home));
    }
    if (focus) document.getElementById(`tab-${next}`)?.focus();
  }
  // Once the tabs stick, the header and its Begin cooking are off screen, so
  // the strip carries a Cook button of its own, except while the Begin
  // cooking after the last step is in view: one red button at a time.
  let tabsStuck = $state(false);
  let endCook = $state();
  let endInView = $state(false);
  let headCook = $state();
  let headInView = $state(true);
  $effect(() => {
    if (!tabbed || !tabsAt) return;
    const check = () => (tabsStuck = tabsAt.getBoundingClientRect().top < stickTop() + 1);
    check();
    addEventListener('scroll', check, { passive: true });
    return () => removeEventListener('scroll', check);
  });
  $effect(() => {
    if (!endCook) return;
    const io = new IntersectionObserver(([e]) => (endInView = e.isIntersecting));
    io.observe(endCook);
    return () => io.disconnect();
  });
  // The Begin cooking after the last step is red only once the header's has
  // scrolled away; on a short recipe both can be on screen at once.
  $effect(() => {
    if (!headCook) return;
    // Under the masthead counts as scrolled away.
    const bar = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--bar')) || 56;
    const io = new IntersectionObserver(([e]) => (headInView = e.isIntersecting), { rootMargin: `-${bar}px 0px 0px 0px` });
    io.observe(headCook);
    return () => io.disconnect();
  });

  // In the Method tab the quantities are a tab away, so a bar at the foot of
  // the screen, in thumb reach, pulls them up over the step you're on.
  let sheet = $state();
  let drag = $state(0); // px the sheet is pulled down by its handle
  let dragFrom = null;
  const openSheet = () => sheet?.showModal();
  const closeSheet = () => sheet?.close();
  function dragStart(e) {
    dragFrom = e.clientY;
    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {}
  }
  function dragMove(e) {
    if (dragFrom !== null) drag = Math.max(0, e.clientY - dragFrom);
  }
  function dragEnd() {
    if (dragFrom === null) return;
    if (drag > 80) closeSheet();
    dragFrom = null;
    drag = 0;
  }

  const nutrition = $derived(recipe?.nutrition_text ? labelledFigures(recipe.nutrition_text) : null);
  // A label's worth of precision: 1063.75 calories prints as 1064, 55.25 g as
  // 55.3 g, and units are the short ones a label uses.
  const UNITS = { gram: 'g', grams: 'g', milligram: 'mg', milligrams: 'mg', microgram: 'µg', micrograms: 'µg', kilocalorie: 'kcal', kilocalories: 'kcal', calories: 'kcal' };
  function figure(text) {
    const n = Number(text.replace(/,/g, ''));
    if (!Number.isFinite(n)) return text;
    return n >= 100 ? String(Math.round(n)) : String(Math.round(n * 10) / 10);
  }
  const unit = (u) => UNITS[u.toLowerCase()] ?? u;
  // Calories with no unit given are kilocalories, as on any label.
  const unitFor = (n) => (n.unit ? unit(n.unit) : /^(calories|energy)$/i.test(n.label) ? 'kcal' : '');

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
      {#if error.status === 404}
        <h1>That recipe isn’t in the library.</h1>
        <p>It may have been moved to the trash, or the link is wrong.</p>
      {:else}
        <h1>This recipe didn’t load.</h1>
        <p>{error.status ? 'Sous had a problem fetching it.' : error.message} Try again in a moment.</p>
      {/if}
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
    <header class="top ticket" class:has-photo={!!recipe.images[0]}>
      <span class="clip" aria-hidden="true"></span>
      <div class="head">
        <h1>{recipe.title}</h1>
        <!-- On a phone, Edit recipe ends the source line (the crumb row is gone). -->
        <p class="src" class:bare={!recipe.source_url && !recipe.tags.length}>
          {#if recipe.tags.length}<span>{recipe.tags.join(', ')}</span>{#if recipe.source_url}<span class="dot" aria-hidden="true">·</span>{/if}{/if}
          <span class="src-line">
            {#if webLink(recipe.source_url)}<a href={recipe.source_url.trim()} target="_blank" rel="noreferrer">{recipe.source_host ?? recipe.source_url}<ExternalLink size={13} aria-hidden="true" /><span class="visually-hidden">{' (opens in a new tab)'}</span></a>{:else if recipe.source_url}<span>{recipe.source_url}</span>{/if}
            <a class="edit-here" href="/r/{id}/edit" use:link><Pencil size={15} aria-hidden="true" />Edit</a>
          </span>
        </p>
        <div class="facts"><Facts {recipe} size="l" count={!narrow} /></div>
        <div class="acts">
          <a class="btn primary cook" href="/r/{id}/cook" use:link bind:this={headCook}>Begin cooking</a>
          <!-- On a phone these are icon buttons beside Begin cooking; the
               name stays for screen readers and as a tooltip. -->
          <button class="btn mark" aria-pressed={recipe.mine.favorite} disabled={isOffline()} title="Favorite" onclick={() => toggle('favorite')}>
            <Heart aria-hidden="true" fill={recipe.mine.favorite ? 'currentColor' : 'none'} /><span class="mark-name">Favorite</span>
          </button>
          <button class="btn mark" aria-pressed={recipe.mine.want_to_cook} disabled={isOffline()} title="Want to cook" onclick={() => toggle('want_to_cook')}>
            <Bookmark aria-hidden="true" fill={recipe.mine.want_to_cook ? 'currentColor' : 'none'} /><span class="mark-name">Want to cook</span>
          </button>
        </div>
        {#if recipe.description}
          <div class="desc-wrap">
            <p class="desc" class:open={descOpen} id="desc" bind:this={descEl}><Markdown text={recipe.description} /></p>
            {#if descLong}<button class="more" aria-expanded={descOpen} aria-controls="desc" onclick={() => (descOpen = !descOpen)}>{descOpen ? 'Less' : 'More'}</button>{/if}
          </div>
        {/if}
      </div>
      <div class="photo" class:none={!recipe.images[0]}><Plate hash={recipe.images[0]} title={recipe.title} eager /></div>
    </header>

    <div class="body">
      {#if tabbed}
        <div class="tabs-at" bind:this={tabsAt}></div>
        <div class="tabs" bind:this={tabs}>
          <div class="tablist" role="tablist" aria-label="Recipe">
          <button role="tab" id="tab-ingredients" aria-selected={tab === 'ingredients'} aria-controls="panel-ingredients" tabindex={tab === 'ingredients' ? 0 : -1} onkeydown={tabKey} onclick={() => show('ingredients')}>
            Ingredients<span class="count fig" aria-hidden="true">{recipe.ingredients.filter((i) => i.item).length}</span><span class="visually-hidden">{`, ${recipe.ingredients.filter((i) => i.item).length} items`}</span>
          </button>
          <button role="tab" id="tab-method" aria-selected={tab === 'method'} aria-controls="panel-method" tabindex={tab === 'method' ? 0 : -1} onkeydown={tabKey} onclick={() => show('method')}>
            Method<span class="count fig" aria-hidden="true">{recipe.steps.length}</span><span class="visually-hidden">{`, ${recipe.steps.length} ${recipe.steps.length === 1 ? 'step' : 'steps'}`}</span>
          </button>
          </div>
          <a class="btn primary tab-cook" class:shown={tabsStuck && !endInView} href="/r/{id}/cook" use:link aria-label="Begin cooking">Cook</a>
        </div>
      {/if}
      <section id="panel-ingredients" class="ingredients sheet" role={tabbed ? 'tabpanel' : undefined} aria-labelledby={tabbed ? 'tab-ingredients' : 'ing-h'} hidden={tabbed && tab !== 'ingredients'} bind:this={ingPanel}>
        <div class="col-head">
          <h2 id="ing-h" class="label" class:visually-hidden={tabbed}>Ingredients</h2>
          <button class="as-written" onclick={() => (asWritten = !asWritten)}>{asWritten ? 'Show parsed' : 'Show as written'}</button>
        </div>
        {@render ingredientList()}
      </section>

      <section id="panel-method" class="method sheet" role={tabbed ? 'tabpanel' : undefined} aria-labelledby={tabbed ? 'tab-method' : 'method-h'} hidden={tabbed && tab !== 'method'} bind:this={methodPanel}>
        <!-- Notes come before the method: make-ahead and timing advice is
             read before you start, not found after the ingredients. -->
        {#if recipe.notes}
          <div class="col-head"><h2 class="label">Notes</h2></div>
          <div class="notes-block" id="notes">
            {#if notesOpen}
              {#each notesParas as para}<p class="notes"><Markdown text={para} /></p>{/each}
            {:else}
              <p class="notes clamped" bind:this={notesEl}><Markdown text={notesParas[0] ?? ''} /></p>
            {/if}
          </div>
          {#if notesLong}<button class="more" aria-expanded={notesOpen} aria-controls="notes" onclick={() => (notesOpen = !notesOpen)}>{notesOpen ? 'Less' : 'More'}</button>{/if}
        {/if}
        <div class="col-head" class:after-notes={recipe.notes}><h2 id="method-h" class="label" class:visually-hidden={tabbed && !recipe.notes}>Method</h2>{#if !tabbed}<span class="label steps">{recipe.steps.length} {recipe.steps.length === 1 ? 'step' : 'steps'}</span>{/if}</div>
        {#each groups(recipe.steps) as g}
          {#if g.section}<h3 class="sect">{g.section}</h3>{/if}
          <ol role="list">
            {#each g.rows as s}
              <li><span class="n fig"><span class="visually-hidden">{'Step '}</span>{recipe.steps.indexOf(s) + 1}</span><p><Markdown text={s.text} /></p></li>
            {/each}
          </ol>
        {/each}

        <div class="method-end" bind:this={endCook}>
          <a class="btn cook" class:primary={!headInView} href="/r/{id}/cook" use:link>Begin cooking</a>
        </div>

        {#if recipe.nutrition_text}
          <h2 class="label aside-h">Nutrition</h2>
          {#if nutrition}
            <dl class="nutrition">
              {#each nutrition as n}
                <div><dt class="label">{n.label}</dt><dd><span class="fig">{figure(n.fig)}</span>{#if unitFor(n)}<span class="unit">{unitFor(n)}</span>{/if}</dd></div>
              {/each}
            </dl>
          {:else}
            <p class="notes"><Markdown text={recipe.nutrition_text} /></p>
          {/if}
        {/if}

        <p class="colophon">
          {#if recipe.created_by === '_sous'}A sample recipe that came with Sous{:else}Added by {person(recipe.created_by)} {ago(recipe.created_at)}{/if}{#if recipe.updated_at !== recipe.created_at}, last edited by {person(recipe.updated_by)} {ago(recipe.updated_at)}{/if}.
        </p>
      </section>
    </div>

    {#if tabbed && tab === 'method'}
      <div class="peek" class:shown={tabsStuck} inert={!tabsStuck}>
        <button class="btn peek-btn" aria-haspopup="dialog" onclick={openSheet}>Ingredients<span class="count fig" aria-hidden="true">{recipe.ingredients.filter((i) => i.item).length}</span></button>
      </div>
    {/if}
    {#if tabbed}
      <dialog class="ing-sheet" bind:this={sheet} aria-labelledby="sheet-h" style:transform={drag ? `translateY(${drag}px)` : null} onclick={(e) => e.target === sheet && closeSheet()}>
        <!-- Pull the head down to close, as with any sheet. A touch extra:
             Back to the method and Escape close it from the keyboard. -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="sheet-head" onpointerdown={dragStart} onpointermove={dragMove} onpointerup={dragEnd} onpointercancel={dragEnd}>
          <span class="handle" aria-hidden="true"></span>
          <h2 id="sheet-h" class="label">Ingredients</h2>
        </div>
        <div class="sheet-in">
          {@render ingredientList()}
        </div>
        <!-- In thumb reach, and always in view, so focus lands here without
             scrolling the list. -->
        <!-- svelte-ignore a11y_autofocus -->
        <div class="sheet-foot"><button class="btn sheet-close" autofocus onclick={closeSheet}>Back to the method</button></div>
      </dialog>
    {/if}
  </main>
{/if}

{#snippet ingredientList()}
  {#each groups(recipe.ingredients) as g}
    {#if g.section}<h3 class="sect">{g.section}</h3>{/if}
    <ul role="list">
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
{/snippet}

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
    padding: var(--space-6) var(--space-6) calc(var(--space-5) + var(--tear));
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
  .src.bare {
    display: none;
  }
  .src-line {
    display: inline-flex;
    align-items: center;
    gap: var(--space-4);
  }
  /* A 44px target on a text line: the extra height is taken back from the
     margins, so the line keeps its height. */
  .src .edit-here {
    display: none;
    min-height: 44px;
    margin: -10px 0;
    font-weight: 600;
    color: var(--ink-2);
    text-decoration: none;
  }
  .src .edit-here:hover {
    color: var(--red);
  }
  .facts {
    margin-top: var(--space-5);
  }
  .desc-wrap {
    margin-top: var(--space-4);
    max-width: 65ch;
  }
  .desc {
    color: var(--ink-2);
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    overflow: hidden;
  }
  .desc.open {
    display: block;
  }
  /* A 44px target around a text-sized link: the extra height is taken back
     from the margins so the line sits where it did. */
  .more {
    min-height: 44px;
    margin: calc(var(--space-1) - 10px) 0 -10px;
    padding: 0;
    border: 0;
    background: none;
    font-size: var(--ui);
    font-weight: 600;
    color: var(--ink);
    text-decoration: underline;
    text-decoration-color: var(--ink-3);
    text-decoration-thickness: 1.5px;
    text-underline-offset: 0.2em;
  }
  .more:hover {
    color: var(--red);
    text-decoration-color: var(--red);
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
  /* No photo: the header text takes the width, not a grey slot. */
  .top:not(.has-photo) {
    grid-template-columns: minmax(0, 1fr);
  }
  .photo.none {
    display: none;
  }
  /* Held short so the recipe starts in the first screen on a laptop or an
     iPad in landscape. */
  .photo :global(.plate) {
    max-height: min(22rem, 46vh);
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
    display: flex;
    margin: 0 calc(var(--gutter) * -1) calc(var(--space-4) * -1);
    padding: 0 var(--gutter);
    background: var(--pass);
    border-bottom: 1px solid var(--perf);
    transition: top 240ms var(--ease-out);
  }
  .tablist {
    display: grid;
    grid-template-columns: 1fr 1fr;
    flex: 1;
  }
  /* Shifted down past the grid gap, so it marks the strip's own top. */
  .tabs-at {
    position: relative;
    top: var(--space-4);
  }
  .tab-cook {
    align-self: center;
    min-height: 44px;
    margin-block: var(--space-1);
    margin-left: var(--space-3);
    visibility: hidden;
  }
  .tab-cook.shown {
    visibility: visible;
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
  /* A laptop or an iPad in landscape: a shorter header so the recipe starts
     in the first screen. */
  @media (max-width: 1180px) and (min-width: 901px) {
    .top {
      padding-top: var(--space-5);
    }
    .desc {
      -webkit-line-clamp: 2;
      line-clamp: 2;
    }
  }
  @media (min-width: 1280px) {
    .body {
      grid-template-columns: minmax(17rem, 27rem) minmax(0, 1fr);
    }
  }
  /* Side by side, the ingredients stay on screen the whole way down the
     method: sticky, and scrolling inside themselves when the list is longer
     than the window. Show as written, their one control, is at the top. */
  .ingredients {
    position: sticky;
    top: calc(var(--bar) + var(--space-4));
    max-height: calc(100dvh - var(--bar) - var(--space-6));
    overflow-y: auto;
    overscroll-behavior: contain;
    scrollbar-width: thin;
    scrollbar-color: var(--ink-3) transparent;
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
    min-height: 44px;
    margin: calc(var(--space-2) * -1) 0;
    border: 0;
    background: none;
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--ink-2);
    text-decoration: underline;
    text-decoration-color: var(--ink-3);
    text-decoration-thickness: 1.5px;
    text-underline-offset: 0.2em;
    padding: 0;
  }
  .as-written:hover {
    color: var(--red);
    text-decoration-color: var(--red);
  }
  /* A section opens on the heavy rule, at reading size. The first one sits
     straight under the panel's own rule, so it doesn't draw a second. */
  .sect {
    font-size: 1.0625rem;
    font-weight: 800;
    margin: var(--space-6) 0 0;
    padding: var(--space-2) 0 var(--space-1);
    border-top: 3px solid var(--ink);
  }
  .col-head + .sect {
    margin-top: var(--space-4);
    padding-top: 0;
    border-top: 0;
  }
  .ingredients li,
  .ing-sheet li {
    display: grid;
    grid-template-columns: 5.25rem 1fr;
    gap: var(--space-3);
    padding: var(--space-2) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
    line-height: 1.4;
  }
  .ingredients li.raw,
  .ing-sheet li.raw {
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
    /* 58ch of Archivo is about 78 characters: ch is the width of a zero,
       wider than the average letter. */
    grid-template-columns: 3rem minmax(0, 58ch);
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
  }
  .notes {
    margin-top: var(--space-3);
    max-width: 58ch;
    white-space: pre-line;
  }
  .notes-block {
    color: var(--ink-2);
  }
  .notes.clamped {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
  }
  .notes-block + .more {
    margin-bottom: calc(var(--space-1) * -1);
  }
  .col-head.after-notes {
    margin-top: var(--space-6);
  }
  .method-end {
    margin-top: var(--space-6);
  }
  .nutrition {
    display: flex;
    flex-wrap: wrap;
    margin: var(--space-3) 0 0;
  }
  .nutrition > div {
    min-width: 6rem;
    padding: var(--space-2) var(--space-5) var(--space-3) 0;
  }
  .nutrition dt {
    color: var(--ink-3);
    margin-bottom: var(--space-1);
  }
  .nutrition dd {
    margin: 0;
  }
  .nutrition .fig {
    font-size: 1.625rem;
  }
  .nutrition .unit {
    margin-left: 3px;
    font-size: 0.9375rem;
    font-weight: 700;
  }

  /* The ingredients peek: a bar at the foot of the phone screen in the
     Method tab, and the sheet it opens. */
  .edit-here,
  .peek {
    display: none;
  }
  .peek {
    position: fixed;
    inset: auto 0 0;
    z-index: 5;
    padding: var(--space-2) var(--gutter) calc(var(--space-2) + env(safe-area-inset-bottom));
    background: var(--ticket);
    border-top: 1px solid var(--perf);
    transform: translateY(100%);
    transition: transform 200ms var(--ease-out);
  }
  .peek.shown {
    transform: none;
  }
  .peek-btn {
    width: 100%;
    min-height: 52px;
    gap: var(--space-2);
  }
  .ing-sheet {
    position: fixed;
    inset: auto 0 0;
    width: 100%;
    max-width: none;
    max-height: 85dvh;
    margin: 0;
    padding: 0;
    border: 0;
    border-top: 3px solid var(--ink);
    background: var(--ticket);
    color: var(--ink);
    flex-direction: column;
    transition: transform 160ms var(--ease-out);
  }
  .ing-sheet[open] {
    display: flex;
  }
  /* The head is the grab: a handle, and the label under it. */
  .sheet-head {
    flex: none;
    padding: var(--space-2) var(--gutter) 0;
    touch-action: none;
    cursor: grab;
  }
  .handle {
    display: block;
    width: 40px;
    height: 4px;
    margin: 0 auto var(--space-3);
    border-radius: 2px;
    background: var(--perf);
  }
  .sheet-head h2 {
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
  }
  .sheet-in {
    flex: 1;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: 0 var(--gutter) var(--space-4);
  }
  .sheet-in > .sect:first-child {
    margin-top: var(--space-4);
    padding-top: 0;
    border-top: 0;
  }
  .sheet-foot {
    flex: none;
    padding: var(--space-3) var(--gutter) calc(var(--space-3) + env(safe-area-inset-bottom));
    border-top: 1px solid var(--perf);
  }
  .ing-sheet::backdrop {
    background: rgb(20 24 28 / 0.4);
  }
  .ing-sheet[open] {
    animation: rise 200ms var(--ease-out);
  }
  @keyframes rise {
    from {
      transform: translateY(24px);
      opacity: 0;
    }
  }
  .sheet-close {
    width: 100%;
    min-height: 52px;
  }
  @media (prefers-reduced-motion: reduce) {
    .peek,
    .ing-sheet[open] {
      transition: none;
      animation: none;
    }
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
    /* The photo follows the header text as a short band (short enough that
       an iPad in portrait still opens on the ingredients), and a photo-less
       recipe shows no initial here. */
    .photo :global(.plate) {
      max-height: min(22vh, 16rem);
    }
    /* Show as written is the panel's only visible head here (the tab names
       it), so it sits at the right like a tool, not alone at the start. */
    .as-written,
    .steps {
      margin-left: auto;
    }
    .body {
      grid-template-columns: 1fr;
    }
    .ingredients {
      position: static;
      max-height: none;
      overflow: visible;
    }
    .peek {
      display: block;
    }
    /* Clear of the peek bar at the foot of the screen. */
    .method[role='tabpanel'] {
      padding-bottom: calc(var(--space-6) + 68px);
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
    .method-end .cook {
      width: 100%;
    }
    /* The masthead already leads back to the library, so the crumb row goes
       and Edit recipe moves into the header, under the title. */
    .crumbs {
      display: none;
    }
    .page {
      padding-top: var(--space-4);
    }
    .src .edit-here {
      display: inline-flex;
    }
    .src.bare {
      display: flex;
    }
    .desc {
      -webkit-line-clamp: 2;
      line-clamp: 2;
    }
    /* Begin cooking and the two marks share one row: the marks as 52px icon
       buttons, named for screen readers. */
    .acts {
      flex-wrap: nowrap;
    }
    .acts .cook {
      flex: 1;
      width: auto;
    }
    .acts .mark {
      flex: none;
      width: 52px;
      padding: 0;
    }
    .mark-name {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }
    /* The photo is a thumbnail beside the title, so the first screen reaches
       the ingredients instead of a band of photo. */
    .top.has-photo {
      grid-template-columns: minmax(0, 1fr) 88px;
      column-gap: var(--space-4);
      row-gap: 0;
    }
    .has-photo .head {
      display: contents;
    }
    .has-photo .head > :global(*) {
      grid-column: 1 / -1;
    }
    .has-photo .head h1 {
      grid-column: 1;
      grid-row: 1;
    }
    .has-photo .head .src {
      grid-column: 1;
      grid-row: 2;
    }
    .has-photo .photo {
      grid-column: 2;
      grid-row: 1 / 3;
    }
    .photo :global(.plate) {
      aspect-ratio: 1 !important;
      max-height: none;
    }
  }
</style>
