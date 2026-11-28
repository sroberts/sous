<script>
  import { onDestroy } from 'svelte';
  import { ArrowLeft, TriangleAlert } from '@lucide/svelte';
  import { api } from '../lib/api.js';
  import { link } from '../lib/router.svelte.js';
  import { asPerson, toast } from '../lib/session.svelte.js';
  import { isOffline } from '../lib/net.svelte.js';
  import { STATUS, bySteps } from '../lib/graphs.js';
  import ActionReview from '../components/ActionReview.svelte';

  let { id } = $props();

  let recipe = $state(null);
  let view = $state(null); // GET /v1/recipes/{id}/graph, or null for none
  let review = $state([]); // what Sous wasn't sure of
  let kitchen = $state(null);
  let upgrades = $state(null);
  let problem = $state('');
  let loaded = $state(false);
  let chosen = $state({}); // "node|field" → value
  let saving = $state(false);
  let active = $state(null); // the node being pointed at, to mark its words

  async function load() {
    try {
      const [r, k, o] = await Promise.all([api.recipe(id), api.kitchen(), api.graphs()]);
      recipe = r;
      kitchen = k;
      upgrades = o.upgrades;
      try {
        view = await api.recipeGraph(id);
        // Every question about this recipe, a page at a time.
        const all = [];
        for (let cursor = ''; ; ) {
          const page = await api.reviewItems(cursor ? { recipe_id: id, limit: 100, cursor } : { recipe_id: id, limit: 100 });
          all.push(...page.items);
          if (!page.next_cursor) break;
          cursor = page.next_cursor;
        }
        review = all;
      } catch (err) {
        if (err.status !== 404) throw err;
        view = null;
        review = [];
      }
      chosen = {};
      problem = '';
    } catch (err) {
      problem = err.code === 'offline' ? 'Step graphs need a connection to Sous.' : err.message;
    } finally {
      loaded = true;
    }
  }
  load();

  const steps = $derived(recipe && view ? bySteps(recipe, view.graph) : []);
  // An action's unsure labels in one order everywhere, so a habit can form.
  const ORDER = ['action', 'duration', 'equipment', 'attention', 'check_interval_min', 'hold', 'timing_anchor', 'interruptible'];
  const byNode = $derived(
    review.reduce((m, it) => ((m[it.node_id] ??= []).push(it), m), Object.create(null)),
  );
  const flaggedOf = (nodeID) => [...(byNode[nodeID] ?? [])].sort((a, b) => ORDER.indexOf(a.field) - ORDER.indexOf(b.field));
  const key = (it) => `${it.node_id}|${it.field}`;

  // The unit of review is the action: those with anything Sous isn't sure
  // of, and how many of them are checked (every unsure label answered).
  const toCheck = $derived(view ? view.graph.nodes.filter((n) => byNode[n.id]) : []);
  const isChecked = (n) => (byNode[n.id] ?? []).every((it) => key(it) in chosen);
  const checkedCount = $derived(toCheck.filter(isChecked).length);
  const unsaved = $derived(Object.keys(chosen).length > 0);

  // Answers live only on the page until saved: say so before leaving.
  function guard(e) {
    if (unsaved) e.preventDefault();
  }
  window.addEventListener('beforeunload', guard);
  onDestroy(() => window.removeEventListener('beforeunload', guard));

  // Looks right: the action's labels as they stand, every one, confirmed.
  const FIELDS = { action: 'action', duration: 'duration', attention: 'attention', equipment: 'equipment', hold: 'hold', check_interval_min: 'check_interval_min' };
  function looksRight(n) {
    const next = { ...chosen };
    for (const [field, prop] of Object.entries(FIELDS)) {
      const k = `${n.id}|${field}`;
      if (k in next || !n[prop]) continue;
      next[k] = n[prop].value;
    }
    chosen = next;
    nextAction(n.id);
  }

  // Which actions are open for fixing.
  let fixing = $state({});

  // J and K move between the actions still to check; focus lands on its
  // Looks right.
  function nextAction(fromID, dir = 1) {
    const ids = toCheck.filter((n) => !isChecked(n) || n.id === fromID).map((n) => n.id);
    const at = ids.indexOf(fromID);
    const to = ids[at < 0 ? 0 : at + dir];
    if (!to) return;
    const el = document.querySelector(`[data-node="${CSS.escape(to)}"]`);
    el?.scrollIntoView({ block: 'center', behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
    el?.querySelector('.looks, .fix')?.focus({ preventScroll: true });
  }
  function keys(e) {
    if (e.metaKey || e.ctrlKey || e.altKey || e.target.matches('input, select, textarea')) return;
    const k = e.key.toLowerCase();
    if (k !== 'j' && k !== 'k') return;
    e.preventDefault();
    const here = document.activeElement?.closest('[data-node]')?.dataset.node ?? null;
    nextAction(here, k === 'j' ? 1 : -1);
  }

  // A problem names an action by its words, not its id, and leads to it,
  // opened for fixing.
  const nodeText = (nodeID) => view?.graph.nodes.find((n) => n.id === nodeID)?.text ?? nodeID;
  function problemText(e) {
    return e.node ? e.message.replaceAll(e.node, `“${nodeText(e.node)}”`) : e.message;
  }
  function goTo(nodeID) {
    fixing = { ...fixing, [nodeID]: true };
    const el = document.getElementById(`node-${nodeID}`);
    if (!el) return;
    el.scrollIntoView({ block: 'center', behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
    active = nodeID;
  }
  const errors = $derived(view?.check.errors ?? []);
  const stale = $derived(view?.graph.status === 'stale');
  const canAccept = $derived(view && !stale && !errors.length && checkedCount === toCheck.length && view.graph.status !== 'verified');

  function choose(it, v) {
    const { [key(it)]: _, ...rest } = chosen;
    chosen = v === undefined ? rest : { ...rest, [key(it)]: v };
  }

  async function save(accept) {
    saving = true;
    try {
      const items = Object.entries(chosen).map(([k, value]) => {
        const [node_id, field] = k.split('|');
        return { node_id, field, value };
      });
      const res = await asPerson(() => api.resolveReview({ recipe_id: id, graph_version: view.graph.version, items, accept }));
      if (!res) return;
      toast(accept ? `${recipe.title}’s step graph is verified.` : 'Saved.');
      fixing = {};
      await load();
    } catch (err) {
      if (err.status === 412) {
        await load();
        problem = 'Someone changed this graph while you were reviewing it. Their version is loaded; check it again.';
      } else problem = err.message;
    } finally {
      saving = false;
    }
  }

  let drafting = $state(false);
  async function draft() {
    drafting = true;
    problem = '';
    try {
      const res = await asPerson(() => api.upgradeRecipe(id, { mode: 'apply' }));
      if (res) await load();
    } catch (err) {
      problem = err.message;
    } finally {
      drafting = false;
    }
  }
</script>

<main class="graph" id="main">
  <a class="back" href="/r/{id}" use:link><ArrowLeft size={16} aria-hidden="true" />{recipe?.title ?? 'Recipe'}</a>
  <h1>Step graph</h1>

  {#if problem}<p class="problem" role="alert">{problem}</p>{/if}

  {#if !loaded}
    <p class="hint" role="status">Loading the step graph…</p>
  {:else if recipe && !view}
    <section class="panel empty">
      <h2>{recipe.title} has no step graph yet</h2>
      <p>A step graph splits each step into single actions and joins them by what they make and use, so a meal plan can fit several dishes around each other.</p>
      {#if upgrades?.graph_model}
        <button class="btn primary" onclick={draft} disabled={drafting || isOffline()}>{drafting ? 'Drafting… this takes a minute' : 'Draft it with Sous'}</button>
      {:else if upgrades?.labels}
        <p>An assistant drafts it and Sous labels each action. <a href="/assistants#graphs" use:link>Set that up</a>, then ask it to draft step graphs for this recipe.</p>
      {:else}
        <p class="hint">Step graphs need Sous’s decision model, which isn’t set up on this Sous.</p>
      {/if}
    </section>
  {:else if view}
    <p class="status">
      <span class="badge" data-status={view.graph.status}>{STATUS[view.graph.status]}</span>
      {view.graph.nodes.length} actions{#if toCheck.length}{' · '}<strong>{toCheck.length - checkedCount} to check</strong>{/if}
    </p>
    {#if toCheck.length}
      <p class="hint">Read each action as Sous labelled it. The parts it isn’t sure of are marked ?; if it reads right, say so, or fix any part.<span class="keys">{' '}J and K move between them.</span></p>
    {/if}

    {#if stale}
      <p class="notice"><TriangleAlert size={18} aria-hidden="true" />The recipe’s words have changed under some actions. Have it drafted again before checking it.</p>
    {/if}
    {#if errors.length}
      <div class="notice">
        <TriangleAlert size={18} aria-hidden="true" />
        <div>
          <p>This graph has {errors.length === 1 ? 'a problem' : `${errors.length} problems`} to fix before it can be verified:</p>
          <ul>
            {#each errors as e, i (i)}
              <li>{#if e.node}<button type="button" class="to-node" onclick={() => goTo(e.node)}>{problemText(e)}</button>{:else}{e.message}{/if}</li>
            {/each}
          </ul>
          <p>Usually an action is marked as using the oven by mistake: open it and fix what it uses. Otherwise have the graph drafted again.</p>
        </div>
      </div>
    {/if}
    {#if !toCheck.length && !errors.length && !stale && view.graph.status !== 'verified'}
      <p class="hint">Sous was sure of every action. Read them over, then accept the graph.</p>
    {/if}

    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <ol class="steps" onkeydown={keys}>
      {#each steps as st (st.number)}
        <li class="step">
          {#if st.section}<h2 class="section">{st.section}</h2>{/if}
          <div class="pair">
            <p class="words"><span class="n fig">{st.number}</span>{#each st.runs as r, i (i)}{#if r.node}<mark class:on={active === r.node}>{r.text}</mark>{:else}{r.text}{/if}{/each}</p>
            <ul class="nodes">
              {#each st.nodes as n (n.id)}
                <li class="node" id="node-{n.id}" class:needs={byNode[n.id] && !isChecked(n)} class:on={active === n.id} onpointerenter={() => (active = n.id)} onpointerleave={() => (active = null)} onfocusin={() => (active = n.id)} onfocusout={() => (active = null)}>
                  <ActionReview node={n} flagged={flaggedOf(n.id)} {chosen} onchoose={choose} onlooksright={looksRight} {kitchen}
                    bind:open={() => !!fixing[n.id], (v) => (fixing = { ...fixing, [n.id]: v })} disabled={stale} />
                </li>
              {/each}
              {#if !st.nodes.length}<li class="node none">No actions from this step.</li>{/if}
            </ul>
          </div>
        </li>
      {/each}
    </ol>

    {#if view.graph.status !== 'verified' || unsaved}
      <div class="bar">
        <p>{#if toCheck.length}{checkedCount} of {toCheck.length} {toCheck.length === 1 ? 'action' : 'actions'} checked{:else}Nothing left to check{/if}{#if errors.length}{' · '}<button type="button" class="to-node" onclick={() => goTo(errors[0].node)}>fix {errors.length === 1 ? 'its problem' : `its ${errors.length} problems`} to accept</button>{/if}</p>
        <button class="btn" onclick={() => save(false)} disabled={saving || !unsaved || stale || isOffline()}>Save</button>
        <button class="btn" class:primary={canAccept} onclick={() => save(true)} disabled={saving || !canAccept || isOffline()}>Accept as verified</button>
      </div>
    {/if}
  {/if}
</main>

<style>
  .graph {
    max-width: 72rem;
    margin: 0 auto;
    padding: var(--space-6) var(--gutter) var(--space-8);
  }
  .back {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 44px;
    color: var(--ink-2);
  }
  h1 {
    font-size: var(--title-l);
    font-weight: 800;
  }
  .status {
    margin-top: var(--space-2);
    color: var(--ink-2);
    font-size: var(--ui);
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
  }
  .status strong {
    color: var(--ink);
  }
  .badge {
    font-size: 0.8125rem;
    font-weight: 800;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    padding: 2px var(--space-2);
    border: 2px solid var(--ink);
    border-radius: 3px;
    color: var(--ink);
  }
  .badge[data-status='verified'] {
    background: var(--ink);
    color: var(--ticket);
  }
  .problem {
    margin-top: var(--space-4);
    font-weight: 700;
  }
  .hint {
    margin-top: var(--space-4);
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .notice {
    margin-top: var(--space-4);
    display: flex;
    gap: var(--space-2);
    align-items: flex-start;
    color: var(--ink-2);
  }
  .notice :global(svg) {
    flex: none;
    margin-top: 3px;
  }
  .notice ul {
    margin: var(--space-1) 0 0;
    padding-left: var(--space-5);
  }
  .panel {
    background: var(--ticket);
    border-top: 3px solid var(--ink);
    padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .empty {
    margin-top: var(--space-5);
    display: grid;
    gap: var(--space-3);
    max-width: 62ch;
  }
  .empty h2 {
    font-size: var(--title-m);
    font-weight: 800;
  }
  .empty .btn {
    justify-self: start;
  }
  .steps {
    list-style: none;
    margin: var(--space-5) 0 0;
    padding: 0;
    display: grid;
    gap: var(--space-3);
  }
  .section {
    font-weight: 800;
    font-size: var(--read);
    padding: var(--space-3) 0 var(--space-2);
  }
  .pair {
    display: grid;
    grid-template-columns: minmax(0, 5fr) minmax(0, 7fr);
    background: var(--ticket);
    border-top: 3px solid var(--ink);
  }
  .words {
    padding: var(--space-4) var(--space-5);
    line-height: 1.55;
    max-width: 58ch;
  }
  .n {
    float: left;
    font-size: 2rem;
    line-height: 1;
    margin-right: var(--space-3);
  }
  mark {
    background: none;
    color: inherit;
    text-decoration: underline;
    text-decoration-color: var(--perf);
    text-decoration-thickness: 2px;
    text-underline-offset: 4px;
  }
  mark.on {
    background: var(--ticket-press);
    text-decoration-color: var(--ink);
  }
  .nodes {
    list-style: none;
    margin: 0;
    padding: 0;
    border-left: 1.5px dashed var(--perf);
  }
  .node {
    padding: var(--space-3) var(--space-5);
    display: grid;
    gap: var(--space-3);
  }
  .node + .node {
    border-top: 1.5px dashed var(--perf);
  }
  .node.none {
    color: var(--ink-3);
  }
  .to-node {
    min-height: 44px;
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    text-align: left;
    text-decoration: underline;
    text-underline-offset: 3px;
    cursor: pointer;
    color: inherit;
  }
  /* No keys to press on a phone. */
  @media (max-width: 900px) {
    .keys {
      display: none;
    }
  }
  .node.on {
    background: var(--ticket-press);
  }
  /* The step's words stay in view while its questions scroll. */
  @media (min-width: 901px) {
    .pair {
      align-items: start;
    }
    .words {
      position: sticky;
      top: calc(var(--bar-shown, 0px) + var(--space-3));
    }
  }
  .bar {
    position: sticky;
    bottom: 0;
    margin-top: var(--space-5);
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-5);
    background: var(--ticket);
    border-top: 1.5px solid var(--perf);
  }
  .bar p {
    flex: 1;
    color: var(--ink-2);
    font-variant-numeric: tabular-nums;
  }
  .bar .btn {
    min-height: 52px;
  }
  @media (max-width: 900px) {
    .pair {
      grid-template-columns: minmax(0, 1fr);
    }
    .nodes {
      border-left: 0;
      border-top: 1.5px dashed var(--perf);
    }
  }
  @media (max-width: 480px) {
    .words,
    .node,
    .bar {
      padding-left: var(--space-4);
      padding-right: var(--space-4);
    }
    .bar {
      gap: var(--space-2) var(--space-3);
      padding-top: var(--space-2);
      padding-bottom: var(--space-2);
    }
    .bar p {
      flex: 1 1 100%;
      font-size: var(--ui);
    }
    .bar .btn {
      flex: 1 1 0;
      padding-left: var(--space-2);
      padding-right: var(--space-2);
    }
  }
</style>
