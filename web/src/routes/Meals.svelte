<script>
  import { ChevronRight } from '@lucide/svelte';
  import { api } from '../lib/api.js';
  import { go, link } from '../lib/router.svelte.js';
  import { asPerson } from '../lib/session.svelte.js';
  import { isOffline } from '../lib/net.svelte.js';
  import { deviceZone, zonedToISO, clockAt, whenAt } from '../lib/meals.js';

  let meals = $state(null);
  let kitchen = $state(null);
  let problem = $state('');

  Promise.all([api.meals(), api.kitchen()]).then(
    ([m, k]) => ((meals = m), (kitchen = k)),
    (err) => (problem = err.code === 'offline' ? 'Meals need a connection to Sous.' : err.message),
  );

  // A new meal defaults to the coming Thursday at 5, in this device's zone:
  // a guess that's easy to change, not a plausible-looking value to keep.
  const zone = deviceZone();
  function nextEvening() {
    const d = new Date();
    d.setDate(d.getDate() + ((4 - d.getDay() + 7) % 7 || 7));
    const p = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T17:00`;
  }
  let title = $state('');
  let serve = $state(nextEvening());
  let titleError = $state(false);
  let saving = $state(false);

  async function create(e) {
    e.preventDefault();
    if (!title.trim()) {
      titleError = true;
      document.getElementById('meal-title')?.focus();
      return;
    }
    saving = true;
    problem = '';
    try {
      const m = await asPerson(() => api.createMeal({ title: title.trim(), serve_at: zonedToISO(serve, zone), timezone: zone }));
      if (m) go(`/m/${m.id}`);
    } catch (err) {
      problem = err.message;
    } finally {
      saving = false;
    }
  }

  const past = (m) => Date.parse(m.serve_at) < Date.now() - 864e5;
  const upcoming = $derived(meals?.filter((m) => !past(m)) ?? []);
  const earlier = $derived(meals?.filter(past).reverse() ?? []);

  function kitchenLine(k) {
    const ovens = k.ovens.length === 1 ? `1 oven, ${k.ovens[0].racks} racks` : `${k.ovens.length} ovens`;
    const dishes = k.dishes.length ? `, ${k.dishes.reduce((n, d) => n + d.count, 0)} serving dishes` : '';
    return `${ovens}, ${k.burners} burners${dishes}`;
  }
</script>

<main class="meals" id="main">
  <h1>Meals</h1>
  <p class="lede">Plan several recipes for one sitting. Sous lays out every step around your oven, your burners and your hands, moves what can be made ahead into the days before, and keeps time on the day.</p>

  {#if problem}<p class="problem" role="alert">{problem}</p>{/if}

  <form class="panel new" onsubmit={create} novalidate>
    <h2 class="label">Plan a meal</h2>
    <div class="row">
      <label class="field grow">
        <span class="label">Name</span>
        <input id="meal-title" bind:value={title} placeholder="e.g. the occasion" aria-invalid={titleError} oninput={() => (titleError = false)} />
        {#if titleError}<span class="field-error">A meal needs a name.</span>{/if}
      </label>
      <label class="field">
        <span class="label">Served</span>
        <input type="datetime-local" bind:value={serve} required />
      </label>
      <button class="btn primary" disabled={saving || isOffline() || !serve}>Plan it</button>
    </div>
    <p class="hint">Times are in {zone.replace(/_/g, ' ')}. You'll add the dishes next.</p>
  </form>

  {#if kitchen}
    <a class="kitchen-row" href="/kitchen" use:link>
      <span class="k-name">Your kitchen</span>
      <span class="k-line">{kitchenLine(kitchen)}</span>
      <ChevronRight size={18} aria-hidden="true" />
    </a>
  {/if}

  {#if meals === null && !problem}
    <p class="hint" role="status">Loading meals…</p>
  {:else if meals}
    {#if upcoming.length}
      <section aria-labelledby="up-h">
        <h2 id="up-h" class="label sect">Coming up</h2>
        <ul class="list">
          {#each upcoming as m (m.id)}
            <li>
              <a href="/m/{m.id}" use:link>
                <span class="when fig">{clockAt(m.serve_at, m.timezone)}</span>
                <span class="t">{m.title}</span>
                <span class="sub">{whenAt(m.serve_at, m.timezone)} · {m.items === 1 ? '1 dish' : `${m.items} dishes`}</span>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {:else if !earlier.length}
      <p class="empty">No meals planned yet. Name one above to start.</p>
    {/if}
    {#if earlier.length}
      <section aria-labelledby="past-h">
        <h2 id="past-h" class="label sect">Earlier</h2>
        <ul class="list past">
          {#each earlier as m (m.id)}
            <li>
              <a href="/m/{m.id}" use:link>
                <span class="when fig">{clockAt(m.serve_at, m.timezone)}</span>
                <span class="t">{m.title}</span>
                <span class="sub">{whenAt(m.serve_at, m.timezone)} · {m.items === 1 ? '1 dish' : `${m.items} dishes`}</span>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  {/if}
</main>

<style>
  .meals {
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
  .panel {
    background: var(--ticket);
    border-top: 3px solid var(--ink);
    padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .new {
    margin-top: var(--space-5);
    display: grid;
    gap: var(--space-3);
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: var(--space-4);
  }
  .field {
    display: grid;
    gap: var(--space-1);
  }
  .grow {
    flex: 1 1 16rem;
  }
  .field .label {
    color: var(--ink-3);
  }
  input {
    min-height: 44px;
    font-size: max(16px, var(--read));
    border: 0;
    border-bottom: 1.5px solid var(--field);
    background: transparent;
    padding: var(--space-1) 0;
  }
  input:focus {
    outline: none;
    border-bottom-color: var(--red);
  }
  input[aria-invalid='true'] {
    border-bottom-color: var(--red);
  }
  .field-error {
    color: var(--red);
    font-size: 0.875rem;
    font-weight: 600;
  }
  .hint,
  .empty {
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .empty {
    margin-top: var(--space-6);
  }
  .problem {
    margin-top: var(--space-4);
    font-weight: 700;
  }
  .kitchen-row {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--space-4);
    margin-top: var(--space-3);
    padding: var(--space-3) var(--space-5);
    background: var(--ticket);
    color: inherit;
    text-decoration: none;
  }
  .kitchen-row:hover {
    background: var(--ticket-press);
  }
  .k-name {
    font-weight: 800;
  }
  .k-line {
    color: var(--ink-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .kitchen-row :global(svg) {
    color: var(--ink-2);
  }
  .sect {
    margin: var(--space-6) 0 var(--space-3);
    color: var(--ink-3);
  }
  .list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: var(--space-2);
  }
  .list a {
    display: grid;
    grid-template-columns: 6.5rem minmax(0, 1fr);
    grid-template-areas: 'when t' 'when sub';
    column-gap: var(--space-4);
    align-items: baseline;
    padding: var(--space-3) var(--space-5);
    background: var(--ticket);
    border-top: 3px solid var(--ticket);
    color: inherit;
    text-decoration: none;
  }
  .list a:hover {
    background: var(--ticket-press);
  }
  .when {
    grid-area: when;
    font-size: 1.625rem;
    align-self: center;
  }
  .t {
    grid-area: t;
    font-size: var(--title-m);
    font-weight: 700;
  }
  .sub {
    grid-area: sub;
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .past a {
    color: var(--ink-2);
  }
  @media (max-width: 480px) {
    .panel {
      padding: var(--space-4);
    }
    .row .btn {
      width: 100%;
    }
    .list a {
      grid-template-columns: 5rem minmax(0, 1fr);
      padding: var(--space-3) var(--space-4);
    }
  }
</style>
