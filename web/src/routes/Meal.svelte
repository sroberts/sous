<script>
  import { ArrowLeft, X, TriangleAlert, Hand, Pin, ArrowUp, ArrowDown, Plus } from '@lucide/svelte';
  import { api } from '../lib/api.js';
  import { go, link } from '../lib/router.svelte.js';
  import { session, asPerson, toast } from '../lib/session.svelte.js';
  import { isOffline } from '../lib/net.svelte.js';
  import { byDay, clockAt, whenAt, span, isoToZoned, zonedToISO } from '../lib/meals.js';

  let { id } = $props();

  let meal = $state(null);
  let plan = $state(null);
  let kitchen = $state(null);
  let problem = $state('');
  let busy = $state(false);

  async function load() {
    try {
      [meal, plan, kitchen] = await Promise.all([api.meal(id), api.mealPlan(id), api.kitchen()]);
      document.title = `${meal.title} · Sous`;
    } catch (err) {
      problem = err.code === 'offline' ? 'Meals need a connection to Sous.' : err.message;
    }
  }
  load();

  const tz = $derived(meal?.timezone ?? 'UTC');

  // Every change saves as it's made. If someone else changed the meal in
  // the meantime, the change is applied again to theirs, once.
  async function patch(body, retry = true) {
    busy = true;
    problem = '';
    try {
      const m = await asPerson(() => api.updateMeal(id, meal.version, body));
      if (!m) return;
      meal = m;
      plan = await api.mealPlan(id);
    } catch (err) {
      if (err.status === 412 && retry) {
        meal = err.body.current;
        return patch(body, false);
      }
      problem = err.message;
    } finally {
      busy = false;
    }
  }

  const itemsInput = (items) => items.map((i) => ({ id: i.id, recipe_id: i.recipe_id, serve_offset_minutes: i.serve_offset_minutes, dish: i.dish }));
  const setItem = (i, change) => patch({ items: itemsInput(meal.items).map((it, j) => (j === i ? { ...it, ...change } : it)) });
  const removeItem = (i) => patch({ items: itemsInput(meal.items).filter((_, j) => j !== i) });
  function moveItem(i, by) {
    const items = itemsInput(meal.items);
    const [it] = items.splice(i, 1);
    items.splice(i + by, 0, it);
    patch({ items });
  }

  // Adding a dish: search the library on this device.
  let query = $state('');
  const matches = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!q || !meal) return [];
    const have = new Set(meal.items.map((i) => i.recipe_id));
    return session.recipes.filter((r) => !have.has(r.id) && r.title.toLowerCase().includes(q)).slice(0, 8);
  });
  async function add(r) {
    query = '';
    await patch({ items: [...itemsInput(meal.items), { recipe_id: r.id }] });
    document.getElementById('add-dish')?.focus();
  }
  function addKey(e) {
    if (e.key === 'Enter' && matches.length) {
      e.preventDefault();
      add(matches[0]);
    }
  }

  const OFFSETS = [0, 15, 30, 45, 60, 90, 120];

  // Pinning keeps a task where it is while the rest of the plan moves.
  async function pin(t) {
    try {
      const p = await asPerson(() => api.updateTask(id, t.key, { pinned_start: t.pinned ? null : t.start }));
      if (p) plan = p;
    } catch (err) {
      problem = err.message;
    }
  }

  let confirmDelete = $state(false);
  async function remove() {
    const ok = await asPerson(() => api.deleteMeal(id, meal.version).then(() => true));
    if (!ok) return;
    const { title } = meal;
    go('/meals');
    toast(`${title} is in the trash.`, {
      label: 'Undo',
      run: async () => {
        try {
          await api.restoreMeal(id);
          go(`/m/${id}`);
        } catch (err) {
          toast(`${title} couldn’t be restored: ${err.message}`);
        }
      },
    });
  }

  const days = $derived(plan ? byDay(plan.tasks, tz) : []);
  const taskTitles = $derived(new Map((plan?.tasks ?? []).map((t) => [t.key, t.recipe_title])));
  const guessed = $derived(new Set((plan?.warnings ?? []).filter((w) => w.code === 'estimated').map((w) => w.item_id)));
  const warnings = $derived((plan?.warnings ?? []).filter((w) => w.code !== 'estimated'));
  const handsOn = $derived((plan?.tasks ?? []).filter((t) => t.hands_on && !t.make_ahead).reduce((n, t) => n + t.minutes, 0));
  const ahead = $derived((plan?.tasks ?? []).filter((t) => t.make_ahead).length);
</script>

<main class="meal" id="main">
  <a class="back" href="/meals" use:link><ArrowLeft size={16} aria-hidden="true" />Meals</a>

  {#if problem && !meal}
    <h1>That meal couldn’t be opened.</h1>
    <p class="problem">{problem}</p>
  {:else if meal && plan}
    <header class="head panel">
      <div class="title-row">
        <div>
          <h1>{meal.title}</h1>
          <p class="served">Served {whenAt(meal.serve_at, tz)}</p>
        </div>
        <a class="btn primary cook" href="/m/{id}/cook" use:link aria-disabled={!plan.tasks.length}>Cook the meal</a>
      </div>
      <dl class="facts">
        <div><dt class="label">Dishes</dt><dd class="fig">{meal.items.length}</dd></div>
        <div><dt class="label">Starts</dt><dd class="fig">{plan.start ? clockAt(plan.start, tz) : '–'}</dd></div>
        <div><dt class="label">On the day</dt><dd class="fig">{span(handsOn) || '–'}<span class="unit">hands-on</span></dd></div>
        <div><dt class="label">Made ahead</dt><dd class="fig">{ahead}<span class="unit">{ahead === 1 ? 'step' : 'steps'}</span></dd></div>
      </dl>
      {#if plan.start}<p class="start-line">Starts {whenAt(plan.start, tz)}{plan.on_time ? '' : ` · about ${span(plan.behind_minutes)} behind`}</p>{/if}
      {#if problem}<p class="problem" role="alert">{problem}</p>{/if}
    </header>

    <div class="cols">
      <div class="side">
        <section class="panel menu" aria-labelledby="menu-h">
          <h2 id="menu-h" class="label">Menu</h2>
          {#if meal.items.length}
            <ol class="items">
              {#each meal.items as it, i (it.id)}
                <li>
                  <div class="it-head">
                    <a class="it-title" href="/r/{it.recipe_id}" use:link>{it.title}</a>
                    <div class="it-acts">
                      <button type="button" class="icon" onclick={() => moveItem(i, -1)} disabled={i === 0 || busy} aria-label="Move {it.title} up"><ArrowUp size={16} /></button>
                      <button type="button" class="icon" onclick={() => moveItem(i, 1)} disabled={i === meal.items.length - 1 || busy} aria-label="Move {it.title} down"><ArrowDown size={16} /></button>
                      <button type="button" class="icon" onclick={() => removeItem(i)} disabled={busy} aria-label="Take {it.title} off the menu"><X size={16} /></button>
                    </div>
                  </div>
                  <div class="it-fields">
                    <label>
                      <span class="visually-hidden">When {it.title} is served</span>
                      <select value={it.serve_offset_minutes} onchange={(e) => setItem(i, { serve_offset_minutes: +e.currentTarget.value })} disabled={busy}>
                        {#each OFFSETS as o}<option value={o}>{o ? `${span(o)} after` : 'With the meal'}</option>{/each}
                        {#if !OFFSETS.includes(it.serve_offset_minutes)}<option value={it.serve_offset_minutes}>{span(it.serve_offset_minutes)} after</option>{/if}
                      </select>
                    </label>
                    <label>
                      <span class="visually-hidden">Serving dish for {it.title}</span>
                      <select value={it.dish ?? ''} onchange={(e) => setItem(i, { dish: e.currentTarget.value || null })} disabled={busy}>
                        <option value="">No dish chosen</option>
                        {#each kitchen?.dishes ?? [] as d}<option value={d.name}>{d.name}</option>{/each}
                        {#if it.dish && !kitchen?.dishes.some((d) => d.name === it.dish)}<option value={it.dish}>{it.dish}</option>{/if}
                      </select>
                    </label>
                  </div>
                  {#if guessed.has(it.id)}
                    <p class="it-note">Some times are guessed from the words. <a href="/r/{it.recipe_id}/edit" use:link>Set its timing</a></p>
                  {/if}
                </li>
              {/each}
            </ol>
          {:else}
            <p class="hint">Add the dishes you’re making.</p>
          {/if}
          <div class="add">
            <label class="visually-hidden" for="add-dish">Add a dish from the library</label>
            <Plus size={18} aria-hidden="true" />
            <input id="add-dish" type="search" bind:value={query} onkeydown={addKey} placeholder="Add a dish from the library" autocomplete="off" disabled={isOffline()} />
          </div>
          {#if matches.length}
            <ul class="matches" aria-label="Recipes that match">
              {#each matches as r (r.id)}
                <li><button type="button" onclick={() => add(r)}>{r.title}</button></li>
              {/each}
            </ul>
          {:else if query.trim()}
            <p class="hint">No recipe in the library is called that.</p>
          {/if}
        </section>

        <section class="panel settings" aria-labelledby="set-h">
          <h2 id="set-h" class="label">The meal</h2>
          <label class="field">
            <span class="label">Name</span>
            <input value={meal.title} onchange={(e) => e.currentTarget.value.trim() && patch({ title: e.currentTarget.value.trim() })} />
          </label>
          <label class="field">
            <span class="label">Served</span>
            <input type="datetime-local" value={isoToZoned(meal.serve_at, tz)} onchange={(e) => e.currentTarget.value && patch({ serve_at: zonedToISO(e.currentTarget.value, tz) })} />
          </label>
          <div class="pair">
            <label class="field">
              <span class="label">Cooks</span>
              <select value={meal.cooks} onchange={(e) => patch({ cooks: +e.currentTarget.value })}>
                {#each [1, 2, 3, 4, 5, 6, 7, 8] as n}<option value={n}>{n === 1 ? '1 cook' : `${n} cooks`}</option>{/each}
              </select>
            </label>
            <label class="field">
              <span class="label">Prep from</span>
              <select value={meal.prep_days} onchange={(e) => patch({ prep_days: +e.currentTarget.value })}>
                <option value={0}>The day itself</option>
                {#each [1, 2, 3, 4, 5, 6, 7] as n}<option value={n}>{n === 1 ? 'The day before' : `${n} days before`}</option>{/each}
              </select>
            </label>
          </div>
          <p class="hint">Times are in {tz.replace(/_/g, ' ')}. <a href="/kitchen" use:link>Your kitchen</a> sets the ovens, burners, dishes and the hours of the day.</p>
          {#if confirmDelete}
            <div class="confirm" role="alertdialog" aria-labelledby="del-q">
              <p id="del-q">Move this meal to the trash?</p>
              <button type="button" class="btn quiet" onclick={remove}>Move to trash</button>
              <button type="button" class="btn" onclick={() => (confirmDelete = false)}>Keep it</button>
            </div>
          {:else}
            <button type="button" class="btn quiet trash" onclick={() => (confirmDelete = true)}>Move to trash</button>
          {/if}
        </section>
      </div>

      <section class="panel plan" aria-labelledby="plan-h">
        <h2 id="plan-h" class="label">Plan</h2>
        {#if warnings.length}
          <ul class="warnings">
            {#each warnings as w}
              <li><TriangleAlert size={18} aria-hidden="true" />{w.message}</li>
            {/each}
          </ul>
        {/if}
        {#if !plan.tasks.length}
          <p class="hint">Add dishes to the menu and their steps appear here, laid out in time.</p>
        {/if}
        {#each days as day (day.key)}
          <h3 class="day">{day.label}</h3>
          <ol class="tasks">
            {#each day.tasks as t (t.key)}
              <li class="task" class:quiet={t.kind === 'preheat'} class:done={t.status === 'done'}>
                <span class="when fig">{clockAt(t.start, tz)}</span>
                <div class="body">
                  <p class="text">{t.text}</p>
                  <p class="meta">
                    {#if t.recipe_title}<span>{t.recipe_title}{t.step ? ` · step ${t.step}` : ''}</span>{/if}
                    <span>{span(t.minutes)}{t.estimated ? ' (guessed)' : ''}</span>
                    {#if t.oven}<span>{t.oven}{t.oven_temp ? ` ${t.oven_temp}°F` : ''}</span>{:else if t.equipment}<span>{t.equipment}</span>{/if}
                    {#if t.hands_on && t.minutes}<span class="hands"><Hand size={14} aria-hidden="true" />hands-on</span>{/if}
                  </p>
                  {#if t.hold}<p class="hold">Then it waits {t.hold.in === 'warm' ? 'somewhere warm' : `in the ${t.hold.in}`} until {whenAt(t.hold.until, tz)}.</p>{/if}
                </div>
                {#if t.kind !== 'preheat'}
                  <button type="button" class="icon pin" aria-pressed={t.pinned} onclick={() => pin(t)} title={t.pinned ? 'Unpin: let the plan move it' : 'Pin it here while the rest moves'} aria-label="{t.pinned ? 'Unpin' : 'Pin'} {t.text}"><Pin size={16} /></button>
                {/if}
              </li>
            {/each}
          </ol>
        {/each}

        {#if plan.ovens.some((o) => o.uses.length)}
          <h3 class="day">Ovens</h3>
          {#each plan.ovens as o}
            <div class="oven">
              <p class="o-name">{o.name}<span class="o-racks">{o.racks} {o.racks === 1 ? 'rack' : 'racks'}</span></p>
              <ul>
                {#each o.uses as u}
                  <li>
                    <span class="fig o-temp">{u.temp ? `${u.temp}°` : '–'}</span>
                    <span>{clockAt(u.start, tz)}–{clockAt(u.end, tz)}</span>
                    <span class="o-what">{[...new Set(u.tasks.map((k) => taskTitles.get(k)).filter(Boolean))].join(', ')}</span>
                  </li>
                {/each}
              </ul>
            </div>
          {/each}
        {/if}

        {#if plan.serving.length}
          <h3 class="day">To the table</h3>
          <ul class="serving">
            {#each plan.serving as s}
              <li><span class="fig">{clockAt(s.serve_at, tz)}</span><span>{s.recipe_title}</span><span class="dish">{s.dish ?? 'No dish chosen'}</span></li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>
  {:else}
    <p class="hint" role="status">Loading the meal…</p>
  {/if}
</main>

<style>
  .meal {
    max-width: 82rem;
    margin: 0 auto;
    padding: var(--space-4) var(--gutter) var(--space-8);
  }
  .back {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    min-height: 44px;
    color: var(--ink-2);
    font-size: var(--ui);
    font-weight: 600;
    text-decoration: none;
  }
  .back:hover {
    color: var(--red);
  }
  h1 {
    font-size: var(--title-xl);
    font-weight: 800;
    line-height: 1.04;
  }
  .panel {
    background: var(--ticket);
    border-top: 3px solid var(--ink);
    padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .panel > .label {
    color: var(--ink-3);
    margin-bottom: var(--space-3);
  }
  .head {
    display: grid;
    gap: var(--space-4);
  }
  .title-row {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }
  .served {
    margin-top: var(--space-2);
    color: var(--ink-2);
  }
  .cook {
    min-height: 52px;
  }
  .cook[aria-disabled='true'] {
    pointer-events: none;
    background: transparent;
    border-color: var(--perf);
    color: var(--ink-3);
  }
  .facts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-5);
    margin: 0;
    padding: var(--space-3) 0;
    border-top: 3px solid var(--ink);
    border-bottom: 1.5px dashed var(--perf);
  }
  .facts div {
    min-width: 4.5rem;
  }
  .facts dt {
    color: var(--ink-3);
  }
  .facts dd {
    margin: var(--space-1) 0 0;
    font-size: 2.25rem;
  }
  .unit {
    margin-left: 0.25em;
    font-size: 0.4em;
    font-stretch: 100%;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .start-line {
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .problem {
    font-weight: 700;
    color: var(--red);
  }
  .cols {
    display: grid;
    grid-template-columns: minmax(18rem, 25rem) minmax(0, 1fr);
    gap: var(--space-4);
    align-items: start;
    margin-top: var(--space-4);
  }
  .side {
    display: grid;
    gap: var(--space-4);
  }
  .hint {
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .hint a,
  .it-note a {
    color: inherit;
  }
  .items {
    list-style: none;
    padding: 0;
    margin: 0;
  }
  .items li {
    padding: var(--space-3) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
  }
  .it-head {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
  }
  .it-title {
    flex: 1;
    font-weight: 700;
    color: inherit;
    text-decoration: none;
    padding-top: 10px;
  }
  .it-title:hover {
    color: var(--red);
  }
  .it-acts {
    display: flex;
  }
  .icon {
    display: inline-grid;
    place-items: center;
    width: 44px;
    height: 44px;
    border-radius: 3px;
    color: var(--ink-2);
  }
  .icon:hover:not(:disabled) {
    background: var(--pass-deep);
    color: var(--ink);
  }
  .icon:disabled {
    color: var(--perf);
    cursor: default;
  }
  .it-fields {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-3);
  }
  select {
    min-height: 40px;
    max-width: 100%;
    font-size: max(16px, var(--ui));
    font-weight: 600;
    background: var(--ticket);
    border: 2px solid var(--ink);
    border-radius: 3px;
    padding: 0 var(--space-2);
  }
  .it-note {
    margin-top: var(--space-2);
    font-size: 0.875rem;
    color: var(--ink-2);
  }
  .add {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-3);
    border-bottom: 1.5px solid var(--field);
    color: var(--ink-3);
  }
  .add:focus-within {
    border-bottom-color: var(--red);
  }
  .add input,
  .field input {
    flex: 1;
    min-height: 44px;
    border: 0;
    background: transparent;
    font-size: max(16px, var(--read));
  }
  .add input:focus,
  .field input:focus {
    outline: none;
  }
  .matches {
    list-style: none;
    margin: var(--space-1) 0 0;
    padding: 0;
  }
  .matches button {
    width: 100%;
    min-height: 44px;
    text-align: left;
    padding: 0 var(--space-2);
    border-radius: 3px;
    font-weight: 600;
  }
  .matches button:hover,
  .matches button:focus-visible {
    background: var(--ticket-press);
  }
  .settings {
    display: grid;
    gap: var(--space-3);
  }
  .field {
    display: grid;
    gap: var(--space-1);
  }
  .field .label {
    color: var(--ink-3);
  }
  .field input {
    border-bottom: 1.5px solid var(--field);
  }
  .field input:focus {
    border-bottom-color: var(--red);
  }
  .pair {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3);
  }
  .trash {
    justify-self: start;
    color: var(--ink-2);
  }
  .confirm {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
  .confirm p {
    flex: 1 0 100%;
    font-weight: 700;
  }
  .warnings {
    list-style: none;
    margin: 0 0 var(--space-4);
    padding: 0 0 var(--space-3);
    border-bottom: 1.5px dashed var(--perf);
    display: grid;
    gap: var(--space-2);
  }
  .warnings li {
    display: grid;
    grid-template-columns: 18px minmax(0, 1fr);
    gap: var(--space-2);
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .warnings :global(svg) {
    margin-top: 2px;
  }
  .day {
    margin: var(--space-5) 0 var(--space-2);
    padding-top: var(--space-2);
    border-top: 3px solid var(--ink);
    font-size: var(--read);
    font-weight: 800;
  }
  .day:first-of-type {
    margin-top: var(--space-2);
  }
  .tasks {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .task {
    display: grid;
    grid-template-columns: 5.25rem minmax(0, 1fr) auto;
    gap: var(--space-3);
    align-items: start;
    padding: var(--space-2) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
  }
  .task .when {
    font-size: 1.375rem;
    padding-top: 2px;
  }
  .text {
    max-width: 58ch;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--space-3);
    margin-top: 2px;
    font-size: 0.875rem;
    color: var(--ink-2);
  }
  .hands {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-weight: 700;
    color: var(--ink);
  }
  .hold {
    margin-top: 2px;
    font-size: 0.875rem;
    font-weight: 700;
  }
  .task.quiet .text,
  .task.quiet .when {
    color: var(--ink-3);
  }
  .task.done .text {
    color: var(--ink-3);
    text-decoration: line-through;
  }
  .pin {
    color: var(--ink-3);
  }
  .pin[aria-pressed='true'] {
    color: var(--red);
  }
  .oven {
    margin-bottom: var(--space-3);
  }
  .o-name {
    font-weight: 700;
  }
  .o-racks {
    margin-left: var(--space-2);
    font-weight: 600;
    font-size: 0.875rem;
    color: var(--ink-3);
  }
  .oven ul,
  .serving {
    list-style: none;
    margin: var(--space-1) 0 0;
    padding: 0;
  }
  .oven li,
  .serving li {
    display: grid;
    grid-template-columns: 5.25rem auto minmax(0, 1fr);
    gap: var(--space-3);
    align-items: baseline;
    padding: var(--space-1) 0;
    font-size: var(--ui);
  }
  .o-temp,
  .serving .fig {
    font-size: 1.375rem;
  }
  .o-what,
  .dish {
    color: var(--ink-2);
  }
  @media (max-width: 960px) {
    .cols {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  @media (max-width: 480px) {
    .panel {
      padding: var(--space-4);
    }
    .cook {
      width: 100%;
    }
    .facts {
      gap: var(--space-4);
    }
    .facts dd {
      font-size: 1.875rem;
    }
    .task,
    .oven li,
    .serving li {
      grid-template-columns: 4.25rem minmax(0, 1fr) auto;
    }
    .task .when,
    .o-temp,
    .serving .fig {
      font-size: 1.125rem;
    }
    .oven li,
    .serving li {
      grid-template-columns: 4.25rem minmax(0, 1fr);
    }
    .o-what,
    .dish {
      grid-column: 2;
    }
  }
</style>
