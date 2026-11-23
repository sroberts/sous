<script>
  import { onDestroy, tick } from 'svelte';
  import { ArrowLeft, X, TriangleAlert, Hand, Pin, ArrowUp, ArrowDown, Plus, ChevronRight } from '@lucide/svelte';
  import { api } from '../lib/api.js';
  import { go, link } from '../lib/router.svelte.js';
  import { session, asPerson, toast } from '../lib/session.svelte.js';
  import { isOffline } from '../lib/net.svelte.js';
  import { byDay, clockAt, whenAt, span, isoToZoned, zonedToISO, holdPlace, lanes, dayKey } from '../lib/meals.js';

  let { id } = $props();

  let meal = $state(null);
  let plan = $state(null);
  let kitchen = $state(null);
  let problem = $state('');
  let busy = $state(false);
  let now = $state(Date.now());
  const ticker = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(ticker));

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
      if (!m) return false;
      meal = m;
      plan = await api.mealPlan(id);
      return true;
    } catch (err) {
      if (err.status === 412 && retry) {
        meal = err.body.current;
        return patch(body, false);
      }
      problem = err.message;
      return false;
    } finally {
      busy = false;
    }
  }

  const itemsInput = (items) => items.map((i) => ({ id: i.id, recipe_id: i.recipe_id, serve_offset_minutes: i.serve_offset_minutes, dish: i.dish }));
  const setItem = (i, change) => patch({ items: itemsInput(meal.items).map((it, j) => (j === i ? { ...it, ...change } : it)) });
  async function removeItem(i) {
    const before = itemsInput(meal.items);
    const { title } = meal.items[i];
    if (await patch({ items: before.filter((_, j) => j !== i) })) {
      toast(`${title} is off the menu.`, { label: 'Undo', run: () => patch({ items: before }) });
    }
  }
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

  const OFFSETS = [-60, -30, 0, 15, 30, 45, 60, 90, 120];
  const offsetLabel = (o) => (o === 0 ? 'With the meal' : o < 0 ? `${span(-o)} before` : `${span(o)} after`);

  // Pinning keeps a task where it is while the rest of the plan moves.
  async function pin(t) {
    try {
      const p = await asPerson(() => api.updateTask(id, t.key, { pinned_start: t.pinned ? null : t.start }));
      if (p) plan = p;
    } catch (err) {
      problem = err.message;
    }
  }

  // Lateness: what's late, why, and the fixes that don't need thought.
  const worst = $derived(plan?.late_dishes?.[0] ?? null);
  const worstItem = $derived(worst && meal ? meal.items.findIndex((it) => it.id === worst.item_id) : -1);
  const CAUSE = {
    missed: 'its make-ahead work was planned for earlier and hasn’t been done',
    late_start: 'work that should have started by now hasn’t',
    busy: 'the kitchen has no room to do it sooner',
  };
  async function serveLater() {
    const it = meal.items[worstItem];
    await setItem(worstItem, { serve_offset_minutes: Math.min(720, it.serve_offset_minutes + roundUp(worst.minutes)) });
  }
  async function moveMeal() {
    await patch({ serve_at: new Date(Date.parse(meal.serve_at) + roundUp(plan.behind_minutes) * 60e3).toISOString() });
  }
  async function missedDone() {
    const keys = plan.tasks.filter((t) => t.item_id === worst.item_id && t.status === 'missed').map((t) => t.key);
    try {
      let p = null;
      for (const k of keys) p = await asPerson(() => api.updateTask(id, k, { done: true }));
      if (p) plan = p;
    } catch (err) {
      problem = err.message;
    }
  }
  const roundUp = (m) => Math.ceil(m / 15) * 15;

  // A warning takes you to what it's about, and marks it for a moment.
  let flash = $state(null);
  async function showMe(w) {
    const target = w.task ? `task-${w.task}` : w.item_id ? `item-${w.item_id}` : null;
    if (!target) return;
    flash = target;
    await tick();
    const el = document.getElementById(target);
    el?.scrollIntoView({ block: 'center', behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
    el?.focus({ preventScroll: true });
    setTimeout(() => flash === target && (flash = null), 2400);
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
  const chart = $derived(plan && kitchen ? lanes(plan, tz, kitchen.ovens) : null);
  const taskTitles = $derived(new Map((plan?.tasks ?? []).map((t) => [t.key, t.recipe_title])));
  // The first step whose time was guessed, per dish, so "Set its timing" opens there.
  const guessedStep = $derived.by(() => {
    const out = new Map();
    for (const t of plan?.tasks ?? []) if (t.estimated && t.step && !out.has(t.item_id)) out.set(t.item_id, t.step);
    return out;
  });
  const warnings = $derived((plan?.warnings ?? []).filter((w) => w.code !== 'estimated' && w.code !== 'behind'));
  const handsOn = $derived((plan?.tasks ?? []).filter((t) => t.hands_on && !t.make_ahead).reduce((n, t) => n + t.minutes, 0));
  const ahead = $derived((plan?.tasks ?? []).filter((t) => t.make_ahead).length);
  const startDay = $derived(plan?.start ? new Date(plan.start).toLocaleDateString(undefined, { timeZone: tz, weekday: 'short' }) : '');
  // The now rule goes before the first task that hasn't ended, on its day.
  const nowKey = $derived.by(() => {
    if (!plan?.tasks.length) return null;
    const first = Date.parse(plan.tasks[0].start);
    const last = Math.max(...plan.tasks.map((t) => Date.parse(t.end)));
    if (now < first || now > last) return null;
    return plan.tasks.find((t) => Date.parse(t.end) > now)?.key ?? null;
  });
  const past = (t) => Date.parse(t.end) <= now || t.status === 'done';
  const chartNow = $derived(chart && now >= chart.start && now <= chart.end ? chart.pos(now) : null);
  const serveDay = $derived(plan ? dayKey(plan.serve_at, tz) : '');
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
          <p class="served">Served {whenAt(meal.serve_at, tz, now)}</p>
        </div>
        <a class="btn primary cook" href="/m/{id}/cook" use:link aria-disabled={!plan.tasks.length}>Cook the meal</a>
      </div>
      <dl class="facts">
        <div><dt class="label">Dishes</dt><dd class="fig">{meal.items.length}</dd></div>
        <div><dt class="label">Starts</dt><dd class="fig">{plan.start ? clockAt(plan.start, tz).replace(/\s?[AP]M$/i, '') : '–'}{#if plan.start}<span class="unit">{clockAt(plan.start, tz).match(/[AP]M$/i)?.[0] ?? ''} {startDay}</span>{/if}</dd></div>
        <div><dt class="label">On the day</dt><dd class="fig">{span(handsOn) || '–'}<span class="unit">hands-on</span></dd></div>
        <div><dt class="label">Made ahead</dt><dd class="fig">{ahead}<span class="unit">{ahead === 1 ? 'step' : 'steps'}</span></dd></div>
        {#if !plan.on_time}
          <div class="late-fig"><dt class="label">Running late</dt><dd class="fig">{span(plan.behind_minutes)}</dd></div>
        {/if}
      </dl>
      {#if worst}
        <div class="late" role="status">
          <p><strong>{worst.recipe_title}</strong> won’t be ready until {clockAt(worst.ready_at, tz)}, {span(worst.minutes)} after it’s served: {CAUSE[worst.cause]}.{plan.late_dishes.length > 1 ? ` ${plan.late_dishes.length - 1} more ${plan.late_dishes.length === 2 ? 'dish is' : 'dishes are'} late too.` : ''}</p>
          <div class="fixes">
            {#if worstItem >= 0}<button type="button" class="btn" onclick={serveLater} disabled={busy}>Serve it at {clockAt(new Date(Date.parse(worst.serve_at) + roundUp(worst.minutes) * 60e3).toISOString(), tz)}</button>{/if}
            <button type="button" class="btn" onclick={moveMeal} disabled={busy}>Move the meal to {clockAt(new Date(Date.parse(meal.serve_at) + roundUp(plan.behind_minutes) * 60e3).toISOString(), tz)}</button>
            {#if worst.cause === 'missed'}<button type="button" class="btn" onclick={missedDone} disabled={busy}>Its make-ahead is done</button>{/if}
          </div>
        </div>
      {/if}
      {#if problem}<p class="problem" role="alert">{problem}</p>{/if}
    </header>

    <div class="cols" class:has-items={meal.items.length > 0}>
      <div class="side">
        <section class="panel menu" aria-labelledby="menu-h">
          <h2 id="menu-h" class="label">Menu</h2>
          {#if meal.items.length}
            <ol class="items">
              {#each meal.items as it, i (it.id)}
                <li id="item-{it.id}" tabindex="-1" class:flash={flash === `item-${it.id}`}>
                  <div class="it-head">
                    <a class="it-title" href="/r/{it.recipe_id}" use:link>{it.title}</a>
                    <div class="it-acts">
                      <button type="button" class="icon" onclick={() => moveItem(i, -1)} disabled={i === 0 || busy} aria-label="Move {it.title} up"><ArrowUp size={18} /></button>
                      <button type="button" class="icon" onclick={() => moveItem(i, 1)} disabled={i === meal.items.length - 1 || busy} aria-label="Move {it.title} down"><ArrowDown size={18} /></button>
                      <button type="button" class="icon rm" onclick={() => removeItem(i)} disabled={busy} aria-label="Take {it.title} off the menu"><X size={18} /></button>
                    </div>
                  </div>
                  <div class="it-fields">
                    <label>
                      <span class="visually-hidden">When {it.title} is served</span>
                      <select value={it.serve_offset_minutes} onchange={(e) => setItem(i, { serve_offset_minutes: +e.currentTarget.value })} disabled={busy}>
                        {#each OFFSETS as o}<option value={o}>{offsetLabel(o)}</option>{/each}
                        {#if !OFFSETS.includes(it.serve_offset_minutes)}<option value={it.serve_offset_minutes}>{offsetLabel(it.serve_offset_minutes)}</option>{/if}
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
                  {#if guessedStep.has(it.id)}
                    <a class="it-note" href="/r/{it.recipe_id}/edit?timing={guessedStep.get(it.id)}" use:link>Times guessed from the words · Set its timing<ChevronRight size={16} aria-hidden="true" /></a>
                  {/if}
                </li>
              {/each}
            </ol>
          {:else}
            <p class="hint">Add the dishes you’re making. Each recipe’s steps are laid out in time around your kitchen.</p>
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
              <button type="button" class="btn" onclick={() => (confirmDelete = false)}>Keep it</button>
              <button type="button" class="btn quiet" onclick={remove}>Move to trash</button>
            </div>
          {:else}
            <button type="button" class="btn quiet trash" onclick={() => (confirmDelete = true)}>Move to trash</button>
          {/if}
        </section>
      </div>

      <section class="panel plan" aria-labelledby="plan-h">
        <div class="plan-head">
          <h2 id="plan-h" class="label">Plan</h2>
          {#if plan.tasks.length}<p class="hint">Pin a task to keep it where it is while the rest moves around it.</p>{/if}
        </div>
        {#if warnings.length}
          <ul class="warnings">
            {#each warnings as w}
              <li>
                {#if w.task || w.item_id}
                  <button type="button" onclick={() => showMe(w)}><TriangleAlert size={18} aria-hidden="true" /><span>{w.message}</span><ChevronRight size={16} aria-hidden="true" /></button>
                {:else}
                  <p><TriangleAlert size={18} aria-hidden="true" /><span>{w.message}</span></p>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}

        {#if chart}
          <figure class="chart" aria-label="The day at a glance: what each oven, the stovetop and the cooks are doing, hour by hour. Every task is listed below.">
            <div class="axis" aria-hidden="true">
              {#each chart.ticks as tk, i}
                {#if tk.label && (i < chart.ticks.length - 1 || chart.ticks.length < 3)}<span style="left: {tk.at}%">{clockAt(tk.iso, tz).replace(':00', '')}</span>{/if}
              {/each}
            </div>
            {#each chart.lanes as lane}
              <div class="lane" aria-hidden="true">
                <span class="lane-name">{lane.name}</span>
                <div class="lane-track" style="--rows: {lane.rows.length}">
                  {#each chart.ticks as tk}<span class="grid-line" style="left: {tk.at}%"></span>{/each}
                  {#each lane.rows as row, r}
                    {#each row as b (b.key)}
                      <span class="bar" class:hands={lane.hands} class:heat={b.kind === 'preheat'} class:was={b.status === 'done'} style="left: {b.left}%; width: max({b.width}%, 3px); top: calc({r} * var(--row))" title="{clockAt(b.start, tz)} {b.text}{b.recipe_title ? ` · ${b.recipe_title}` : ''}">
                        {#if b.width > 7}{b.kind === 'preheat' ? `${b.oven_temp}°` : `${b.recipe_title ?? ''}${b.oven_temp && !lane.hands ? ` ${b.oven_temp}°` : ''}`}{/if}
                      </span>
                    {/each}
                  {/each}
                </div>
              </div>
            {/each}
            {#if chartNow !== null}<span class="chart-now" style="left: calc(var(--lane-name) + (100% - var(--lane-name)) * {chartNow / 100})" aria-hidden="true"></span>{/if}
            <figcaption class="legend" aria-hidden="true"><span class="key solid"></span>Hands-on <span class="key hatched"></span>Looks after itself</figcaption>
          </figure>
        {/if}

        {#if !plan.tasks.length}
          <p class="hint">Add dishes to the menu and their steps appear here, laid out in time.</p>
        {/if}
        {#each days as day (day.key)}
          <h3 class="day">{day.label}{#if day.key === serveDay}<span class="day-note">the meal</span>{/if}</h3>
          <ol class="tasks">
            {#each day.tasks as t (t.key)}
              {#if t.key === nowKey}<li class="now-rule" aria-label="Now"><span class="fig">{clockAt(new Date(now).toISOString(), tz)}</span>Now</li>{/if}
              <li class="task" id="task-{t.key}" tabindex="-1" class:quiet={t.kind === 'preheat'} class:past={past(t)} class:pinned={t.pinned} class:flash={flash === `task-${t.key}`}>
                <span class="when fig">{clockAt(t.start, tz)}</span>
                <div class="body">
                  <p class="text">{t.text}</p>
                  <p class="meta">
                    {#if t.recipe_title}<span>{t.recipe_title}{t.step ? ` · step ${t.step}` : ''}</span>{/if}
                    <span>{span(t.minutes)}{t.estimated ? ' (guessed)' : ''}</span>
                    {#if t.oven}<span>{t.oven}{t.oven_temp ? ` ${t.oven_temp}°F` : ''}</span>{:else if t.equipment}<span>{t.equipment}</span>{/if}
                    {#if t.hands_on && t.minutes}<span class="hands"><Hand size={14} aria-hidden="true" />hands-on</span>{/if}
                    {#if t.status === 'missed'}<span class="flag">missed</span>{:else if t.status === 'late'}<span class="flag">should have started</span>{/if}
                  </p>
                  {#if t.hold}<p class="hold">Then it waits {holdPlace(t.hold.in)} until {whenAt(t.hold.until, tz, now)}.</p>{/if}
                </div>
                {#if t.kind !== 'preheat'}
                  <button type="button" class="pin" aria-pressed={t.pinned} onclick={() => pin(t)} aria-label="{t.pinned ? 'Unpin' : 'Pin'} {t.text}"><Pin size={16} aria-hidden="true" />{t.pinned ? 'Pinned' : 'Pin'}</button>
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
                    <span class="fig o-temp">{u.temp_max ? `${u.temp}–${u.temp_max}°` : u.temp ? `${u.temp}°` : '–'}</span>
                    <span>{dayKey(u.start, tz) === serveDay ? '' : `${new Date(u.start).toLocaleDateString(undefined, { timeZone: tz, weekday: 'short' })} `}{clockAt(u.start, tz)}–{clockAt(u.end, tz)}</span>
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
              <li><span class="fig">{clockAt(s.serve_at, tz)}</span><span>{s.recipe_title}</span><span class="dish" class:none={!s.dish}>{s.dish ?? 'No dish chosen'}</span></li>
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
    max-width: 84rem;
    margin: 0 auto;
    padding: var(--space-3) var(--gutter) var(--space-8);
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
  .panel .label {
    color: var(--ink-3);
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
    padding: 0 var(--space-5);
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
    gap: var(--space-3) var(--space-6);
    margin: 0;
    padding-top: var(--space-3);
    border-top: 3px solid var(--ink);
  }
  .facts div {
    min-width: 4.5rem;
  }
  .facts dd {
    margin: var(--space-1) 0 0;
    font-size: 2.25rem;
  }
  .late-fig dd {
    color: var(--red);
  }
  .unit {
    margin-left: 0.25em;
    font-size: 0.4em;
    font-stretch: 100%;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  /* Late: what, why, and the fixes, before anything else. */
  .late {
    display: grid;
    gap: var(--space-3);
    padding-top: var(--space-3);
    border-top: 1.5px dashed var(--perf);
  }
  .late p {
    max-width: 70ch;
  }
  .fixes {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .problem {
    font-weight: 700;
    color: var(--red);
  }

  .cols {
    display: grid;
    grid-template-columns: minmax(17rem, 24rem) minmax(0, 1fr);
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
  .hint a {
    color: inherit;
  }
  .items {
    list-style: none;
    padding: 0;
    margin: var(--space-2) 0 0;
  }
  .items li {
    padding: var(--space-3) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
    outline: none;
    transition: background-color 600ms var(--ease-out);
  }
  .it-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .it-title {
    flex: 1;
    display: flex;
    align-items: center;
    min-height: 44px;
    font-weight: 700;
    color: inherit;
    text-decoration: none;
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
    padding: 0;
    border: 0;
    border-radius: 3px;
    background: none;
    color: var(--ink-3);
  }
  @media (hover: hover) {
    .icon:hover:not(:disabled) {
      background: var(--pass-deep);
      color: var(--ink);
    }
    .icon.rm:hover:not(:disabled) {
      color: var(--red);
    }
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
    min-height: 44px;
    max-width: 100%;
    font-size: max(16px, var(--ui));
    font-weight: 600;
    background: var(--ticket);
    border: 2px solid var(--ink);
    border-radius: 3px;
    padding: 0 var(--space-2);
  }
  .it-note {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    min-height: 44px;
    margin-top: var(--space-1);
    font-size: var(--ui);
    font-weight: 600;
    color: var(--ink-2);
    text-decoration: none;
  }
  .it-note:hover {
    color: var(--red);
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
    border: 0;
    border-radius: 3px;
    background: none;
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
  .pair select {
    width: 100%;
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

  .plan-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-1) var(--space-4);
  }
  .plan-head .hint {
    font-size: 0.875rem;
  }
  .warnings {
    list-style: none;
    margin: var(--space-3) 0 0;
    padding: 0;
    display: grid;
  }
  .warnings button,
  .warnings p {
    width: 100%;
    display: grid;
    grid-template-columns: 18px minmax(0, 1fr) 16px;
    gap: var(--space-2);
    align-items: start;
    min-height: 44px;
    padding: var(--space-2) var(--space-1);
    border: 0;
    border-radius: 3px;
    background: none;
    text-align: left;
    font-size: var(--ui);
    color: var(--ink-2);
  }
  .warnings button:hover {
    background: var(--ticket-press);
    color: var(--ink);
  }
  .warnings :global(svg) {
    margin-top: 2px;
  }

  /* The day at a glance: a lane each for the ovens, the stovetop and the
     cooks' hands. Ink is hands-on; hatching looks after itself. */
  .chart {
    --lane-name: 5.5rem;
    --row: 22px;
    position: relative;
    margin: var(--space-4) 0 0;
    padding: var(--space-3) 0 var(--space-2);
    border-top: 1.5px dashed var(--perf);
  }
  .axis {
    position: relative;
    height: 18px;
    margin-left: var(--lane-name);
  }
  .axis span {
    position: absolute;
    transform: translateX(-50%);
    font-size: 0.75rem;
    font-weight: 700;
    color: var(--ink-3);
    white-space: nowrap;
  }
  .axis span:first-child {
    transform: none;
  }
  .lane {
    display: grid;
    grid-template-columns: var(--lane-name) minmax(0, 1fr);
    align-items: start;
    padding: 4px 0;
    border-bottom: 1px solid var(--ticket-press);
  }
  .lane-name {
    font-size: 0.8125rem;
    font-weight: 700;
    color: var(--ink-2);
    line-height: var(--row);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    padding-right: var(--space-2);
  }
  .lane-track {
    position: relative;
    height: calc(var(--rows) * var(--row));
  }
  .grid-line {
    position: absolute;
    top: -4px;
    bottom: -4px;
    border-left: 1px dashed var(--ticket-press);
  }
  .bar {
    position: absolute;
    height: calc(var(--row) - 4px);
    margin-top: 2px;
    padding: 0 4px;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
    font-size: 0.75rem;
    font-weight: 700;
    line-height: calc(var(--row) - 4px);
    color: var(--ink);
    background: repeating-linear-gradient(135deg, var(--pass-deep) 0 3px, var(--ticket-press) 3px 6px);
    box-shadow: inset 0 0 0 1px var(--steel);
  }
  .bar.hands {
    background: var(--ink);
    color: var(--ticket);
    box-shadow: none;
  }
  .bar.heat {
    background: none;
    box-shadow: inset 0 0 0 1px var(--perf);
    color: var(--ink-3);
  }
  .bar.was {
    opacity: 0.45;
  }
  .chart-now {
    position: absolute;
    top: var(--space-3);
    bottom: 26px;
    width: 0;
    border-left: 2px solid var(--red);
  }
  .legend {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-2);
    margin-left: var(--lane-name);
    font-size: 0.8125rem;
    color: var(--ink-2);
  }
  .key {
    display: inline-block;
    width: 18px;
    height: 10px;
  }
  .key.hatched {
    margin-left: var(--space-3);
    background: repeating-linear-gradient(135deg, var(--pass-deep) 0 3px, var(--ticket-press) 3px 6px);
    box-shadow: inset 0 0 0 1px var(--steel);
  }
  .key.solid {
    background: var(--ink);
  }

  .day {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    margin: var(--space-5) 0 var(--space-1);
    padding-top: var(--space-2);
    border-top: 3px solid var(--ink);
    font-size: var(--read);
    font-weight: 800;
  }
  .day-note {
    font-size: var(--label);
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--ink-3);
  }
  .tasks {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .task {
    display: grid;
    grid-template-columns: 5.5rem minmax(0, 1fr) auto;
    gap: var(--space-3);
    align-items: start;
    padding: var(--space-2) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
    outline: none;
    transition: background-color 600ms var(--ease-out);
  }
  .task .when {
    font-size: 1.5rem;
    padding-top: 1px;
  }
  .text {
    max-width: 62ch;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--space-3);
    margin-top: 2px;
    font-size: var(--ui);
    color: var(--ink-2);
  }
  .hands {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-weight: 700;
    color: var(--ink);
  }
  .flag {
    font-weight: 700;
    color: var(--ink);
  }
  .hold {
    margin-top: 2px;
    font-size: var(--ui);
    font-weight: 700;
  }
  .task.quiet .text,
  .task.quiet .when {
    color: var(--ink-3);
  }
  .task.past .text,
  .task.past .when,
  .task.past .meta {
    color: var(--ink-3);
  }
  .flash {
    background: var(--ticket-press);
    box-shadow: -8px 0 0 var(--ticket-press), 8px 0 0 var(--ticket-press);
  }
  .now-rule {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    margin: var(--space-1) 0;
    padding-top: var(--space-1);
    border-top: 2px solid var(--red);
    font-size: var(--label);
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--red);
  }
  .now-rule .fig {
    width: 5.5rem;
    font-size: 1.125rem;
    letter-spacing: 0;
  }
  /* Pin: quiet until the row is pointed at, always there under a finger,
     and always there once pinned. */
  .pin {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-height: 44px;
    padding: 0 var(--space-2);
    border: 0;
    border-radius: 3px;
    background: none;
    font-size: 0.875rem;
    font-weight: 700;
    color: var(--ink-3);
  }
  .pin[aria-pressed='true'] {
    color: var(--red);
  }
  @media (hover: hover) and (pointer: fine) {
    .pin {
      opacity: 0;
    }
    .task:hover .pin,
    .task:focus-within .pin,
    .pin[aria-pressed='true'] {
      opacity: 1;
    }
    .pin:hover {
      background: var(--pass-deep);
      color: var(--ink);
    }
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
    grid-template-columns: 5.5rem auto minmax(0, 1fr);
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
  .dish.none {
    color: var(--ink-3);
  }

  /* A laptop narrower than the side column likes, or an iPad in landscape. */
  @media (max-width: 1100px) {
    .cols {
      grid-template-columns: minmax(16rem, 20rem) minmax(0, 1fr);
    }
    .facts {
      gap: var(--space-3) var(--space-5);
    }
  }
  /* An iPad held upright keeps two columns, the plan beside a narrower menu. */
  @media (max-width: 900px) {
    .panel {
      padding: var(--space-4);
    }
    .cols {
      grid-template-columns: 15.5rem minmax(0, 1fr);
      gap: var(--space-3);
    }
    .it-fields select {
      width: 100%;
    }
    .pair {
      grid-template-columns: 1fr;
    }
    .facts dd {
      font-size: 1.875rem;
    }
    .task {
      grid-template-columns: 4.5rem minmax(0, 1fr) auto;
    }
    .task .when {
      font-size: 1.25rem;
    }
    .chart {
      --lane-name: 4.5rem;
    }
    .now-rule .fig {
      width: 4.5rem;
    }
  }
  /* A phone: one column, the plan first once there are dishes. */
  @media (max-width: 760px) {
    .cols {
      grid-template-columns: minmax(0, 1fr);
    }
    .cols.has-items .plan {
      order: -1;
    }
    .cook {
      width: 100%;
    }
    .task,
    .oven li,
    .serving li {
      grid-template-columns: 4.25rem minmax(0, 1fr) auto;
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
