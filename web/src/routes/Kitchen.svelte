<script>
  import { ArrowLeft, X, Plus } from '@lucide/svelte';
  import { api } from '../lib/api.js';
  import { link } from '../lib/router.svelte.js';
  import { session, asPerson, toast } from '../lib/session.svelte.js';
  import { isOffline } from '../lib/net.svelte.js';
  import { ago, personName } from '../lib/format.js';

  let k = $state(null);
  let form = $state(null);
  let problem = $state('');
  let saving = $state(false);
  let dirty = $state(false);

  function fill(kitchen) {
    k = kitchen;
    form = {
      ovens: kitchen.ovens.map((o) => ({ ...o })),
      burners: kitchen.burners,
      appliances: kitchen.appliances.map((a) => ({ ...a })),
      dishes: kitchen.dishes.map((d) => ({ ...d })),
      day_start: kitchen.day_start,
      day_end: kitchen.day_end,
    };
    dirty = false;
  }
  api.kitchen().then(fill, (err) => (problem = err.code === 'offline' ? 'The kitchen needs a connection to Sous.' : err.message));

  const touch = () => (dirty = true);
  const num = (v, min) => Math.max(min, Math.round(+v || 0));

  async function save(e) {
    e.preventDefault();
    if (form.day_end <= form.day_start) {
      problem = 'The day has to end after it starts.';
      return;
    }
    saving = true;
    problem = '';
    const body = {
      ovens: form.ovens.map((o) => ({ name: o.name.trim(), racks: num(o.racks, 1) })),
      burners: num(form.burners, 0),
      appliances: form.appliances.filter((a) => a.name.trim()).map((a) => ({ name: a.name.trim(), count: num(a.count, 1) })),
      dishes: form.dishes.filter((d) => d.name.trim()).map((d) => ({ name: d.name.trim(), count: num(d.count, 1) })),
      day_start: form.day_start,
      day_end: form.day_end,
    };
    try {
      const saved = await asPerson(() => api.updateKitchen(k.version, body));
      if (!saved) return;
      fill(saved);
      toast('The kitchen is saved.');
    } catch (err) {
      if (err.status === 412) {
        problem = `${personName(err.body.current.updated_by, session.people)} changed the kitchen ${ago(err.body.current.updated_at)}. Their version is loaded; make your change again.`;
        fill(err.body.current);
      } else {
        problem = err.message;
      }
    } finally {
      saving = false;
    }
  }
</script>

<main class="kitchen" id="main">
  <a class="back" href="/meals" use:link><ArrowLeft size={16} aria-hidden="true" />Meals</a>
  <h1>Your kitchen</h1>
  <p class="lede">What meal plans fit around. An oven holds one temperature at a time and as many dishes as it has racks; hands-on work is kept to the hours of the day.</p>

  {#if problem}<p class="problem" role="alert">{problem}</p>{/if}

  {#if form}
    <form class="panel" onsubmit={save} oninput={touch} novalidate>
      <fieldset>
        <legend class="label">Ovens</legend>
        {#each form.ovens as o, i}
          <div class="row">
            <label class="field grow"><span class="visually-hidden">Oven {i + 1} name</span><input bind:value={o.name} placeholder="e.g. main oven" /></label>
            <label class="field num"><input type="number" min="1" max="8" bind:value={o.racks} aria-label="Racks in {o.name || `oven ${i + 1}`}" /><span class="unit">racks</span></label>
            <button type="button" class="icon" onclick={() => (form.ovens.splice(i, 1), touch())} aria-label="Remove {o.name || `oven ${i + 1}`}"><X size={16} /></button>
          </div>
        {/each}
        <button type="button" class="add" onclick={() => (form.ovens.push({ name: '', racks: 2 }), touch())}><Plus size={16} aria-hidden="true" />Add an oven</button>
      </fieldset>

      <fieldset>
        <legend class="label">Stovetop</legend>
        <label class="field num"><input type="number" min="0" max="16" bind:value={form.burners} aria-label="Burners" /><span class="unit">burners</span></label>
      </fieldset>

      <fieldset>
        <legend class="label">Other appliances</legend>
        <p class="hint">Named as recipe steps name them, such as slow cooker, grill or pressure cooker.</p>
        {#each form.appliances as a, i}
          <div class="row">
            <label class="field grow"><span class="visually-hidden">Appliance {i + 1}</span><input bind:value={a.name} placeholder="e.g. slow cooker" /></label>
            <label class="field num"><input type="number" min="1" max="20" bind:value={a.count} aria-label="How many {a.name || 'of these'}" /><span class="unit">of them</span></label>
            <button type="button" class="icon" onclick={() => (form.appliances.splice(i, 1), touch())} aria-label="Remove {a.name || `appliance ${i + 1}`}"><X size={16} /></button>
          </div>
        {/each}
        <button type="button" class="add" onclick={() => (form.appliances.push({ name: '', count: 1 }), touch())}><Plus size={16} aria-hidden="true" />Add an appliance</button>
      </fieldset>

      <fieldset>
        <legend class="label">Serving dishes</legend>
        <p class="hint">So a meal can check every dish has something to go to the table in.</p>
        {#each form.dishes as d, i}
          <div class="row">
            <label class="field grow"><span class="visually-hidden">Dish {i + 1}</span><input bind:value={d.name} placeholder="e.g. what it looks like" /></label>
            <label class="field num"><input type="number" min="1" max="50" bind:value={d.count} aria-label="How many {d.name || 'of these'}" /><span class="unit">of them</span></label>
            <button type="button" class="icon" onclick={() => (form.dishes.splice(i, 1), touch())} aria-label="Remove {d.name || `dish ${i + 1}`}"><X size={16} /></button>
          </div>
        {/each}
        <button type="button" class="add" onclick={() => (form.dishes.push({ name: '', count: 1 }), touch())}><Plus size={16} aria-hidden="true" />Add a dish</button>
      </fieldset>

      <fieldset>
        <legend class="label">The kitchen’s day</legend>
        <div class="row">
          <label class="field"><span class="label">From</span><input type="time" bind:value={form.day_start} /></label>
          <label class="field"><span class="label">Until</span><input type="time" bind:value={form.day_end} /></label>
        </div>
        <p class="hint">Hands-on work is planned in these hours when it can be; things that look after themselves (a brine, a slow braise) can run overnight.</p>
      </fieldset>

      <div class="acts">
        {#if k.updated_by}<span class="by">Last changed by {personName(k.updated_by, session.people)} {ago(k.updated_at)}</span>{/if}
        {#if isOffline()}<span class="by" role="status">You’re offline. Saving needs a connection.</span>{/if}
        <button class="btn primary" disabled={saving || !dirty || isOffline()}>{saving ? 'Saving…' : 'Save the kitchen'}</button>
      </div>
    </form>
  {:else if !problem}
    <p class="hint" role="status">Loading the kitchen…</p>
  {/if}
</main>

<style>
  .kitchen {
    max-width: 44rem;
    margin: 0 auto;
    padding: var(--space-4) var(--gutter) var(--space-8);
  }
  .back {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    min-height: 44px;
    color: var(--ink-2);
    font-size: var(--ui);
    font-weight: 600;
    text-decoration: none;
  }
  .back:hover {
    color: var(--red);
  }
  h1 {
    font-size: var(--title-l);
    font-weight: 800;
  }
  .lede {
    margin-top: var(--space-2);
    color: var(--ink-2);
    max-width: 60ch;
  }
  .problem {
    margin-top: var(--space-4);
    font-weight: 700;
    color: var(--red);
  }
  .panel {
    margin-top: var(--space-5);
    background: var(--ticket);
    border-top: 3px solid var(--ink);
    padding: var(--space-4) var(--space-5) var(--space-5);
    display: grid;
    gap: var(--space-5);
  }
  fieldset {
    border: 0;
    margin: 0;
    padding: 0 0 var(--space-4);
    border-bottom: 1.5px dashed var(--perf);
    display: grid;
    gap: var(--space-2);
  }
  legend {
    color: var(--ink-3);
    margin-bottom: var(--space-2);
  }
  .hint {
    color: var(--ink-2);
    font-size: var(--ui);
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: var(--space-3);
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }
  .field .label {
    color: var(--ink-3);
  }
  .grow {
    flex: 1 1 14rem;
  }
  .num {
    flex-direction: row;
    align-items: baseline;
    gap: var(--space-2);
  }
  .num input {
    width: 4.5rem;
    text-align: right;
    font-stretch: 62%;
    font-weight: 800;
    font-size: 1.5rem;
    font-variant-numeric: tabular-nums;
  }
  .unit {
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--ink-2);
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
  .icon {
    display: inline-grid;
    place-items: center;
    width: 44px;
    height: 44px;
    padding: 0;
    border: 0;
    border-radius: 3px;
    background: none;
    color: var(--ink-3);
  }
  .icon:hover {
    background: var(--pass-deep);
    color: var(--red);
  }
  .add {
    justify-self: start;
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 44px;
    padding: 0;
    border: 0;
    background: none;
    font-weight: 700;
    font-size: var(--ui);
    text-decoration: underline;
    text-underline-offset: 3px;
  }
  .add:hover {
    color: var(--red);
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-3);
  }
  .by {
    margin-right: auto;
    font-size: var(--ui);
    color: var(--ink-2);
  }
  @media (max-width: 480px) {
    .panel {
      padding: var(--space-4);
    }
    .acts .btn {
      width: 100%;
    }
  }
</style>
