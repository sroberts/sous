<script>
  import { link } from '../lib/router.svelte.js';
  import { api } from '../lib/api.js';
  import { amount, duration } from '../lib/format.js';
  import Plate from './Plate.svelte';

  let { summary, using = [] } = $props();

  // Full records are fetched as rows are selected, and remembered.
  const cache = (globalThis.__sousPreview ??= new Map());
  let recipe = $state(cache.get(summary.id) ?? null);
  $effect(() => {
    if (recipe && recipe.version === summary.version) return;
    let live = true;
    api.recipe(summary.id).then((r) => {
      cache.set(r.id, r);
      if (live) recipe = r;
    });
    return () => (live = false);
  });

  const LIMIT = 12;
  const onHand = (i) => using.some((t) => i.item.toLowerCase().includes(t));
</script>

<article class="pv">
  <Plate hash={summary.cover} title={summary.title} />
  <div class="body">
    <h2>{summary.title}</h2>
    {#if summary.source_host}<p class="src">{summary.source_host}</p>{/if}
    {#if recipe}
      <dl class="facts">
        {#if recipe.yield_text}<div><dt class="caps">Yield</dt><dd>{recipe.yield_text.replace(/^yield:?\s*/i, '')}</dd></div>{/if}
        {#if recipe.prep_minutes}<div><dt class="caps">Prep</dt><dd>{duration(recipe.prep_minutes)}</dd></div>{/if}
        {#if recipe.cook_minutes}<div><dt class="caps">Cook</dt><dd>{duration(recipe.cook_minutes)}</dd></div>{/if}
        {#if !recipe.prep_minutes && !recipe.cook_minutes && (recipe.total_minutes || recipe.time_text)}
          <div><dt class="caps">Time</dt><dd>{duration(recipe.total_minutes) || recipe.time_text}</dd></div>
        {/if}
      </dl>
    {/if}
    <div class="acts">
      <a class="btn primary" href="/r/{summary.id}/cook" use:link>Begin cooking</a>
      <a class="btn" href="/r/{summary.id}" use:link>Open</a>
      <a class="btn quiet" href="/r/{summary.id}/edit" use:link>Edit</a>
    </div>
    {#if recipe}
      <h3 class="caps">Ingredients</h3>
      <ul class="ing">
        {#each recipe.ingredients.slice(0, LIMIT) as i}
          <li class:have={onHand(i)}><span class="amt">{amount(i)}</span><span>{i.item}</span></li>
        {/each}
      </ul>
      {#if recipe.ingredients.length > LIMIT}
        <p class="more">and {recipe.ingredients.length - LIMIT} more</p>
      {/if}
    {/if}
  </div>
</article>

<style>
  .pv {
    padding: var(--space-6) var(--space-6) var(--space-7);
  }
  .body {
    margin-top: var(--space-5);
  }
  h2 {
    font-size: var(--title-l);
  }
  .src {
    margin-top: var(--space-1);
    font-style: italic;
    color: var(--ink-soft);
    font-size: 1rem;
  }
  .facts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-5);
    margin: var(--space-4) 0 0;
  }
  .facts dt {
    color: var(--ink-soft);
    font-size: 0.6875rem;
  }
  .facts dd {
    margin: 0;
    font-family: var(--sans);
    font-size: 0.9375rem;
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin: var(--space-5) 0 var(--space-6);
  }
  h3 {
    color: var(--ink);
    padding-bottom: var(--space-2);
    border-bottom: 1px solid var(--ink);
  }
  .ing li {
    display: grid;
    grid-template-columns: 5.5em 1fr;
    gap: var(--space-3);
    padding: 5px 0;
    border-bottom: 1px solid var(--rule);
    font-size: 1rem;
    line-height: 1.4;
  }
  .amt {
    font-family: var(--sans);
    font-size: 0.875rem;
    padding-top: 2px;
    color: var(--ink-soft);
  }
  .have .amt,
  .have span:last-child {
    color: var(--brick);
  }
  .more {
    margin-top: var(--space-2);
    font-style: italic;
    color: var(--ink-soft);
    font-size: 0.9375rem;
  }
</style>
