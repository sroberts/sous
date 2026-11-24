<script>
  // When a meal is running late: which dish is holding it up and what's in
  // its way, then what would help, each worked out in full by the planner
  // (another cook, the meal without that dish, the meal served later), the
  // one that brings the meal in soonest first. The dish's own fixes and
  // other late dishes fold away. Every fix can be undone. Shared by the
  // planner and the run of show, so the cook can act on it from either.
  import { ChevronDown } from '@lucide/svelte';
  import { api } from '../lib/api.js';
  import { session, asPerson, toast } from '../lib/session.svelte.js';
  import { personName } from '../lib/format.js';
  import { clockAt, span, mealWord, problemText, itemsOf, withItem, withoutItem, restoreItem } from '../lib/meals.js';

  let { meal, plan, tz, onupdate, compact = false } = $props();

  let busy = $state(false);
  let problem = $state('');

  const at = (iso, plus) => new Date(Date.parse(iso) + plus * 60e3).toISOString();
  const word = $derived(mealWord(meal.serve_at, tz));
  const lower = $derived(word.toLowerCase());
  const landsAt = $derived(at(plan.serve_at, plan.behind_minutes));
  // Latest first: the first dish sets when the meal lands.
  const lead = $derived(plan.late_dishes[0]);
  const tied = $derived(plan.late_dishes.slice(1).filter((d) => d.minutes >= lead.minutes));
  const rest = $derived(plan.late_dishes.slice(1).filter((d) => d.minutes < lead.minutes));
  const options = $derived(plan.options?.length ? plan.options : [{ kind: 'move', lands_at: landsAt, behind_minutes: 0, in_kitchen_day: true }]);
  // Earliest landing first, as the planner orders them; dropping a dish is
  // never the red recommendation, wherever it sits.
  const ordered = $derived(options);
  const red = $derived(lead.cause === 'missed' ? null : ordered.find((o) => o.kind !== 'without'));
  const CAUSE = {
    missed: 'its make-ahead is still to do',
    late_start: 'its work started later than planned',
    busy: 'the kitchen has no room for it sooner',
  };

  // What's in the way, named: the oven, the burners or the cooks' hands,
  // busy with another dish until a time.
  function blocker(d) {
    const task = plan.tasks.find((t) => t.key === d.task);
    if (d.cause !== 'busy' || !task) return CAUSE[d.cause];
    const start = Date.parse(task.start);
    const uses = (u) =>
      task.oven ? u.oven === task.oven : task.equipment === 'stovetop' ? u.equipment === 'stovetop' : task.hands_on ? u.hands_on : false;
    const by = plan.tasks
      .filter((u) => u.kind !== 'preheat' && u.item_id !== task.item_id && uses(u) && Date.parse(u.end) <= start + 60e3 && Date.parse(u.end) > start - 45 * 60e3)
      .sort((a, b) => Date.parse(b.end) - Date.parse(a.end))[0];
    if (!by) return CAUSE.busy;
    const until = clockAt(by.end, tz);
    if (task.oven) return `the ${task.oven.toLowerCase()} is busy with ${by.recipe_title}${by.oven_temp ? ` at ${by.oven_temp}°F` : ''} until ${until}`;
    if (task.equipment === 'stovetop') return `the burners are busy with ${by.recipe_title} until ${until}`;
    return `${meal.cooks > 1 ? 'every cook’s hands are' : 'your hands are'} on ${by.recipe_title} until ${until}`;
  }

  // Dishes ready long before the meal now lands, unless it moves.
  const early = $derived.by(() => {
    const late = new Set(plan.late_dishes.map((d) => d.item_id));
    const held = new Set(plan.tasks.filter((t) => t.hold).map((t) => t.item_id));
    const gap = (s) => Math.round((Date.parse(landsAt) - Date.parse(s.serve_at)) / 60e3);
    const list = plan.serving.filter((s) => !late.has(s.item_id) && !held.has(s.item_id) && gap(s) >= 60);
    return list.length ? { names: list.map((s) => s.recipe_title), by: gap(list[0]) } : null;
  });
  const joinAnd = (xs) => (xs.length < 2 ? xs.join('') : `${xs.slice(0, -1).join(', ')} and ${xs.at(-1)}`);

  // A change to the meal. If someone else changed it a moment before, the
  // change goes onto theirs, once, and the message says so.
  async function patch(body, said, undo, retry = true) {
    busy = true;
    problem = '';
    try {
      // A function of the meal is worked out again on a newer version.
      const b = typeof body === 'function' ? body(meal) : body;
      const m = await asPerson(() => api.updateMeal(meal.id, meal.version, b));
      if (!m) return;
      onupdate({ meal: m, plan: await api.mealPlan(meal.id) });
      // An Undo after the block has gone finds its own change in the way: no news.
      const other = retry || meal.updated_by === session.person?.username ? '' : ` ${personName(meal.updated_by, session.people)} had changed the meal just before; yours went on top.`;
      if (said) toast(said + other, undo ? { label: 'Undo', run: undo } : null);
    } catch (err) {
      if (err.status === 412 && retry) {
        meal = err.body.current;
        busy = false;
        return patch(body, said, undo, false);
      }
      problem = problemText(err);
    } finally {
      busy = false;
    }
  }


  function label(o) {
    if (o.kind === 'more_cooks') return `Plan for ${o.cooks} cooks`;
    if (o.kind === 'without') return `Take ${o.recipe_title} off the menu`;
    return `Move ${lower} to ${clockAt(o.lands_at, tz)}`;
  }
  function outcome(o) {
    if (o.kind === 'move') return o.in_kitchen_day ? 'Every dish moves with it, so nothing is rushed.' : 'Every dish moves with it, but that’s past your kitchen’s hours.';
    const lands = `${word} would land at ${clockAt(o.lands_at, tz)}`;
    return o.behind_minutes ? `${lands}, ${span(o.behind_minutes)} late.` : `${lands}, on time.`;
  }
  function take(o) {
    if (o.kind === 'move') {
      const was = meal.serve_at;
      return patch({ serve_at: o.lands_at }, `${word} is now at ${clockAt(o.lands_at, tz)}.`, () => patch({ serve_at: was }, `${word} is back at ${clockAt(was, tz)}.`));
    }
    if (o.kind === 'more_cooks') {
      const was = meal.cooks;
      return patch({ cooks: o.cooks }, `Planned for ${o.cooks} cooks.`, () => patch({ cooks: was }, `Back to ${was === 1 ? '1 cook' : `${was} cooks`}.`));
    }
    const at = itemsOf(meal).findIndex((i) => i.id === o.item_id);
    const it = itemsOf(meal)[at];
    patch(withoutItem(o.item_id), `${o.recipe_title} is off the menu.`, () => patch(restoreItem(it, at), `${o.recipe_title} is back on the menu.`));
  }

  function serveLater(d) {
    const it = itemsOf(meal).find((i) => i.id === d.item_id);
    if (!it) return;
    const was = it.serve_offset_minutes;
    const when = clockAt(at(d.serve_at, d.minutes), tz);
    patch(withItem(d.item_id, { serve_offset_minutes: Math.min(720, was + d.minutes) }), `${d.recipe_title} now goes to the table at ${when}.`, () => patch(withItem(d.item_id, { serve_offset_minutes: was }), `${d.recipe_title} is back with the ${lower}.`));
  }

  // Make-ahead work: done after all, or started now.
  async function tasks(d, body, said, undoBody) {
    busy = true;
    problem = '';
    const keys = plan.tasks.filter((t) => t.item_id === d.item_id && t.status === 'missed').map((t) => t.key);
    const which = body.started ? keys.slice(0, 1) : keys;
    try {
      let p = null;
      for (const k of which) p = await asPerson(() => api.updateTask(meal.id, k, body));
      if (!p) return;
      onupdate({ meal, plan: p });
      toast(said, {
        label: 'Undo',
        run: async () => {
          let q = null;
          for (const k of which) q = await asPerson(() => api.updateTask(meal.id, k, undoBody));
          if (q) onupdate({ meal, plan: q });
        },
      });
    } catch (err) {
      problem = problemText(err);
    } finally {
      busy = false;
    }
  }
  const doneAhead = (d) => tasks(d, { done: true }, `${d.recipe_title}’s make-ahead is ticked off.`, { done: false });
  const startAhead = (d) => tasks(d, { started: true }, `${d.recipe_title}’s make-ahead is under way.`, { started: false });
</script>

<div class="late" class:compact>
  <p class="cause">
    <strong>{lead.recipe_title}</strong>{tied.length ? ` and ${joinAnd(tied.map((d) => d.recipe_title))}` : ''} {tied.length ? 'are' : 'is'} holding {lower} up: {blocker(lead)}.
  </p>

  <!-- The recommendation fixes the cause where it can (start the missed
       make-ahead), never drops a dish, and says what each choice does. -->
  <ul class="opts">
    {#if lead.cause === 'missed'}
      <li>
        <button type="button" class="btn primary" onclick={() => startAhead(lead)} disabled={busy} aria-describedby="opt-ahead">Start its make-ahead now</button>
        <button type="button" class="btn quiet" onclick={() => doneAhead(lead)} disabled={busy} aria-describedby="opt-made">It’s already made</button>
        <p class="hint" id="opt-ahead">Started now, {lower} lands at {clockAt(landsAt, tz)} and gets no later.</p>
        <p class="hint" id="opt-made">If it’s made, Sous ticks it off and replans the rest without it.</p>
      </li>
    {/if}
    {#each ordered as o (o.kind)}
      <li>
        <button type="button" class="btn" class:primary={o === red} onclick={() => take(o)} disabled={busy} aria-describedby="opt-{o.kind}">{label(o)}</button>
        <p class="hint" id="opt-{o.kind}" class:warn={o.kind === 'move' && !o.in_kitchen_day}>{outcome(o)}</p>
      </li>
    {/each}
  </ul>

  {#if early}
    <p class="hint">If {lower} isn’t moved, {joinAnd(early.names)} will be ready {span(early.by)} before the rest.</p>
  {/if}

  <details class="rest">
    <summary><ChevronDown size={18} aria-hidden="true" />{rest.length ? `Other fixes, and ${rest.length === 1 ? '1 more late dish' : `${rest.length} more late dishes`} that won’t change when ${lower} lands` : 'Other fixes'}</summary>
    <ul>
      <li>
        <p><strong>{lead.recipe_title}</strong> on its own, after the rest of the {lower}.</p>
        <div class="acts"><button type="button" class="btn quiet" onclick={() => serveLater(lead)} disabled={busy}>Serve it at {clockAt(at(lead.serve_at, lead.minutes), tz)}</button></div>
      </li>
      {#each rest as d (d.item_id)}
        <li>
          <p><strong>{d.recipe_title}</strong>: ready at {clockAt(d.ready_at, tz)}, {span(d.minutes)} after it’s due at the table; {blocker(d)}.</p>
          <div class="acts">
            {#if d.cause === 'missed'}<button type="button" class="btn" onclick={() => doneAhead(d)} disabled={busy}>It’s already made</button>{/if}
            <button type="button" class="btn quiet" onclick={() => serveLater(d)} disabled={busy}>Serve it at {clockAt(at(d.serve_at, d.minutes), tz)}</button>
          </div>
        </li>
      {/each}
    </ul>
  </details>
  {#if problem}<p class="problem" role="alert">{problem}</p>{/if}
</div>

<style>
  .late {
    display: grid;
    gap: var(--space-3);
  }
  .cause {
    font-size: var(--title-m);
    font-weight: 600;
    max-width: 52ch;
  }
  .opts {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: var(--space-2);
  }
  .opts li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-4);
  }
  /* The make-ahead row: its two buttons, then a line for each underneath. */
  .opts li:has(#opt-ahead) .hint {
    flex: 1 0 100%;
  }
  .opts .btn {
    min-height: 52px;
    padding: 0 var(--space-5);
    font-size: 1.0625rem;
  }
  .hint {
    font-size: var(--ui);
    color: var(--ink-2);
  }
  .hint.warn {
    font-weight: 700;
    color: var(--ink);
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  /* Dish names are long: these buttons wrap rather than push the page wide. */
  .late .btn {
    max-width: 100%;
    white-space: normal;
    text-align: left;
  }
  .rest {
    border-top: 1.5px dashed var(--ticket-press);
  }
  .rest summary {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 44px;
    font-size: var(--ui);
    font-weight: 700;
    color: var(--ink-2);
    cursor: pointer;
    list-style: none;
  }
  .rest summary::-webkit-details-marker {
    display: none;
  }
  .rest summary :global(svg) {
    flex: none;
    transition: transform 200ms var(--ease-out);
  }
  .rest[open] summary :global(svg) {
    transform: rotate(180deg);
  }
  .rest summary:hover {
    color: var(--ink);
  }
  .rest ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .rest li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2) var(--space-4);
    padding: var(--space-2) 0;
    border-top: 1.5px dashed var(--ticket-press);
  }
  .rest p {
    flex: 1 1 22rem;
    max-width: 62ch;
  }
  .problem {
    font-weight: 700;
    color: var(--red);
  }
  .compact .btn {
    min-height: 56px;
  }
  .compact .opts .btn {
    min-height: 64px;
    font-size: 1.1875rem;
  }
</style>
