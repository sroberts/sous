<script>
  import { Search, Plus, ChevronDown, Check } from '@lucide/svelte';
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
    // Typing refines the menu in place; from elsewhere it opens the menu.
    go(`/${params.size ? `?${params}` : ''}`, { replace: route.name === 'library' });
  }

  function onkey(e) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      field?.focus();
      field?.select();
    }
  }
</script>

<svelte:window onkeydown={onkey} />

<header class="masthead">
  <div class="inner">
    <a class="mark" href="/" use:link aria-label="Sous, the menu">Sous</a>
    <nav aria-label="Library">
      <a href="/" use:link class:on={route.name === 'library' && !show}>Menu</a>
      <a href="/?show=want" use:link class:on={show === 'want'}>Want to cook</a>
      <a href="/?show=fav" use:link class:on={show === 'fav'}>Favorites</a>
    </nav>
    <label class="search">
      <Search size={16} aria-hidden="true" />
      <span class="visually-hidden">Search recipes</span>
      <input bind:this={field} type="search" placeholder="Search the menu" value={q} oninput={search} autocomplete="off" />
      <kbd>⌘K</kbd>
    </label>
    <a class="btn quiet new" href="/new" use:link><Plus aria-hidden="true" />New recipe</a>
    <div class="who">
      <button class="who-btn" aria-haspopup="menu" aria-expanded={menuOpen} onclick={() => (menuOpen = !menuOpen)}>
        <span class="caps" class:as={session.person}>{session.person ? 'Cooking as' : 'Who’s cooking?'}</span>
        {#if session.person}<span class="name">{session.person.name}</span>{/if}
        <ChevronDown size={14} aria-hidden="true" />
      </button>
      {#if menuOpen}
        <ul class="menu" role="menu">
          {#each session.people as p (p.username)}
            <li role="none">
              <button role="menuitemradio" aria-checked={session.person?.username === p.username} onclick={() => (choosePerson(p), (menuOpen = false))}>
                {p.name}
                {#if session.person?.username === p.username}<Check size={14} aria-hidden="true" />{/if}
              </button>
            </li>
          {/each}
          <li role="none"><button role="menuitem" onclick={() => ((session.asking = true), (menuOpen = false))}>Someone else…</button></li>
        </ul>
      {/if}
    </div>
  </div>
  <hr class="double-rule" />
</header>

<style>
  .masthead {
    position: sticky;
    top: 0;
    z-index: 10;
    background: var(--paper);
  }
  .inner {
    height: var(--masthead);
    display: flex;
    align-items: center;
    gap: var(--space-6);
    padding: 0 var(--gutter);
  }
  .mark {
    font-family: var(--serif);
    font-weight: 700;
    font-size: 2rem;
    line-height: 1;
    letter-spacing: 0.01em;
    text-decoration: none;
  }
  nav {
    display: flex;
    gap: var(--space-5);
  }
  nav a {
    font-family: var(--sans);
    font-size: var(--caps);
    font-weight: 600;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    text-decoration: none;
    color: var(--ink-soft);
    padding: 6px 0 4px;
    border-bottom: 2px solid transparent;
  }
  nav a:hover {
    color: var(--ink);
  }
  nav a.on {
    color: var(--ink);
    border-bottom-color: var(--brick);
  }
  .search {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: min(360px, 32vw);
    border-bottom: 1.5px solid var(--ink);
    padding: 4px 2px;
    color: var(--ink-soft);
  }
  .search:focus-within {
    border-bottom-color: var(--brick);
  }
  .search input {
    flex: 1;
    min-width: 0;
    border: 0;
    background: none;
    font-family: var(--sans);
    font-size: var(--ui);
    color: var(--ink);
    outline: none;
  }
  .search input::-webkit-search-cancel-button {
    display: none;
  }
  kbd {
    font-family: var(--sans);
    font-size: 11px;
    font-weight: 600;
    color: var(--ink-soft);
    border: 1px solid var(--rule);
    border-radius: 3px;
    padding: 1px 5px;
  }
  .who {
    position: relative;
  }
  .who-btn {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    border: 0;
    background: none;
    padding: var(--space-2) 0;
    color: var(--ink-soft);
  }
  .who-btn .name {
    font-family: var(--serif);
    font-size: 1rem;
    color: var(--ink);
  }
  .menu {
    position: absolute;
    right: 0;
    top: calc(100% + 4px);
    min-width: 200px;
    background: var(--paper);
    border: 1.5px solid var(--ink);
    padding: var(--space-1) 0;
  }
  .menu button {
    width: 100%;
    display: flex;
    justify-content: space-between;
    align-items: center;
    border: 0;
    background: none;
    text-align: left;
    padding: var(--space-2) var(--space-4);
    font-size: 1rem;
  }
  .menu button:hover {
    background: var(--paper-press);
  }
  .menu :global(svg) {
    color: var(--brick);
  }

  @media (max-width: 960px) {
    .new {
      display: none;
    }
  }
  @media (max-width: 760px) {
    .inner {
      height: auto;
      flex-wrap: wrap;
      gap: var(--space-3) var(--space-5);
      padding-top: var(--space-3);
      padding-bottom: var(--space-3);
    }
    .mark {
      font-size: 1.75rem;
    }
    .who {
      margin-left: auto;
    }
    .who-btn .caps.as {
      display: none;
    }
    nav {
      order: 3;
      width: 100%;
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
