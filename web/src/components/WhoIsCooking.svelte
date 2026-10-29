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

<dialog bind:this={dialog} onclose={() => (session.asking = false)} aria-labelledby="who-title">
  <h2 id="who-title">Who's cooking?</h2>
  <p>Changes are signed with your name, so the household can see who did what. This device will remember you.</p>
  <ul>
    {#each session.people as p (p.username)}
      <li><button class="person" onclick={() => pick(p)}>{p.name}</button></li>
    {/each}
  </ul>
  <form onsubmit={add}>
    <label for="who-new" class="caps">Someone new</label>
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
    border: 1.5px solid var(--ink);
    outline: 3px double var(--ink);
    outline-offset: 4px;
    border-radius: 0;
    background: var(--paper);
    color: var(--ink);
    padding: var(--space-6);
  }
  dialog::backdrop {
    background: rgb(42 36 32 / 0.35);
  }
  h2 {
    font-size: var(--title-l);
  }
  p {
    margin-top: var(--space-2);
    color: var(--ink-soft);
    font-size: 1rem;
  }
  ul {
    margin: var(--space-5) 0;
    border-top: 1px solid var(--rule);
  }
  .person {
    width: 100%;
    text-align: left;
    border: 0;
    border-bottom: 1px solid var(--rule);
    background: none;
    padding: var(--space-3) 0;
    font-size: var(--dish);
  }
  .person:hover {
    color: var(--brick);
  }
  .row {
    display: flex;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }
  input {
    flex: 1;
    border: 0;
    border-bottom: 1.5px solid var(--ink);
    background: none;
    padding: var(--space-2) 0;
    outline: none;
  }
  input:focus {
    border-bottom-color: var(--brick);
  }
  .error {
    color: var(--brick);
  }
  .cancel {
    margin-top: var(--space-5);
    padding-left: 0;
  }
</style>
