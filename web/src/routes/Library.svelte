<script>
  import { flip } from 'svelte/animate';
  import { X, Heart, Bookmark } from '@lucide/svelte';
  import { route, go, link } from '../lib/router.svelte.js';
  import { session } from '../lib/session.svelte.js';
  import { recipeTime } from '../lib/format.js';
  import Preview from '../components/Preview.svelte';

  const params = $derived(route.search);
  const q = $derived((params.get('q') ?? '').trim().toLowerCase());
  const using = $derived((params.get('using') ?? '').split(',').map((s) => s.trim()).filter(Boolean));
  const show = $derived(params.get('show'));

  let draft = $state('');
  let allCourses = $state(false);
  const COURSES = 10;
  let selectedId = $state(null);
  let wide = $state(matchMedia('(min-width: 1100px)').matches);
  $effect(() => {
    const mq = matchMedia('(min-width: 1100px)');
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

  const pool = $derived(
    session.recipes.filter((r) => (show === 'fav' ? r.mine.favorite : show === 'want' ? r.mine.want_to_cook : true)),
  );

  const words = $derived(q.split(/\s+/).filter(Boolean));

  function haystack(r) {
    return `${r.title} ${r.tags.join(' ')} ${r.source_host ?? ''} ${r.ingredient_items.join(' ')}`.toLowerCase();
  }

  // Ranked view: search words must all appear; ingredients on hand rank by
  // how many of them a recipe uses. Rows keep their identity as they rerank.
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

  // Menu view: one section per recipe's first tag, the fullest courses first.
  const sections = $derived.by(() => {
    if (ranked) return [];
    const by = new Map();
    for (const r of pool) {
      const course = r.tags[0] ?? 'Other dishes';
      if (!by.has(course)) by.set(course, []);
      by.get(course).push(r);
    }
    return [...by]
      .map(([name, list]) => ({ name, list: list.sort((a, b) => a.title.localeCompare(b.title)) }))
      .sort((a, b) => (a.name === 'Other dishes') - (b.name === 'Other dishes') || b.list.length - a.list.length);
  });

  const visible = $derived(ranked ? ranked.map((x) => x.r) : sections.flatMap((s) => s.list));

  $effect(() => {
    if (!visible.some((r) => r.id === selectedId)) selectedId = visible[0]?.id ?? null;
  });

  const heading = $derived(
    show === 'fav' ? 'Favorites' : show === 'want' ? 'Want to cook' : `${new Date().toLocaleDateString(undefined, { weekday: 'long' })}’s menu`,
  );

  const subhead = $derived.by(() => {
    const n = visible.length;
    const of = (k) => `${k} ${k === 1 ? 'recipe' : 'recipes'}`;
    if (ranked && using.length) return `${of(n)} use ${list(using)}`;
    if (ranked) return `${of(n)} match “${params.get('q')}”`;
    if (show) return of(n);
    return `${of(n)} in the house`;
  });

  function list(xs) {
    return xs.length < 2 ? xs.join('') : `${xs.slice(0, -1).join(', ')} or ${xs.at(-1)}`;
  }

  function select(r, e) {
    if (!wide) return; // narrow screens open the recipe as a link
    e.preventDefault();
    selectedId = r.id;
  }

  function onkey(e) {
    const tag = e.target.tagName;
    const inField = tag === 'TEXTAREA' || (tag === 'INPUT' && e.target.type !== 'search');
    if (inField || !visible.length || session.asking) return;
    const i = visible.findIndex((r) => r.id === selectedId);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      const next = visible[Math.max(0, Math.min(visible.length - 1, i + (e.key === 'ArrowDown' ? 1 : -1)))];
      selectedId = next.id;
      document.getElementById(`dish-${next.id}`)?.scrollIntoView({ block: 'nearest' });
    } else if (e.key === 'Enter' && selectedId && tag !== 'BUTTON') {
      e.preventDefault();
      go(`/r/${selectedId}`);
    }
  }

  const selected = $derived(session.recipes.find((r) => r.id === selectedId));
</script>

<svelte:window onkeydown={onkey} />

<div class="library" class:wide>
  <main class="menu" aria-labelledby="menu-title">
    <header class="head">
      <h1 id="menu-title">{heading}</h1>
      <span class="sub">{session.loaded ? subhead : 'Setting the menu…'}</span>
    </header>

    <form class="using" onsubmit={addIngredient}>
      <label for="using-input" class="caps">Using</label>
      <ul class="tickets" aria-label="Ingredients on hand">
        {#each using as term (term)}
          <li class="ticket">
            {term}
            <button type="button" onclick={() => removeIngredient(term)} aria-label="Remove {term}"><X size={14} /></button>
          </li>
        {/each}
      </ul>
      <input id="using-input" list="pantry" bind:value={draft} placeholder={using.length ? 'and…' : 'an ingredient you have'} autocomplete="off" />
      <datalist id="pantry">
        {#each pantry as item}<option value={item}></option>{/each}
      </datalist>
      {#if using.length}
        <button type="button" class="clear" onclick={() => setParam('using', '')}>Clear</button>
      {/if}
    </form>

    {#if !ranked && sections.length > 1}
      <nav class="courses" aria-label="Courses">
        {#each allCourses ? sections : sections.slice(0, COURSES) as s (s.name)}
          <a href="#course-{s.name}">{s.name}</a>
        {/each}
        {#if sections.length > COURSES}
          <button class="more-courses" onclick={() => (allCourses = !allCourses)} aria-expanded={allCourses}>
            {allCourses ? 'Fewer' : `${sections.length - COURSES} more`}
          </button>
        {/if}
      </nav>
    {/if}

    {#if !session.loaded}
      <ol class="dishes" aria-hidden="true">
        {#each Array(9) as _, i}
          <li class="ghost" style:--w="{55 + ((i * 37) % 35)}%"></li>
        {/each}
      </ol>
    {:else if !visible.length}
      <div class="empty">
        {#if !session.recipes.length}
          <p>The menu is empty.</p>
          <a class="btn primary" href="/new" use:link>Add the first recipe</a>
        {:else if using.length}
          <p>Nothing on the menu uses {list(using)}.</p>
          <button class="btn" onclick={() => setParam('using', '')}>Clear ingredients</button>
        {:else if q}
          <p>Nothing on the menu matches “{params.get('q')}”.</p>
          <button class="btn" onclick={() => setParam('q', '')}>Clear search</button>
        {:else if show === 'fav'}
          <p>No favorites yet. Use the heart on a recipe to keep it here.</p>
        {:else}
          <p>Nothing saved to cook yet. Use the bookmark on a recipe to plan it.</p>
        {/if}
      </div>
    {:else if ranked}
      <ol class="dishes">
        {#each ranked as { r, matched } (r.id)}
          <li id="dish-{r.id}" animate:flip={{ duration: 360, easing: (t) => 1 - Math.pow(2, -10 * t) }}>
            {@render dish(r, matched)}
          </li>
        {/each}
      </ol>
    {:else}
      {#each sections as s (s.name)}
        <section id="course-{s.name}" class="course" aria-labelledby="h-{s.name}">
          <h2 id="h-{s.name}" class="caps">{s.name}</h2>
          <ol class="dishes">
            {#each s.list as r (r.id)}
              <li id="dish-{r.id}">{@render dish(r, null)}</li>
            {/each}
          </ol>
        </section>
      {/each}
    {/if}
  </main>

  {#if wide && selected}
    <aside class="preview" aria-label="Selected recipe">
      {#key selected.id}<Preview summary={selected} {using} />{/key}
    </aside>
  {/if}
</div>

{#snippet dish(r, matched)}
  <a class="dish" class:sel={wide && r.id === selectedId} href="/r/{r.id}" use:link onclick={(e) => select(r, e)} ondblclick={() => go(`/r/${r.id}`)} aria-current={wide && r.id === selectedId ? 'true' : undefined}>
    <span class="line">
      <span class="name">
        {r.title}
        {#if r.mine.favorite}<Heart class="mark-icon" size={14} aria-label="Favorite" />{/if}
        {#if r.mine.want_to_cook}<Bookmark class="mark-icon" size={14} aria-label="Want to cook" />{/if}
      </span>
      {#if recipeTime(r) || (matched != null && using.length)}<span class="leader" aria-hidden="true"></span>{/if}
      {#if matched != null && using.length}
        <span class="match" class:all={matched === using.length}>{matched} of {using.length}</span>
      {/if}
      <span class="time">{recipeTime(r)}</span>
    </span>
    {#if r.source_host || r.tags.length > 1}
      <span class="by">{[r.source_host, ...r.tags.slice(1)].filter(Boolean).join(' · ')}</span>
    {/if}
  </a>
{/snippet}

<style>
  .library {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }
  .library.wide {
    grid-template-columns: minmax(0, 1fr) minmax(380px, 30vw);
  }
  .menu {
    padding: var(--space-6) var(--gutter) var(--space-8);
    max-width: 52rem;
    width: 100%;
    margin: 0 auto;
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    align-items: baseline;
    gap: 0 var(--space-3);
    text-align: center;
  }
  h1 {
    font-size: 1.5rem;
    font-style: italic;
  }
  .sub {
    font-style: italic;
    color: var(--ink-soft);
    font-size: 1.0625rem;
  }
  .sub::before {
    content: '\2014\2002';
    color: var(--ink-soft);
  }

  .using {
    margin: var(--space-4) 0 var(--space-2);
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    padding: var(--space-3) 0;
    border-top: 1px solid var(--rule);
    border-bottom: 1px solid var(--rule);
  }
  .using > label {
    color: var(--brick);
  }
  .tickets {
    display: contents;
  }
  .ticket {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding: 2px 2px 2px 10px;
    border: 1.5px solid var(--brick);
    color: var(--brick);
    font-family: var(--sans);
    font-size: 0.875rem;
    font-weight: 600;
    border-radius: 2px;
  }
  .ticket button {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border: 0;
    background: none;
    color: inherit;
  }
  .ticket button:hover {
    background: var(--brick-wash);
  }
  .using input {
    flex: 1 1 12rem;
    min-width: 10rem;
    border: 0;
    background: none;
    outline: none;
    font-family: var(--serif);
    font-style: italic;
    font-size: 1.0625rem;
    padding: var(--space-1) 0;
  }
  .clear {
    border: 0;
    background: none;
    font-family: var(--sans);
    font-size: var(--caps);
    font-weight: 600;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--ink-soft);
    padding: var(--space-2);
  }
  .clear:hover {
    color: var(--ink);
  }

  .courses {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: var(--space-1) var(--space-4);
    margin: var(--space-3) 0 0;
    font-family: var(--sans);
    font-size: 0.8125rem;
  }
  .courses a {
    color: var(--ink-soft);
    text-decoration: none;
  }
  .courses a:hover {
    color: var(--brick);
  }
  .more-courses {
    border: 0;
    background: none;
    padding: 0;
    font: inherit;
    font-style: italic;
    color: var(--brick);
  }

  .course {
    margin-top: var(--space-5);
    scroll-margin-top: calc(var(--masthead) + var(--space-5));
  }
  .course h2 {
    text-align: center;
    color: var(--ink);
    letter-spacing: 0.24em;
    margin-bottom: var(--space-3);
  }
  ol.dishes {
    margin-top: var(--space-2);
  }
  .dishes > li {
    scroll-margin: calc(var(--masthead) + var(--space-5)) 0 var(--space-5);
  }
  .dish {
    display: block;
    padding: var(--space-1) 0;
    text-decoration: none;
  }
  .dish:hover .name {
    text-decoration: underline;
    text-decoration-color: var(--ink-faint);
    text-underline-offset: 0.18em;
  }
  .dish.sel .name {
    color: var(--brick);
  }
  .dish.sel .leader {
    border-bottom-color: var(--brick);
  }
  .line {
    display: flex;
    align-items: last baseline;
    gap: var(--space-2);
  }
  .name {
    font-size: var(--dish);
    line-height: 1.3;
    max-width: 44ch;
  }
  .name :global(.mark-icon) {
    display: inline;
    vertical-align: -1px;
    margin-left: 4px;
    color: var(--brick);
    stroke-width: 2;
  }
  .leader {
    flex: 1;
    min-width: var(--space-5);
    border-bottom: 2px dotted var(--ink-faint);
    transform: translateY(-5px);
  }
  .time,
  .match {
    font-family: var(--sans);
    font-size: 0.875rem;
    white-space: nowrap;
  }
  .time {
    color: var(--ink);
    min-width: 4.5em;
    text-align: right;
  }
  .match {
    font-weight: 650;
    color: var(--ink-soft);
  }
  .match.all {
    color: var(--brick);
  }
  .by {
    display: block;
    font-style: italic;
    font-size: 0.9375rem;
    line-height: 1.3;
    color: var(--ink-soft);
  }

  .ghost {
    height: 1.4rem;
    margin: var(--space-4) 0;
    width: var(--w);
    background: var(--paper-press);
  }
  .empty {
    margin: var(--space-7) auto;
    text-align: center;
    display: grid;
    gap: var(--space-4);
    justify-items: center;
    font-style: italic;
    color: var(--ink-soft);
  }

  .preview {
    position: sticky;
    top: calc(var(--masthead) + 4px);
    align-self: start;
    max-height: calc(100dvh - var(--masthead) - 4px);
    overflow-y: auto;
    border-left: 1px solid var(--rule);
    scrollbar-width: thin;
  }

  @media (max-width: 760px) {
    .menu {
      padding-top: var(--space-5);
    }
    .sub {
      flex-basis: 100%;
    }
    .sub::before {
      content: none;
    }
    .name {
      font-size: 1.125rem;
    }
    .course {
      margin-top: var(--space-6);
    }
  }
</style>
