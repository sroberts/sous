<script>
  import { untrack } from 'svelte';
  import { link } from '../lib/router.svelte.js';
  import { getRecipe } from '../lib/library.js';
  import { amount } from '../lib/format.js';
  import { NOT_ON_DEVICE } from '../lib/net.svelte.js';
  import Facts from './Facts.svelte';
  import Plate from './Plate.svelte';

  let { summary, using = [] } = $props();

  // Full records are fetched as rows are selected, and remembered. The
  // library keys this on the id, so it fetches again only when the summary's
  // version moves; what comes back may be older (a copy on this device, or a
  // stale index) and is shown as it is.
  const cache = (globalThis.__sousPreview ??= new Map());
  let recipe = $state(cache.get(summary.id) ?? null);
  let missing = $state(false); // offline, and not stored on this device yet
  $effect(() => {
    const version = summary.version;
    if (untrack(() => recipe)?.version === version) return;
    let live = true;
    getRecipe(summary.id).then(
      (r) => {
        cache.set(r.id, r);
        if (live) recipe = r;
      },
      (err) => live && (missing = err.code === 'not_on_device'),
    );
    return () => (live = false);
  });

  const LIMIT = 12;
  const onHand = (i) => using.some((t) => i.item.toLowerCase().includes(t));
</script>

<article class="pv">
  <Plate hash={summary.cover} title={summary.title} ratio="16 / 10" />
  <div class="body">
    <h2>{summary.title}</h2>
    {#if summary.source_host}<p class="src">{summary.source_host}</p>{/if}
    <div class="facts">
      {#if recipe}<Facts {recipe} />{:else}<div class="ghost"></div>{/if}
    </div>
    {#if missing && !recipe}<p class="missing">{NOT_ON_DEVICE}</p>{/if}
    <div class="acts">
      <a class="btn primary" href="/r/{summary.id}/cook" use:link>Begin cooking</a>
      <a class="btn" href="/r/{summary.id}" use:link>Open</a>
      <a class="btn quiet" href="/r/{summary.id}/edit" use:link>Edit</a>
    </div>
    {#if recipe}
      <h3 class="label">Ingredients</h3>
      <ul class="ing">
        {#each recipe.ingredients.filter((i) => i.item).slice(0, LIMIT) as i}
          <li class:have={onHand(i)}><span class="amt fig">{amount(i)}</span><span>{i.item}</span></li>
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
    padding: var(--space-5) var(--space-5) var(--space-7);
  }
  .body {
    margin-top: var(--space-4);
  }
  h2 {
    font-size: var(--title-l);
    font-weight: 800;
    line-height: 1.1;
  }
  .src {
    margin-top: var(--space-1);
    color: var(--ink-3);
    font-size: 0.875rem;
  }
  .facts {
    margin-top: var(--space-4);
  }
  .ghost {
    height: 76px;
    border-top: 3px solid var(--ink);
  }
  .missing {
    margin-top: var(--space-3);
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin: var(--space-4) 0 var(--space-6);
  }
  .acts .primary {
    flex: 1;
    min-height: 48px;
  }
  h3 {
    color: var(--ink-3);
    padding-bottom: var(--space-2);
  }
  .ing li {
    display: grid;
    grid-template-columns: 5rem 1fr;
    gap: var(--space-3);
    padding: 6px 0;
    border-top: 1px solid var(--ticket-press);
    font-size: 0.9375rem;
    line-height: 1.35;
  }
  .amt {
    font-size: 1.0625rem;
    color: var(--ink-2);
  }
  .have,
  .have .amt {
    color: var(--red);
    font-weight: 700;
  }
  .more {
    margin-top: var(--space-2);
    color: var(--ink-3);
    font-size: 0.875rem;
  }
</style>
