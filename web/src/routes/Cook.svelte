<script>
  import { X, Timer, Pause, Play, ListChecks, Sun, Check } from '@lucide/svelte';
  import { go } from '../lib/router.svelte.js';
  import { api } from '../lib/api.js';
  import { amount } from '../lib/format.js';
  import { timers, findDurations, startTimer, remaining, togglePause, dismiss, clock } from '../lib/timers.svelte.js';

  let { id } = $props();
  let recipe = $state(null);
  let error = $state(null);
  let awake = $state(false);
  let sheet = $state(false); // ingredients, on narrow screens

  const KEY = `sous.cook.${id}`;
  const saved = (() => {
    try {
      return JSON.parse(sessionStorage.getItem(KEY)) ?? {};
    } catch {
      return {};
    }
  })();
  let step = $state(saved.step ?? 0);
  let checked = $state(saved.checked ?? []);

  $effect(() => {
    try {
      sessionStorage.setItem(KEY, JSON.stringify({ step, checked }));
    } catch {}
  });

  api.recipe(id).then(
    (r) => {
      recipe = r;
      step = Math.min(step, r.steps.length - 1);
      document.title = `Cooking ${r.title} · Sous`;
    },
    (err) => (error = err),
  );

  // Keep the screen on while cooking, and take the lock back when the tab returns.
  $effect(() => {
    let lock = null;
    const take = async () => {
      try {
        lock = await navigator.wakeLock?.request('screen');
        awake = !!lock;
        lock?.addEventListener('release', () => (awake = false));
      } catch {
        awake = false;
      }
    };
    const onVisible = () => document.visibilityState === 'visible' && take();
    take();
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      document.removeEventListener('visibilitychange', onVisible);
      lock?.release();
    };
  });

  const current = $derived(recipe?.steps[step]);
  const total = $derived(recipe?.steps.length ?? 0);
  const durations = $derived(current ? findDurations(current.text) : []);
  const last = $derived(step === total - 1);
  const queued = $derived(recipe ? recipe.steps.slice(step + 1, step + 3).map((s, k) => ({ ...s, n: step + 2 + k })) : []);
  const left = $derived(recipe ? recipe.ingredients.filter((r) => r.item).length - checked.length : 0);

  function move(by) {
    if (!recipe) return;
    const next = step + by;
    if (next < 0) return;
    if (next >= total) return finish();
    step = next;
  }

  function finish() {
    try {
      sessionStorage.removeItem(KEY);
    } catch {}
    go(`/r/${id}`);
  }

  function toggleIngredient(i) {
    checked = checked.includes(i) ? checked.filter((x) => x !== i) : [...checked, i];
  }

  function onkey(e) {
    if (e.target.closest?.('button') && (e.key === ' ' || e.key === 'Enter')) return;
    if (e.key === 'ArrowRight' || e.key === ' ' || e.key === 'PageDown') (e.preventDefault(), move(1));
    else if (e.key === 'ArrowLeft' || e.key === 'PageUp') (e.preventDefault(), move(-1));
    else if (e.key === 'Escape') sheet ? (sheet = false) : go(`/r/${id}`);
  }

  // A sideways swipe anywhere on the step turns the page.
  let startX = null;
  const down = (e) => (startX = e.clientX);
  function up(e) {
    if (startX == null) return;
    const dx = e.clientX - startX;
    startX = null;
    if (Math.abs(dx) > 70) move(dx < 0 ? 1 : -1);
  }

  const groups = $derived.by(() => {
    const out = [];
    recipe?.ingredients.forEach((row, index) => {
      if (!out.length || out.at(-1).section !== row.section) out.push({ section: row.section, rows: [] });
      out.at(-1).rows.push({ row, index });
    });
    return out;
  });
</script>

<svelte:window onkeydown={onkey} />

<div class="cook">
  <header class="bar">
    <button class="icon-btn" onclick={() => go(`/r/${id}`)} aria-label="Leave cook mode"><X size={26} /></button>
    <div class="title">
      <p class="name">{recipe?.title ?? ''}</p>
      {#if recipe}<p class="where label">Step {step + 1} of {total}{#if awake}<span class="awake" title="The screen won't sleep while cooking"><Sun size={13} aria-hidden="true" />Screen stays on</span>{/if}</p>{/if}
    </div>
    <button class="btn ing-toggle" aria-expanded={sheet} onclick={() => (sheet = !sheet)}><ListChecks aria-hidden="true" />Ingredients<span class="fig left">{left}</span></button>
  </header>

  {#if error}
    <main class="stage"><p class="step-text">{error.message}</p></main>
  {:else if recipe}
    <div class="body">
      <aside class="ingredients sheet" class:open={sheet} aria-label="Ingredients">
        <div class="ing-head"><h2 class="label">Ingredients</h2><span class="label">{checked.length} of {checked.length + left} gathered</span></div>
        {#each groups as g}
          {#if g.section}<h3>{g.section}</h3>{/if}
          <ul>
            {#each g.rows as { row, index }}
              {#if row.item}
                <li>
                  <button class="ing" class:got={checked.includes(index)} aria-pressed={checked.includes(index)} onclick={() => toggleIngredient(index)}>
                    <span class="box" aria-hidden="true">{#if checked.includes(index)}<Check size={18} strokeWidth={3} />{/if}</span>
                    <span class="amt fig">{amount(row)}</span>
                    <span>{row.item}{#if row.note}<span class="note">, {row.note}</span>{/if}</span>
                  </button>
                </li>
              {/if}
            {/each}
          </ul>
        {/each}
        <button class="btn sheet-close" onclick={() => (sheet = false)}>Back to the step</button>
      </aside>

      <main class="stage" onpointerdown={down} onpointerup={up}>
        <div class="rail" aria-hidden="true"></div>
        <div class="hung" class:timed={timers.list.length > 0}>
          {#key step}
            <article class="step ticket" aria-live="polite">
              <span class="clip" aria-hidden="true"></span>
              <header class="step-head">
                <span class="num fig">{step + 1}</span>
                <span class="of">of {total}</span>
                {#if current?.section}<span class="section">{current.section}</span>{/if}
              </header>
              <hr class="perf" />
              <p class="step-text" class:long={(current?.text.length ?? 0) > 280}>{current?.text}</p>
              {#if durations.length}
                <div class="timer-btns">
                  {#each durations as d}
                    {@const t = timers.list.find((x) => x.step === step && x.label === d.label && !x.done)}
                    {#if t}
                      <button class="btn timer-btn" aria-pressed="true" onclick={() => togglePause(t)}>
                        {#if t.paused}<Play aria-hidden="true" />Resume {d.label}{:else}<Pause aria-hidden="true" />Pause {d.label}{/if}
                      </button>
                    {:else}
                      <button class="btn timer-btn" onclick={() => startTimer(d.label, d.seconds, step)}><Timer aria-hidden="true" />Time {d.label}</button>
                    {/if}
                  {/each}
                </div>
              {/if}
            </article>
          {/key}
          {#each queued as q (q.n)}
            <button class="next ticket" onclick={() => move(q.n - 1 - step)} aria-label="Go to step {q.n}">
              <span class="clip" aria-hidden="true"></span>
              <span class="next-head"><span class="fig">{q.n}</span><span class="label">Up next</span></span>
              <span class="next-text">{q.text}</span>
            </button>
          {/each}
          {#if timers.list.length}
            <ul class="timers" aria-label="Timers">
              {#each timers.list as t (t.id)}
                <li class="tick ticket" class:done={t.done} class:paused={t.paused}>
                  <span class="clip" aria-hidden="true"></span>
                  <span class="t-label label">Step {t.step + 1} · {t.label}</span>
                  <span class="t-row">
                    <span class="t-clock fig" role="timer" aria-live={t.done ? 'assertive' : 'off'}>{t.done ? 'Done' : clock(remaining(t))}</span>
                    {#if !t.done}
                      <button class="icon-btn small" onclick={() => togglePause(t)} aria-label={t.paused ? 'Resume timer' : 'Pause timer'}>
                        {#if t.paused}<Play size={20} />{:else}<Pause size={20} />{/if}
                      </button>
                    {/if}
                    <button class="icon-btn small" onclick={() => dismiss(t)} aria-label={t.done ? 'Clear timer' : 'Cancel timer'}><X size={20} /></button>
                  </span>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      </main>
    </div>

    <nav class="pager" aria-label="Steps">
      <button class="btn big" onclick={() => move(-1)} disabled={step === 0}>Previous</button>
      <button class="btn primary big" onclick={() => move(1)}>{last ? 'Finish' : 'Next step'}</button>
    </nav>
  {/if}
</div>

<style>
  .cook {
    height: 100dvh;
    display: flex;
    flex-direction: column;
    background: var(--pass);
    overflow: hidden;
  }
  .bar {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-4);
    padding-top: max(var(--space-2), env(safe-area-inset-top));
    background: var(--ticket);
    border-bottom: 1px solid var(--perf);
  }
  .title {
    flex: 1;
    min-width: 0;
  }
  .name {
    font-size: var(--title-m);
    font-weight: 800;
    letter-spacing: -0.01em;
    line-height: 1.2;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .where {
    display: flex;
    gap: var(--space-4);
    color: var(--ink-3);
  }
  .awake {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .icon-btn {
    display: grid;
    place-items: center;
    flex: none;
    width: 56px;
    height: 56px;
    border: 0;
    border-radius: 3px;
    background: none;
  }
  .icon-btn:hover {
    background: var(--ticket-press);
  }
  .icon-btn.small {
    width: 44px;
    height: 44px;
  }
  .ing-toggle {
    display: none;
    min-height: 52px;
  }
  .left {
    font-size: 1.125rem;
    color: var(--red);
  }

  .body {
    flex: 1;
    display: grid;
    grid-template-columns: minmax(18rem, 24rem) minmax(0, 1fr);
    gap: var(--space-4);
    padding: var(--space-4) var(--space-4) 0;
    min-height: 0;
  }
  .ingredients {
    overflow-y: auto;
    align-self: start;
    max-height: 100%;
    background: var(--ticket);
    padding: var(--space-4) var(--space-4) var(--space-5);
    scrollbar-width: thin;
  }
  .ing-head {
    display: flex;
    justify-content: space-between;
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
  }
  .ing-head span {
    color: var(--ink-3);
  }
  .ingredients h3 {
    font-size: var(--ui);
    font-weight: 800;
    margin: var(--space-4) 0 0;
  }
  .ing {
    width: 100%;
    min-height: 56px;
    display: grid;
    grid-template-columns: 26px 4.5rem 1fr;
    gap: var(--space-3);
    align-items: start;
    text-align: left;
    border: 0;
    border-bottom: 1.5px dashed var(--ticket-press);
    background: none;
    padding: var(--space-3) var(--space-1);
    font-size: 1.125rem;
    line-height: 1.35;
  }
  .ing:hover {
    background: var(--ticket-press);
  }
  .box {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    border: 2px solid var(--ink);
    border-radius: 3px;
    margin-top: -1px;
  }
  .amt {
    font-size: 1.25rem;
    padding-top: 1px;
  }
  .note {
    color: var(--ink-3);
  }
  .ing.got {
    color: var(--ink-3);
    text-decoration: line-through;
    text-decoration-thickness: 2px;
  }
  .ing.got .box {
    background: var(--ink);
    border-color: var(--ink);
    color: var(--ticket);
  }
  .sheet-close {
    display: none;
  }

  /* The stage: a rail with the fired step hanging from it. */
  .stage {
    position: relative;
    overflow-y: auto;
    padding-bottom: var(--space-5);
    touch-action: pan-y;
    scrollbar-width: thin;
  }
  .stage > .rail {
    position: sticky;
    top: 0;
    z-index: 1;
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
  /* Timers are a lane of the same hanging row, clipped to the rail. While
     one runs, the queue narrows to a single ticket so the step keeps room. */
  .timed .next {
    flex-basis: 12rem;
  }
  .timed .next + .next {
    display: none;
  }
  .timed .t-clock {
    font-size: 1.875rem;
  }
  .timers {
    flex: 0 0 12.5rem;
    display: grid;
    gap: var(--space-3);
    margin-left: auto;
  }
  .tick {
    position: relative;
    display: grid;
    gap: 2px;
    padding: var(--space-3) var(--space-2) calc(var(--space-1) + var(--tear)) var(--space-4);
  }
  .t-label {
    color: var(--ink-3);
    padding-top: 4px;
  }
  .t-row {
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }
  .t-clock {
    font-size: 2.25rem;
    min-width: 4.2ch;
    color: var(--red);
  }
  .tick.paused .t-clock {
    color: var(--ink-3);
  }
  .tick.done {
    background: var(--red);
    color: var(--ticket);
    animation: ring 1.2s var(--ease-out) 3;
  }
  .tick.done .t-label,
  .tick.done .t-clock {
    color: var(--ticket);
  }
  .tick.done .icon-btn:hover {
    background: var(--red-press);
  }
  @keyframes ring {
    0%, 30%, 100% {
      transform: rotate(0);
    }
    10% {
      transform: rotate(-2deg);
    }
    20% {
      transform: rotate(2deg);
    }
  }

  .hung {
    display: flex;
    align-items: flex-start;
    gap: var(--space-4);
    padding: 0 var(--space-4);
    margin-top: -4px;
  }
  .step {
    position: relative;
    flex: 1 1 46rem;
    min-width: 0;
    max-width: 50rem;
    padding: var(--space-5) clamp(var(--space-4), 3vw, var(--space-7)) calc(var(--space-6) + var(--tear));
    transform-origin: top center;
    animation: fire 420ms var(--ease-out);
  }
  @keyframes fire {
    0% {
      opacity: 0;
      transform: translateX(56px) rotate(1.5deg);
    }
    55% {
      opacity: 1;
      transform: translateX(0) rotate(-0.6deg);
    }
    100% {
      transform: none;
    }
  }
  .step-head {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    padding-top: var(--space-2);
    margin-bottom: var(--space-3);
  }
  .num {
    font-size: clamp(4rem, 2.6rem + 4vw, 6.5rem);
    color: var(--red);
  }
  .of {
    font-size: 1.125rem;
    font-weight: 700;
    color: var(--ink-3);
  }
  .section {
    margin-left: auto;
    font-size: var(--ui);
    font-weight: 700;
    color: var(--ink-2);
  }
  .step-text {
    margin-top: var(--space-4);
    max-width: 36ch;
    font-size: clamp(1.5rem, 0.95rem + 1.5vw, 2.25rem);
    font-weight: 500;
    line-height: 1.32;
    letter-spacing: -0.01em;
  }
  /* A long step drops a size so it still fits the counter-distance view. */
  .step-text.long {
    font-size: clamp(1.25rem, 0.85rem + 1.1vw, 1.75rem);
    max-width: 44ch;
  }
  .timer-btns {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-top: var(--space-6);
  }
  .timer-btn {
    min-height: 60px;
    font-size: 1.0625rem;
    padding: 0 var(--space-5);
  }
  .timer-btn :global(svg) {
    width: 22px;
    height: 22px;
  }
  .next {
    position: relative;
    flex: 0 0 15rem;
    display: grid;
    gap: var(--space-2);
    padding: var(--space-5) var(--space-4) calc(var(--space-4) + var(--tear));
    border: 0;
    text-align: left;
    opacity: 0.72;
    transition: opacity 160ms;
  }
  .next:hover {
    opacity: 1;
  }
  .next + .next {
    flex-basis: 12rem;
    opacity: 0.5;
  }
  .next-head {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    color: var(--ink-2);
  }
  .next-head .fig {
    font-size: 2rem;
  }
  .next-text {
    font-size: var(--ui);
    line-height: 1.4;
    color: var(--ink-2);
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 5;
    -webkit-box-orient: vertical;
  }

  .pager {
    display: grid;
    grid-template-columns: 1fr 2fr;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    padding-bottom: max(var(--space-3), env(safe-area-inset-bottom));
    background: var(--ticket);
    border-top: 1px solid var(--perf);
  }
  .big {
    min-height: 72px;
    font-size: 1.125rem;
  }

  @media (max-width: 1439px) {
    .next + .next,
    .timed .next {
      display: none;
    }
  }
  @media (max-width: 1180px) {
    .next {
      display: none;
    }
  }
  @media (max-width: 900px) {
    .body {
      grid-template-columns: 1fr;
      position: relative;
      padding: var(--space-3) var(--space-2) 0;
    }
    .ing-toggle {
      display: inline-flex;
    }
    .ingredients {
      position: absolute;
      inset: var(--space-3) var(--space-2) 0;
      z-index: 2;
      max-height: none;
      visibility: hidden;
      transform: translateY(16px);
      opacity: 0;
      transition: opacity 200ms var(--ease-out), transform 200ms var(--ease-out), visibility 0s 200ms;
    }
    .ingredients.open {
      visibility: visible;
      opacity: 1;
      transform: none;
      transition: opacity 200ms var(--ease-out), transform 200ms var(--ease-out);
    }
    .sheet-close {
      display: flex;
      width: 100%;
      margin-top: var(--space-5);
      min-height: 56px;
    }
    .awake {
      display: none;
    }
    .hung {
      position: relative;
      padding: 0 var(--space-2);
    }
    /* On a phone the timer hangs over the step's top-right corner. */
    .timers {
      position: absolute;
      top: 0;
      right: var(--space-4);
      z-index: 1;
      width: 10.5rem;
    }
    .timed .step-head {
      padding-right: 10.5rem;
    }
    .tick .t-clock {
      font-size: 1.75rem;
    }
    .tick {
      background: var(--ticket-press);
      box-shadow: inset 0 0 0 1.5px var(--perf);
    }
  }
  @media (max-width: 560px) {
    .bar .ing-toggle {
      padding: 0 var(--space-3);
    }
    .name {
      font-size: var(--ui);
    }
  }
</style>
