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

<Toast />

<style>
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
