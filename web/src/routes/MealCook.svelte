<script>
  import { onDestroy } from 'svelte';
  import { X, Check, Play, Undo2, ChefHat } from '@lucide/svelte';
  import { api } from '../lib/api.js';
  import { go, link } from '../lib/router.svelte.js';
  import { asPerson } from '../lib/session.svelte.js';
  import { clock } from '../lib/timers.svelte.js';
  import { clockAt, whenAt, span, runOfShow, remaining } from '../lib/meals.js';

  let { id } = $props();

  let meal = $state(null);
  let plan = $state(null);
  let problem = $state('');
  let now = $state(Date.now());
  let pending = $state(new Set()); // task keys with a change on its way

  async function refresh() {
    try {
      const [m, p] = await Promise.all([meal ? Promise.resolve(meal) : api.meal(id), api.mealPlan(id)]);
      meal = m;
      plan = p;
      problem = '';
      document.title = `Cooking ${m.title} · Sous`;
    } catch (err) {
      // A failed refresh keeps the plan on screen; the clock carries on.
      problem = err.code === 'offline' ? 'Sous can’t be reached; the plan shown may be out of date.' : err.message;
    }
  }
  refresh();

  // The clock ticks every second for the countdowns; the plan is fetched
  // again each minute, and as soon as the screen comes back.
  const tick = setInterval(() => (now = Date.now()), 1000);
  const poll = setInterval(refresh, 60_000);
  const onVisible = () => document.visibilityState === 'visible' && (refresh(), keepAwake());
  document.addEventListener('visibilitychange', onVisible);

  // The screen stays on while the meal is being cooked, as in cook mode.
  let lock = null;
  async function keepAwake() {
    try {
      lock = await navigator.wakeLock?.request('screen');
    } catch {
      lock = null;
    }
  }
  keepAwake();

  onDestroy(() => {
    clearInterval(tick);
    clearInterval(poll);
    document.removeEventListener('visibilitychange', onVisible);
    lock?.release?.().catch(() => {});
  });

  async function change(t, body) {
    pending = new Set([...pending, t.key]);
    try {
      const p = await asPerson(() => api.updateTask(id, t.key, body));
      if (p) plan = p;
      problem = '';
    } catch (err) {
      problem = err.code === 'offline' ? 'That didn’t reach Sous. Check the connection and try again.' : err.message;
    } finally {
      pending = new Set([...pending].filter((k) => k !== t.key));
    }
  }

  let confirmReset = $state(false);
  async function reset() {
    confirmReset = false;
    const p = await asPerson(() => api.resetMeal(id)).catch((err) => ((problem = err.message), null));
    if (p) plan = p;
  }

  const tz = $derived(meal?.timezone ?? 'UTC');
  const show = $derived(plan ? runOfShow(plan, now) : { live: [], next: [], done: [] });
  // The rail always holds something: what's on now, or else the next task.
  const hanging = $derived(show.live.length ? show.live : show.next.slice(0, 1));
  const upNext = $derived(show.live.length ? show.next.slice(0, 6) : show.next.slice(1, 7));
  const firstActionable = $derived(hanging.find((t) => t.kind !== 'preheat' && (show.live.includes(t))));
  const finished = $derived(plan && plan.tasks.length > 0 && show.live.length === 0 && show.next.length === 0);

  function until(iso) {
    const min = Math.round((Date.parse(iso) - now) / 60e3);
    if (min <= 0) return 'now';
    if (min < 60) return `in ${min} min`;
    // Past half a day a clock is easier to read than a countdown.
    if (min >= 720) return whenAt(iso, tz, now);
    return `in ${span(min)}`;
  }

  // What each oven is doing now, or next.
  const ovens = $derived.by(() => {
    if (!plan) return [];
    return plan.ovens.map((o) => {
      const cur = o.uses.find((u) => Date.parse(u.start) <= now && Date.parse(u.end) > now);
      const nxt = o.uses.find((u) => Date.parse(u.start) > now);
      const title = (u) => [...new Set(u.tasks.map((k) => plan.tasks.find((t) => t.key === k)?.recipe_title).filter(Boolean))].join(', ');
      return { name: o.name, cur: cur && { ...cur, what: title(cur) }, nxt: nxt && { ...nxt, what: title(nxt) } };
    });
  });

  // One quiet announcement as tasks come due.
  let said = $state('');
  let seen = new Set();
  $effect(() => {
    const fresh = show.live.filter((t) => !seen.has(t.key));
    for (const t of show.live) seen.add(t.key);
    if (fresh.length && plan) said = `Now: ${fresh.map((t) => t.text).join('; ')}`;
  });

  const label = (t) => {
    if (t.kind === 'preheat') return 'Preheat';
    if (t.status === 'active') return remaining(t, now) <= 0 ? 'Time’s up' : 'Under way';
    if (t.status === 'late') return 'Late';
    if (Date.parse(t.start) > now) return Date.parse(t.start) - now >= 720 * 60e3 ? 'Next up' : `Starts ${until(t.start)}`;
    return 'Now';
  };
</script>

<div class="run">
  <header class="bar">
    <a class="icon-btn" href="/m/{id}" use:link aria-label="Back to the plan"><X size={22} /></a>
    <div class="who">
      <p class="name">{meal?.title ?? 'Meal'}</p>
      {#if meal}<p class="served">Served {whenAt(meal.serve_at, tz, now)}</p>{/if}
    </div>
    <p class="now fig" aria-label="The time">{clockAt(new Date(now).toISOString(), tz)}</p>
    {#if plan}
      <p class="status" class:behind={!plan.on_time} role="status">{plan.on_time ? 'On time' : `About ${span(plan.behind_minutes)} behind`}</p>
    {/if}
  </header>

  {#if problem}<p class="problem" role="alert">{problem}</p>{/if}

  <main class="stage" id="main">
    {#if !plan}
      <p class="hint" role="status">Loading the plan…</p>
    {:else if !plan.tasks.length}
      <div class="empty">
        <h1>Nothing to cook yet.</h1>
        <p>Add dishes to the menu first.</p>
        <a class="btn" href="/m/{id}" use:link>Back to the plan</a>
      </div>
    {:else}
      <h1 class="visually-hidden">Cooking {meal?.title}</h1>
      <div class="rail" aria-hidden="true"></div>
      {#if finished}
        <div class="hung">
          <article class="ticket card end">
            <span class="clip" aria-hidden="true"></span>
            <ChefHat size={40} aria-hidden="true" />
            <p class="end-title">That’s {meal.title}.</p>
            <p class="end-sub">Time to eat.</p>
          </article>
        </div>
      {:else}
        <div class="hung">
          {#each hanging as t (t.key)}
            {@const left = t.status === 'active' ? remaining(t, now) : null}
            <article class="ticket card" class:upcoming={!show.live.includes(t)} class:over={left !== null && left <= 0} aria-busy={pending.has(t.key)}>
              <span class="clip" aria-hidden="true"></span>
              <div class="c-head">
                {#if left !== null}
                  <span class="big fig live">{left > 0 ? clock(left) : clock(-left)}</span>
                {:else}
                  <span class="big fig">{clockAt(t.start, tz)}</span>
                {/if}
                <span class="label state">{label(t)}{left !== null && left <= 0 ? ` · over by ${clock(-left)}` : ''}</span>
              </div>
              <hr class="perf" />
              <p class="text">{t.text}</p>
              <p class="meta">
                {#if t.recipe_title}<span>{t.recipe_title}{t.step ? ` · step ${t.step}` : ''}</span>{/if}
                <span>{span(t.minutes)}</span>
                {#if t.oven}<span>{t.oven}{t.oven_temp ? ` · ${t.oven_temp}°F` : ''}</span>{:else if t.equipment}<span>{t.equipment}</span>{/if}
                {#if t.hands_on && t.minutes}<span class="hands">hands-on</span>{/if}
              </p>
              {#if t.hold}<p class="hold">Then into the {t.hold.in === 'warm' ? 'warm' : t.hold.in} until {whenAt(t.hold.until, tz, now)}.</p>{/if}
              {#if t.kind !== 'preheat' && show.live.includes(t)}
                <div class="acts">
                  {#if t.status !== 'active'}
                    <button class="btn" onclick={() => change(t, { started: true })} disabled={pending.has(t.key)}><Play aria-hidden="true" />Start</button>
                  {/if}
                  <button class="btn" class:primary={t === firstActionable} onclick={() => change(t, { done: true })} disabled={pending.has(t.key)}><Check aria-hidden="true" />Done</button>
                </div>
                {#if t.recipe_id && t.step}
                  <a class="open" href="/r/{t.recipe_id}/cook?step={t.step}" use:link>Open the recipe at step {t.step}</a>
                {/if}
              {/if}
            </article>
          {/each}
        </div>
      {/if}

      <div class="lower">
        {#if upNext.length}
          <section class="panel" aria-labelledby="next-h">
            <h2 id="next-h" class="label">Up next</h2>
            <ol class="list">
              {#each upNext as t (t.key)}
                <li class:quiet={t.kind === 'preheat'}>
                  <span class="fig when">{clockAt(t.start, tz)}</span>
                  <span class="what">{t.text}{#if t.recipe_title}<span class="sub"> · {t.recipe_title}</span>{/if}</span>
                  <span class="in">{until(t.start)}</span>
                </li>
              {/each}
            </ol>
          </section>
        {/if}

        {#if ovens.length}
          <section class="panel" aria-labelledby="ovens-h">
            <h2 id="ovens-h" class="label">Ovens</h2>
            <ul class="list">
              {#each ovens as o}
                <li>
                  <span class="fig when" class:live={o.cur}>{o.cur?.temp ? `${o.cur.temp}°` : o.cur ? 'On' : 'Off'}</span>
                  <span class="what">
                    {o.name}{#if o.cur}<span class="sub"> · {o.cur.what} until {clockAt(o.cur.end, tz)}</span>{/if}
                    {#if o.nxt}<span class="sub"> · next {o.nxt.temp ? `${o.nxt.temp}°F` : ''} at {clockAt(o.nxt.start, tz)}</span>{/if}
                  </span>
                </li>
              {/each}
            </ul>
          </section>
        {/if}

        {#if show.done.length}
          <details class="panel done">
            <summary><span class="label">Done</span> <span class="count fig">{show.done.length}</span></summary>
            <ul class="list">
              {#each show.done.filter((t) => t.kind !== 'preheat') as t (t.key)}
                <li>
                  <span class="fig when">{t.done_at ? clockAt(t.done_at, tz) : clockAt(t.end, tz)}</span>
                  <span class="what">{t.text}{#if t.recipe_title}<span class="sub"> · {t.recipe_title}</span>{/if}</span>
                  <button class="btn quiet" onclick={() => change(t, { done: false })} disabled={pending.has(t.key)} aria-label="Undo {t.text}"><Undo2 aria-hidden="true" />Undo</button>
                </li>
              {/each}
            </ul>
          </details>
        {/if}

        <div class="foot">
          {#if confirmReset}
            <p>Forget what’s been started and ticked off?</p>
            <button class="btn quiet" onclick={reset}>Start over</button>
            <button class="btn" onclick={() => (confirmReset = false)}>Keep going</button>
          {:else}
            <button class="btn quiet" onclick={() => (confirmReset = true)}>Start over</button>
            <a class="btn quiet" href="/m/{id}" use:link>The whole plan</a>
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
    min-height: 64px;
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
    font-size: 0.875rem;
    color: var(--ink-2);
  }
  .now {
    font-size: 2rem;
  }
  .status {
    font-size: var(--ui);
    font-weight: 700;
    color: var(--ink-2);
  }
  .status.behind {
    color: var(--red);
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
  .rail {
    position: relative;
    z-index: 1;
  }
  .hung {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: var(--space-4);
    padding: 0 var(--space-4);
    margin-top: -4px;
  }
  .card {
    position: relative;
    flex: 1 1 20rem;
    max-width: 30rem;
    display: grid;
    gap: var(--space-3);
    padding: var(--space-5) var(--space-5) calc(var(--space-5) + var(--tear));
  }
  .card.upcoming {
    max-width: 26rem;
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
    font-size: 2.75rem;
  }
  .live {
    color: var(--red);
  }
  .state {
    color: var(--ink-3);
    text-align: right;
  }
  .card.over {
    background: var(--red);
    color: var(--ticket);
  }
  .card.over .live,
  .card.over .state,
  .card.over .meta,
  .card.over .open {
    color: var(--ticket);
  }
  .card.over .btn {
    border-color: var(--ticket);
  }
  .text {
    font-size: clamp(1.25rem, 1rem + 0.8vw, 1.625rem);
    font-weight: 500;
    line-height: 1.3;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--space-3);
    font-size: var(--ui);
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
    min-height: 60px;
    font-size: 1.0625rem;
  }
  .open {
    font-size: var(--ui);
    color: var(--ink-2);
  }
  .end {
    justify-items: center;
    text-align: center;
  }
  .end :global(svg) {
    color: var(--red);
  }
  .end-title {
    font-size: var(--title-l);
    font-weight: 800;
  }
  .end-sub {
    color: var(--ink-3);
  }
  .lower {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 22rem), 1fr));
    gap: var(--space-4);
    margin-top: var(--space-6);
    align-items: start;
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
    grid-template-columns: 5.25rem minmax(0, 1fr) auto;
    gap: var(--space-3);
    align-items: center;
    padding: var(--space-2) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
  }
  .list .when {
    font-size: 1.5rem;
  }
  .list li.quiet .what,
  .list li.quiet .when {
    color: var(--ink-3);
  }
  .sub {
    color: var(--ink-2);
  }
  .in {
    font-size: 0.875rem;
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
    grid-column: 1 / -1;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3);
  }
  .foot p {
    font-weight: 700;
  }
  @media (max-width: 760px) {
    .bar {
      flex-wrap: wrap;
      gap: var(--space-2) var(--space-3);
    }
    .now {
      font-size: 1.625rem;
    }
    .status {
      flex: 1 0 100%;
      padding-left: calc(52px + var(--space-3));
    }
    .hung {
      padding: 0 var(--space-2);
    }
    .card {
      flex-basis: 100%;
      max-width: none;
      padding: var(--space-4) var(--space-4) calc(var(--space-4) + var(--tear));
    }
    .list li {
      grid-template-columns: 4.25rem minmax(0, 1fr) auto;
    }
    .list .when {
      font-size: 1.25rem;
    }
  }
</style>
