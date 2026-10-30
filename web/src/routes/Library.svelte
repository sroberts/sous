<script>
  import { flip } from 'svelte/animate';
  import { X, Heart, Bookmark } from '@lucide/svelte';
  import { route, go, link } from '../lib/router.svelte.js';
  import { session } from '../lib/session.svelte.js';
  import { ticketTime } from '../lib/format.js';
  import Plate from '../components/Plate.svelte';
  import Preview from '../components/Preview.svelte';

  const params = $derived(route.search);
  const q = $derived((params.get('q') ?? '').trim().toLowerCase());
  const using = $derived((params.get('using') ?? '').split(',').map((s) => s.trim()).filter(Boolean));
  const show = $derived(params.get('show'));
  const course = $derived(params.get('course'));

  let draft = $state('');
  let selectedId = $state(null);
  let grid = $state();
  let wide = $state(matchMedia('(min-width: 1180px)').matches);
  $effect(() => {
    const mq = matchMedia('(min-width: 1180px)');
    const on = () => (wide = mq.matches);
    mq.addEventListener('change', on);
    return () => mq.removeEventListener('change', on);
  });

  function setParam(key, value) {
    const next = new URLSearchParams(params);
    value ? next.set(key, value) : next.delete(key);
    go(`/${next.size ? `?${next}` : ''}`, { replace: true });
  }

  function addIngredient(e) {
    e.preventDefault();
    const term = draft.trim().toLowerCase();
    if (term && !using.includes(term)) setParam('using', [...using, term].join(','));
    draft = '';
  }

  const removeIngredient = (term) => setParam('using', using.filter((t) => t !== term).join(','));

  // Ingredient names across the library, most common first, for suggestions.
  const pantry = $derived.by(() => {
    const counts = new Map();
    for (const r of session.recipes) for (const item of new Set(r.ingredient_items)) counts.set(item, (counts.get(item) ?? 0) + 1);
    return [...counts].filter(([, n]) => n > 2).sort((a, b) => b[1] - a[1]).slice(0, 300).map(([i]) => i);
  });

  // Courses are each recipe's first tag, the fullest first.
  const courses = $derived.by(() => {
    const by = new Map();
    for (const r of session.recipes) {
      const c = r.tags[0];
      if (c) by.set(c, (by.get(c) ?? 0) + 1);
    }
    return [...by].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  });

  const pool = $derived(
    session.recipes.filter(
      (r) =>
        (show === 'fav' ? r.mine.favorite : show === 'want' ? r.mine.want_to_cook : true) &&
        (!course || r.tags.some((t) => t.toLowerCase() === course.toLowerCase())),
    ),
  );

  const words = $derived(q.split(/\s+/).filter(Boolean));

  function haystack(r) {
    return `${r.title} ${r.tags.join(' ')} ${r.source_host ?? ''} ${r.ingredient_items.join(' ')}`.toLowerCase();
  }

  // Search words must all appear; ingredients on hand rank recipes by how
  // many of them they use. Tickets keep their identity as they rerank.
  const ranked = $derived.by(() => {
    if (!words.length && !using.length) return null;
    const out = [];
    for (const r of pool) {
      const hay = haystack(r);
      if (!words.every((w) => hay.includes(w))) continue;
      const matched = using.filter((t) => r.ingredient_items.some((i) => i.includes(t)) || r.title.toLowerCase().includes(t)).length;
      if (using.length && !matched) continue;
      const titleHit = words.length && words.every((w) => r.title.toLowerCase().includes(w));
      out.push({ r, matched, score: matched * 10 + (titleHit ? 5 : 0) });
    }
    return out.sort((a, b) => b.score - a.score || a.r.title.localeCompare(b.r.title));
  });

  const rows = $derived(ranked ?? [...pool].sort((a, b) => a.title.localeCompare(b.title)).map((r) => ({ r, matched: null })));
  const visible = $derived(rows.map((x) => x.r));

  // The rail holds what this person means to cook, on the plain library view.
  const rail = $derived(!show && !course && !ranked ? session.recipes.filter((r) => r.mine.want_to_cook) : []);

  $effect(() => {
    if (!visible.some((r) => r.id === selectedId)) selectedId = visible[0]?.id ?? null;
  });

  const heading = $derived(
    ranked && !using.length ? `“${params.get('q')}”` : show === 'fav' ? 'Favorites' : show === 'want' ? 'Want to cook' : (course ?? 'Library'),
  );

  const of = (k) => `${k} ${k === 1 ? 'recipe' : 'recipes'}`;
  const subhead = $derived(ranked && using.length ? `${of(visible.length)} use ${list(using)}` : of(visible.length));

  function list(xs) {
    return xs.length < 2 ? xs.join('') : `${xs.slice(0, -1).join(', ')} or ${xs.at(-1)}`;
  }

  function select(r, e) {
    if (!wide) return; // narrow screens open the recipe as a link
    e.preventDefault();
    selectedId = r.id;
  }

  // Arrow keys move through the grid as it is laid out on screen.
  function onkey(e) {
    const tag = e.target.tagName;
    const inField = tag === 'TEXTAREA' || (tag === 'INPUT' && e.target.type !== 'search');
    if (inField || !visible.length || session.asking) return;
    const i = Math.max(0, visible.findIndex((r) => r.id === selectedId));
    const items = grid?.children ?? [];
    let cols = 1;
    while (cols < items.length && items[cols].offsetTop === items[0].offsetTop) cols++;
    const step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: cols, ArrowUp: -cols }[e.key];
    if (step && !(tag === 'INPUT' && (e.key === 'ArrowLeft' || e.key === 'ArrowRight'))) {
      e.preventDefault();
      const next = visible[Math.max(0, Math.min(visible.length - 1, i + step))];
      selectedId = next.id;
      document.getElementById(`t-${next.id}`)?.scrollIntoView({ block: 'nearest' });
    } else if (e.key === 'Enter' && selectedId && tag !== 'BUTTON') {
      e.preventDefault();
      go(`/r/${selectedId}`);
    }
  }

  const selected = $derived(session.recipes.find((r) => r.id === selectedId));
  const ease = (t) => 1 - Math.pow(2, -10 * t);
</script>

<svelte:window onkeydown={onkey} />

<div class="library" class:wide>
  <nav class="courses" aria-label="Courses">
    <a href="/{show ? `?show=${show}` : ''}" use:link class:on={!course} aria-current={!course ? 'true' : undefined}>
      <span>Everything</span><span class="fig n">{session.recipes.length}</span>
    </a>
    {#each courses as [name, n] (name)}
      <a href="/?{new URLSearchParams({ ...(show ? { show } : {}), course: name })}" use:link class:on={course === name} aria-current={course === name ? 'true' : undefined}>
        <span>{name}</span><span class="fig n">{n}</span>
      </a>
    {/each}
  </nav>

  <main class="pass" aria-labelledby="lib-title">
    {#if rail.length}
      <section class="rail-wrap" aria-labelledby="rail-title">
        <h2 id="rail-title" class="label">Want to cook <span class="count">{rail.length} on the rail</span></h2>
        <div class="rail"></div>
        <ul class="hung">
          {#each rail as r (r.id)}
            {@const t = ticketTime(r.total_minutes, r.time_text)}
            <li>
              <a class="hang ticket" href="/r/{r.id}" use:link>
                <span class="clip" aria-hidden="true"></span>
                <span class="hang-head">
                  {#if t?.fig}<span class="fig big">{t.fig}<small>{t.unit}</small></span>{:else}<span class="fig big" title={t?.text}>—<small>{t?.text ? 'see recipe' : 'no time'}</small></span>{/if}
                  <span class="fig ing">{r.ingredient_items.length} ing</span>
                </span>
                <hr class="perf" />
                <span class="hang-title">{r.title}</span>
                <span class="host">{r.source_host ?? r.tags[0] ?? ''}</span>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <header class="head">
      <h1 id="lib-title">{heading}</h1>
      <span class="sub">{session.loaded ? subhead : 'Loading the library…'}</span>
      <form class="using" onsubmit={addIngredient}>
        <label for="using-input" class="label">Using</label>
        {#each using as term (term)}
          <span class="chip">
            {term}
            <button type="button" onclick={() => removeIngredient(term)} aria-label="Remove {term}"><X size={14} /></button>
          </span>
        {/each}
        <input id="using-input" list="pantry" bind:value={draft} placeholder={using.length ? 'and…' : 'ingredients you have'} autocomplete="off" />
        <datalist id="pantry">
          {#each pantry as item}<option value={item}></option>{/each}
        </datalist>
        {#if using.length}
          <button type="button" class="clear" onclick={() => setParam('using', '')}>Clear</button>
        {/if}
      </form>
    </header>

    {#if !session.loaded}
      <ol class="grid" aria-hidden="true">
        {#each Array(12) as _}
          <li class="ticket ghost"><span></span></li>
        {/each}
      </ol>
    {:else if !visible.length}
      <div class="empty">
        {#if !session.recipes.length}
          <p>No recipes yet.</p>
          <a class="btn primary" href="/new" use:link>Add the first recipe</a>
        {:else if using.length}
          <p>Nothing here uses {list(using)}.</p>
          <button class="btn" onclick={() => setParam('using', '')}>Clear ingredients</button>
        {:else if q}
          <p>Nothing matches “{params.get('q')}”.</p>
          <button class="btn" onclick={() => setParam('q', '')}>Clear search</button>
        {:else if show === 'fav'}
          <p>No favorites yet. Use the heart on a recipe to keep it here.</p>
        {:else}
          <p>Nothing on the rail yet. Use “Want to cook” on a recipe to plan it.</p>
        {/if}
      </div>
    {:else}
      <ol class="grid" bind:this={grid}>
        {#each rows as { r, matched } (r.id)}
          {@const t = ticketTime(r.total_minutes, r.time_text)}
          {@const sel = wide && r.id === selectedId}
          <li id="t-{r.id}" animate:flip={{ duration: ranked ? 360 : 0, easing: ease }}>
            <a class="ticket card" class:sel href="/r/{r.id}" use:link onclick={(e) => select(r, e)} ondblclick={() => go(`/r/${r.id}`)} aria-current={sel ? 'true' : undefined}>
              <Plate hash={r.cover} title={r.title} size="s" />
              <span class="body">
                <span class="title">
                  {r.title}
                  {#if r.mine.favorite}<Heart class="mark" size={14} aria-label="Favorite" />{/if}
                  {#if r.mine.want_to_cook}<Bookmark class="mark" size={14} aria-label="Want to cook" />{/if}
                </span>
                {#if r.source_host}<span class="host">{r.source_host}</span>{/if}
              </span>
              <hr class="perf" />
              <span class="foot">
                {#if t?.fig}
                  <span class="fig time">{t.fig}<small>{t.unit}</small></span>
                {:else}
                  <span class="notime">{t?.text ?? ''}</span>
                {/if}
                {#if matched != null && using.length}
                  <span class="fig match" class:all={matched === using.length}>{matched}/{using.length}</span>
                {/if}
                <span class="fig ing">{r.ingredient_items.length} ing</span>
              </span>
            </a>
          </li>
        {/each}
      </ol>
    {/if}
  </main>

  {#if wide && selected}
    <aside class="preview" aria-label="Selected recipe">
      {#key selected.id}<Preview summary={selected} {using} />{/key}
    </aside>
  {/if}
</div>

<style>
  .library {
    display: grid;
    grid-template-columns: 200px minmax(0, 1fr);
    min-height: calc(100dvh - var(--bar));
  }
  .library.wide {
    grid-template-columns: 200px minmax(0, 1fr) minmax(360px, 26vw);
  }

  /* Courses */
  .courses {
    position: sticky;
    top: var(--bar);
    align-self: start;
    max-height: calc(100dvh - var(--bar));
    overflow-y: auto;
    scrollbar-width: thin;
    padding: var(--space-4) 0 var(--space-6);
    border-right: 1px solid var(--perf);
  }
  .courses a {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: var(--space-2);
    padding: 6px var(--space-4) 6px var(--gutter);
    font-size: var(--ui);
    font-weight: 500;
    color: var(--ink-2);
    text-decoration: none;
  }
  .courses a:first-child {
    font-weight: 700;
    color: var(--ink);
    margin-bottom: var(--space-2);
  }
  .courses a:hover {
    background: var(--pass-deep);
    color: var(--ink);
  }
  .courses a:hover .n {
    color: var(--ink-2);
  }
  .courses a.on {
    background: var(--ticket);
    color: var(--ink);
    font-weight: 700;
  }
  .courses .n {
    font-size: 1.0625rem;
    color: var(--ink-3);
  }
  .courses a.on .n {
    color: var(--red);
  }

  .pass {
    padding: var(--space-5) var(--gutter) var(--space-8);
    min-width: 0;
  }

  /* The rail */
  .rail-wrap {
    margin-bottom: var(--space-6);
  }
  .rail-wrap h2 {
    margin-bottom: var(--space-2);
  }
  .count {
    color: var(--ink-3);
    margin-left: var(--space-2);
  }
  .rail-wrap .rail {
    position: relative;
    z-index: 1;
  }
  .hung {
    display: flex;
    gap: var(--space-4);
    padding: 0 var(--space-4);
    margin-top: -4px;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .hang {
    position: relative;
    display: grid;
    gap: var(--space-2);
    width: 15rem;
    padding: var(--space-4) var(--space-4) calc(var(--space-4) + var(--tear));
    text-decoration: none;
    transform-origin: top center;
    transition: transform 240ms var(--ease-out);
  }
  .hang:hover {
    transform: rotate(-1deg);
  }
  .clip {
    position: absolute;
    top: 0;
    left: 50%;
    width: 36px;
    height: 9px;
    margin-left: -18px;
    border-radius: 0 0 3px 3px;
    background: linear-gradient(var(--steel), var(--steel-lo));
  }
  .hang-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    padding-top: var(--space-1);
  }
  .big {
    font-size: 2.75rem;
  }
  .big small,
  .time small {
    font-size: 0.4em;
    font-stretch: 100%;
    font-weight: 700;
    margin-left: 3px;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }
  .ing {
    font-size: 0.9375rem;
    color: var(--ink-2);
    text-transform: uppercase;
    letter-spacing: 0.02em;
  }
  .hang-title {
    font-size: var(--title-m);
    font-weight: 700;
    line-height: 1.2;
    letter-spacing: -0.01em;
  }
  .host {
    font-size: 0.8125rem;
    color: var(--ink-3);
    /* A long domain is one unbreakable word; it mustn't widen a narrow card. */
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Head and the Using filter */
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-2) var(--space-3);
    margin-bottom: var(--space-4);
  }
  h1 {
    font-size: var(--title-l);
    font-weight: 800;
  }
  .sub {
    color: var(--ink-2);
    font-weight: 500;
  }
  .using {
    margin-left: auto;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    min-height: 40px;
    padding: 0 var(--space-2) 0 var(--space-3);
    background: var(--ticket);
    border: 2px solid var(--perf);
    border-radius: 3px;
    align-self: center;
  }
  .using:focus-within {
    border-color: var(--ink);
  }
  .using > label {
    color: var(--red);
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding-left: var(--space-2);
    border: 2px solid var(--ink);
    border-radius: 3px;
    font-size: 0.875rem;
    font-weight: 700;
  }
  .chip button {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    border: 0;
    background: none;
  }
  .chip button:hover {
    color: var(--red);
  }
  .using input {
    width: 11rem;
    border: 0;
    background: none;
    outline: none;
    font-size: var(--ui);
    padding: var(--space-2) 0;
  }
  .clear {
    border: 0;
    background: none;
    font-size: 0.8125rem;
    font-weight: 700;
    color: var(--ink-2);
    padding: var(--space-2);
  }
  .clear:hover {
    color: var(--red);
  }

  /* The ticket grid */
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(176px, 1fr));
    gap: var(--space-4) var(--space-3);
  }
  .grid > li {
    scroll-margin: calc(var(--bar) + var(--space-4)) 0 var(--space-4);
  }
  .card {
    display: flex;
    flex-direction: column;
    height: 100%;
    text-decoration: none;
    border-top: 3px solid var(--ticket);
    transition: background-color 120ms;
  }
  .card:hover {
    background: var(--ticket-press);
    border-top-color: var(--ticket-press);
  }
  .card.sel {
    border-top-color: var(--red);
  }
  .card :global(.plate) {
    margin: 0 var(--space-2);
    width: auto;
  }
  .body {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--space-2) var(--space-3) var(--space-3);
  }
  .title {
    font-size: 0.9375rem;
    font-weight: 700;
    line-height: 1.25;
    letter-spacing: -0.005em;
  }
  .title :global(.mark) {
    display: inline;
    vertical-align: -2px;
    margin-left: 3px;
    color: var(--red);
    stroke-width: 2.25;
  }
  .card .perf {
    margin: 0 var(--space-3);
  }
  .foot {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3) var(--space-1);
  }
  .time {
    font-size: 1.625rem;
  }
  .card.sel .time {
    color: var(--red);
  }
  .notime {
    font-size: 0.75rem;
    color: var(--ink-3);
    line-height: 1.2;
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .foot .ing {
    margin-left: auto;
    font-size: 0.875rem;
    color: var(--ink-3);
  }
  .match {
    font-size: 0.875rem;
    padding: 2px 4px;
    border: 1.5px solid var(--ink-3);
    border-radius: 2px;
    color: var(--ink-2);
  }
  .match.all {
    border-color: var(--red);
    color: var(--red);
  }

  .ghost {
    height: 15rem;
    background: var(--pass-deep);
  }
  .empty {
    margin: var(--space-7) 0;
    display: grid;
    gap: var(--space-4);
    justify-items: start;
    color: var(--ink-2);
    font-size: var(--title-m);
  }

  .preview {
    position: sticky;
    top: var(--bar);
    align-self: start;
    max-height: calc(100dvh - var(--bar));
    overflow-y: auto;
    scrollbar-width: thin;
    background: var(--ticket);
    border-left: 1px solid var(--perf);
  }

  @media (max-width: 860px) {
    .library {
      grid-template-columns: minmax(0, 1fr);
    }
    .courses {
      position: static;
      display: flex;
      gap: var(--space-1);
      overflow-x: auto;
      max-height: none;
      padding: var(--space-3) var(--gutter) 0;
      border-right: 0;
      scrollbar-width: none;
    }
    .courses a,
    .courses a:first-child {
      flex: none;
      margin: 0;
      padding: 6px 12px;
      border-radius: 3px;
      background: var(--ticket);
      gap: var(--space-2);
    }
    .courses a.on {
      box-shadow: inset 0 -3px 0 var(--red);
    }
    .pass {
      padding-top: var(--space-4);
    }
    .using {
      margin-left: 0;
      width: 100%;
    }
    .using input {
      flex: 1;
    }
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
      gap: var(--space-3) var(--space-2);
    }
    .hang {
      width: 13rem;
    }
  }
</style>
