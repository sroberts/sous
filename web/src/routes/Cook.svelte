<script>
  import { X, Timer, Pause, Play, ListChecks, Sun } from '@lucide/svelte';
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
    <p class="title">{recipe?.title ?? ''}</p>
    <div class="bar-end">
      {#if awake}<span class="awake caps" title="The screen won't sleep while cooking"><Sun size={14} aria-hidden="true" />Screen stays on</span>{/if}
      <button class="icon-btn ing-toggle" aria-expanded={sheet} onclick={() => (sheet = !sheet)} aria-label="Ingredients"><ListChecks size={26} /></button>
    </div>
  </header>

  {#if timers.list.length}
    <ul class="timers" aria-label="Timers">
      {#each timers.list as t (t.id)}
        <li class:done={t.done}>
          <span class="t-label">Step {t.step + 1} · {t.label}</span>
          <span class="t-clock" role="timer" aria-live={t.done ? 'assertive' : 'off'}>{t.done ? 'Done' : clock(remaining(t))}</span>
          {#if !t.done}
            <button class="icon-btn small" onclick={() => togglePause(t)} aria-label={t.paused ? 'Resume timer' : 'Pause timer'}>
              {#if t.paused}<Play size={20} />{:else}<Pause size={20} />{/if}
            </button>
          {/if}
          <button class="icon-btn small" onclick={() => dismiss(t)} aria-label={t.done ? 'Clear timer' : 'Cancel timer'}><X size={20} /></button>
        </li>
      {/each}
    </ul>
  {/if}

  {#if error}
    <main class="stage"><p class="step-text">{error.message}</p></main>
  {:else if recipe}
    <div class="body">
      <aside class="ingredients" class:open={sheet} aria-label="Ingredients">
        <h2 class="caps">Ingredients</h2>
        {#each groups as g}
          {#if g.section}<h3>{g.section}</h3>{/if}
          <ul>
            {#each g.rows as { row, index }}
              <li>
                <button class="ing" class:got={checked.includes(index)} aria-pressed={checked.includes(index)} onclick={() => toggleIngredient(index)}>
                  <span class="amt">{amount(row)}</span>
                  <span>{row.item}{#if row.note}<span class="note">, {row.note}</span>{/if}</span>
                </button>
              </li>
            {/each}
          </ul>
        {/each}
        <button class="btn sheet-close" onclick={() => (sheet = false)}>Back to the step</button>
      </aside>

      <main class="stage" onpointerdown={down} onpointerup={up}>
        {#key step}
          <article class="step" aria-live="polite">
            {#if current?.section}<p class="section">{current.section}</p>{/if}
            <p class="count"><span class="num">{step + 1}</span><span class="of">of {total}</span></p>
            <p class="step-text">{current?.text}</p>
            {#if durations.length}
              <div class="timer-btns">
                {#each durations as d}
                  {@const t = timers.list.find((x) => x.step === step && x.label === d.label && !x.done)}
                  {#if t}
                    <button class="btn timer-btn running" aria-pressed="true" onclick={() => togglePause(t)}>
                      {#if t.paused}<Play aria-hidden="true" />{:else}<Pause aria-hidden="true" />{/if}{d.label} · {clock(remaining(t))}{t.paused ? ' paused' : ''}
                    </button>
                  {:else}
                    <button class="btn timer-btn" onclick={() => startTimer(d.label, d.seconds, step)}><Timer aria-hidden="true" />Time {d.label}</button>
                  {/if}
                {/each}
              </div>
            {/if}
          </article>
        {/key}
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
    background: var(--paper);
    overflow: hidden;
  }
  .bar {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-4);
    padding-top: max(var(--space-2), env(safe-area-inset-top));
    border-bottom: 3px double var(--ink);
  }
  .title {
    flex: 1;
    min-width: 0;
    text-align: center;
    font-style: italic;
    font-size: 1.0625rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .bar-end {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .awake {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--ink-soft);
    font-size: 0.6875rem;
  }
  .icon-btn {
    display: grid;
    place-items: center;
    width: 56px;
    height: 56px;
    border: 0;
    background: none;
    border-radius: 2px;
  }
  .icon-btn:hover {
    background: var(--paper-press);
  }
  .icon-btn.small {
    width: 44px;
    height: 44px;
  }
  .ing-toggle {
    display: none;
  }

  .timers {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-4);
    border-bottom: 1px solid var(--rule);
  }
  .timers li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding-left: var(--space-4);
    border: 1.5px solid var(--ink);
    border-radius: 2px;
  }
  .t-label {
    font-size: 0.9375rem;
    font-style: italic;
  }
  .t-clock {
    font-family: var(--sans);
    font-size: 1.5rem;
    font-weight: 650;
    min-width: 4ch;
  }
  .timers li.done {
    background: var(--brick);
    border-color: var(--brick);
    color: var(--paper);
    animation: ring 1.2s var(--ease-out) 3;
  }
  .timers li.done .icon-btn:hover {
    background: var(--brick-press);
  }
  @keyframes ring {
    0% {
      transform: scale(1);
    }
    12% {
      transform: scale(1.04);
    }
    30% {
      transform: scale(1);
    }
  }

  .body {
    flex: 1;
    display: grid;
    grid-template-columns: minmax(18rem, 24rem) minmax(0, 1fr);
    min-height: 0;
  }
  .ingredients {
    overflow-y: auto;
    padding: var(--space-5) var(--space-5) var(--space-7);
    border-right: 1px solid var(--rule);
    scrollbar-width: thin;
  }
  .ingredients h2 {
    color: var(--ink);
    padding-bottom: var(--space-2);
    border-bottom: 1px solid var(--ink);
  }
  .ingredients h3 {
    font-style: italic;
    font-size: 1.0625rem;
    margin: var(--space-4) 0 0;
  }
  .ing {
    width: 100%;
    display: grid;
    grid-template-columns: 5em 1fr;
    gap: var(--space-3);
    text-align: left;
    border: 0;
    border-bottom: 1px solid var(--rule);
    background: none;
    padding: var(--space-3) var(--space-1);
    font-size: 1.125rem;
    line-height: 1.35;
    min-height: 52px;
  }
  .ing:hover {
    background: var(--paper-press);
  }
  .amt {
    font-family: var(--sans);
    font-size: 1rem;
    font-weight: 500;
    padding-top: 2px;
  }
  .note {
    font-style: italic;
    color: var(--ink-soft);
  }
  .ing.got {
    color: var(--ink-soft);
    text-decoration: line-through;
    text-decoration-color: var(--ink-soft);
  }
  .ing.got .note {
    color: inherit;
  }
  .sheet-close {
    display: none;
  }

  .stage {
    overflow-y: auto;
    display: grid;
    align-items: center;
    padding: var(--space-6) clamp(var(--space-5), 6vw, var(--space-8));
    touch-action: pan-y;
  }
  .step {
    max-width: 34ch;
    font-size: clamp(1.625rem, 0.9rem + 2.1vw, 2.75rem);
    animation: press 280ms var(--ease-out);
  }
  @keyframes press {
    from {
      opacity: 0;
      transform: translateY(10px);
    }
  }
  .section {
    font-style: italic;
    font-size: 0.6em;
    color: var(--ink-soft);
    margin-bottom: var(--space-2);
  }
  .count {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    font-family: var(--sans);
    margin-bottom: var(--space-4);
  }
  .num {
    font-size: 2.1em;
    font-weight: 700;
    line-height: 1;
    color: var(--brick);
  }
  .of {
    font-size: 0.5em;
    color: var(--ink-soft);
    font-weight: 500;
  }
  .step-text {
    line-height: 1.4;
  }
  .timer-btns {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-top: var(--space-6);
  }
  .timer-btn {
    min-height: 60px;
    font-size: 0.9375rem;
    padding: 0 var(--space-5);
  }
  .timer-btn.running {
    border-color: var(--brick);
    color: var(--brick);
  }
  .timer-btn :global(svg) {
    width: 22px;
    height: 22px;
  }

  .pager {
    display: grid;
    grid-template-columns: 1fr 2fr;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    padding-bottom: max(var(--space-3), env(safe-area-inset-bottom));
    border-top: 1px solid var(--rule);
  }
  .big {
    min-height: 72px;
    font-size: 1rem;
  }

  @media (max-width: 900px) {
    .body {
      grid-template-columns: 1fr;
      position: relative;
    }
    .ing-toggle {
      display: grid;
    }
    .ingredients {
      position: absolute;
      inset: 0;
      z-index: 2;
      background: var(--paper);
      border-right: 0;
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
    .stage {
      align-items: start;
    }
  }
</style>
