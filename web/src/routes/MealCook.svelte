<script>
  import { onDestroy } from 'svelte';
  import { X, Check, Play, Undo2, BookOpen, Flame, ChevronDown } from '@lucide/svelte';
  import LateFixes from '../components/LateFixes.svelte';
  import { api } from '../lib/api.js';
  import { link } from '../lib/router.svelte.js';
  import { session, asPerson, toast } from '../lib/session.svelte.js';
  import { personName } from '../lib/format.js';
  import { clock } from '../lib/timers.svelte.js';
  import { clockAt, whenAt, span, runOfShow, remaining, cookedMinutes, holdPlace, mealWord, problemText } from '../lib/meals.js';

  let { id } = $props();

  let meal = $state(null);
  let plan = $state(null);
  let problem = $state('');
  let now = $state(Date.now());
  let pending = $state(new Set()); // task keys with a change on its way
  let barH = $state(68); // the bar's real height, for the rail sticking under it
  let fixesOpen = $state(false);
  let missedOpen = $state(false);

  // Each change made here counts, so a refresh fetched before one landed
  // doesn't put the older plan back over it.
  let writes = 0;
  async function refresh() {
    const at = writes;
    try {
      const p = await api.mealPlan(id);
      // The meal is fetched again only when it has changed (another cook
      // moved it, say), so its time and version stay current.
      const m = meal && meal.version === p.meal_version ? meal : await api.meal(id);
      if (at !== writes) return;
      if (plan) othersTicked(plan, p);
      meal = m;
      plan = p;
      problem = '';
      document.title = `Cooking ${m.title} · Sous`;
    } catch (err) {
      // A failed refresh keeps the plan on screen; the clock carries on.
      problem = err.code === 'offline' ? 'Sous can’t be reached; the plan shown may be out of date.' : err.message;
    }
  }
  // Another cook's ticks arrive with the next fetch; say whose, so a cook
  // coming back to the screen knows what changed while they were away.
  function othersTicked(before, after) {
    const was = new Map(before.tasks.map((t) => [t.key, t.status]));
    const me = session.person?.username;
    const fresh = after.tasks.filter((t) => t.status === 'done' && was.get(t.key) !== 'done' && t.done_by && t.done_by !== me);
    if (!fresh.length) return;
    const who = personName(fresh[0].done_by, session.people);
    const what = fresh.length === 1 ? fresh[0].text.replace(/\.$/, '') : `${fresh.length} steps`;
    toast(`${who} ticked off ${what}.`);
    said = `${who} ticked off ${what}.`;
  }
  refresh();

  // The clock ticks every second for the countdowns. The plan is fetched
  // every 20 seconds, so a second cook's taps show up quickly, and as soon
  // as the screen comes back.
  const tick = setInterval(() => (now = Date.now()), 1000);
  const poll = setInterval(() => document.visibilityState === 'visible' && refresh(), 20_000);
  // Back after a while away: say what came up meanwhile.
  let hiddenAt = null;
  async function onVisible() {
    if (document.visibilityState !== 'visible') {
      hiddenAt = Date.now();
      return;
    }
    const away = hiddenAt;
    hiddenAt = null;
    await refresh();
    keepAwake();
    if (!away || Date.now() - away < 2 * 60e3 || !plan) return;
    const came = plan.tasks.filter((t) => t.status !== 'done' && Date.parse(t.start) > away && Date.parse(t.start) <= Date.now());
    if (came.length) toast(`While you were away, ${came.length === 1 ? `“${came[0].text.replace(/\.$/, '')}” came up` : `${came.length} steps came up`}.`);
  }
  document.addEventListener('visibilitychange', onVisible);

  // The screen stays on while the meal is being cooked, as in cook mode.
  let lock = null;
  let gone = false;
  async function keepAwake() {
    try {
      const l = await navigator.wakeLock?.request('screen');
      // Left the screen while asking: let it sleep again.
      if (gone) l?.release?.().catch(() => {});
      else lock = l;
    } catch {
      lock = null;
    }
  }
  keepAwake();

  onDestroy(() => {
    gone = true;
    clearInterval(tick);
    clearInterval(poll);
    document.removeEventListener('visibilitychange', onVisible);
    lock?.release?.().catch(() => {});
  });

  async function change(tasks, body, undoable = true) {
    const keys = tasks.map((t) => t.key);
    pending = new Set([...pending, ...keys]);
    writes++;
    try {
      let p = null;
      for (const t of tasks) p = await asPerson(() => api.updateTask(id, t.key, body));
      if (p) plan = p;
      problem = '';
      // A tick can be taken back right where it was made: wet hands slip.
      if (p && body.done && undoable) {
        const what = tasks.length === 1 ? tasks[0].text.replace(/\.$/, '') : `${tasks.length} steps`;
        toast(`Ticked off: ${what}.`, { label: 'Undo', run: () => change(tasks, { done: false }, false) });
      }
    } catch (err) {
      problem = problemText(err);
    } finally {
      pending = new Set([...pending].filter((k) => !keys.includes(k)));
    }
  }

  let confirmReset = $state(false);
  async function reset() {
    confirmReset = false;
    writes++;
    const p = await asPerson(() => api.resetMeal(id)).catch((err) => ((problem = problemText(err)), null));
    if (p) plan = p;
  }

  const tz = $derived(meal?.timezone ?? 'UTC');
  const word = $derived(meal ? mealWord(meal.serve_at, tz) : 'The meal');
  const show = $derived(plan ? runOfShow(plan, now, { cooks: meal?.cooks ?? 1 }) : null);
  const open = $derived(show ? show.focus.length + show.timers.length + show.waiting.length + show.missed.length + show.next.length : 0);
  const lastServe = $derived(plan ? Math.max(Date.parse(plan.serve_at), ...plan.serving.map((s) => Date.parse(s.serve_at))) : 0);
  const over = $derived(plan && now > lastServe + 3 * 3600e3);
  const finished = $derived(plan && plan.tasks.length > 0 && (open === 0 || over));
  // When nothing is due, the rail holds the next task, so it is never empty.
  const nextUp = $derived(show && !show.focus.length && !show.timers.length ? show.next.find((t) => t.kind !== 'preheat') : null);
  const upNext = $derived(show ? show.next.filter((t) => t !== nextUp).slice(0, 8) : []);
  const farOff = $derived(nextUp && Date.parse(nextUp.start) - now > 12 * 3600e3);
  const cooked = $derived(plan ? cookedMinutes(plan) : null);

  function until(iso) {
    const min = Math.round((Date.parse(iso) - now) / 60e3);
    if (min <= 0) return 'now';
    if (min < 60) return `in ${min} min`;
    // Past half a day the day says it; the clock is beside it already.
    if (min >= 720) return new Date(iso).toLocaleDateString(undefined, { timeZone: tz, weekday: 'short', month: 'short', day: 'numeric' });
    return `in ${span(min)}`;
  }

  // What each oven is doing now, or next; a preheat counts as on.
  const ovens = $derived.by(() => {
    if (!plan) return [];
    const title = (u) => [...new Set(u.tasks.map((k) => plan.tasks.find((t) => t.key === k)?.recipe_title).filter(Boolean))].join(', ');
    const temp = (u) => (u.temp_max ? `${u.temp}–${u.temp_max}°` : u.temp ? `${u.temp}°` : 'On');
    return plan.ovens.map((o) => {
      // In use now means someone has the work in it under way.
      const under = (u) => u.tasks.some((k) => plan.tasks.find((t) => t.key === k)?.status === 'active');
      const cur = o.uses.find((u) => Date.parse(u.start) <= now && Date.parse(u.end) > now && under(u));
      const nxt = o.uses.find((u) => Date.parse(u.start) > now);
      // An oven heats once a cook has said it's on; a preheat that's due but
      // unconfirmed leaves the oven off.
      const heating = plan.tasks.find((t) => t.kind === 'preheat' && t.oven === o.name && t.status === 'active');
      const toTurnOn = plan.tasks.find((t) => t.kind === 'preheat' && t.oven === o.name && t.status !== 'active' && t.status !== 'done' && Date.parse(t.start) <= now + 5 * 60e3);
      return {
        name: o.name,
        figure: heating ? `${heating.oven_temp}°` : cur && !toTurnOn ? temp(cur) : 'Off',
        live: !!heating || !!(cur && !toTurnOn),
        line: cur && !toTurnOn ? `${title(cur)} until ${clockAt(cur.end, tz)}` : heating ? (remaining(heating, now) > 0 ? `Heating to ${heating.oven_temp}°F` : `Hot at ${heating.oven_temp}°F`) : toTurnOn ? `Off · turn on to ${toTurnOn.oven_temp}°F by ${clockAt(toTurnOn.start, tz)}` : nxt ? `Next: ${temp(nxt)}F at ${clockAt(nxt.start, tz)}` : 'Nothing more today',
      };
    });
  });

  // One quiet announcement as work comes up.
  let said = $state('');
  const seen = new Set();
  $effect(() => {
    if (!show) return;
    const fresh = show.focus.filter((t) => !seen.has(t.key));
    for (const t of show.focus) seen.add(t.key);
    if (fresh.length) said = `Now: ${fresh.map((t) => t.text).join('; ')}`;
  });

  // When the meal's landing time moves, say so once, as the bar changes.
  let landed = null;
  $effect(() => {
    if (!plan) return;
    const lands = plan.on_time ? 'on time' : clockAt(new Date(Date.parse(plan.serve_at) + plan.behind_minutes * 60e3).toISOString(), tz);
    if (landed !== null && lands !== landed) said = plan.on_time ? `${word} is on time again.` : `${word} now lands at ${lands}.`;
    landed = lands;
  });

  // A dish held for the meal waits until the meal, wherever it now lands.
  function holdUntil(t) {
    const s = plan.serving.find((x) => x.item_id === t.item_id);
    if (plan.on_time || !s || t.hold.until < s.serve_at) return whenAt(t.hold.until, tz, now);
    return `it goes to the table`;
  }

  const label = (t) => {
    if (t.status === 'active') return 'Under way';
    // Sous has already moved late work to start from now, so a start still
    // ahead is the truth; late wording only once that start has come.
    if (Date.parse(t.start) > now) return `Starts ${until(t.start)}`;
    if (t.planned_for) return `Start now · was ${clockAt(t.planned_for, tz)}`;
    return 'Now';
  };
</script>

<div class="run" style="--bar-h: {barH}px">
  <header class="bar" bind:clientHeight={barH}>
    <a class="icon-btn" href="/m/{id}" use:link aria-label="Back to the plan"><X size={22} /></a>
    <div class="who">
      <p class="name">{meal?.title ?? 'Meal'}</p>
      {#if meal}<p class="served">{plan && !plan.on_time ? 'Planned for' : 'Served'} {whenAt(meal.serve_at, tz, now)}</p>{/if}
    </div>
    {#if plan && !finished}
      {#if plan.on_time}
        <p class="status" role="status">On time · {word.toLowerCase()} at {clockAt(plan.serve_at, tz)}</p>
      {:else}
        <!-- Lead with when dinner now lands; the fixes are one tap away. -->
        <button type="button" class="status behind" aria-expanded={fixesOpen} aria-controls="fixes" onclick={() => (fixesOpen = !fixesOpen)}>
          {word} {clockAt(new Date(Date.parse(plan.serve_at) + plan.behind_minutes * 60e3).toISOString(), tz)}<span class="late-by">{span(plan.behind_minutes)} late</span><ChevronDown size={18} aria-hidden="true" />
        </button>
      {/if}
    {/if}
    <p class="now fig"><span class="visually-hidden">The time is </span>{clockAt(new Date(now).toISOString(), tz)}</p>
  </header>

  {#if fixesOpen && plan && !plan.on_time && meal}
    <section class="fixes" id="fixes" aria-label="Running late">
      <LateFixes {meal} {plan} {tz} compact onupdate={(u) => (writes++, (meal = u.meal), (plan = u.plan), u.plan.on_time && (fixesOpen = false))} />
    </section>
  {/if}

  {#if problem}<p class="problem" role="alert">{problem}</p>{/if}

  <main class="stage" id="main">
    {#if !plan || !show}
      <p class="hint" role="status">Loading the plan…</p>
    {:else if !plan.tasks.length}
      <div class="empty">
        <h1>Nothing to cook yet.</h1>
        <p>Add dishes to the menu first.</p>
        <a class="btn" href="/m/{id}" use:link>Back to the plan</a>
      </div>
    {:else}
      <h1 class="visually-hidden">Cooking {meal?.title}</h1>

      {#if show.missed.length && !finished}
        <!-- One quiet row: make-ahead still to do, sorted out when there's a
             moment, without pushing the step in hand down the screen. -->
        <section class="missed" aria-label="Make-ahead still to do">
          <button type="button" class="m-row" aria-expanded={missedOpen} onclick={() => (missedOpen = !missedOpen)}>
            <span><strong>Make-ahead still to do:</strong> {show.missed.map((m) => m.recipe_title).join(', ')}</span>
            <span class="m-sort">{missedOpen ? 'Close' : 'Start or tick off'}<ChevronDown size={18} aria-hidden="true" /></span>
          </button>
          {#if missedOpen}
            <ul>
              {#each show.missed as m (m.item_id)}
                {@const busy = m.tasks.some((t) => pending.has(t.key))}
                <li>
                  <p><strong>{m.recipe_title}</strong>: {m.tasks.length === 1 ? '1 step' : `${m.tasks.length} steps`}, {span(m.tasks.reduce((n, t) => n + t.minutes, 0))}. The plan has room for it at {clockAt(m.tasks[0].start, tz)}.</p>
                  <div class="m-acts">
                    <button class="btn" onclick={() => change([m.tasks[0]], { started: true })} disabled={busy} aria-label="Start {m.recipe_title}’s make-ahead now"><Play aria-hidden="true" />Start it now</button>
                    <button class="btn" onclick={() => change(m.tasks, { done: true })} disabled={busy} aria-label="{m.recipe_title}’s make-ahead is already made"><Check aria-hidden="true" />It’s already made</button>
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/if}

      <div class="rail-wrap"><div class="rail" aria-hidden="true"></div></div>
      {#if finished}
        <div class="hung">
          <article class="ticket card end">
            <span class="clip" aria-hidden="true"></span>
            {#if open === 0}
              <svg class="check" viewBox="0 0 48 48" aria-hidden="true"><path d="M10 25 L20 35 L38 14" /></svg>
              <p class="label">{plan.tasks.filter((t) => t.kind !== 'preheat').length} tasks</p>
              <p class="end-title">That’s {meal.title}.</p>
              {#if cooked}<p class="fig end-fig">{span(cooked)}</p>{/if}
              <p class="end-sub">Time to eat.</p>
            {:else}
              <p class="end-title">{meal.title} was served {whenAt(lastServe ? new Date(lastServe).toISOString() : plan.serve_at, tz, now)}.</p>
              <p class="end-sub">{show.done.length} of {plan.tasks.filter((t) => t.kind !== 'preheat').length} tasks were ticked off.</p>
            {/if}
          </article>
        </div>
      {:else}
        <div class="hung">
          <div class="focus">
            {#each show.focus.length ? show.focus : nextUp ? [nextUp] : [] as t, i (t.key)}
              {@const left = t.status === 'active' ? remaining(t, now) : null}
              {@const isNext = t === nextUp}
              <article class="ticket card" class:upcoming={isNext} class:over={left !== null && left <= 0} aria-busy={pending.has(t.key)}>
                <span class="clip" aria-hidden="true"></span>
                <div class="c-head">
                  {#if left !== null}
                    <span class="big fig live">{clock(Math.abs(left))}</span>
                    <span class="state">{left > 0 ? 'left' : 'over'}</span>
                  {:else}
                    <span class="big fig">{clockAt(t.start, tz)}</span>
                    <span class="state">{isNext ? `Next · ${until(t.start)}` : label(t)}</span>
                  {/if}
                </div>
                <hr class="perf" />
                <p class="text">{t.kind === 'preheat' ? `Turn the ${t.oven?.toLowerCase() ?? 'oven'} on to ${t.oven_temp}°F` : t.text}</p>
                <p class="meta">
                  {#if t.recipe_title}<span>{t.recipe_title}{t.step ? ` · step ${t.step}` : ''}</span>{/if}
                  <span>{span(t.minutes)}</span>
                  {#if t.oven}<span>{t.oven}{t.oven_temp ? ` · ${t.oven_temp}°F` : ''}</span>{:else if t.equipment}<span>{t.equipment}</span>{/if}
                  {#if t.hands_on && t.minutes}<span class="hands">hands-on</span>{/if}
                </p>
                {#if t.hold}<p class="hold">Then it waits {holdPlace(t.hold.in)} until {holdUntil(t)}.</p>{/if}
                {#if t.kind === 'preheat' && !isNext}
                  <div class="acts">
                    <button class="btn" class:primary={i === 0} onclick={() => change([t], { started: true })} disabled={pending.has(t.key)} aria-label="It’s on: {t.oven ?? 'oven'} heating to {t.oven_temp}°F"><Flame aria-hidden="true" />It’s on</button>
                  </div>
                {:else if !isNext}
                  <!-- Start leads until the step is under way; then Done does.
                       Before that, Done is a quiet "Already done", away from Start. -->
                  <div class="acts" class:started={t.status === 'active'}>
                    {#if t.status !== 'active'}
                      <button class="btn" class:primary={i === 0} onclick={() => change([t], { started: true })} disabled={pending.has(t.key)} aria-label="Start: {t.text} ({label(t)})"><Play aria-hidden="true" />Start</button>
                      <button class="btn quiet already" onclick={() => change([t], { done: true })} disabled={pending.has(t.key)} aria-label="Already done: {t.text}"><Check aria-hidden="true" />Already done</button>
                    {:else}
                      <button class="btn" class:primary={i === 0} onclick={() => change([t], { done: true })} disabled={pending.has(t.key)} aria-label="Done: {t.text}"><Check aria-hidden="true" />Done</button>
                    {/if}
                  </div>
                {/if}
                {#if t.recipe_id && t.step && t.kind !== 'preheat'}
                  <a class="open" href="/r/{t.recipe_id}/cook?step={t.step}" use:link><BookOpen size={18} aria-hidden="true" />The recipe at step {t.step}</a>
                {/if}
              </article>
            {/each}
            {#if farOff}
              <p class="calm">Nothing to do until {whenAt(nextUp.start, tz, now)}. Sous will keep the plan up to date until then.</p>
            {/if}
          </div>

          {#if show.timers.length}
            <ul class="timers" aria-label="Under way by themselves">
              {#each show.timers as t (t.key)}
                {@const left = remaining(t, now)}
                <li class="ticket tick" class:done={left <= 0 && t.kind !== 'preheat'}>
                  <span class="clip" aria-hidden="true"></span>
                  <p class="t-label">{#if t.kind === 'preheat'}<Flame size={16} aria-hidden="true" />{/if}{t.kind === 'preheat' ? `Heating to ${t.oven_temp}°F` : t.recipe_title}</p>
                  <p class="t-what">{t.kind === 'preheat' ? t.oven : t.text}</p>
                  <div class="t-row">
                    <span class="t-clock"><span class="fig">{left > 0 ? clock(left) : t.kind === 'preheat' ? 'Hot' : `+${clock(-left)}`}</span>{#if left > 0 || t.kind !== 'preheat'}<span class="t-unit">{left > 0 ? 'left' : 'over'}</span>{/if}</span>
                    {#if t.kind !== 'preheat'}
                      <button class="btn small" onclick={() => change([t], { done: true })} disabled={pending.has(t.key)} aria-label="Done: {t.text}"><Check aria-hidden="true" />Done</button>
                    {/if}
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}

      <div class="lower">
        {#if !finished && (show.waiting.length || upNext.length)}
          <section class="panel next" aria-labelledby="next-h">
            <h2 id="next-h" class="label">Up next</h2>
            <ol class="list">
              {#each show.waiting as t (t.key)}
                <!-- Due, but the cooks' hands are full: whoever's free can start it. -->
                <li class="due">
                  <span class="fig when">{clockAt(t.start, tz)}</span>
                  <span class="what">{t.text}{#if t.recipe_title}<span class="sub">{` · ${t.recipe_title}`}</span>{/if}<span class="sub">{" · when a cook is free"}</span></span>
                  <button class="btn" onclick={() => change([t], { started: true })} disabled={pending.has(t.key)} aria-label="Start: {t.text}"><Play aria-hidden="true" />Start</button>
                </li>
              {/each}
              {#each upNext as t (t.key)}
                <li class:quiet={t.kind === 'preheat'}>
                  <span class="fig when">{clockAt(t.start, tz)}</span>
                  <span class="what">{t.text}{#if t.recipe_title}<span class="sub">{` · ${t.recipe_title}`}</span>{/if}</span>
                  <span class="in">{until(t.start)}</span>
                </li>
              {/each}
            </ol>
          </section>
        {/if}

        {#if ovens.length && !finished}
          <section class="panel" aria-labelledby="ovens-h">
            <h2 id="ovens-h" class="label">Ovens</h2>
            <ul class="list">
              {#each ovens as o}
                <li>
                  <span class="fig when" class:live={o.live}>{o.figure}</span>
                  <span class="what">{o.name}<span class="sub">{` · ${o.line}`}</span></span>
                </li>
              {/each}
            </ul>
          </section>
        {/if}

        {#if show.done.length}
          <details class="panel done" open={finished}>
            <summary><span class="label">Done</span> <span class="count fig">{show.done.length}</span></summary>
            <ul class="list">
              {#each show.done as t (t.key)}
                <li>
                  <span class="fig when">{t.done_at ? clockAt(t.done_at, tz) : clockAt(t.end, tz)}</span>
                  <span class="what">{t.text}{#if t.recipe_title}<span class="sub">{` · ${t.recipe_title}`}</span>{/if}</span>
                  <button class="btn quiet" onclick={() => change([t], { done: false })} disabled={pending.has(t.key)} aria-label="Undo {t.text}"><Undo2 aria-hidden="true" />Undo</button>
                </li>
              {/each}
            </ul>
          </details>
        {/if}

        <div class="foot">
          {#if confirmReset}
            <p>Forget everything started and ticked off on this meal?</p>
            <button class="btn" onclick={() => (confirmReset = false)}>Keep it</button>
            <button class="btn quiet" onclick={reset}>Forget progress</button>
          {:else}
            <a class="btn quiet" href="/m/{id}" use:link>The whole plan</a>
            <button class="btn quiet" onclick={() => (confirmReset = true)}>Start over</button>
          {/if}
        </div>
      </div>
    {/if}
  </main>
  <p class="visually-hidden" aria-live="polite">{said}</p>
</div>

<style>
  .run {
    min-height: 100dvh;
    /* A ticket firing onto the rail slides in from the right; clip it, so
       the page never scrolls sideways (clip, unlike hidden, keeps sticky). */
    overflow-x: clip;
    display: flex;
    flex-direction: column;
  }
  .bar {
    position: sticky;
    top: 0;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: var(--space-4);
    min-height: var(--bar-h);
    padding: var(--space-2) var(--gutter);
    padding-top: max(var(--space-2), env(safe-area-inset-top));
    background: var(--ticket);
    border-bottom: 1px solid var(--perf);
  }
  .icon-btn {
    display: inline-grid;
    place-items: center;
    width: 52px;
    height: 52px;
    border-radius: 3px;
    color: var(--ink);
  }
  .icon-btn:hover {
    background: var(--ticket-press);
  }
  .who {
    flex: 1;
    min-width: 0;
  }
  .name {
    font-size: var(--title-m);
    font-weight: 800;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .served {
    font-size: var(--ui);
    color: var(--ink-2);
  }
  .now {
    font-size: 2.25rem;
  }
  .status {
    font-size: 1.0625rem;
    font-weight: 700;
    color: var(--ink-2);
  }
  button.status {
    display: inline-flex;
    align-items: baseline;
    gap: var(--space-2);
    min-height: 52px;
    padding: 0 var(--space-3);
    border: 2px solid var(--red);
    border-radius: 3px;
    background: none;
    align-self: center;
    align-items: center;
  }
  .status.behind {
    color: var(--red);
  }
  .late-by {
    font-weight: 600;
  }
  .fixes {
    position: relative;
    z-index: 4;
    padding: var(--space-4) var(--gutter);
    background: var(--ticket);
    border-bottom: 3px solid var(--ink);
  }
  .problem {
    padding: var(--space-2) var(--gutter);
    background: var(--ink);
    color: var(--ticket);
    font-weight: 600;
  }
  .stage {
    flex: 1;
    padding: var(--space-4) var(--gutter) var(--space-8);
  }
  .hint {
    color: var(--ink-2);
  }
  .empty {
    display: grid;
    gap: var(--space-3);
    justify-items: start;
    max-width: 36rem;
    margin: var(--space-7) auto;
  }
  .empty h1 {
    font-size: var(--title-l);
  }

  /* Missed make-ahead: a plain strip above the rail, one choice per dish. */
  .missed {
    margin-bottom: var(--space-4);
    padding: var(--space-1) var(--space-5);
    background: var(--ticket);
    border-top: 3px solid var(--ink);
  }
  .m-row {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    min-height: 44px;
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
    color: var(--ink-2);
  }
  .m-row strong {
    color: var(--ink);
  }
  .m-sort {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-weight: 700;
    color: var(--ink);
    white-space: nowrap;
  }
  .m-row[aria-expanded='true'] :global(svg) {
    transform: rotate(180deg);
  }
  .missed ul {
    list-style: none;
    margin: var(--space-2) 0 0;
    padding: 0;
  }
  .missed li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2) var(--space-4);
    padding: var(--space-2) 0;
    border-top: 1.5px dashed var(--ticket-press);
  }
  .m-acts {
    display: flex;
    gap: var(--space-2);
  }
  .m-acts .btn {
    min-height: 52px;
  }

  /* The rail stays under the bar while the tickets scroll, as in cook mode. */
  /* Just the rail sticks, flush under the bar: it covers nothing but the
     strip of pass the tickets hang below. */
  .rail-wrap {
    position: sticky;
    top: var(--bar-h);
    z-index: 3;
  }
  .hung {
    display: flex;
    align-items: flex-start;
    gap: var(--space-4);
    padding: 0 var(--space-4);
    margin-top: -4px;
  }
  .focus {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: var(--space-4);
  }
  .card {
    position: relative;
    flex: 1 1 24rem;
    max-width: 42rem;
    display: grid;
    gap: var(--space-3);
    padding: var(--space-5) clamp(var(--space-4), 2.5vw, var(--space-6)) calc(var(--space-5) + var(--tear));
    animation: fire 420ms var(--ease-out);
  }
  @keyframes fire {
    0% {
      opacity: 0;
      transform: translateX(40px) rotate(1.2deg);
    }
    60% {
      opacity: 1;
      transform: translateX(0) rotate(-0.4deg);
    }
    100% {
      transform: none;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .card {
      animation: none;
    }
  }
  .card.upcoming .text {
    color: var(--ink-2);
  }
  .clip {
    position: absolute;
    top: 0;
    left: 50%;
    width: 40px;
    height: 10px;
    margin-left: -20px;
    border-radius: 0 0 3px 3px;
    background: linear-gradient(var(--steel), var(--steel-lo));
  }
  .c-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
    padding-top: var(--space-2);
  }
  .big {
    font-size: clamp(2.75rem, 2rem + 2vw, 3.5rem);
    white-space: nowrap;
  }
  .live {
    color: var(--red);
  }
  /* Read from across the counter: what to do about the time, not a caption. */
  .state {
    font-size: 1.125rem;
    font-weight: 700;
    color: var(--ink-2);
    text-align: right;
  }
  .text {
    font-size: clamp(1.5rem, 0.95rem + 1.5vw, 2.125rem);
    font-weight: 500;
    line-height: 1.28;
    max-width: 36ch;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--space-3);
    font-size: 1.0625rem;
    color: var(--ink-2);
  }
  .hands {
    font-weight: 700;
    color: var(--ink);
  }
  .hold {
    font-weight: 700;
  }
  .acts {
    display: flex;
    gap: var(--space-3);
  }
  .acts .btn {
    flex: 1;
    min-height: 72px;
    font-size: 1.1875rem;
  }
  /* Before a step starts, Done steps back: quieter, and set apart from Start. */
  .acts .btn.already {
    flex: 0 1 auto;
    margin-left: var(--space-3);
    font-size: 1.0625rem;
    color: var(--ink-2);
  }
  .acts .btn :global(svg) {
    width: 22px;
    height: 22px;
  }
  .open {
    justify-self: start;
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 44px;
    padding: 0 var(--space-2);
    margin-left: calc(var(--space-2) * -1);
    border-radius: 3px;
    font-size: var(--ui);
    font-weight: 600;
    color: var(--ink-2);
    text-decoration: none;
  }
  .open:hover {
    background: var(--ticket-press);
    color: var(--ink);
  }
  /* A ticket whose time is up turns solid red, like a finished timer; its
     buttons turn to paper so Done still reads. */
  .card.over {
    background: var(--red);
    color: var(--ticket);
  }
  .card.over .live,
  .card.over .state,
  .card.over .meta,
  .card.over .hands,
  .card.over .open {
    color: var(--ticket);
  }
  .card.over .perf {
    border-top-color: var(--ticket);
  }
  .card.over .btn {
    border-color: var(--ticket);
    background: transparent;
    color: var(--ticket);
  }
  .card.over .btn.primary {
    background: var(--ticket);
    color: var(--red);
  }
  /* Its own line under the ticket: a max-width here would let it sit
     beside the ticket and squeeze it. */
  .calm {
    flex: 1 0 100%;
    color: var(--ink-2);
  }

  /* Hands-off work and heating ovens: a lane of small tickets beside the step. */
  .timers {
    flex: 0 0 16rem;
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: var(--space-3);
  }
  .tick {
    position: relative;
    display: grid;
    gap: 2px;
    padding: var(--space-4) var(--space-3) calc(var(--space-2) + var(--tear)) var(--space-4);
  }
  .t-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.9375rem;
    font-weight: 700;
    color: var(--ink-2);
  }
  .t-what {
    font-size: 0.9375rem;
    color: var(--ink-2);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .t-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin-top: var(--space-1);
  }
  .t-clock {
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    font-size: 2.25rem;
    color: var(--red);
  }
  .t-clock .fig {
    min-width: 4.2ch;
  }
  /* A countdown says it's one, so 2:02 never reads as a time of day. */
  .t-unit {
    font-size: 1rem;
    font-weight: 700;
  }
  .btn.small {
    min-height: 52px;
  }
  .tick.done {
    background: var(--red);
    color: var(--ticket);
  }
  .tick.done .t-label,
  .tick.done .t-what,
  .tick.done .t-clock {
    color: var(--ticket);
  }
  .tick.done .btn {
    background: var(--ticket);
    border-color: var(--ticket);
    color: var(--red);
  }

  .end {
    flex: 0 1 34rem;
    justify-items: center;
    text-align: center;
  }
  .check {
    width: 56px;
    height: 56px;
    fill: none;
    stroke: var(--red);
    stroke-width: 5;
    stroke-linecap: square;
    stroke-dasharray: 48;
    stroke-dashoffset: 48;
    animation: draw 520ms var(--ease-out) 200ms forwards;
  }
  @keyframes draw {
    to {
      stroke-dashoffset: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .check {
      animation: none;
      stroke-dashoffset: 0;
    }
  }
  .end .label {
    color: var(--ink-3);
  }
  .end-title {
    font-size: var(--title-l);
    font-weight: 800;
  }
  .end-fig {
    font-size: 2.75rem;
  }
  .end-sub {
    color: var(--ink-3);
  }

  .lower {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
    gap: var(--space-4);
    margin-top: var(--space-6);
    align-items: start;
  }
  .lower > .done,
  .foot {
    grid-column: 1 / -1;
  }
  .panel {
    background: var(--ticket);
    border-top: 3px solid var(--ink);
    padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .panel .label {
    color: var(--ink-3);
  }
  .list {
    list-style: none;
    margin: var(--space-2) 0 0;
    padding: 0;
  }
  .list li {
    display: grid;
    grid-template-columns: 5.5rem minmax(0, 1fr) auto;
    gap: var(--space-3);
    align-items: baseline;
    padding: var(--space-2) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
  }
  .list .when {
    font-size: 1.625rem;
  }
  .list li.quiet .what,
  .list li.quiet .when {
    color: var(--ink-3);
  }
  .what {
    font-size: 1.0625rem;
  }
  .sub {
    color: var(--ink-2);
  }
  .in {
    font-size: var(--ui);
    color: var(--ink-2);
    white-space: nowrap;
  }
  .done summary {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    min-height: 44px;
    cursor: pointer;
  }
  .done .count {
    font-size: 1.375rem;
  }
  .done .what {
    color: var(--ink-2);
  }
  .foot {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3);
  }
  .foot p {
    flex: 1 0 100%;
    font-weight: 700;
  }

  /* An iPad held upright: the timers lane sits under the step, side by
     side, and the lower panels stack. */
  @media (max-width: 1000px) {
    .hung {
      flex-direction: column;
      align-items: stretch;
    }
    .timers {
      flex: none;
      grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
    }
    .card {
      max-width: none;
    }
    .lower {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  @media (max-width: 760px) {
    .bar {
      flex-wrap: wrap;
      gap: var(--space-1) var(--space-3);
    }
    .now {
      font-size: 1.75rem;
    }
    .status {
      order: 4;
      flex: 1 0 100%;
      padding-left: calc(52px + var(--space-3));
    }
    .hung {
      padding: 0 var(--space-2);
    }
    .card {
      padding: var(--space-4) var(--space-4) calc(var(--space-4) + var(--tear));
    }
    .acts .btn {
      min-height: 64px;
    }
    .missed {
      padding: var(--space-1) var(--space-4);
    }
    button.status {
      justify-self: start;
    }
    .list li {
      grid-template-columns: 4.5rem minmax(0, 1fr);
    }
    .list .in {
      grid-column: 2;
    }
    .list .when {
      font-size: 1.375rem;
    }
  }
</style>
