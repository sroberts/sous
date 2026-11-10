<script>
  import { flushSync } from 'svelte';
  import { Search, Plus, Link2, ChevronDown, Check, ExternalLink, CloudOff } from '@lucide/svelte';
  import { route, go, link } from '../lib/router.svelte.js';
  import { session, choosePerson } from '../lib/session.svelte.js';
  import { net, isOffline } from '../lib/net.svelte.js';

  let menuOpen = $state(false);
  let addOpen = $state(false); // New recipe and Import, on narrower screens
  let field;
  let addWrap = $state();
  let whoWrap = $state();

  // A click anywhere else closes an open menu.
  function onpointerdown(e) {
    if (addOpen && !addWrap?.contains(e.target)) addOpen = false;
    if (menuOpen && !whoWrap?.contains(e.target)) menuOpen = false;
  }

  const q = $derived(route.name === 'library' ? route.search.get('q') ?? '' : '');
  const show = $derived(route.name === 'library' ? route.search.get('show') : null);

  // Typing refines the library in place; from elsewhere it opens the library.
  // In the library the search waits for a pause in typing, so a fast typist
  // isn't made to wait on a search (and a history entry) per keystroke. It
  // reads the field when it runs, so nothing typed meanwhile is lost.
  let pending = null;
  function search() {
    clearTimeout(pending);
    if (route.name !== 'library') return apply();
    pending = setTimeout(apply, field.value ? 150 : 0);
  }

  function apply() {
    clearTimeout(pending);
    pending = null;
    const params = new URLSearchParams(route.name === 'library' ? route.search : '');
    const value = field.value;
    value ? params.set('q', value) : params.delete('q');
    go(`/${params.size ? `?${params}` : ''}`, { replace: route.name === 'library' });
  }

  // Enter and the up and down arrows act on the results, so a search still
  // waiting runs first (and the library settles its selection) rather than
  // acting on the results from before the last few keystrokes.
  function flush(e) {
    if (pending !== null && (e.key === 'Enter' || e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
      apply();
      flushSync();
    }
  }

  // A search still waiting belongs to the page it was typed on: going
  // anywhere else (a link, Back, Clear search) drops it and the field shows
  // that page's search. Nor does a timer outlive the masthead.
  $effect(() => {
    route.search;
    if (field) field.value = q;
    return () => clearTimeout(pending);
  });

  // On a phone the masthead takes a fifth of the screen, so it slides away
  // while you scroll down and comes back as soon as you scroll up (120px up
  // while a page sets data-steady-bar), at the top of the page, or when focus
  // or the person menu is in it. --bar-shown
  // is how much of it is showing, for anything that sticks below it.
  let bar;
  let hidden = $state(false);
  let height = $state(0);
  $effect(() => {
    const phone = matchMedia('(max-width: 760px)');
    let last = scrollY;
    const onscroll = () => {
      const y = scrollY;
      // The page moved itself (scrollQuietly): not the reader scrolling.
      if ('quietScroll' in document.documentElement.dataset) {
        last = y;
        return;
      }
      if (!phone.matches || y < height || menuOpen || addOpen || bar.contains(document.activeElement)) {
        hidden = false;
        last = y;
      } else if (y > last + 8) {
        hidden = true;
        last = y;
      } else if (last - y > ('steadyBar' in document.documentElement.dataset ? 120 : 8)) {
        // Reading a recipe's method, a nudge back up to re-read a line
        // shouldn't bring the masthead down over it; a real scroll up does.
        hidden = false;
        last = y;
      }
    };
    const ro = new ResizeObserver(() => (height = bar.offsetHeight));
    ro.observe(bar);
    addEventListener('scroll', onscroll, { passive: true });
    phone.addEventListener('change', onscroll);
    return () => {
      ro.disconnect();
      removeEventListener('scroll', onscroll);
      phone.removeEventListener('change', onscroll);
    };
  });
  $effect(() => {
    if (menuOpen || addOpen) hidden = false;
  });
  // Moving to another page closes the add menu.
  $effect(() => {
    route.name;
    addOpen = false;
  });
  $effect(() => {
    document.documentElement.style.setProperty('--bar-shown', `${hidden ? 0 : height}px`);
  });

  function onkey(e) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      field?.focus();
      field?.select();
    } else if (e.key === 'Escape' && (menuOpen || addOpen)) {
      const back = addOpen ? addWrap?.querySelector('button') : whoWrap?.querySelector('button');
      menuOpen = addOpen = false;
      back?.focus();
    }
  }
</script>

<svelte:window onkeydown={onkey} {onpointerdown} />

<header class="bar" class:hidden bind:this={bar} onfocusin={() => (hidden = false)}>
  <a class="mark" href="/" use:link aria-label="Sous, the library">Sous</a>
  <nav aria-label="Library">
    <a href="/" use:link class:on={route.name === 'library' && !show} aria-current={route.name === 'library' && !show ? 'page' : undefined}>Library</a>
    <a href="/?show=want" use:link class:on={show === 'want'} aria-current={show === 'want' ? 'page' : undefined}>Want to cook</a>
    <a href="/?show=fav" use:link class:on={show === 'fav'} aria-current={show === 'fav' ? 'page' : undefined}>Favorites</a>
  </nav>
  <label class="search">
    <Search size={17} aria-hidden="true" />
    <span class="visually-hidden">Search recipes</span>
    <input bind:this={field} type="search" placeholder="Search recipes, tags, ingredients" value={q} oninput={search} onkeydown={flush} autocomplete="off" />
    <kbd>⌘K</kbd>
  </label>
  <div class="add">
    <a class="btn" href="/import" use:link aria-current={route.name === 'import' ? 'page' : undefined}><Link2 aria-hidden="true" />Import</a>
    <a class="btn" href="/new" use:link aria-current={route.name === 'new' ? 'page' : undefined}><Plus aria-hidden="true" />New recipe</a>
  </div>
  <div class="add-menu" bind:this={addWrap}>
    <button class="btn add-btn" aria-haspopup="menu" aria-expanded={addOpen} onclick={() => (addOpen = !addOpen)}><Plus aria-hidden="true" />Add</button>
    {#if addOpen}
      <ul class="menu" role="menu">
        <li role="none"><a role="menuitem" href="/new" use:link>New recipe<Plus size={16} aria-hidden="true" /></a></li>
        <li role="none"><a role="menuitem" href="/import" use:link>Import from a link<Link2 size={16} aria-hidden="true" /></a></li>
      </ul>
    {/if}
  </div>
  <div class="who" bind:this={whoWrap}>
    <!-- Quiet, not an alarm: reading and cooking carry on from this device. -->
    <span class="net" role="status" title="You can read and cook from the library; changes need a connection.">
      {#if isOffline()}<CloudOff size={17} aria-hidden="true" /><span class="net-text">{net.online ? 'Can’t reach Sous' : 'Offline'}</span>{/if}
    </span>
    {#if session.outpost}
      <!-- Outpost's login decides who this is, so there is no one to switch
           to; the menu leads back to Outpost for the account and sign-out. -->
      {#if session.portal}
        <button class="who-btn" aria-haspopup="menu" aria-expanded={menuOpen} onclick={() => (menuOpen = !menuOpen)}>
          <span class="avatar fig" aria-hidden="true">{session.person.name.slice(0, 1)}</span>
          <span class="name">{session.person.name}</span>
          <ChevronDown size={15} aria-hidden="true" />
        </button>
        {#if menuOpen}
          <ul class="menu" role="menu">
            <li role="none" class="note label">Signed in with Outpost</li>
            <li role="none"><a role="menuitem" href="{session.portal}/account">Account and sign out<ExternalLink size={16} aria-hidden="true" /></a></li>
            <li role="none"><a role="menuitem" href="{session.portal}/">All apps<ExternalLink size={16} aria-hidden="true" /></a></li>
          </ul>
        {/if}
      {:else}
        <span class="who-btn signed" title="Signed in with Outpost">
          <span class="avatar fig" aria-hidden="true">{session.person.name.slice(0, 1)}</span>
          <span class="name">{session.person.name}</span>
        </span>
      {/if}
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
    text-overflow: ellipsis; /* a narrow box ends its hint with …, not mid-word */
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
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .net {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    white-space: nowrap;
    font-size: var(--ui);
    font-weight: 600;
    color: var(--ink-2);
  }
  .net:empty {
    display: none;
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
  .name {
    white-space: nowrap;
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
  .menu button,
  .menu a {
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
    color: var(--ink);
    text-decoration: none;
  }
  .menu .note {
    padding: var(--space-2) var(--space-4) var(--space-1);
    color: var(--ink-3);
  }
  .menu a:hover,
  .menu button:hover {
    background: var(--ticket-press);
  }
  .menu :global(svg) {
    color: var(--ink-2);
  }
  .add-menu {
    position: relative;
    display: none;
  }
  .add-menu .menu {
    left: 0;
    right: auto;
  }
  .add-btn {
    min-height: 40px;
    padding: 0 var(--space-3);
  }

  @media (max-width: 1080px) {
    .add {
      display: none;
    }
    .add-menu {
      display: block;
    }
  }
  /* An iPad in portrait: one row still, so the bar keeps its 56px. The
     shortcut hint goes (there's no keyboard), the gaps close up, and the
     connection note keeps its icon and says the rest to screen readers. */
  @media (min-width: 761px) and (max-width: 1080px) {
    .bar {
      gap: var(--space-4);
    }
    nav a {
      padding: 7px 10px;
    }
    .search {
      min-width: 10rem;
    }
    kbd {
      display: none;
    }
    .net-text {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }
  }
  @media (min-width: 761px) and (max-width: 860px) {
    .bar {
      gap: var(--space-3);
    }
    nav a {
      padding: 7px 8px;
    }
    .search {
      min-width: 8rem;
    }
  }
  @media (max-width: 760px) {
    .bar {
      transition: transform 240ms var(--ease-out);
      height: auto;
      flex-wrap: wrap;
      gap: var(--space-2) var(--space-4);
      padding-top: var(--space-2);
      padding-bottom: var(--space-3);
    }
    .bar.hidden {
      transform: translateY(-100%);
    }
    .add-menu {
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
