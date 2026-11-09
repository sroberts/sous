<script>
  import { X, Timer, Pause, Play, ListChecks, Sun, Check, ArrowLeft } from '@lucide/svelte';
  import { tick } from 'svelte';
  import { go } from '../lib/router.svelte.js';
  import { session, toast } from '../lib/session.svelte.js';
  import { getRecipe } from '../lib/library.js';
  import { amount, duration } from '../lib/format.js';
  import { timers, findDurations, startTimer, remaining, togglePause, dismiss, restore, counting, clearAll, clock } from '../lib/timers.svelte.js';
  import Markdown from '../components/Markdown.svelte';
  import { plainText } from '../lib/markdown.js';

  let { id } = $props();
  let recipe = $state(null);
  let error = $state(null);
  let awake = $state(false);
  let sheet = $state(false); // ingredients, on narrow screens
  let narrow = $state(matchMedia('(max-width: 900px)').matches);
  $effect(() => {
    const mq = matchMedia('(max-width: 900px)');
    const on = () => (narrow = mq.matches);
    mq.addEventListener('change', on);
    return () => mq.removeEventListener('change', on);
  });
  // While the sheet covers the step on a narrow screen, what's under it is
  // out of reach for keys and screen readers too, and focus moves with it.
  const covered = $derived(sheet && narrow);
  async function setSheet(open) {
    sheet = open;
    if (!narrow) return;
    await tick();
    document.querySelector(open ? '.sheet-done' : '.ing-toggle')?.focus();
  }
  let leaving = $state(false); // asking before leaving with timers running
  let said = $state(''); // the step, for screen readers

  const KEY = `sous.cook.${id}`;
  const saved = (() => {
    try {
      return JSON.parse(sessionStorage.getItem(KEY)) ?? {};
    } catch {
      return {};
    }
  })();
  let step = $state(saved.step ?? 0); // one past the last step is the end card
  let checked = $state(saved.checked ?? []);
  const started = saved.started ?? Date.now();

  $effect(() => {
    try {
      sessionStorage.setItem(KEY, JSON.stringify({ step, checked, started }));
    } catch {}
  });

  getRecipe(id).then(
    (r) => {
      recipe = r;
      step = Math.min(step, r.steps.length);
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
  const durations = $derived(current ? findDurations(plainText(current.text)) : []);
  // A duration said twice in a step ("1 minute… 1 minute more") names its
  // own clause on the button, so the two can be told apart.
  const twice = $derived(new Set(durations.map((d) => d.label).filter((l, i, all) => all.indexOf(l) !== i)));
  const last = $derived(step === total - 1);
  const end = $derived(!!recipe && step >= total);
  const queued = $derived(recipe ? recipe.steps.slice(step + 1, step + 3).map((s, k) => ({ ...s, n: step + 2 + k })) : []);
  const left = $derived(recipe ? recipe.ingredients.filter((r) => r.item).length - checked.length : 0);

  function move(by) {
    if (!recipe) return;
    const next = step + by;
    // Past the last step comes the end card, and there the pages stop: only
    // Finish leaves, so a stray swipe or key can't end the cook.
    if (next < 0 || next > total) return;
    step = next;
  }

  // One live region outside the step ticket, which is rebuilt on each step,
  // so a screen reader hears every step it moves to.
  $effect(() => {
    if (current) said = `Step ${step + 1} of ${total}. ${plainText(current.text)}`;
    else if (end) said = `All ${total} steps done. Finish leaves cook mode.`;
  });

  // Each step opens at its top, with its number in view, not wherever the
  // last step had been scrolled to.
  $effect(() => {
    step;
    document.querySelector('.stage')?.scrollTo({ top: 0 });
  });

  // Timers live only in cook mode, so leaving with one still counting asks
  // first. Finishing the recipe also forgets where the cook had got to.
  let done = false;
  async function leave({ done: finished = false } = {}) {
    done = finished;
    if (counting() && !leaving) {
      leaving = true;
      await tick();
      document.getElementById('keep-cooking')?.focus();
      return;
    }
    leaving = false;
    clearAll();
    if (session.toast?.action) session.toast = null; // a timer's Undo

    if (done) {
      try {
        sessionStorage.removeItem(KEY);
      } catch {}
    }
    go(`/r/${id}`);
  }

  async function stay() {
    leaving = false;
    await tick();
    document.querySelector('.cook .bar .icon-btn')?.focus();
  }

  // Notes at the foot of the screen sit above the pager, not over it.
  $effect(() => {
    document.documentElement.style.setProperty('--toast-lift', '96px');
    return () => document.documentElement.style.removeProperty('--toast-lift');
  });

  // A cancelled timer can be put back for a while.
  function cancel(t) {
    dismiss(t);
    if (!t.done) toast(`Timer for step ${t.step + 1} cancelled.`, { label: 'Undo', run: () => restore(t) });
  }

  // What a timer is for, in the words of the clause it ends ("Brown the
  // beef"), so a finished one says what to go and check.
  const action = (t) => t.what ?? t.label;

  // Starting a timer swaps its button for Pause; keep focus on it.
  async function time(d, key) {
    startTimer(d.label, d.seconds, step, key, d.what);
    await tick();
    document.querySelector(`.timer-btn[data-key="${key}"]`)?.focus({ preventScroll: true });
  }

  // Finished timers lead the lane, and when one finishes the strip scrolls
  // back to its start, so a ringing timer is never out of sight.
  const lane = $derived([...timers.list].sort((a, b) => b.done - a.done));
  const finished = $derived(timers.list.filter((t) => t.done).length);
  $effect(() => {
    if (finished) tick().then(() => document.querySelector('.timers')?.scrollTo({ left: 0 }));
  });

  // The ingredients come first in the page; a skip link takes the keyboard
  // straight to the step and its timers.
  function toStep(e) {
    e.preventDefault();
    const el = document.querySelector('.stage .step');
    el?.focus();
  }

  // From the first step to the end card, for the last ticket.
  const took = $derived(end ? Math.round((Date.now() - started) / 60000) : 0);

  function toggleIngredient(i) {
    checked = checked.includes(i) ? checked.filter((x) => x !== i) : [...checked, i];
  }

  function onkey(e) {
    if (e.key === 'Escape') return leaving ? stay() : sheet ? setSheet(false) : leave();
    if (leaving) return;
    if (e.target.closest?.('button, a') && (e.key === ' ' || e.key === 'Enter')) return;
    if (e.key === 'ArrowRight' || e.key === ' ' || e.key === 'PageDown') (e.preventDefault(), move(1));
    else if (e.key === 'ArrowLeft' || e.key === 'PageUp') (e.preventDefault(), move(-1));
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
    <button class="icon-btn" onclick={() => leave()} aria-label="Leave cook mode"><X size={26} /></button>
    <div class="title">
      <h1 class="name">{recipe?.title ?? (error ? 'Cook mode' : '')}</h1>
      {#if recipe}<p class="where label">{end ? 'All steps done' : `Step ${step + 1} of ${total}`}{#if awake}<span class="awake" title="The screen won’t sleep while cooking"><Sun size={13} aria-hidden="true" />Screen stays on</span>{/if}</p>{/if}
    </div>
    <button class="btn ing-toggle" aria-expanded={sheet} aria-controls="cook-ingredients" onclick={() => setSheet(!sheet)}><ListChecks aria-hidden="true" />Ingredients{#if recipe}<span class="fig left">{left}<span class="visually-hidden"> left to gather</span></span>{/if}</button>
  </header>

  <p class="visually-hidden" aria-live="polite">{said}</p>

  {#if leaving}
    {@const n = counting()}
    <div class="leaving" role="alertdialog" aria-labelledby="leaving-q">
      <p id="leaving-q">{n === 1 ? 'A timer is' : `${n} timers are`} still running. Leaving stops {n === 1 ? 'it' : 'them'}.</p>
      <button class="btn" onclick={() => leave({ done })}>Stop {n === 1 ? 'the timer' : 'timers'} and leave</button>
      <button class="btn" id="keep-cooking" onclick={stay}>Keep cooking</button>
    </div>
  {/if}

  {#if error}
    <main class="stage failed">
      <h2>This recipe didn’t open.</h2>
      <p>{error.message || 'Sous couldn’t load it.'}</p>
      <a class="btn" href="/" onclick={(e) => (e.preventDefault(), go('/'))}><ArrowLeft aria-hidden="true" />Back to the library</a>
    </main>
  {:else if recipe}
    <div class="body">
      <a class="skip-step" href="#step" onclick={toStep}>Skip to the step</a>
      <aside class="ingredients sheet" id="cook-ingredients" class:open={sheet} aria-label="Ingredients">
        <div class="ing-head"><h2 class="label">Ingredients</h2><span class="label">{checked.length} of {checked.length + left} gathered</span><button class="btn sheet-done" onclick={() => setSheet(false)}>Done</button></div>
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
        <button class="btn sheet-close" onclick={() => setSheet(false)}>Back to the step</button>
      </aside>

      <main class="stage" inert={covered} onpointerdown={down} onpointerup={up}>
        <div class="rail" aria-hidden="true"></div>
        <div class="hung" class:timed={timers.list.length > 0}>
          {#key step}
            {#if end}
              <article class="step ticket end" id="step" tabindex="-1">
                <span class="clip" aria-hidden="true"></span>
                <header class="step-head">
                  <span class="num fig" aria-hidden="true"><Check strokeWidth={3} /></span>
                  <span class="of">{total} of {total}</span>
                </header>
                <hr class="perf" />
                <h2 class="end-title">That’s {recipe.title}.</h2>
                {#if took > 0}<p class="end-took"><span class="fig">{duration(took)}</span> from the first step to the last</p>{/if}
                <p class="end-note">Time to eat.</p>
              </article>
            {:else}
            <article class="step ticket" id="step" tabindex="-1">
              <span class="clip" aria-hidden="true"></span>
              <header class="step-head">
                <span class="num fig">{step + 1}</span>
                <span class="of">of {total}</span>
                {#if current?.section}<span class="section">{current.section}</span>{/if}
              </header>
              <hr class="perf" />
              <p class="step-text" class:long={plainText(current?.text ?? '').length > 280}>{#if current}<Markdown text={current.text} links={false} />{/if}</p>
              {#if durations.length}
                <div class="timer-btns">
                  {#each durations as d, i}
                    {@const key = `${step}:${i}`}
                    {@const t = timers.list.find((x) => x.key === key && !x.done)}
                    {@const name = twice.has(d.label) ? `${d.label} · ${d.what}` : d.label}
                    {#if t}
                      <button class="btn timer-btn" data-key={key} aria-pressed="true" onclick={() => togglePause(t)}>
                        {#if t.paused}<Play aria-hidden="true" />Resume {name}{:else}<Pause aria-hidden="true" />Pause {name}{/if}
                      </button>
                    {:else}
                      <button class="btn timer-btn" data-key={key} onclick={() => time(d, key)}><Timer aria-hidden="true" />Time {name}</button>
                    {/if}
                  {/each}
                </div>
              {/if}
            </article>
            {/if}
          {/key}
          {#each queued as q (q.n)}
            <button class="next ticket" onclick={() => move(q.n - 1 - step)}>
              <span class="clip" aria-hidden="true"></span>
              <span class="next-head"><span class="fig">{q.n}</span><span class="label">Up next</span></span>
              <span class="next-text"><Markdown text={q.text} links={false} /></span>
            </button>
          {/each}
          {#if timers.list.length}
            <ul class="timers" aria-label="Timers">
              {#each lane as t (t.id)}
                <li class="tick ticket" class:done={t.done} class:paused={t.paused}>
                  <span class="clip" aria-hidden="true"></span>
                  <span class="t-label label">Step {t.step + 1} · {action(t)}</span>
                  <span class="t-row">
                    <span class="t-clock fig" role="timer" aria-live={t.done ? 'assertive' : 'off'}>{#if t.done}<span class="visually-hidden">Step {t.step + 1}, {action(t)}: </span>Done{:else}{clock(remaining(t))}{/if}</span>
                    {#if !t.done}
                      <button class="icon-btn small" onclick={() => togglePause(t)} aria-label="{t.paused ? 'Resume' : 'Pause'} the timer for step {t.step + 1}, {action(t)}">
                        {#if t.paused}<Play size={20} />{:else}<Pause size={20} />{/if}
                      </button>
                    {/if}
                    <button class="icon-btn small" onclick={() => cancel(t)} aria-label="{t.done ? 'Clear' : 'Cancel'} the timer for step {t.step + 1}, {action(t)}"><X size={20} /></button>
                  </span>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      </main>
    </div>

    <nav class="pager" aria-label="Steps" inert={covered}>
      <button class="btn big" onclick={() => move(-1)} disabled={step === 0}>Previous</button>
      {#if end}
        <button class="btn primary big" onclick={() => leave({ done: true })}>Finish</button>
      {:else}
        <button class="btn primary big" onclick={() => move(1)}>{last ? 'Done' : 'Next step'}</button>
      {/if}
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
    color: var(--ink-2); /* a count, not a live thing: no red */
  }

  .body {
    flex: 1;
    display: grid;
    grid-template-columns: minmax(18rem, 24rem) minmax(0, 1fr);
    gap: var(--space-4);
    padding: var(--space-4) var(--space-4) 0;
    min-height: 0;
    position: relative;
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
  .sheet-done {
    display: none;
  }
  /* Out of sight until the keyboard reaches it. */
  .skip-step {
    position: absolute;
    left: var(--space-4);
    top: var(--space-2);
    z-index: 5;
    padding: var(--space-2) var(--space-3);
    background: var(--ink);
    color: var(--ticket);
    font-weight: 700;
    text-decoration: none;
  }
  .skip-step:not(:focus) {
    width: 1px;
    height: 1px;
    padding: 0;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
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
  @media (hover: hover) {
    .ing:hover {
      background: var(--ticket-press);
    }
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
    /* Its own stack, so the rail can sit above the held timer strip without
       rising over the ingredients sheet beside it. */
    isolation: isolate;
    overflow-y: auto;
    padding-bottom: var(--space-5);
    touch-action: pan-y;
    scrollbar-width: thin;
  }
  .stage > .rail {
    position: sticky;
    top: 0;
    z-index: 2;
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
  /* The lane is held under the rail while a long step scrolls, so a running
     clock never leaves the screen. */
  .timers {
    flex: 0 0 13.5rem;
    display: grid;
    gap: var(--space-3);
    margin-left: auto;
    position: sticky;
    top: 6px;
    z-index: 1;
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
    gap: var(--space-3);
  }
  .t-row .icon-btn + .icon-btn {
    margin-left: auto;
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
  /* A long label ("Time 1 minute · Sprinkle in the flour") wraps inside the
     ticket rather than running past it. */
  .timer-btn {
    min-height: 60px;
    max-width: 100%;
    font-size: 1.0625rem;
    line-height: 1.25;
    white-space: normal;
    text-align: left;
    padding: var(--space-2) var(--space-5);
  }
  /* A running timer is already red on the rail; its button here stays ink,
     with only the icon in red, so the live thing is red once. */
  .timer-btn[aria-pressed='true'] {
    border-color: var(--ink);
    color: var(--ink);
  }
  .timer-btn[aria-pressed='true'] :global(svg) {
    color: var(--red);
  }
  .timer-btn :global(svg) {
    width: 22px;
    height: 22px;
  }
  /* The end card: the last ticket of the cook. The check draws itself in as
     the ticket fires, the one moment cook mode allows itself. */
  .end .step-head {
    align-items: center;
  }
  .end .num :global(svg) {
    width: 0.78em;
    height: 0.78em;
    vertical-align: -0.04em;
  }
  .end .num :global(path) {
    stroke-dasharray: 24;
    stroke-dashoffset: 24;
    animation: draw 520ms 260ms var(--ease-out) forwards;
  }
  @keyframes draw {
    to {
      stroke-dashoffset: 0;
    }
  }
  .end-title {
    margin-top: var(--space-4);
    max-width: 22ch;
    font-size: clamp(1.75rem, 1rem + 2vw, 2.75rem);
    font-weight: 800;
    line-height: 1.12;
    letter-spacing: -0.02em;
    text-wrap: balance;
  }
  .end-took {
    margin-top: var(--space-4);
    font-size: 1.125rem;
    font-weight: 600;
    color: var(--ink-2);
  }
  .end-took .fig {
    font-size: 1.75rem;
    color: var(--ink);
    margin-right: var(--space-1);
  }
  .end-note {
    margin-top: var(--space-5);
    max-width: 40ch;
    color: var(--ink-3);
  }
  .next {
    position: relative;
    flex: 0 0 15rem;
    display: grid;
    gap: var(--space-2);
    padding: var(--space-5) var(--space-4) calc(var(--space-4) + var(--tear));
    border: 0;
    text-align: left;
    color: var(--ink-2);
    transition: background-color 160ms;
  }
  .next:hover {
    background: var(--ticket-press);
  }
  .next + .next {
    flex-basis: 12rem;
  }
  .next + .next .next-head,
  .next + .next .next-text {
    color: var(--ink-3);
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

  .leaving {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    background: var(--ticket);
    border-bottom: 3px solid var(--ink);
  }
  .leaving p {
    flex: 1 1 16rem;
    font-weight: 700;
  }
  .leaving .btn {
    min-height: 52px;
  }
  .failed {
    display: grid;
    gap: var(--space-4);
    justify-items: start;
    align-content: start;
    padding: var(--space-7) var(--gutter);
  }
  .failed h2 {
    font-size: var(--title-l);
    font-weight: 800;
  }
  .failed p {
    color: var(--ink-2);
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
  /* Without room beside the step, one Up next ticket sits under it, so a
     portrait iPad still shows what's coming. */
  @media (max-width: 1180px) {
    .hung {
      flex-wrap: wrap;
    }
    .step {
      flex: 1 1 0;
    }
    /* Timers hang from the rail in a strip of their own above the step,
       never over its words; more than fit scroll sideways. */
    .timers {
      order: -1;
      flex: 1 1 100%;
      display: flex;
      gap: var(--space-3);
      margin-left: 0;
      /* Held under the rail while the step scrolls, so a clock never slides
         out of sight; the pass behind it hides the step passing beneath. */
      position: sticky;
      top: 6px;
      z-index: 1;
      padding-bottom: var(--space-3);
      margin-bottom: calc(var(--space-3) * -1);
      background: var(--pass);
      overflow-x: auto;
      scroll-snap-type: x mandatory;
      scrollbar-width: none;
    }
    .tick {
      flex: 0 0 min(13.5rem, 78%);
      scroll-snap-align: start;
    }
    .timed .step > .clip {
      display: none;
    }
    .next,
    .timed .next {
      display: grid;
      order: 2;
      flex: 1 1 100%;
      max-width: 50rem;
      padding-top: var(--space-4);
    }
    .next + .next,
    .timed .next + .next,
    .next .clip {
      display: none;
    }
    .next-text {
      -webkit-line-clamp: 2;
      line-clamp: 2;
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
    /* The sheet fills the stage between its insets and scrolls inside, with
       its head and Done held at the top. */
    .ingredients {
      position: absolute;
      inset: var(--space-3) var(--space-2) 0;
      z-index: 2;
      align-self: stretch;
      max-height: none;
      overscroll-behavior: contain;
      padding-top: 0;
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
    .ing-head {
      position: sticky;
      top: 0;
      z-index: 1;
      align-items: center;
      gap: var(--space-3);
      padding-top: var(--space-3);
      background: var(--ticket);
    }
    .ing-head span {
      margin-left: auto;
    }
    .sheet-done {
      display: inline-flex;
      min-height: 44px;
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
    .of {
      white-space: nowrap;
    }
    .tick .t-clock {
      font-size: 2.25rem;
    }
  }
  /* A short screen (an iPad in landscape, a laptop) sets the step a little smaller, so
     the step and its timer buttons fit above the pager. */
  @media (min-width: 901px) and (max-height: 920px) {
    .step {
      padding-top: var(--space-4);
      padding-bottom: calc(var(--space-5) + var(--tear));
    }
    .num {
      font-size: 3.5rem;
    }
    .step-head {
      margin-bottom: var(--space-2);
    }
    .step-text {
      margin-top: var(--space-3);
      font-size: 1.625rem;
    }
    .step-text.long {
      font-size: 1.375rem;
    }
    .timer-btns {
      margin-top: var(--space-4);
    }
  }
  /* A portrait tablet reads the step from across the counter: set it larger. */
  @media (min-width: 600px) and (max-width: 1180px) and (orientation: portrait) {
    .step-text {
      font-size: 2.125rem;
      max-width: 34ch;
    }
    .step-text.long {
      font-size: 1.625rem;
      max-width: 42ch;
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
