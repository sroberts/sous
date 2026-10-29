<script>
  // A recipe's figures in fixed columns: the times, the yield and the
  // ingredient count, read across the top of a ticket.
  import { ticketTime } from '../lib/format.js';

  let { recipe, size = 'm' } = $props();

  const cells = $derived.by(() => {
    const r = recipe;
    const out = [];
    const total = r.total_minutes ?? (r.prep_minutes || r.cook_minutes ? (r.prep_minutes ?? 0) + (r.cook_minutes ?? 0) : null);
    const t = ticketTime(total, r.time_text);
    if (t) out.push({ label: 'Total', ...t });
    for (const [label, min] of [['Prep', r.prep_minutes], ['Cook', r.cook_minutes]]) {
      if (min && min !== total) out.push({ label, ...ticketTime(min) });
    }
    const yieldText = r.yield_text?.replace(/^(yield|serves|makes):?\s*/i, '');
    if (r.servings && yieldText && /^\d+\s*(servings?|people)?$/i.test(yieldText)) out.push({ label: 'Serves', fig: String(r.servings) });
    else if (yieldText) out.push({ label: 'Yield', text: yieldText });
    if (r.ingredients?.length) out.push({ label: 'Ingredients', fig: String(r.ingredients.filter((i) => i.item).length) });
    return out;
  });
</script>

{#if cells.length}
  <dl class="facts {size}">
    {#each cells as c}
      <div class:wordy={!c.fig}>
        <dt class="label">{c.label}</dt>
        <dd>
          {#if c.fig}<span class="fig">{c.fig}</span>{#if c.unit}<span class="unit">{c.unit}</span>{/if}{:else}{c.text}{/if}
        </dd>
      </div>
    {/each}
  </dl>
{/if}

<style>
  .facts {
    display: flex;
    flex-wrap: wrap;
    margin: 0;
    border-top: 3px solid var(--ink);
    border-bottom: 1.5px dashed var(--perf);
  }
  .facts > div {
    display: flex;
    flex-direction: column;
    flex: 0 1 auto;
    min-width: 4.5rem;
    padding: var(--space-2) var(--space-5) var(--space-3) 0;
  }
  .facts > div.wordy {
    max-width: 14rem;
  }
  dt {
    color: var(--ink-3);
    margin-bottom: var(--space-1);
  }
  dd {
    margin: auto 0 0;
    font-weight: 600;
    line-height: 1.25;
  }
  .fig {
    font-size: 2.25rem;
  }
  .l .fig {
    font-size: 3rem;
  }
  .unit {
    margin-left: 3px;
    font-size: 0.8125rem;
    font-weight: 700;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }
</style>
