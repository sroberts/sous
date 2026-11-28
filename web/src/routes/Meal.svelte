<script>
  import { onDestroy, tick } from 'svelte';
  import { ArrowLeft, X, TriangleAlert, Hand, Pin, ArrowUp, ArrowDown, Plus, ChevronRight } from '@lucide/svelte';
  import { api } from '../lib/api.js';
  import { go, link } from '../lib/router.svelte.js';
  import { session, asPerson, toast } from '../lib/session.svelte.js';
  import { isOffline } from '../lib/net.svelte.js';
  import { byDay, clockAt, whenAt, span, isoToZoned, zonedToISO, holdPlace, lanes, dayKey, dayWord, mealWord, problemText, deviceZone, itemsOf, withItem, withoutItem, restoreItem, moveItemBy, addRecipe } from '../lib/meals.js';
  import { personName } from '../lib/format.js';
  import LateFixes from '../components/LateFixes.svelte';

  let { id } = $props();

  let meal = $state(null);
  let plan = $state(null);
  let kitchen = $state(null);
  let problem = $state('');
  let busy = $state(false);
  let now = $state(Date.now());
  // On the day the plan moves with the clock and other cooks' taps, so it's
  // fetched again every 30 seconds while the page is in view.
  const ticker = setInterval(() => {
    now = Date.now();
    if (plan && meal && !busy && document.visibilityState === 'visible' && Math.abs(Date.parse(meal.serve_at) - now) < 24 * 3600e3) refreshPlan();
  }, 30_000);
  onDestroy(() => clearInterval(ticker));
  // Each change on this page counts here, so a refresh fetched before one
  // landed doesn't put the older meal and plan back over it.
  let writes = 0;
  async function refreshPlan() {
    const at = writes;
    try {
      const [m, p] = await Promise.all([api.meal(id), api.mealPlan(id)]);
      if (!busy && at === writes) [meal, plan] = [m, p];
    } catch {
      // A failed refresh keeps what's on screen; the next one tries again.
    }
  }

  async function load() {
    try {
      [meal, plan, kitchen] = await Promise.all([api.meal(id), api.mealPlan(id), api.kitchen()]);
      document.title = `${meal.title} · Sous`;
    } catch (err) {
      problem = err.code === 'offline' ? 'Meals need a connection to Sous.' : err.message;
    }
  }
  load();

  // Each dish's step graph, for the menu to say which are ready. Quiet:
  // nothing shows when step graphs aren't set up.
  let graphs = $state(null);
  api.graphs().then((o) => (graphs = o.upgrades.labels ? o : null), () => {});
  const graphOf = (recipeId) => graphs?.items.find((r) => r.recipe_id === recipeId) ?? null;
  function graphNote(recipeId) {
    const g = graphOf(recipeId);
    if (g?.status === 'stale') return 'Step graph · the words have changed';
    return g?.review_count ? `Step graph · ${g.review_count} to review` : null;
  }
  const ungraphed = $derived(graphs && meal ? meal.items.filter((it) => !graphOf(it.recipe_id)).length : 0);

  const tz = $derived(meal?.timezone ?? 'UTC');

  // Every change saves as it's made. If someone else changed the meal in
  // the meantime, the change is applied again to theirs, once, and the
  // message says so. A change that moves the plan says how much moved.
  // body may be a function of the meal, so a retry works the change out
  // again on the newer version rather than undoing someone else's edit.
  async function patch(body, retry = true) {
    busy = true;
    writes++;
    problem = '';
    const before = plan;
    try {
      const b = typeof body === 'function' ? body(meal) : body;
      const m = await asPerson(() => api.updateMeal(id, meal.version, b));
      if (!m) return false;
      meal = m;
      plan = await api.mealPlan(id);
      const moved = before ? plan.tasks.filter((t) => before.tasks.some((b) => b.key === t.key && b.start !== t.start)).length : 0;
      lastNews = [retry ? '' : `${personName(other, session.people)} had changed the meal just before; yours went on top.`, moved ? `${moved === 1 ? '1 step' : `${moved} steps`} moved.` : ''].filter(Boolean).join(' ');
      return true;
    } catch (err) {
      if (err.status === 412 && retry) {
        meal = err.body.current;
        other = meal.updated_by;
        busy = false;
        return patch(body, false);
      }
      problem = problemText(err);
      return false;
    } finally {
      busy = false;
    }
  }
  let other = null;
  let lastNews = '';
  // A setting changed, with its message and a way back.
  async function setting(body, said) {
    const was = Object.fromEntries(Object.keys(body).map((k) => [k, meal[k]]));
    if (await patch(body)) toast([said, lastNews].filter(Boolean).join(' '), { label: 'Undo', run: () => patch(was) });
  }

  async function setItem(i, change) {
    const it = itemsOf(meal)[i];
    const was = Object.fromEntries(Object.keys(change).map((k) => [k, it[k]]));
    if (await patch(withItem(it.id, change))) {
      toast([`${meal.items.find((x) => x.id === it.id)?.title ?? 'The dish'} is changed.`, lastNews].join(' ').trim(), { label: 'Undo', run: () => patch(withItem(it.id, was)) });
    }
  }
  async function removeItem(i) {
    const it = itemsOf(meal)[i];
    const { title } = meal.items[i];
    if (await patch(withoutItem(it.id))) {
      toast(`${title} is off the menu.`, { label: 'Undo', run: () => patch(restoreItem(it, i)) });
    }
  }
  async function moveItem(i, by) {
    const { id: itemId, title } = meal.items[i];
    if (await patch(moveItemBy(itemId, by))) toast(`${title} moved ${by < 0 ? 'up' : 'down'} the menu.`, { label: 'Undo', run: () => patch(moveItemBy(itemId, -by)) });
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
    await patch(addRecipe(r.id));
    document.getElementById('add-dish')?.focus();
  }
  let pick = $state(0);
  $effect(() => {
    query;
    pick = 0;
  });
  function addKey(e) {
    if (!matches.length) return;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      pick = (pick + (e.key === 'ArrowDown' ? 1 : -1) + matches.length) % matches.length;
    } else if (e.key === 'Enter') {
      e.preventDefault();
      add(matches[pick] ?? matches[0]);
    } else if (e.key === 'Escape') {
      query = '';
    }
  }

  // Keys at the desk: C cooks the meal, D adds a dish, ? lists them.
  let keysDialog = $state();
  const KEYS = [
    ['C', 'Cook the meal'],
    ['D', 'Add a dish; ↑ ↓ pick from the matches, Enter adds'],
    ['← →', 'In the lanes: the bar before or after'],
    ['↑ ↓', 'In the lanes: the lane above or below'],
    ['?', 'These keys'],
  ];
  function onkey(e) {
    if (!meal || session.asking || e.altKey || e.ctrlKey || e.metaKey) return;
    if (e.target.closest?.('input, textarea, select, [contenteditable], dialog[open]')) return;
    const k = e.key.toLowerCase();
    if (k === 'c' && plan?.tasks.length) go(`/m/${id}/cook`);
    else if (k === 'd') document.getElementById('add-dish')?.focus();
    else if (e.key === '?') keysDialog?.showModal();
    else return;
    e.preventDefault();
  }

  const OFFSETS = [-60, -30, 0, 15, 30, 45, 60, 90, 120];
  const offsetLabel = (o) => (o === 0 ? 'With the meal' : o < 0 ? `${span(-o)} before` : `${span(o)} after`);

  // Pinning keeps a task where it is while the rest of the plan moves.
  async function pin(t) {
    writes++;
    try {
      const p = await asPerson(() => api.updateTask(id, t.key, { pinned_start: t.pinned ? null : t.start }));
      if (p) plan = p;
    } catch (err) {
      problem = problemText(err);
    }
  }


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
  const chart = $derived(plan && kitchen ? lanes(plan, tz, kitchen.ovens, now) : null);
  let chartW = $state(0);
  const trackW = $derived(Math.max(0, chartW - 88));
  // A bar's label goes inside when it fits, just after it when there's room
  // before the next bar, and otherwise only in its name for screen readers.
  // The recipe's name, with the oven's temperature when it fits; a narrow
  // bar falls back to the name's last word, which names the dish ("turkey",
  // "casserole") where the first names how it's made ("Roast"). Preheats
  // are a thin strip with no label: the bar after them says the temperature.
  const PX = 7.4;
  function labelFor(b, lane, room) {
    if (b.kind === 'preheat') return null;
    const name = b.recipe_title ?? '';
    const noun = name.split(' ').at(-1);
    const tries = [b.oven_temp && !lane.hands ? `${name} ${b.oven_temp}°` : null, name, noun && noun[0].toUpperCase() + noun.slice(1)].filter(Boolean);
    return tries.find((t) => t.length * PX + 10 <= room) ?? null;
  }
  function placeLabel(b, row, k, lane) {
    const w = (b.width / 100) * trackW;
    const inside = labelFor(b, lane, w);
    if (inside) return { at: 'in', text: inside };
    const right = ((b.left + b.width) / 100) * trackW;
    const next = row[k + 1] ? (row[k + 1].left / 100) * trackW : trackW;
    const out = labelFor(b, lane, next - right - 4);
    return out ? { at: 'out', text: out } : { at: 'none', text: '' };
  }
  // The chart is one stop for Tab; the arrow keys walk its bars in time.
  const bars = $derived(chart ? chart.lanes.flatMap((l) => l.rows.flat().map((b) => ({ key: `${l.name}|${b.key}`, lane: l.name, s: b.s }))) : []);
  let barFocus = $state(null);
  const barId = (k) => `bar-${k.replace(/[^\w-]/g, '_')}`;
  const focusKey = $derived(bars.some((b) => b.key === barFocus) ? barFocus : bars[0]?.key);
  function barKey(e) {
    const i = bars.findIndex((b) => b.key === focusKey);
    if (i < 0) return;
    const lanesOrder = [...new Set(bars.map((b) => b.lane))];
    let to = null;
    if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
      const same = bars.filter((b) => b.lane === bars[i].lane).sort((a, b) => a.s - b.s);
      const j = same.findIndex((b) => b.key === focusKey) + (e.key === 'ArrowRight' ? 1 : -1);
      to = same[j];
    } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      const l = lanesOrder[lanesOrder.indexOf(bars[i].lane) + (e.key === 'ArrowDown' ? 1 : -1)];
      const there = bars.filter((b) => b.lane === l);
      to = there.reduce((best, b) => (!best || Math.abs(b.s - bars[i].s) < Math.abs(best.s - bars[i].s) ? b : best), null);
    } else if (e.key === 'Home' || e.key === 'End') {
      to = e.key === 'Home' ? bars[0] : bars.at(-1);
    }
    if (!to) return;
    e.preventDefault();
    barFocus = to.key;
    document.getElementById(barId(to.key))?.focus();
  }
  // The meal's day in one line per oven, for a phone.
  const ovenLines = $derived.by(() => {
    if (!plan) return [];
    return plan.ovens
      .map((o) => ({
        name: o.name,
        uses: o.uses.filter((u) => dayKey(u.start, tz) === serveDay && Date.parse(u.end) > now - 30 * 60e3),
      }))
      .filter((o) => o.uses.length);
  });
  const taskTitles = $derived(new Map((plan?.tasks ?? []).map((t) => [t.key, t.recipe_title])));
  // The first step whose time was guessed, per dish, so "Set its timing" opens there.
  const guessedStep = $derived.by(() => {
    const out = new Map();
    for (const t of plan?.tasks ?? []) if (t.estimated && t.step && !out.has(t.item_id)) out.set(t.item_id, t.step);
    return out;
  });
  const warnings = $derived((plan?.warnings ?? []).filter((w) => w.code !== 'estimated' && w.code !== 'behind'));
  const handsOn = $derived((plan?.tasks ?? []).filter((t) => t.hands_on && !t.make_ahead).reduce((n, t) => n + t.minutes, 0));
  // Made ahead counts what's still ahead of the day, not missed work now on it.
  const ahead = $derived((plan?.tasks ?? []).filter((t) => t.make_ahead && t.status !== 'missed').length);
  const startDay = $derived(plan?.start ? dayWord(plan.start, tz, now) : '');
  const landsAt = $derived(plan ? new Date(Date.parse(plan.serve_at) + plan.behind_minutes * 60e3).toISOString() : null);
  // Which dishes have their serving fields open.
  let editing = $state(new Set());
  const toggleEdit = (itemId) => (editing = editing.has(itemId) ? new Set([...editing].filter((x) => x !== itemId)) : new Set([...editing, itemId]));
  // The now rule goes before the first task that hasn't ended, on its day.
  const nowKey = $derived.by(() => {
    if (!plan?.tasks.length) return null;
    const first = Date.parse(plan.tasks[0].start);
    const last = Math.max(...plan.tasks.map((t) => Date.parse(t.end)));
    if (now < first || now > last) return null;
    // Before the first task still to start; what's under way sits above it.
    return plan.tasks.find((t) => Date.parse(t.start) > now && t.status !== 'active' && t.status !== 'done')?.key ?? null;
  });
  const past = (t) => Date.parse(t.end) <= now || t.status === 'done';
  const chartNow = $derived(chart && now >= chart.start && now <= chart.end ? chart.pos(now) : null);
  const serveDay = $derived(plan ? dayKey(plan.serve_at, tz) : '');
  const word = $derived(meal ? mealWord(meal.serve_at, tz) : 'Dinner');
  // When each dish is really ready: late dishes say so beside their time.
  const readyAt = $derived(new Map((plan?.late_dishes ?? []).map((d) => [d.item_id, d.ready_at])));
  const anyPinned = $derived((plan?.tasks ?? []).some((t) => t.pinned));
  const otherZone = $derived(tz !== deviceZone());
  const missedSteps = $derived((plan?.tasks ?? []).filter((t) => t.status === 'missed').length);
  const firstMissed = $derived((plan?.tasks ?? []).find((t) => t.status === 'missed')?.start ?? null);
  // Cook the meal is the step forward only once cooking is near: before
  // then, the plan is what needs attention.
  const onTheDay = $derived(plan?.start ? Date.parse(plan.start) - now < 18 * 3600e3 : false);
  const movedFrom = (t) => `moved from ${dayKey(t.planned_for, tz) === dayKey(t.start, tz) ? '' : `${dayWord(t.planned_for, tz, now)} `}${clockAt(t.planned_for, tz)}`;
  // A dish held for the meal waits until the meal, wherever it now lands.
  function holdUntil(t) {
    const s = plan.serving.find((x) => x.item_id === t.item_id);
    if (plan.on_time || !s || t.hold.until < s.serve_at) return whenAt(t.hold.until, tz, now);
    return 'it goes to the table';
  }
  // When the meal's landing time moves, say so once.
  let said = $state('');
  let landed = null;
  $effect(() => {
    if (!plan) return;
    const lands = plan.on_time ? 'on time' : clockAt(landsAt, tz);
    if (landed !== null && lands !== landed) said = plan.on_time ? `${word} is on time.` : `${word} now lands at ${lands}.`;
    landed = lands;
  });
</script>

<svelte:window onkeydown={onkey} />

<main class="meal" id="main">
  <div class="crumbs">
    <a class="back" href="/meals" use:link><ArrowLeft size={16} aria-hidden="true" />Meals</a>
    {#if meal}<button type="button" class="back" aria-haspopup="dialog" aria-keyshortcuts="?" title="Keys on a meal (?)" onclick={() => keysDialog?.showModal()}><kbd aria-hidden="true">?</kbd>Keys</button>{/if}
  </div>

  {#if problem && !meal}
    <h1>That meal couldn’t be opened.</h1>
    <p class="problem">{problem}</p>
  {:else if meal && plan}
    <header class="head panel">
      <div class="title-row">
        <div>
          <h1>{meal.title}</h1>
          {#if !plan.on_time}
            <!-- Late, the news leads: when it now lands, then what was planned. -->
            <p class="lands">{word} now lands <strong class="fig">{clockAt(landsAt, tz)}</strong>{` ${dayWord(landsAt, tz, now).toLowerCase()} · ${span(plan.behind_minutes)} late`}</p>
          {/if}
          <p class="served" class:was={!plan.on_time}>{plan.on_time ? 'Served' : 'Planned for'} {dayWord(meal.serve_at, tz, now)} {clockAt(meal.serve_at, tz)}</p>
          {#if missedSteps}
            <!-- Make-ahead whose time passed is replanned (in the kitchen's
                 hours when there's time): say so, and from when. -->
            <p class="missed-note">{missedSteps === 1 ? '1 make-ahead step' : `${missedSteps} make-ahead steps`} didn’t happen when planned; Sous has replanned {missedSteps === 1 ? 'it' : 'them'}{firstMissed ? `, from ${dayWord(firstMissed, tz, now).toLowerCase() === 'today' ? '' : `${dayWord(firstMissed, tz, now)} `}${clockAt(firstMissed, tz)}` : ''}.</p>
          {/if}
        </div>
        {#if plan.tasks.length}
          <!-- One red button in view: while the late block's fix is red, this steps back. -->
          <a class="btn cook" class:primary={plan.on_time && onTheDay} href="/m/{id}/cook" use:link aria-keyshortcuts="c" title="Cook the meal (C)">Cook the meal</a>
        {:else}
          <button type="button" class="btn cook" disabled>Cook the meal</button>
        {/if}
      </div>
      <dl class="facts">
        <div><dt class="label">Dishes</dt><dd class="fig">{meal.items.length}</dd></div>
        <div><dt class="label">Starts</dt><dd class="fig">{plan.start ? clockAt(plan.start, tz).replace(/\s?[AP]M$/i, '') : '–'}{#if plan.start}<span class="unit">{clockAt(plan.start, tz).match(/[AP]M$/i)?.[0] ?? ''} {startDay}</span>{/if}</dd></div>
        <div><dt class="label">On the day</dt><dd class="fig">{span(handsOn) || '–'}<span class="unit">hands-on</span></dd></div>
        <div><dt class="label">Made ahead</dt><dd class="fig">{ahead}<span class="unit">{ahead === 1 ? 'step' : 'steps'}</span></dd></div>
      </dl>
      {#if !plan.on_time && plan.late_dishes.length}
        <div class="late-block">
          <LateFixes {meal} {plan} {tz} onupdate={(u) => (writes++, (meal = u.meal), (plan = u.plan))} />
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
                  <a class="it-title" href="/r/{it.recipe_id}" use:link>{it.title}</a>
                  <button type="button" class="it-summary" aria-expanded={editing.has(it.id)} onclick={() => toggleEdit(it.id)}>
                    <span class="it-sum">{[it.serve_offset_minutes ? offsetLabel(it.serve_offset_minutes) : '', it.dish ?? 'No dish chosen'].filter(Boolean).join(' · ')}</span><span class="it-change">{editing.has(it.id) ? 'Close' : 'Change'}</span>
                  </button>
                  {#if editing.has(it.id)}
                  <div class="it-fields">
                    <label>
                      <span class="label">Served</span>
                      <select value={it.serve_offset_minutes} onchange={(e) => setItem(i, { serve_offset_minutes: +e.currentTarget.value })} disabled={busy}>
                        {#each OFFSETS as o}<option value={o}>{offsetLabel(o)}</option>{/each}
                        {#if !OFFSETS.includes(it.serve_offset_minutes)}<option value={it.serve_offset_minutes}>{offsetLabel(it.serve_offset_minutes)}</option>{/if}
                      </select>
                    </label>
                    <label>
                      <span class="label">Goes to the table in</span>
                      <select value={it.dish ?? ''} onchange={(e) => setItem(i, { dish: e.currentTarget.value || null })} disabled={busy}>
                        <option value="">No dish chosen</option>
                        {#each kitchen?.dishes ?? [] as d}<option value={d.name}>{d.name}</option>{/each}
                        {#if it.dish && !kitchen?.dishes.some((d) => d.name === it.dish)}<option value={it.dish}>{it.dish}</option>{/if}
                      </select>
                    </label>
                    <div class="it-acts">
                      <button type="button" class="icon" onclick={() => moveItem(i, -1)} disabled={i === 0 || busy} aria-label="Move {it.title} up the menu"><ArrowUp size={18} /></button>
                      <button type="button" class="icon" onclick={() => moveItem(i, 1)} disabled={i === meal.items.length - 1 || busy} aria-label="Move {it.title} down the menu"><ArrowDown size={18} /></button>
                      <button type="button" class="btn quiet rm" onclick={() => removeItem(i)} disabled={busy}><X size={18} aria-hidden="true" />Take off the menu</button>
                    </div>
                  </div>
                  {/if}
                  {#if guessedStep.has(it.id)}
                    <a class="it-note" href="/r/{it.recipe_id}/edit?timing={guessedStep.get(it.id)}" use:link>Times guessed from the words · Set its timing<ChevronRight size={16} aria-hidden="true" /></a>
                  {/if}
                  {#if graphNote(it.recipe_id)}
                    <a class="it-note" href="/r/{it.recipe_id}/graph" use:link>{graphNote(it.recipe_id)}<ChevronRight size={16} aria-hidden="true" /></a>
                  {/if}
                </li>
              {/each}
            </ol>
            {#if ungraphed}
              <a class="it-note graphs-note" href="/graphs" use:link>{ungraphed === 1 ? '1 dish has' : `${ungraphed} dishes have`} no step graph yet<ChevronRight size={16} aria-hidden="true" /></a>
            {/if}
          {:else}
            <p class="hint">Add the dishes you’re making. Each recipe’s steps are laid out in time around your kitchen.</p>
          {/if}
          <div class="add">
            <label class="visually-hidden" for="add-dish">Add a dish from the library</label>
            <Plus size={18} aria-hidden="true" />
            <input id="add-dish" type="search" bind:value={query} onkeydown={addKey} placeholder="Add a dish" autocomplete="off" disabled={isOffline()} aria-keyshortcuts="d" role="combobox" aria-expanded={matches.length > 0} aria-controls="dish-matches" aria-activedescendant={matches.length ? `match-${pick}` : undefined} />
          </div>
          {#if matches.length}
            <ul class="matches" id="dish-matches" role="listbox" aria-label="Recipes that match">
              {#each matches as r, j (r.id)}
                <li id="match-{j}" role="option" aria-selected={j === pick} class:picked={j === pick}><button type="button" tabindex="-1" onclick={() => add(r)}>{r.title}</button></li>
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
            <input value={meal.title} onchange={(e) => e.currentTarget.value.trim() && setting({ title: e.currentTarget.value.trim() }, 'Renamed.')} />
          </label>
          <label class="field">
            <span class="label">Served <span class="said">{dayWord(meal.serve_at, tz, now)} {clockAt(meal.serve_at, tz)}</span></span>
            <input type="datetime-local" value={isoToZoned(meal.serve_at, tz)} onchange={(e) => { const v = e.currentTarget.value; if (v) setting({ serve_at: zonedToISO(v, tz) }, `${mealWord(zonedToISO(v, tz), tz)} is at ${clockAt(zonedToISO(v, tz), tz)}.`); }} />
          </label>
          <div class="pair">
            <label class="field">
              <span class="label">Cooks</span>
              <select value={meal.cooks} onchange={(e) => setting({ cooks: +e.currentTarget.value }, `Planned for ${e.currentTarget.value === '1' ? '1 cook' : `${e.currentTarget.value} cooks`}.`)}>
                {#each [1, 2, 3, 4, 5, 6, 7, 8] as n}<option value={n}>{n === 1 ? '1 cook' : `${n} cooks`}</option>{/each}
              </select>
            </label>
            <label class="field">
              <span class="label">Prep from</span>
              <select value={meal.prep_days} onchange={(e) => setting({ prep_days: +e.currentTarget.value }, 'The prep window is changed.')}>
                <option value={0}>The day itself</option>
                {#each [1, 2, 3, 4, 5, 6, 7] as n}<option value={n}>{n === 1 ? 'The day before' : `${n} days before`}</option>{/each}
              </select>
            </label>
          </div>
          {#if otherZone}<p class="hint">Times are in {tz.replace(/_/g, ' ')}, not this device’s zone.</p>{/if}
          <a class="kitchen-link" href="/kitchen" use:link>Your kitchen: ovens, burners, dishes and hours<ChevronRight size={16} aria-hidden="true" /></a>
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
          {#if plan.tasks.length && !anyPinned}<p class="hint">Pin a task to keep it where it is while the rest moves around it.</p>{/if}
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
          {#if ovenLines.length}
            <div class="oven-lines">
              {#each ovenLines as o}
                <p class="ol-name">{o.name}</p>
                <ul>
                  {#each o.uses as u}
                    <li><span class="fig">{u.temp_max ? `${u.temp}–${u.temp_max}°` : u.temp ? `${u.temp}°` : 'On'}</span><span>{clockAt(u.start, tz)}–{clockAt(u.end, tz)}</span></li>
                  {/each}
                </ul>
              {/each}
            </div>
          {/if}
          <div class="chart" role="group" bind:clientWidth={chartW} aria-label="The run-up to the meal: what each oven, the stovetop and the cooks are doing, hour by hour. Arrow keys move between bars; a bar opens its step.">
            <div class="axis" aria-hidden="true">
              {#each chart.ticks as tk, i}
                {#if tk.label && (i < chart.ticks.length - 1 || chart.ticks.length < 3)}<span style="left: {tk.at}%" class:day-tick={tk.day}>{tk.day ?? clockAt(tk.iso, tz).replace(':00', '')}</span>{/if}
              {/each}
            </div>
            {#each chart.lanes as lane}
              <div class="lane">
                <span class="lane-name" aria-hidden="true">{lane.name}</span>
                <div class="lane-track" style="--rows: {lane.rows.length}">
                  {#each chart.ticks as tk}<span class="grid-line" style="left: {tk.at}%" aria-hidden="true"></span>{/each}
                  {#each lane.rows as row, r}
                    {#each row as b, k (b.key)}
                      {@const place = placeLabel(b, row, k, lane)}
                      {@const bk = `${lane.name}|${b.key}`}
                      <button type="button" id={barId(bk)} tabindex={bk === focusKey ? 0 : -1} onfocus={() => (barFocus = bk)} onkeydown={barKey} class="bar" class:hands={lane.hands} class:heat={b.kind === 'preheat'} class:was={b.status === 'done'} style="left: {b.left}%; width: max({b.width}%, 3px); top: calc({r} * var(--row))" title="{clockAt(b.start, tz)} {b.text}{b.recipe_title ? ` · ${b.recipe_title}` : ''}" aria-label="{lane.name}, {clockAt(b.start, tz)} to {clockAt(b.end, tz)}: {b.kind === 'preheat' ? `heating to ${b.oven_temp}°F` : b.text}{b.recipe_title ? `, ${b.recipe_title}` : ''}" onclick={() => showMe({ task: b.key })}>
                        {#if place.at === 'in'}{place.text}{/if}
                      </button>
                      {#if place.at === 'out'}<span class="bar-out" style="left: calc({b.left + b.width}% + 4px); top: calc({r} * var(--row))" aria-hidden="true">{place.text}</span>{/if}
                    {/each}
                  {/each}
                </div>
              </div>
            {/each}
            {#if chartNow !== null}<span class="chart-now" style="left: calc(var(--lane-name) + (100% - var(--lane-name)) * {chartNow / 100})" aria-hidden="true"></span>{/if}
            <p class="legend"><span class="key solid"></span>Hands-on <span class="key hatched"></span>Looks after itself <span class="key heat"></span>Oven heating</p>
          </div>
        {/if}

        {#if !plan.tasks.length}
          <p class="hint">Add dishes to the menu and their steps appear here, laid out in time.</p>
        {/if}
        {#each days as day (day.key)}
          <h3 class="day">{day.label}{#if day.key === serveDay}<span class="day-note">the meal</span>{/if}</h3>
          <ol class="tasks">
            {#each day.tasks as t (t.key)}
              {#if t.key === nowKey}<li class="now-rule" role="separator" aria-label="Now, {clockAt(new Date(now).toISOString(), tz)}"><span class="fig">{clockAt(new Date(now).toISOString(), tz)}</span>Now</li>{/if}
              <li class="task" id="task-{t.key}" tabindex="-1" class:quiet={t.kind === 'preheat'} class:past={past(t)} class:pinned={t.pinned} class:flash={flash === `task-${t.key}`}>
                <span class="when fig">{clockAt(t.start, tz)}</span>
                <div class="body">
                  <p class="text">{t.text}</p>
                  <p class="meta">
                    {#if t.recipe_title}<span>{t.recipe_title}{t.step ? ` · step ${t.step}` : ''}</span>{/if}
                    <span>{span(t.minutes)}{t.estimated ? ' (guessed)' : ''}</span>
                    {#if t.oven}<span>{t.oven}{t.oven_temp ? ` ${t.oven_temp}°F` : ''}</span>{:else if t.equipment}<span>{t.equipment}</span>{/if}
                    {#if t.hands_on && t.minutes}<span class="hands"><Hand size={14} aria-hidden="true" />hands-on</span>{/if}
                    {#if t.planned_for}<span class="flag">{t.status === 'missed' ? `make-ahead, was for ${dayWord(t.planned_for, tz, now)} ${clockAt(t.planned_for, tz)}` : movedFrom(t)}</span>{/if}
                  </p>
                  {#if t.hold}<p class="hold">Then it waits {holdPlace(t.hold.in)} until {holdUntil(t)}.</p>{/if}
                </div>
                {#if t.kind !== 'preheat'}
                  <button type="button" class="pin" aria-pressed={t.pinned} onclick={() => pin(t)} aria-label="{t.pinned ? 'Unpin' : 'Pin'} {t.text}"><Pin size={16} aria-hidden="true" /><span class="pin-word">{t.pinned ? 'Pinned' : 'Pin'}</span></button>
                {/if}
              </li>
            {/each}
          </ol>
        {/each}

        {#if plan.ovens.some((o) => o.uses.length)}
          <h3 class="section">Ovens</h3>
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
          <h3 class="section">To the table</h3>
          <ul class="serving">
            {#each plan.serving as s}
              {@const ready = readyAt.get(s.item_id)}
              <li>
                <span class="fig">{clockAt(ready ?? s.serve_at, tz)}</span>
                <span class="s-what">{s.recipe_title}{#if ready}<span class="was-at">planned for {clockAt(s.serve_at, tz)}</span>{/if}</span>
                <span class="dish" class:none={!s.dish}>{s.dish ?? 'No dish chosen'}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>
  {:else}
    <p class="hint" role="status">Loading the meal…</p>
  {/if}
  <p class="visually-hidden" aria-live="polite">{said}</p>
</main>

<dialog class="keys" bind:this={keysDialog} aria-labelledby="keys-h" onclick={(e) => e.target === keysDialog && keysDialog.close()}>
  <h2 id="keys-h" class="label">Keys on a meal</h2>
  <dl>
    {#each KEYS as [key, what]}<div><dt><kbd>{key}</kbd></dt><dd>{what}</dd></div>{/each}
  </dl>
  <form method="dialog"><button class="btn">Done</button></form>
</dialog>

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
  .crumbs {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  button.back {
    border: 0;
    background: none;
    padding: 0;
    gap: var(--space-2);
  }
  @media (hover: none) {
    button.back {
      display: none;
    }
  }
  .keys {
    width: min(28rem, calc(100% - 2 * var(--gutter)));
    padding: var(--space-5);
    border: 0;
    border-top: 3px solid var(--ink);
    background: var(--ticket);
    color: var(--ink);
  }
  .keys::backdrop {
    background: rgb(20 24 28 / 0.4);
  }
  .keys h2 {
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
  }
  .keys dl {
    margin: var(--space-2) 0 var(--space-4);
  }
  .keys dl > div {
    display: grid;
    grid-template-columns: 3.5rem 1fr;
    gap: var(--space-3);
    align-items: baseline;
    padding: var(--space-2) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
  }
  .keys dd {
    margin: 0;
  }
  kbd {
    display: inline-block;
    min-width: 1.75rem;
    padding: 1px var(--space-1);
    border: 1.5px solid var(--ink-3);
    border-radius: 3px;
    font-family: inherit;
    font-size: 0.875rem;
    font-weight: 700;
    line-height: 1.4;
    text-align: center;
  }
  .keys form {
    display: flex;
    justify-content: flex-end;
  }
  /* Late, the time it now lands leads the header, in red: the one figure
     that matters most today. */
  .lands {
    margin-top: var(--space-2);
    font-size: var(--title-m);
    font-weight: 700;
  }
  .lands .fig {
    font-size: 2rem;
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
  /* Running late, the planned time steps back for the time it now lands. */
  .served.was {
    color: var(--ink-3);
  }
  .missed-note {
    margin-top: var(--space-1);
    font-size: var(--ui);
    font-weight: 600;
    color: var(--ink-2);
  }
  .cook {
    min-height: 52px;
    padding: 0 var(--space-5);
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
  .late-block {
    padding-top: var(--space-3);
    border-top: 1.5px dashed var(--perf);
  }
  .unit {
    margin-left: 0.25em;
    font-size: 0.4em;
    font-stretch: 100%;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
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
    min-width: 0;
  }
  .side > .panel {
    min-width: 0;
  }
  .hint {
    color: var(--ink-2);
    font-size: var(--ui);
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
  .it-title {
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
    grid-column: 1 / -1;
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }
  .it-acts .rm {
    margin-left: auto;
    color: var(--ink-2);
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
  }
  .icon:disabled {
    color: var(--perf);
    cursor: default;
  }
  .it-summary {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    min-height: 44px;
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
    font-size: var(--ui);
    color: var(--ink-2);
  }
  .it-sum {
    min-width: 0;
  }
  .it-change {
    font-weight: 700;
    color: var(--ink);
    text-decoration: underline;
    text-underline-offset: 3px;
  }
  .it-summary:hover .it-change {
    color: var(--red);
  }
  .it-fields {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
    gap: var(--space-2) var(--space-3);
    padding-bottom: var(--space-2);
  }
  .it-fields label {
    display: grid;
    gap: 2px;
  }
  .it-fields .label {
    color: var(--ink-3);
  }
  .it-fields select {
    width: 100%;
  }
  .kitchen-link {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    min-height: 44px;
    font-size: var(--ui);
    font-weight: 600;
    color: var(--ink-2);
    text-decoration: none;
  }
  .kitchen-link:hover {
    color: var(--red);
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
  .matches .picked button {
    background: var(--ticket-press);
  }
  .said {
    margin-left: var(--space-2);
    font-weight: 600;
    letter-spacing: 0;
    text-transform: none;
    color: var(--ink-2);
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
    --lane-name: 88px;
    --row: 28px;
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
  .axis .day-tick {
    color: var(--ink);
  }
  .axis span {
    position: absolute;
    transform: translateX(-50%);
    font-size: 0.75rem;
    font-weight: 700;
    color: var(--ink-3);
    white-space: nowrap;
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
    /* No side padding: a bar is exactly as wide as its time, so short bars
       side by side never cover each other; the label is indented instead. */
    padding: 0;
    text-indent: 4px;
    box-sizing: border-box;
    border: 0;
    border-radius: 0;
    text-align: left;
    cursor: pointer;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
    font-size: 0.8125rem;
    font-weight: 700;
    line-height: calc(var(--row) - 4px);
    color: var(--ink);
    background: repeating-linear-gradient(135deg, var(--pass-deep) 0 3px, var(--ticket-press) 3px 6px);
    box-shadow: inset 0 0 0 1px var(--steel);
  }
  /* Hands-on bars that meet keep a ticket-white seam, so two tasks never
     read as one. */
  .bar.hands {
    background: var(--ink);
    color: var(--ticket);
    box-shadow: inset -2px 0 0 var(--ticket);
  }
  /* An oven heating: a thin steel strip along the foot of the row, leading
     into the bake it heats for. */
  .bar.heat {
    height: 6px;
    margin-top: calc(var(--row) - 8px);
    background: var(--steel);
    box-shadow: none;
  }
  .bar:focus-visible {
    outline: 2px solid var(--red);
    outline-offset: 2px;
    z-index: 1;
  }
  .bar.was {
    opacity: 0.45;
  }
  .bar:hover {
    box-shadow: inset 0 0 0 2px var(--ink);
  }
  .bar-out {
    position: absolute;
    margin-top: 2px;
    font-size: 0.8125rem;
    font-weight: 700;
    line-height: calc(var(--row) - 4px);
    color: var(--ink-2);
    white-space: nowrap;
    pointer-events: none;
  }
  .oven-lines {
    display: none;
    margin: var(--space-3) 0 0;
    padding: var(--space-3) 0 0;
    border-top: 1.5px dashed var(--perf);
    font-size: var(--ui);
    color: var(--ink-2);
  }
  .ol-name {
    font-weight: 700;
    color: var(--ink);
  }
  .oven-lines ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .oven-lines li {
    display: grid;
    grid-template-columns: 4.25rem minmax(0, 1fr);
    gap: var(--space-3);
    align-items: baseline;
  }
  .oven-lines .fig {
    font-size: 1.25rem;
    color: var(--ink);
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
  .key.heat {
    margin-left: var(--space-3);
    height: 5px;
    background: var(--steel);
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
  /* Pin's icon is always there to be found; at a desk its word shows on
     the row being pointed at, and once pinned, so twenty rows aren't
     twenty labels. */
  @media (hover: hover) and (pointer: fine) {
    .pin-word {
      opacity: 0;
    }
    .task:hover .pin-word,
    .task:focus-within .pin-word,
    .pin[aria-pressed='true'] .pin-word {
      opacity: 1;
    }
  }
  @media (hover: hover) and (pointer: fine) {
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
  /* The dish's name takes what it needs; the serving dish sits at the end. */
  .serving li {
    grid-template-columns: 5.5rem minmax(0, 1fr) auto;
  }
  .serving .dish {
    text-align: right;
  }
  .o-temp,
  .serving .fig {
    font-size: 1.375rem;
  }
  .o-what,
  .dish,
  .was-at {
    color: var(--ink-2);
  }
  /* Section heads below the days: labels on a dashed rule, not more days. */
  .section {
    margin: var(--space-6) 0 var(--space-1);
    padding-top: var(--space-2);
    border-top: 1.5px dashed var(--perf);
    font-size: var(--label);
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--ink-3);
  }
  .s-what {
    min-width: 0;
  }
  .was-at {
    display: block;
    font-size: 0.875rem;
  }
  .dish.none {
    color: var(--ink-3);
  }

  /* Beside the plan, the menu stays in view as the plan scrolls, scrolling
     on its own when it's the taller of the two. */
  @media (min-width: 761px) {
    .side {
      position: sticky;
      /* Below the masthead while it shows (it publishes its height). */
      top: calc(var(--bar-shown, 0px) + var(--space-3));
      max-height: calc(100dvh - var(--bar-shown, 0px) - var(--space-5));
      overflow-y: auto;
      overscroll-behavior: contain;
    }
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
    .now-rule .fig {
      width: 4.5rem;
    }
  }
  /* A phone: one column, the plan first once there are dishes; the day's
     ovens in a line each rather than a chart too small to read. */
  @media (max-width: 600px) {
    .chart {
      display: none;
    }
    .oven-lines {
      display: grid;
      gap: var(--space-1);
    }
  }
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
    .serving .dish {
      text-align: left;
    }
  }
  /* A narrow phone: the time sits above its task, so the step gets the width. */
  @media (max-width: 480px) {
    .task {
      grid-template-columns: minmax(0, 1fr) auto;
      gap: 0 var(--space-2);
    }
    .task .when {
      grid-column: 1;
      font-size: 1.125rem;
    }
    .task .body {
      grid-column: 1;
    }
    .task .pin {
      grid-column: 2;
      grid-row: 1 / span 2;
      align-self: center;
    }
  }
</style>
