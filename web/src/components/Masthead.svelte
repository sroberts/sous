<script>
  import { Search, Plus, Link2, ChevronDown, Check } from '@lucide/svelte';
  import { route, go, link } from '../lib/router.svelte.js';
  import { session, choosePerson } from '../lib/session.svelte.js';

  let menuOpen = $state(false);
  let field;

  const q = $derived(route.name === 'library' ? route.search.get('q') ?? '' : '');
  const show = $derived(route.name === 'library' ? route.search.get('show') : null);

  function search(e) {
    const params = new URLSearchParams(route.name === 'library' ? route.search : '');
    const value = e.currentTarget.value;
    value ? params.set('q', value) : params.delete('q');
    // Typing refines the library in place; from elsewhere it opens the library.
    go(`/${params.size ? `?${params}` : ''}`, { replace: route.name === 'library' });
  }

  function onkey(e) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      field?.focus();
      field?.select();
    } else if (e.key === 'Escape') {
      menuOpen = false;
    }
  }
</script>

<svelte:window onkeydown={onkey} />

<header class="bar">
  <a class="mark" href="/" use:link aria-label="Sous, the library">Sous</a>
  <nav aria-label="Library">
    <a href="/" use:link class:on={route.name === 'library' && !show} aria-current={route.name === 'library' && !show ? 'page' : undefined}>Library</a>
    <a href="/?show=want" use:link class:on={show === 'want'} aria-current={show === 'want' ? 'page' : undefined}>Want to cook</a>
    <a href="/?show=fav" use:link class:on={show === 'fav'} aria-current={show === 'fav' ? 'page' : undefined}>Favorites</a>
  </nav>
  <label class="search">
    <Search size={17} aria-hidden="true" />
    <span class="visually-hidden">Search recipes</span>
    <input bind:this={field} type="search" placeholder="Search recipes, tags, ingredients" value={q} oninput={search} autocomplete="off" />
    <kbd>⌘K</kbd>
  </label>
  <div class="add">
    <a class="btn" href="/import" use:link aria-current={route.name === 'import' ? 'page' : undefined}><Link2 aria-hidden="true" />Import</a>
    <a class="btn primary" href="/new" use:link><Plus aria-hidden="true" />New recipe</a>
  </div>
  <div class="who">
    {#if session.outpost}
      <!-- Outpost's login decides who this is; there is no one to switch to. -->
      <span class="who-btn signed" title="Signed in with Outpost">
        <span class="avatar fig" aria-hidden="true">{session.person.name.slice(0, 1)}</span>
        <span class="name">{session.person.name}</span>
      </span>
    {:else}
      <button class="who-btn" aria-haspopup="menu" aria-expanded={menuOpen} onclick={() => (menuOpen = !menuOpen)}>
        {#if session.person}
          <span class="avatar fig" aria-hidden="true">{session.person.name.slice(0, 1)}</span>
          <span class="name">{session.person.name}</span>
        {:else}
          <span class="name">Who’s cooking?</span>
        {/if}
        <ChevronDown size={15} aria-hidden="true" />
      </button>
      {#if menuOpen}
        <ul class="menu" role="menu">
          {#each session.people as p (p.username)}
            <li role="none">
              <button role="menuitemradio" aria-checked={session.person?.username === p.username} onclick={() => (choosePerson(p), (menuOpen = false))}>
                {p.name}
                {#if session.person?.username === p.username}<Check size={16} aria-hidden="true" />{/if}
              </button>
            </li>
          {/each}
          <li role="none"><button role="menuitem" onclick={() => ((session.asking = true), (menuOpen = false))}>Someone else…</button></li>
        </ul>
      {/if}
    {/if}
  </div>
</header>

<style>
  .bar {
    position: sticky;
    top: 0;
    z-index: 10;
    height: var(--bar);
    display: flex;
    align-items: center;
    gap: var(--space-5);
    padding: 0 var(--gutter);
    background: var(--ticket);
    border-bottom: 1px solid var(--perf);
  }
  .mark {
    font-size: 1.5rem;
    font-weight: 850;
    font-stretch: 112%;
    letter-spacing: -0.03em;
    line-height: 1;
    text-decoration: none;
  }
  nav {
    display: flex;
    gap: var(--space-1);
  }
  nav a {
    white-space: nowrap;
    padding: 7px 12px;
    border-radius: 3px;
    font-size: var(--ui);
    font-weight: 600;
    color: var(--ink-2);
    text-decoration: none;
  }
  nav a:hover {
    color: var(--ink);
    background: var(--ticket-press);
  }
  nav a.on {
    color: var(--ink);
    background: var(--pass);
  }
  .search {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: min(440px, 34vw);
    min-width: 12rem;
    flex-shrink: 1;
    height: 40px;
    padding: 0 var(--space-3);
    border: 2px solid var(--ink);
    border-radius: 3px;
    background: var(--ticket);
    color: var(--ink-2);
  }
  .search:focus-within {
    border-color: var(--red);
  }
  .search input {
    flex: 1;
    min-width: 0;
    border: 0;
    background: none;
    font-size: var(--ui);
    color: var(--ink);
    outline: none;
  }
  .search input::-webkit-search-cancel-button {
    display: none;
  }
  kbd {
    font: 600 0.75rem var(--sans);
    color: var(--ink-3);
    border: 1px solid var(--perf);
    border-radius: 3px;
    padding: 1px 5px;
  }
  .add {
    display: flex;
    gap: var(--space-2);
  }
  .add .btn {
    min-height: 40px;
  }
  .who {
    position: relative;
  }
  .who-btn {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 40px;
    border: 0;
    background: none;
    padding: 0 var(--space-1);
    font-size: var(--ui);
    font-weight: 600;
  }
  .signed {
    cursor: default;
  }
  .avatar {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--ink);
    color: var(--ticket);
    font-size: 1rem;
  }
  .menu {
    position: absolute;
    right: 0;
    top: calc(100% + 6px);
    min-width: 220px;
    padding: var(--space-1) 0;
    background: var(--ticket);
    border: 2px solid var(--ink);
    border-radius: 3px;
  }
  .menu button {
    width: 100%;
    min-height: 44px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    border: 0;
    background: none;
    text-align: left;
    padding: 0 var(--space-4);
    font-size: var(--ui);
  }
  .menu button:hover {
    background: var(--ticket-press);
  }
  .menu :global(svg) {
    color: var(--red);
  }

  @media (max-width: 1080px) {
    .add {
      display: none;
    }
  }
  @media (max-width: 760px) {
    .bar {
      height: auto;
      flex-wrap: wrap;
      gap: var(--space-2) var(--space-4);
      padding-top: var(--space-2);
      padding-bottom: var(--space-3);
    }
    .who {
      margin-left: auto;
    }
    nav {
      order: 3;
      width: 100%;
      margin: 0 calc(var(--space-3) * -1);
    }
    .search {
      order: 4;
      width: 100%;
      margin-left: 0;
    }
    kbd {
      display: none;
    }
  }
</style>
