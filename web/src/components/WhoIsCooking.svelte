<script>
  // Asked once per device when the gateway isn't telling us (spec.md §5).
  // A modal is right here: the change the cook just made waits on it.
  import { session, choosePerson } from '../lib/session.svelte.js';
  import { api } from '../lib/api.js';

  let dialog;
  let name = $state('');
  let error = $state('');

  $effect(() => {
    dialog.showModal();
  });

  function pick(p) {
    choosePerson(p);
    session.asking = false;
  }

  async function add(e) {
    e.preventDefault();
    error = '';
    try {
      const p = await api.addPerson(name);
      session.people = [...session.people, p];
      pick(p);
    } catch (err) {
      error = err.message;
    }
  }
</script>

<dialog class="ticket" bind:this={dialog} onclose={() => (session.asking = false)} aria-labelledby="who-title">
  <h2 id="who-title">Who's cooking?</h2>
  <p>Changes are signed with your name, so the household can see who did what. This device will remember you.</p>
  <ul>
    {#each session.people as p (p.username)}
      <li><button class="person" onclick={() => pick(p)}>{p.name}</button></li>
    {/each}
  </ul>
  <form onsubmit={add}>
    <label for="who-new" class="label">Someone new</label>
    <div class="row">
      <input id="who-new" bind:value={name} placeholder="Their name" autocomplete="given-name" />
      <button class="btn" disabled={!name.trim()}>Add</button>
    </div>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </form>
  <button class="btn quiet cancel" onclick={() => dialog.close()}>Not now</button>
</dialog>

<style>
  dialog {
    width: min(420px, calc(100vw - 32px));
    border: 0;
    border-top: 4px solid var(--ink);
    border-radius: 0;
    background: var(--ticket);
    color: var(--ink);
    padding: var(--space-6) var(--space-6) calc(var(--space-6) + var(--tear));
  }
  dialog::backdrop {
    background: rgb(20 21 22 / 0.45);
  }
  h2 {
    font-size: var(--title-l);
    font-weight: 800;
  }
  p {
    margin-top: var(--space-2);
    color: var(--ink-2);
    font-size: var(--ui);
  }
  ul {
    margin: var(--space-5) 0;
    border-top: 3px solid var(--ink);
  }
  .person {
    width: 100%;
    min-height: 52px;
    text-align: left;
    border: 0;
    border-bottom: 1.5px dashed var(--perf);
    background: none;
    padding: 0 var(--space-2);
    font-size: var(--title-m);
    font-weight: 700;
  }
  .person:hover {
    background: var(--ticket-press);
    color: var(--red);
  }
  .label {
    color: var(--ink-3);
  }
  .row {
    display: flex;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }
  input {
    flex: 1;
    min-width: 0;
    border: 0;
    border-bottom: 2px solid var(--ink);
    background: none;
    padding: var(--space-2) 0;
    font-size: var(--read);
    outline: none;
  }
  input:focus {
    border-bottom-color: var(--red);
  }
  .error {
    color: var(--red);
    font-weight: 600;
  }
  .cancel {
    margin-top: var(--space-5);
  }
</style>
