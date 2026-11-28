<script>
  // One label to confirm or correct, as answer buttons; one press (or its
  // number key) picks, pressing it again clears it, and focus moves on to
  // the next question. When Sous is fairly sure (60% or more) its answer
  // leads, dashed as its guess, with how likely it thought each; when it
  // isn't, the answers come in their natural order with no guess, so a
  // near coin-toss doesn't steer the cook. Something else opens a field for
  // any other answer.
  import { ASK, show, pct, same, choicesFor, HOLD_IN } from '../lib/graphs.js';

  let { item, chosen = undefined, onchoose, kitchen = null } = $props();

  const confident = $derived(item.confidence >= 0.6);
  const options = $derived.by(() => {
    const out = [{ value: item.value, p: item.confidence }];
    // Answers it gave less than a 5% chance are noise; Something else
    // still reaches them.
    for (const a of item.alternatives ?? []) if (a.probability >= 0.05 && !out.some((o) => same(o.value, a.value))) out.push({ value: a.value, p: a.probability });
    // "Use at once" is the commonest answer to whether something can wait,
    // so it's always offered, even when the model didn't weigh it.
    if (item.field === 'hold' && !out.some((o) => o.value === null)) out.push({ value: null, p: null });
    // What an action uses is nearly always nothing, a burner or the oven:
    // those three are always there, so the right one is never hidden.
    if (item.field === 'equipment') {
      for (const r of ['none', 'burner', 'oven']) {
        const v = r === 'none' ? [] : [{ resource: r, count: 1, scope: 'whole_node' }];
        if (!out.some((o) => (o.value?.[0]?.resource ?? 'none') === r)) out.push({ value: v, p: null });
      }
    }
    if (!confident) out.sort((a, b) => rank(a.value) - rank(b.value));
    return out;
  });

  // An answer's place in a field's natural order, for when there's no
  // guess to lead with.
  function rank(v) {
    const f = item.field;
    const fixed = choicesFor(f);
    if (fixed) return fixed.findIndex(([k]) => k === v);
    if (f === 'equipment') {
      const r = v?.[0]?.resource ?? 'none';
      return ['none', 'burner', 'oven', 'fridge', 'counter'].indexOf(r) + 1 || 9;
    }
    if (f === 'hold') return v ? v.max_min : 0;
    if (f === 'duration') return v.typical;
    return 0;
  }

  // Picking: again clears it; otherwise on to the next question.
  function pick(v, e) {
    if (chosen !== undefined && same(chosen, v)) {
      onchoose(undefined);
      return;
    }
    onchoose(v);
    const all = [...document.querySelectorAll('.choice')];
    const next = all[all.indexOf(e.currentTarget.closest('.choice')) + 1];
    // To the middle of the screen, clear of the sticky bar, then focus.
    next?.scrollIntoView({ block: 'center', behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
    next?.querySelector('.opt')?.focus({ preventScroll: true });
  }
  const other = $derived(chosen !== undefined && !options.some((o) => same(o.value, chosen)));
  let open = $state(false);

  // The editor's draft for fields without a fixed set, seeded from the
  // current value each time it opens.
  let dMin = $state(0);
  let dMax = $state(0);
  let hMin = $state(60);
  let hWhere = $state('fridge');
  let eRes = $state('none');
  let eTemp = $state(350);
  let check = $state(10);
  function toggle() {
    if (!open) {
      const v = item.value;
      if (item.field === 'duration') [dMin, dMax] = [v.min, v.max];
      if (item.field === 'hold' && v) [hMin, hWhere] = [v.max_min, v.where];
      if (item.field === 'equipment') [eRes, eTemp] = [v[0]?.resource ?? 'none', v[0]?.temp_f ?? 350];
      if (item.field === 'check_interval_min') check = v;
    }
    open = !open;
  }

  const resources = $derived([
    ['none', 'Nothing'],
    ['oven', 'Oven'],
    ['burner', 'A burner'],
    ['fridge', 'Fridge'],
    ['counter', 'Counter space'],
    ...(kitchen?.appliances ?? []).map((a) => [`appliance:${a.name}`, a.name]),
    ...(kitchen?.vessels ?? []).map((v) => [`vessel:${v.name}`, v.name]),
  ]);

  function useOther() {
    const f = item.field;
    const whole = (v, lo) => Math.max(lo, Math.round(+v || 0));
    if (f === 'duration') {
      const lo = whole(dMin, 0);
      const hi = Math.max(lo, whole(dMax, 0));
      onchoose({ min: lo, typical: hi, max: hi });
    } else if (f === 'hold') onchoose(+hMin > 0 ? { max_min: whole(hMin, 1), where: hWhere } : null);
    else if (f === 'equipment') onchoose(eRes === 'none' ? [] : [{ resource: eRes, count: 1, scope: 'whole_node', ...(eRes === 'oven' ? { temp_f: whole(eTemp, 150) } : {}) }]);
    else if (f === 'check_interval_min') onchoose(whole(check, 1));
    open = false;
  }
  // 1, 2, 3… pick the answers in order while focus is in the question.
  function pickKey(e) {
    if (e.metaKey || e.ctrlKey || e.altKey || e.target.matches('input, select')) return;
    const n = +e.key;
    if (n >= 1 && n <= options.length) {
      e.preventDefault();
      const btn = e.currentTarget.querySelectorAll('.opt')[n - 1];
      pick(options[n - 1].value, { currentTarget: btn ?? e.currentTarget });
    }
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<fieldset class="choice" onkeydown={pickKey}>
  <legend>{ASK[item.field] ?? item.field}</legend>
  <div class="opts">
    {#each options as o, i (i)}
      <!-- Sous's answer as it stands is always marked until something is
           picked; it leads only when Sous is fairly sure. -->
      {@const guess = chosen === undefined && same(o.value, item.value)}
      <button type="button" class="opt" class:guess aria-pressed={chosen !== undefined && same(chosen, o.value)} onclick={(e) => pick(o.value, e)} aria-keyshortcuts={String(i + 1)}
        title={o.p !== null && !confident ? `Sous thought this ${pct(o.p)} likely` : undefined}>
        <span class="v">{show(item.field, o.value)}</span>{#if guess}<span class="visually-hidden">, Sous’s guess</span>{/if}{#if o.p !== null && confident}<span class="p">{pct(o.p)}</span>{/if}
      </button>
    {/each}
    {#if other}
      <button type="button" class="opt" aria-pressed="true" onclick={() => open || toggle()}><span class="v">{show(item.field, chosen)}</span></button>
    {/if}
    <button type="button" class="else" aria-expanded={open} onclick={toggle}>Something else</button>
  </div>
  {#if open}
    <div class="editor">
      {#if choicesFor(item.field)}
        <div class="opts">
          {#each choicesFor(item.field) as [v, words] (v)}
            {#if !options.some((o) => o.value === v)}
              <button type="button" class="opt" aria-pressed={chosen === v} onclick={() => (onchoose(v), (open = false))}><span class="v">{words}</span></button>
            {/if}
          {/each}
        </div>
      {:else if item.field === 'duration'}
        <label class="num"><input type="number" min="0" bind:value={dMin} aria-label="At least, in minutes" /><span>to</span></label>
        <label class="num"><input type="number" min="0" bind:value={dMax} aria-label="At most, in minutes" /><span>min</span></label>
        <button type="button" class="btn" onclick={useOther}>Use this</button>
      {:else if item.field === 'hold'}
        <label class="num"><input type="number" min="0" bind:value={hMin} aria-label="Keeps for, in minutes; 0 for use at once" /><span>min</span></label>
        <select bind:value={hWhere} aria-label="Where it waits">
          {#each Object.entries(HOLD_IN) as [v, words] (v)}<option value={v}>{words}</option>{/each}
        </select>
        <button type="button" class="btn" onclick={useOther}>Use this</button>
      {:else if item.field === 'equipment'}
        <select bind:value={eRes} aria-label="What it uses">
          {#each resources as [v, words] (v)}<option value={v}>{words}</option>{/each}
        </select>
        {#if eRes === 'oven'}<label class="num"><input type="number" min="150" max="900" step="5" bind:value={eTemp} aria-label="Oven temperature, °F" /><span>°F</span></label>{/if}
        <button type="button" class="btn" onclick={useOther}>Use this</button>
      {:else if item.field === 'check_interval_min'}
        <label class="num"><span>Every</span><input type="number" min="1" bind:value={check} aria-label="Minutes between checks" /><span>min</span></label>
        <button type="button" class="btn" onclick={useOther}>Use this</button>
      {/if}
    </div>
  {/if}
</fieldset>

<style>
  .choice {
    border: 0;
    padding: 0;
    margin: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--space-1) var(--space-3);
    align-items: baseline;
  }
  legend {
    float: left;
    font-weight: 700;
    font-size: var(--ui);
    padding: 0;
  }
  .opts,
  .editor {
    grid-column: 1 / -1;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
  .editor {
    padding-top: var(--space-2);
    border-top: 1.5px dashed var(--perf);
  }
  .opt {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 44px;
    padding: 0 var(--space-3);
    border: 2px solid var(--ink);
    border-radius: 3px;
    background: var(--ticket);
    color: var(--ink);
    font: inherit;
    font-weight: 700;
    text-align: left;
    cursor: pointer;
  }
  .opt:hover {
    background: var(--ticket-press);
  }
  /* Focus is an ink ring set off the button; red means chosen. */
  .opt:focus-visible {
    outline: 2px solid var(--ink);
    outline-offset: 3px;
  }
  /* Sous's guess, until something is picked: a dashed stroke, not red. */
  .opt.guess {
    border-style: dashed;
  }
  .opt[aria-pressed='true'] {
    border-color: var(--red);
    color: var(--red);
  }
  .p {
    font-weight: 400;
    color: var(--ink-3);
    font-size: 0.875rem;
    font-variant-numeric: tabular-nums;
  }
  .opt[aria-pressed='true'] .p {
    color: var(--red);
  }
  .else {
    min-height: 44px;
    padding: 0;
    border: 0;
    background: none;
    color: var(--ink-2);
    font: inherit;
    text-decoration: underline;
    text-underline-offset: 3px;
    cursor: pointer;
  }
  .num {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    color: var(--ink-2);
  }
  .num input {
    width: 5rem;
    min-height: 44px;
    font: inherit;
    font-size: max(16px, var(--read));
    font-variant-numeric: tabular-nums;
    border: 0;
    border-bottom: 1.5px solid var(--field);
    background: transparent;
  }
  .num input:focus,
  select:focus {
    outline: none;
    border-bottom-color: var(--red);
  }
  select {
    min-height: 44px;
    font: inherit;
    font-size: max(16px, var(--read));
    border: 0;
    border-bottom: 1.5px solid var(--field);
    background: transparent;
  }
</style>
