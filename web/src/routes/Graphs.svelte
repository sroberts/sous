<script>
  import { onDestroy } from 'svelte';
  import { ArrowLeft, ChevronRight } from '@lucide/svelte';
  import { api } from '../lib/api.js';
  import { link } from '../lib/router.svelte.js';
  import { asPerson } from '../lib/session.svelte.js';
  import { isOffline } from '../lib/net.svelte.js';
  import { STATUS } from '../lib/graphs.js';
  import ActionReview from '../components/ActionReview.svelte';

  let overview = $state(null);
  let queue = $state([]);
  let kitchen = $state(null);
  let problem = $state('');
  let chosen = $state({}); // recipe → { "node|field": value }
  let graphs = $state({}); // recipe → its graph, for the actions' labels
  let busy = $state(null); // the recipe being saved
  let job = $state(null);
  let gone = false; // the page has been left: stop following the job
  onDestroy(() => (gone = true));

  async function load() {
    try {
      const [o, k] = await Promise.all([api.graphs(), api.kitchen()]);
      // Every label to check, a page at a time: a household's queue is
      // small, and counting actions needs all of it.
      const all = [];
      for (let cursor = ''; ; ) {
        const page = await api.reviewItems(cursor ? { limit: 100, cursor } : { limit: 100 });
        all.push(...page.items);
        if (!page.next_cursor) break;
        cursor = page.next_cursor;
      }
      overview = o;
      queue = all;
      kitchen = k;
      // The graph of each recipe in the queue, for its actions' sentences.
      const ids = [...new Set(all.map((it) => it.recipe_id))];
      const got = await Promise.all(ids.map((rid) => api.recipeGraph(rid).catch(() => null)));
      graphs = Object.fromEntries(ids.map((rid, i) => [rid, got[i]?.graph]));
      problem = '';
    } catch (err) {
      problem = err.code === 'offline' ? 'Step graphs need a connection to Sous.' : err.message;
    }
  }
  load();

  // The queue grouped by recipe, in the queue's order: what matters most
  // first.
  const groups = $derived.by(() => {
    const out = [];
    for (const it of queue) {
      let g = out.find((x) => x.id === it.recipe_id);
      if (!g) out.push((g = { id: it.recipe_id, title: it.recipe_title, version: it.graph_version, items: [] }));
      g.items.push(it);
    }
    return out;
  });
  // The unit of review is the action: a recipe's actions with anything
  // Sous isn't sure of, in the queue's order.
  const SHOWN = 3;
  const flaggedIn = (g, nodeID) => g.items.filter((it) => it.node_id === nodeID);
  const actionsOf = (g) => [...new Set(g.items.map((it) => it.node_id))];
  const nodeOf = (g, nodeID) => graphs[g.id]?.nodes.find((n) => n.id === nodeID);
  const totalActions = $derived(groups.reduce((n, g) => n + actionsOf(g).length, 0));
  const answeredIn = (g) => Object.keys(chosen[g.id] ?? {}).length;
  const answered = $derived(groups.reduce((n, g) => n + answeredIn(g), 0));

  function choose(g, it, v) {
    const k = `${it.node_id}|${it.field}`;
    const { [k]: _, ...rest } = chosen[g.id] ?? {};
    chosen = { ...chosen, [g.id]: v === undefined ? rest : { ...rest, [k]: v } };
  }
  const FIELDS = { action: 'action', duration: 'duration', attention: 'attention', equipment: 'equipment', hold: 'hold', check_interval_min: 'check_interval_min' };
  function looksRight(g, n) {
    const mine = { ...(chosen[g.id] ?? {}) };
    for (const [field, prop] of Object.entries(FIELDS)) {
      const k = `${n.id}|${field}`;
      if (!(k in mine) && n[prop]) mine[k] = n[prop].value;
    }
    chosen = { ...chosen, [g.id]: mine };
  }

  // One save for every recipe checked here, recipe by recipe.
  async function saveAll() {
    busy = 'all';
    problem = '';
    try {
      for (const g of groups) {
        const items = Object.entries(chosen[g.id] ?? {}).map(([k, value]) => {
          const [node_id, field] = k.split('|');
          return { node_id, field, value };
        });
        if (!items.length) continue;
        try {
          const res = await asPerson(() => api.resolveReview({ recipe_id: g.id, graph_version: g.version, items }));
          if (!res) return;
        } catch (err) {
          problem = err.status === 412 ? `Someone changed ${g.title}’s graph meanwhile; check it again.` : err.message;
        }
      }
      chosen = {};
      await load();
    } finally {
      busy = null;
    }
  }

  async function upgradeAll() {
    try {
      job = await asPerson(() => api.queueUpgrades({ mode: 'apply' }));
      if (job) follow();
    } catch (err) {
      problem = err.message;
    }
  }
  async function follow() {
    while (!gone && job && (job.status === 'queued' || job.status === 'running')) {
      await new Promise((r) => setTimeout(r, 5000));
      try {
        job = await api.upgradeJob(job.id);
      } catch {
        return;
      }
    }
    if (!gone) load();
  }
  const done = $derived(job ? job.items.length - (job.counts.pending ?? 0) - (job.counts.running ?? 0) : 0);

  const order = ['needs_review', 'draft', 'stale', 'verified'];
  const others = $derived(overview ? overview.items.filter((r) => !groups.some((g) => g.id === r.recipe_id)).sort((a, b) => order.indexOf(a.status) - order.indexOf(b.status)) : []);
</script>

<main class="graphs" id="main">
  <a class="back" href="/meals" use:link><ArrowLeft size={16} aria-hidden="true" />Meals</a>
  <h1>Step graphs</h1>
  <p class="lede">A step graph splits each step into single actions joined by what they make and use, so meal plans can fit dishes around each other. Sous labels each action; you check the ones it isn’t sure of.</p>

  {#if problem}<p class="problem" role="alert">{problem}</p>{/if}

  {#if !overview && !problem}
    <p class="hint" role="status">Loading step graphs…</p>
  {:else if overview}
    <dl class="counts">
      <div><dt class="label">To check</dt><dd><span class="fig">{groups.length}</span><span class="of">{totalActions} {totalActions === 1 ? 'action' : 'actions'}</span></dd></div>
      <div><dt class="label">Verified</dt><dd><span class="fig">{overview.counts.verified}</span></dd></div>
      {#if overview.counts.stale}<div><dt class="label">Edited since</dt><dd><span class="fig">{overview.counts.stale}</span></dd></div>{/if}
      <div><dt class="label">No graph yet</dt><dd><span class="fig">{overview.counts.none}</span></dd></div>
    </dl>

    {#if overview.counts.none}
      <section class="panel todo" aria-labelledby="todo-h">
        <h2 id="todo-h" class="label">The rest of the library</h2>
        {#if !overview.upgrades.labels}
          <p>Step graphs need Sous’s decision model, which isn’t set up on this Sous.</p>
        {:else if overview.upgrades.graph_model}
          {#if job}
            <p role="status">{job.status === 'done' ? 'Done' : 'Drafting'}: {done} of {job.items.length}{#if job.counts.needs_review} · {job.counts.needs_review} to review{/if}{#if job.counts.failed} · {job.counts.failed} failed{/if}</p>
          {:else}
            <p>Sous can draft the {overview.counts.none === 1 ? 'recipe' : `${overview.counts.none} recipes`} without a graph itself, one at a time in the background.</p>
            <button class="btn" onclick={upgradeAll} disabled={isOffline()}>Draft {overview.counts.none === 1 ? 'it' : `all ${overview.counts.none}`}</button>
          {/if}
        {:else}
          <p>An assistant drafts each graph and Sous labels it. <a href="/assistants#graphs" use:link>Set that up</a>, then ask it to draft step graphs, for the whole library or one meal’s dishes.</p>
        {/if}
      </section>
    {/if}

    {#if groups.length}
      <section aria-labelledby="q-h">
        <h2 id="q-h" class="label sect">To check · {groups.length} {groups.length === 1 ? 'recipe' : 'recipes'}, {totalActions} {totalActions === 1 ? 'action' : 'actions'}</h2>
        <ol class="queue">
          {#each groups as g (g.id)}
            <li class="group">
              <div class="g-head">
                <a href="/r/{g.id}/graph" use:link><span class="t">{g.title}</span><span class="sub">{actionsOf(g).length} to check</span><ChevronRight size={18} aria-hidden="true" /></a>
              </div>
              <ul class="items">
                {#each actionsOf(g).slice(0, SHOWN) as nodeID (nodeID)}
                  {@const n = nodeOf(g, nodeID)}
                  {#if n}
                    <li>
                      <ActionReview node={n} flagged={flaggedIn(g, nodeID)} chosen={chosen[g.id] ?? {}} onchoose={(it, v) => choose(g, it, v)}
                        onlooksright={(node) => looksRight(g, node)} {kitchen} quote />
                    </li>
                  {/if}
                {/each}
              </ul>
              {#if actionsOf(g).length > SHOWN}
                <div class="g-foot">
                  <a class="all" href="/r/{g.id}/graph" use:link>All {actionsOf(g).length} actions, beside its steps<ChevronRight size={16} aria-hidden="true" /></a>
                </div>
              {/if}
            </li>
          {/each}
        </ol>
      </section>
    {:else if overview.items.length}
      <p class="empty">Nothing to check. Sous was sure of every action, or you’ve checked them all.</p>
    {/if}

    {#if others.length}
      <section aria-labelledby="all-h">
        <h2 id="all-h" class="label sect">Recipes with a graph</h2>
        <ul class="list">
          {#each others as r (r.recipe_id)}
            <li><a href="/r/{r.recipe_id}/graph" use:link><span class="t">{r.title}</span><span class="badge" data-status={r.status}>{STATUS[r.status]}</span></a></li>
          {/each}
        </ul>
      </section>
    {/if}
  {/if}
  {#if answered}
    <div class="save-bar">
      <p>Not yet saved</p>
      <button class="btn primary" onclick={saveAll} disabled={busy !== null || isOffline()}>Save</button>
    </div>
  {/if}
</main>

<style>
  .graphs {
    max-width: 56rem;
    margin: 0 auto;
    padding: var(--space-6) var(--gutter) var(--space-8);
  }
  h1 {
    font-size: var(--title-l);
    font-weight: 800;
  }
  .lede {
    margin-top: var(--space-2);
    color: var(--ink-2);
    max-width: 62ch;
  }
  .problem {
    margin-top: var(--space-4);
    font-weight: 700;
  }
  .hint,
  .empty {
    margin-top: var(--space-4);
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .counts {
    margin: var(--space-5) 0 0;
    padding-top: var(--space-3);
    border-top: 3px solid var(--ink);
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--space-4);
  }
  .counts div {
    display: grid;
    gap: var(--space-1);
  }
  .counts dt {
    color: var(--ink-3);
  }
  .counts dd {
    margin: 0;
    display: grid;
    gap: var(--space-1);
  }
  .counts .fig {
    font-size: 2.25rem;
  }
  .counts .of {
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .back {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 44px;
    color: var(--ink-2);
  }
  .all {
    margin-right: auto;
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    min-height: 44px;
    color: var(--ink-2);
  }
  .panel {
    background: var(--ticket);
    border-top: 3px solid var(--ink);
    padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .todo {
    margin-top: var(--space-5);
    display: grid;
    gap: var(--space-3);
  }
  .todo p {
    max-width: 62ch;
  }
  .todo .btn {
    justify-self: start;
  }
  .sect {
    margin: var(--space-6) 0 var(--space-3);
    color: var(--ink-3);
  }
  .queue,
  .items,
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .queue {
    display: grid;
    gap: var(--space-4);
  }
  .group {
    background: var(--ticket);
    border-top: 3px solid var(--ink);
  }
  .g-head a,
  .list a {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto auto;
    align-items: baseline;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-5);
    color: inherit;
    text-decoration: none;
  }
  .g-head a:hover,
  .list a:hover {
    background: var(--ticket-press);
  }
  .g-head :global(svg) {
    align-self: center;
    color: var(--ink-2);
  }
  .t {
    font-size: var(--title-m);
    font-weight: 700;
  }
  .sub {
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .items li {
    padding: var(--space-3) var(--space-5);
    border-top: 1.5px dashed var(--perf);
    display: grid;
    gap: var(--space-2);
  }
  .save-bar {
    position: sticky;
    bottom: 0;
    margin-top: var(--space-5);
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-5);
    background: var(--ticket);
    border-top: 1.5px solid var(--perf);
  }
  .save-bar p {
    flex: 1;
    color: var(--ink-2);
  }
  .save-bar .btn {
    min-height: 52px;
  }
  .g-foot {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-5) var(--space-4);
    border-top: 1.5px dashed var(--perf);
  }
  .list {
    display: grid;
    gap: var(--space-2);
  }
  .list a {
    grid-template-columns: minmax(0, 1fr) auto;
    background: var(--ticket);
  }
  .badge {
    font-size: 0.8125rem;
    font-weight: 800;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    padding: 2px var(--space-2);
    border: 2px solid var(--ink);
    border-radius: 3px;
  }
  .badge[data-status='verified'] {
    background: var(--ink);
    color: var(--ticket);
  }
  @media (max-width: 600px) {
    .counts {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  @media (max-width: 480px) {
    .panel,
    .g-head a,
    .list a,
    .items li,
    .g-foot {
      padding-left: var(--space-4);
      padding-right: var(--space-4);
    }
  }
</style>
