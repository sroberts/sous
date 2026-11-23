<script>
  // When a meal is running late: when dinner now lands, every dish that's
  // late and why, and the fixes that don't need thought. Shared by the
  // planner and the run of show, so the cook can act on it from either.
  import { api } from '../lib/api.js';
  import { asPerson } from '../lib/session.svelte.js';
  import { clockAt, span } from '../lib/meals.js';

  let { meal, plan, tz, onupdate, compact = false } = $props();

  let busy = $state(false);
  let problem = $state('');

  const roundUp = (m) => Math.ceil(m / 15) * 15;
  const at = (iso, plus) => new Date(Date.parse(iso) + plus * 60e3).toISOString();
  const landsAt = $derived(at(plan.serve_at, plan.behind_minutes));
  const CAUSE = {
    missed: 'its make-ahead is still to do',
    late_start: 'its work started later than planned',
    busy: 'the kitchen has no room for it sooner',
  };

  // A change to the meal, applied again to someone else's newer version once.
  async function patch(body, retry = true) {
    busy = true;
    problem = '';
    try {
      const m = await asPerson(() => api.updateMeal(meal.id, meal.version, body));
      if (m) onupdate({ meal: m, plan: await api.mealPlan(meal.id) });
    } catch (err) {
      if (err.status === 412 && retry) {
        meal = err.body.current;
        return patch(body, false);
      }
      problem = err.message;
    } finally {
      busy = false;
    }
  }

  function serveLater(d) {
    const items = meal.items.map((i) => ({ id: i.id, recipe_id: i.recipe_id, serve_offset_minutes: i.serve_offset_minutes, dish: i.dish }));
    const it = items.find((i) => i.id === d.item_id);
    if (!it) return;
    it.serve_offset_minutes = Math.min(720, it.serve_offset_minutes + roundUp(d.minutes));
    patch({ items });
  }

  async function makeAheadDone(d) {
    busy = true;
    try {
      let p = null;
      for (const t of plan.tasks.filter((t) => t.item_id === d.item_id && t.status === 'missed')) {
        p = await asPerson(() => api.updateTask(meal.id, t.key, { done: true }));
      }
      if (p) onupdate({ meal, plan: p });
    } catch (err) {
      problem = err.message;
    } finally {
      busy = false;
    }
  }
</script>

<div class="late" class:compact>
  <p class="lands">Dinner now lands at <strong class="fig">{clockAt(landsAt, tz)}</strong>, {span(plan.behind_minutes)} late.</p>
  <ul class="dishes">
    {#each plan.late_dishes as d (d.item_id)}
      <li>
        <p><strong>{d.recipe_title}</strong> is ready at {clockAt(d.ready_at, tz)}, {span(d.minutes)} after it’s served: {CAUSE[d.cause]}.</p>
        <div class="acts">
          {#if d.cause === 'missed'}<button type="button" class="btn" onclick={() => makeAheadDone(d)} disabled={busy}>Its make-ahead is done</button>{/if}
          <button type="button" class="btn" onclick={() => serveLater(d)} disabled={busy}>Serve it at {clockAt(at(d.serve_at, roundUp(d.minutes)), tz)}</button>
        </div>
      </li>
    {/each}
  </ul>
  <div class="all">
    <button type="button" class="btn" onclick={() => patch({ serve_at: at(meal.serve_at, roundUp(plan.behind_minutes)) })} disabled={busy}>Move dinner to {clockAt(at(meal.serve_at, roundUp(plan.behind_minutes)), tz)}</button>
    <span class="hint">Moves every dish, so nothing is rushed.</span>
  </div>
  {#if problem}<p class="problem" role="alert">{problem}</p>{/if}
</div>

<style>
  .late {
    display: grid;
    gap: var(--space-2);
  }
  .lands {
    font-size: var(--title-m);
    font-weight: 700;
  }
  .lands .fig {
    font-size: 1.75rem;
    color: var(--red);
  }
  .dishes {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .dishes li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2) var(--space-4);
    padding: var(--space-2) 0;
    border-top: 1.5px dashed var(--ticket-press);
  }
  .dishes p {
    flex: 1 1 22rem;
    max-width: 62ch;
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .all {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    padding-top: var(--space-2);
    border-top: 1.5px dashed var(--ticket-press);
  }
  .hint {
    font-size: var(--ui);
    color: var(--ink-2);
  }
  .problem {
    font-weight: 700;
    color: var(--red);
  }
  .compact .btn {
    min-height: 52px;
  }
</style>
