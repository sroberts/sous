<script>
  // One action, reviewed as a whole: its labels read as a sentence, the
  // parts Sous isn't sure of marked "?"; Looks right confirms the lot, Fix
  // opens every label as a question (the unsure ones first). An action Sous
  // was sure of is the sentence and a quiet Fix. Answers live in the parent
  // (chosen, keyed "<node>|<field>") until it saves them.
  import { Check } from '@lucide/svelte';
  import { show } from '../lib/graphs.js';
  import LabelChoice from './LabelChoice.svelte';

  let { node, flagged = [], chosen = {}, onchoose, onlooksright, kitchen = null, quote = false, open = $bindable(false), disabled = false } = $props();

  const key = (field) => `${node.id}|${field}`;
  const FIELD = { action: 'action', duration: 'duration', attention: 'attention', equipment: 'equipment', hold: 'hold', check_interval_min: 'check_interval_min' };
  const flaggedField = $derived(new Set(flagged.map((it) => it.field)));
  const needs = $derived(flagged.length > 0);
  const checked = $derived(needs && flagged.every((it) => key(it.field) in chosen));
  const valueOf = (field) => (key(field) in chosen ? chosen[key(field)] : node[FIELD[field]]?.value ?? (field === 'equipment' ? [] : null));

  // The sentence: what it is, how long, how hands-on, what it uses (when
  // anything), whether it waits (when it does or Sous asks).
  const parts = $derived(
    ['action', 'duration', 'attention', 'equipment', 'hold']
      .filter((f) => {
        const v = valueOf(f);
        if (f === 'equipment') return flaggedField.has(f) || v?.length;
        if (f === 'hold') return flaggedField.has(f) || v;
        return true;
      })
      .map((f) => ({ field: f, text: show(f, valueOf(f)), unsure: flaggedField.has(f) && !(key(f) in chosen) })),
  );

  // Every label as a question: the unsure ones first, as asked; the rest
  // at Sous's own confidence.
  const questions = $derived.by(() => {
    const out = [...flagged];
    for (const f of ['action', 'duration', 'attention', 'equipment', 'hold']) {
      if (flaggedField.has(f)) continue;
      const l = node[FIELD[f]];
      out.push({
        node_id: node.id, field: f, implied: node.implied,
        value: f === 'hold' ? (l?.value ?? null) : f === 'equipment' ? (l?.value ?? []) : l.value,
        confidence: l?.confidence ?? 1, alternatives: l?.alternatives ?? [],
      });
    }
    return out;
  });
</script>

<div class="action" class:needs class:checked class:implied={node.implied} data-node={node.id}>
  <p class="t">
    {node.text}{#if node.implied}<span class="added">Added by Sous</span>{/if}
    {#if checked}<span class="ok"><Check size={16} aria-hidden="true" />Checked</span>{/if}
  </p>
  {#if quote && node.source.quote}<p class="q">“{node.source.quote}”</p>{/if}
  <p class="sentence">
    {#each parts as p, i (p.field)}{#if i}<span aria-hidden="true">{' · '}</span>{/if}<span class:unsure={p.unsure}>{p.text}{#if p.unsure}<span aria-hidden="true">?</span><span class="visually-hidden"> (Sous isn’t sure)</span>{/if}</span>{/each}
  </p>
  {#if !open}
    <div class="acts">
      {#if needs && !checked}<button type="button" class="btn looks" onclick={() => onlooksright(node)} {disabled}>Looks right</button>{/if}
      <button type="button" class="fix" aria-expanded="false" onclick={() => (open = true)}>Fix</button>
    </div>
  {:else}
    <div class="questions">
      {#each questions as it (it.field)}
        <LabelChoice item={it} chosen={chosen[key(it.field)]} onchoose={(v) => onchoose(it, v)} {kitchen} />
      {/each}
      <button type="button" class="fix" aria-expanded="true" onclick={() => (open = false)}>Done</button>
    </div>
  {/if}
</div>

<style>
  .action {
    display: grid;
    gap: var(--space-2);
  }
  .t {
    font-weight: 700;
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-2);
  }
  .implied .t {
    color: var(--ink-2);
  }
  .added {
    font-weight: 400;
    font-size: var(--ui);
    color: var(--ink-3);
  }
  .ok {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-weight: 600;
    font-size: var(--ui);
    color: var(--ink-2);
  }
  .q {
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .sentence {
    color: var(--ink-2);
  }
  /* What Sous isn't sure of: ink and a dashed underline, read first. */
  .unsure {
    color: var(--ink);
    font-weight: 600;
    text-decoration: underline dashed var(--ink-3);
    text-underline-offset: 4px;
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3);
  }
  .looks {
    min-height: 44px;
  }
  .fix {
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
  .questions {
    display: grid;
    gap: var(--space-3);
    padding-top: var(--space-2);
    border-top: 1.5px dashed var(--perf);
  }
  .questions .fix {
    justify-self: start;
  }
</style>
