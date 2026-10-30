<script>
  import { route } from './lib/router.svelte.js';
  import { session, boot } from './lib/session.svelte.js';
  import Masthead from './components/Masthead.svelte';
  import WhoIsCooking from './components/WhoIsCooking.svelte';
  import Toast from './components/Toast.svelte';
  import Library from './routes/Library.svelte';
  import Recipe from './routes/Recipe.svelte';
  import Cook from './routes/Cook.svelte';
  import Editor from './routes/Editor.svelte';
  import Import from './routes/Import.svelte';

  boot();

  const titles = { library: 'Sous', new: 'New recipe · Sous', import: 'Import a recipe · Sous' };
  $effect(() => {
    if (titles[route.name]) document.title = titles[route.name];
  });
</script>

{#if route.name !== 'cook'}
  <Masthead />
{/if}

{#if session.error && !session.loaded}
  <main class="down">
    <h1>Sous isn't answering.</h1>
    <p>{session.error.message}</p>
    <button class="btn" onclick={boot}>Try again</button>
  </main>
{:else if route.name === 'library'}
  <Library />
{:else if route.name === 'recipe'}
  {#key route.id}<Recipe id={route.id} />{/key}
{:else if route.name === 'cook'}
  {#key route.id}<Cook id={route.id} />{/key}
{:else if route.name === 'edit'}
  {#key route.id}<Editor id={route.id} />{/key}
{:else if route.name === 'new'}
  <Editor id={null} />
{:else if route.name === 'import'}
  <!-- A new ?url= is a new import, so the page starts over. -->
  {#key route.search.toString()}<Import />{/key}
{/if}

{#if session.asking}
  <WhoIsCooking />
{/if}

{#if session.signedOut}
  <!-- Reloading this page takes the gateway's login, which returns here. -->
  <div class="signed-out" role="alert">
    <p>Your Outpost sign-in has ended, so changes aren't being saved.</p>
    <a class="btn" href={location.href}>Sign in again</a>
  </div>
{:else}
  <Toast />
{/if}

<style>
  .signed-out {
    position: fixed;
    left: 50%;
    bottom: max(var(--space-5), env(safe-area-inset-bottom));
    transform: translateX(-50%);
    z-index: 30;
    width: min(34rem, calc(100vw - 32px));
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-3) var(--space-3) var(--space-5);
    background: var(--ink);
    color: var(--ticket);
    border-top: 4px solid var(--red);
    border-radius: 3px;
  }
  .signed-out p {
    flex: 1;
    font-size: var(--ui);
    font-weight: 600;
  }
  .signed-out .btn {
    flex: none;
    background: var(--red);
    border-color: var(--red);
    color: var(--ticket);
  }
  .signed-out .btn:hover {
    background: var(--red-press);
  }
  .down {
    max-width: 36rem;
    margin: var(--space-8) auto;
    padding: 0 var(--gutter);
    display: grid;
    gap: var(--space-4);
    justify-items: start;
  }
  .down h1 {
    font-size: var(--title-l);
  }
  .down p {
    color: var(--ink-2);
  }
</style>
