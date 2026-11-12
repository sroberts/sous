<script>
  import { tick } from 'svelte';
  import { ArrowLeft, Heart, Bookmark, Pencil, ExternalLink, ChevronUp, ArrowDown, ArrowUp } from '@lucide/svelte';
  import { link, go } from '../lib/router.svelte.js';
  import { api } from '../lib/api.js';
  import { session, asPerson, patchSummary, toast, refreshIndex } from '../lib/session.svelte.js';
  import { getRecipe } from '../lib/library.js';
  import { isOffline } from '../lib/net.svelte.js';
  import { amount, ago, personName, webLink } from '../lib/format.js';
  import Plate from '../components/Plate.svelte';
  import Facts from '../components/Facts.svelte';
  import Markdown from '../components/Markdown.svelte';
  import { scrollQuietly } from '../lib/stick.js';
  import { labelledFigures } from '../lib/markdown.js';

  let { id } = $props();
  let recipe = $state(null);
  let error = $state(null);
  let asWritten = $state(false);

  const load = () =>
    getRecipe(id).then(
      (r) => {
        recipe = r;
        error = null;
        document.title = `${r.title} · Sous`;
      },
      (err) => (error = err),
    );
  load();

  // A server's own message reads as a sentence; a failed request has none.
  const why = (err) => (err.status ? err.message : 'Sous couldn’t be reached. Try again in a moment.');

  // A recipe in the trash still opens (from a link, or the change log), so
  // the page says so and brings it back from here.
  let restoring = $state(false);
  async function restore() {
    restoring = true;
    try {
      const r = await asPerson(() => api.restoreRecipe(id));
      if (!r) return;
      recipe = r;
      await refreshIndex();
      toast(`Restored ${r.title} from the trash.`);
    } catch (err) {
      toast(`Couldn’t restore it. ${why(err)}`);
    } finally {
      restoring = false;
    }
  }

  const groups = (rows) => {
    const out = [];
    for (const row of rows) {
      if (!out.length || out.at(-1).section !== row.section) out.push({ section: row.section, rows: [] });
      out.at(-1).rows.push(row);
    }
    return out;
  };

  async function toggle(flag, say = false) {
    const next = { ...recipe.mine, [flag]: !recipe.mine[flag] };
    let saved;
    try {
      saved = await asPerson(() => api.setMine(id, { [flag]: next[flag] }));
    } catch (err) {
      toast(`Couldn’t save ${flag === 'favorite' ? 'Favorite' : 'Want to cook'}. ${why(err)}`);
      return;
    }
    if (!saved) return;
    recipe.mine = saved;
    patchSummary(id, { mine: saved });
    // From a key there's no button under the pointer to show it changed.
    if (say) {
      const said = flag === 'favorite' ? (saved.favorite ? 'Added to your favorites.' : 'Taken out of your favorites.') : saved.want_to_cook ? 'Added to Want to cook.' : 'Taken off Want to cook.';
      toast(said, { label: 'Undo', run: () => toggle(flag) });
    }
  }

  const person = (u) => personName(u, session.people);

  // A long description is held to three lines so the recipe itself starts in
  // the first screen; More opens the rest.
  let descEl = $state();
  let descOpen = $state(false);
  let descLong = $state(false);
  let descClipped = $state(false);
  $effect(() => {
    if (!descEl) return;
    const measure = () => {
      descClipped = descEl.scrollHeight > descEl.clientHeight + 1;
      descLong = descOpen || descClipped;
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(descEl);
    return () => ro.disconnect();
  });

  // Notes open the method, but read once and returned to never: collapsed,
  // one paragraph shows, held to two lines; Show all opens them all. That
  // paragraph is the first piece of advice, not a credit ("Inspired by…").
  let notesEl = $state();
  let notesOpen = $state(false);
  let notesLong = $state(false);
  let notesClipped = $state(false);
  const CREDIT = /^(inspired by|adapted from|based on|recipe (from|by)|source:|from the book)\b/i;
  const notesParas = $derived(recipe?.notes ? recipe.notes.split(/\n\s*\n/).map((p) => p.trim()).filter(Boolean) : []);
  const notesLead = $derived(notesParas.find((p) => !CREDIT.test(p)) ?? notesParas[0] ?? '');
  // Open, the advice still leads and the credit comes last, quietly, like a
  // source line, so the paragraph you were reading doesn't move.
  const notesInOrder = $derived([...notesParas.filter((p) => !CREDIT.test(p)), ...notesParas.filter((p) => CREDIT.test(p))]);
  $effect(() => {
    if (!notesEl) return;
    const measure = () => {
      notesClipped = notesEl.scrollHeight > notesEl.clientHeight + 1;
      notesLong = notesOpen || notesParas.length > 1 || notesClipped;
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(notesEl);
    return () => ro.disconnect();
  });

  // Below 900px the columns stack. A short recipe stays that way, both lists
  // in one scroll. A long one would put the whole ingredient list between
  // you and the method, so there they become two tabs that stick under the
  // masthead, each keeping its own place on the page.
  let narrow = $state(false);
  let fits = $state(null); // both lists in about a screen and a half (null: not measured yet)
  let tabbed = $derived(narrow && fits === false);
  let tab = $state('ingredients');
  let tabs = $state();
  let tabsAt = $state(); // where the tabs sit before they stick
  let ingPanel = $state();
  let methodPanel = $state();
  const placeOf = { ingredients: null, method: null };
  $effect(() => {
    const mq = matchMedia('(max-width: 900px)');
    const set = () => {
      narrow = mq.matches;
      fits = null;
    };
    set();
    mq.addEventListener('change', set);
    return () => mq.removeEventListener('change', set);
  });
  // Measured once, stacked, before the first paint; not again on scroll or
  // Show as written, so the layout doesn't change under you.
  $effect(() => {
    if (!narrow || fits !== null || !ingPanel || !methodPanel) return;
    fits = ingPanel.offsetHeight + methodPanel.offsetHeight <= innerHeight * 1.5;
  });

  // Where the tabs stick: the masthead's height when it's showing. Read from
  // --bar-shown, not the strip's own top, which is mid-transition as the
  // masthead slides.
  const stickTop = () => parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--bar-shown')) || 0;

  async function show(next, focus = false) {
    if (next !== tab) {
      const home = tabsAt.getBoundingClientRect().top + scrollY - stickTop();
      const stuck = scrollY > home;
      placeOf[tab] = stuck ? scrollY : null;
      tab = next;
      await tick();
      // Scrolled into one tab: open the other where you left it, or at its top.
      if (stuck) scrollQuietly(Math.max(placeOf[next] ?? home, home));
    }
    if (focus) document.getElementById(`tab-${next}`)?.focus();
  }
  // Once the header's Begin cooking is off screen, the strip carries a Cook
  // button of its own, except while the Begin cooking after the last step is
  // in view: one red button at a time. (Keyed to the header, not to the strip
  // sticking: a short tab may never scroll far enough to stick.)
  let tabsStuck = $state(false);
  let endCook = $state();
  let endInView = $state(false);
  let headCook = $state();
  let headInView = $state(true);
  $effect(() => {
    if (!tabbed || !tabsAt) return;
    const check = () => (tabsStuck = tabsAt.getBoundingClientRect().top < stickTop() + 1);
    check();
    addEventListener('scroll', check, { passive: true });
    return () => removeEventListener('scroll', check);
  });
  $effect(() => {
    if (!endCook) return;
    const io = new IntersectionObserver(([e]) => (endInView = e.isIntersecting));
    io.observe(endCook);
    return () => io.disconnect();
  });
  // The Ingredients bar steps aside for the Begin cooking after the last step
  // only once that button reaches the bar's strip at the foot of the screen,
  // so it's there for the last steps. On an iPad held upright it's a corner
  // button that never meets it, and never steps aside.
  let endAtBar = $state(false);
  let corner = $state(false);
  $effect(() => {
    const mq = matchMedia('(min-width: 761px)');
    const set = () => (corner = mq.matches);
    set();
    mq.addEventListener('change', set);
    return () => mq.removeEventListener('change', set);
  });
  $effect(() => {
    if (!endCook) return;
    const io = new IntersectionObserver(([e]) => (endAtBar = e.isIntersecting), { rootMargin: '0px 0px -84px 0px' });
    io.observe(endCook);
    return () => io.disconnect();
  });
  const peekShown = $derived(tabsStuck && (corner || !endAtBar));
  // On a phone the last Begin cooking is under the bar until it reaches the
  // bar's strip, so the strip's Cook stays until then: a red button in view
  // the whole way down, never two.
  const tabCookShown = $derived(!headInView && !(corner ? endInView : endAtBar));

  // The Begin cooking after the last step is red only once the header's has
  // scrolled away; on a short recipe both can be on screen at once.
  $effect(() => {
    if (!headCook) return;
    // Under the masthead counts as scrolled away.
    const bar = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--bar')) || 56;
    const io = new IntersectionObserver(([e]) => (headInView = e.isIntersecting), { rootMargin: `-${bar}px 0px 0px 0px` });
    io.observe(headCook);
    return () => io.disconnect();
  });

  // In the Method tab the quantities are a tab away, so a bar at the foot of
  // the screen, in thumb reach, pulls them up over the step you're on. The
  // sheet opens no taller than its list or 58% of the screen, so the step
  // stays in view above it; the handle (a tap, or a pull up) opens it to 85%.
  let sheet = $state();
  let sheetList = $state();
  let sheetTall = $state(false);
  let drag = $state(0); // px the sheet is pulled down (positive) or up by its handle
  let dragFrom = null;
  let dragBase = $state(0); // the sheet's height when the pull began
  let dragging = $state(false);
  let above = $state(null); // the group above the one at the top of the sheet: { name, el }
  // Where a group starts in its list. Group headings stick, so the heading's
  // own offset moves; the list under it doesn't.
  const groupTop = (h) => (h.nextElementSibling?.offsetTop ?? h.offsetTop) - h.offsetHeight;
  function openSheet() {
    if (!sheet) return;
    sheetTall = false;
    sheet.showModal();
    // The step to cook from is one you can still see above the sheet, not
    // the one at the reading line, which the sheet now covers.
    readingStep = stepNow(innerHeight - sheet.offsetHeight - 24); // its first line in view
    // Open at the ingredients for the part of the method you're reading. A
    // last group can't reach the top; it shows whole at the foot instead.
    if (!sheetList) return;
    const g = sheetList.querySelector(`[data-sect="${sectionNow()}"]`);
    sheetList.scrollTop = !g || g === sheetList.querySelector('[data-sect]') ? 0 : groupTop(g) + 3; // its rule under the sheet's
    sheetScrolled();
  }
  function sheetScrolled() {
    const heads = [...(sheetList?.querySelectorAll('h3.sect') ?? [])];
    let i = -1;
    for (const [k, h] of heads.entries()) if (groupTop(h) <= sheetList.scrollTop + 8) i = k;
    above = i > 0 ? { name: heads[i - 1].textContent, el: heads[i - 1] } : null;
  }
  function toAbove() {
    if (above) sheetList.scrollTo({ top: above.el === sheetList.querySelector('h3.sect') ? 0 : groupTop(above.el) + 3, behavior: still() ? 'auto' : 'smooth' });
  }
  const closeSheet = () => sheet?.close();
  function sheetClosed() {
    sheetTall = false;
    readingStep = stepNow();
  }
  // The pointer is captured only once the pull has begun, so a tap on a
  // button in the head (the handle, the group above) stays a tap.
  function dragStart(e) {
    dragFrom = e.clientY;
    dragBase = sheet.offsetHeight;
  }
  function dragMove(e) {
    if (dragFrom === null) return;
    const d = e.clientY - dragFrom;
    if (!dragging && Math.abs(d) > 6) {
      dragging = true;
      try {
        e.currentTarget.setPointerCapture(e.pointerId);
      } catch {}
    }
    if (dragging) drag = d;
  }
  function dragEnd() {
    if (dragFrom === null) return;
    if (dragging) {
      if (drag < -40) sheetTall = true;
      else if (drag > 80 && sheetTall) sheetTall = false;
      else if (drag > 80) closeSheet();
    }
    dragFrom = null;
    dragging = false;
    drag = 0;
  }

  // The step at the reading line, a little under halfway down the screen,
  // so "Cook from step 7" starts cook mode where you are: in the Method tab,
  // and side by side, where the ingredients column offers it.
  let readingStep = $state(1);
  const tracking = $derived((tabbed && tab === 'method') || (!narrow && !!recipe));
  function stepNow(line = innerHeight * 0.45) {
    let n = 1;
    for (const [k, li] of [...(methodPanel?.querySelectorAll('ol > li') ?? [])].entries()) {
      if (li.getBoundingClientRect().top > line) break;
      n = k + 1;
    }
    return n;
  }
  $effect(() => {
    if (!tracking) {
      readingStep = 1;
      return;
    }
    let frame = 0;
    const read = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        if (!sheet?.open) readingStep = stepNow();
      });
    };
    read();
    addEventListener('scroll', read, { passive: true });
    return () => {
      cancelAnimationFrame(frame);
      removeEventListener('scroll', read);
    };
  });
  const cookFrom = (n) => (n > 1 ? `/r/${id}/cook?step=${n}` : `/r/${id}/cook`);
  // A step's first words, to say which one "Cook from step 7" means.
  function stepWords(n) {
    const t = (recipe?.steps[n - 1]?.text ?? '').replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').replace(/[*_`#>]/g, '').trim();
    const clause = t.split(/[,.;:(]/)[0].trim();
    const words = clause.split(/\s+/);
    if (words.length <= 5) return clause;
    const cut = words.slice(0, 5);
    while (cut.length > 2 && /^(a|an|the|on|in|of|to|at|for|with|and|or|into|onto|over)$/i.test(cut.at(-1))) cut.pop();
    return `${cut.join(' ')}…`;
  }
  // Side by side, once the header's Begin cooking has gone and before the
  // one after the last step arrives, the ingredients column's foot offers
  // the hand-off: the one red button in view.
  const colCookShown = $derived(!narrow && !headInView && !endInView);

  // Back from cook mode (?step=7), the method opens at the step you left,
  // at the reading line. The address drops it, as cook mode's does.
  let backTo = (() => {
    const n = parseInt(new URLSearchParams(location.search).get('step') ?? '', 10);
    if (!(n >= 1)) return null;
    history.replaceState(history.state, '', location.pathname);
    return n;
  })();
  $effect(() => {
    if (!backTo || !recipe || !methodPanel || (narrow && fits === null)) return;
    const n = backTo;
    backTo = null;
    (async () => {
      if (tabbed) {
        tab = 'method';
        await tick();
      }
      const li = methodPanel.querySelectorAll('ol > li')[n - 1];
      if (!li) return;
      const under = stickTop() + (tabbed ? (tabs?.offsetHeight ?? 0) : 0) + 16;
      scrollQuietly(li.getBoundingClientRect().top + scrollY - Math.max(under, innerHeight * 0.45 - 32));
    })();
  });

  // Reading the method on a phone, a nudge back up doesn't bring the
  // masthead down over the line you're on.
  $effect(() => {
    if (!tabbed || tab !== 'method') return;
    document.documentElement.dataset.steadyBar = '';
    return () => delete document.documentElement.dataset.steadyBar;
  });

  // On a short recipe the Begin cooking after the last step is in view with
  // the header's from the start: one is enough. Judged once, as it lands.
  let endRedundant = $state(false);
  let endJudged = false;
  $effect(() => {
    if (!endCook || endJudged) return;
    endJudged = true;
    if (endCook.getBoundingClientRect().bottom + scrollY <= innerHeight) endRedundant = true;
  });

  // Method sections and ingredient groups pair by name: "The garnish" and
  // "For the garnish" are one key, "garnish". A method section with no
  // ingredients of its own ("To finish") keeps the last group that had some.
  const sectKey = (name) => (name ?? '').toLowerCase().replace(/[:.]/g, '').replace(/^(for|to make|to)\s+/, '').replace(/^the\s+/, '').trim();
  const ingKeys = $derived(new Set((recipe?.ingredients ?? []).map((i) => sectKey(i.section)).filter(Boolean)));
  // The ingredient group for the part of the method at the reading line,
  // a little under halfway down the screen, where the eye is.
  function sectionNow() {
    let key = '';
    for (const h of methodPanel?.querySelectorAll('h3.sect[data-sect]') ?? []) {
      if (h.getBoundingClientRect().top > innerHeight * 0.45) break;
      if (ingKeys.has(h.dataset.sect)) key = h.dataset.sect;
    }
    return key;
  }

  // Side by side, the ingredients column is no taller than the window. When
  // the list is longer, a foot says how much is below, and the column follows
  // the method: reading into "The garnish" brings "For the garnish" into it.
  let below = $state(0); // rows hidden under the column's foot
  let overflowing = $state(false);
  function measureBelow() {
    if (!ingPanel || narrow) {
      below = 0;
      overflowing = false;
      return;
    }
    overflowing = ingPanel.scrollHeight > ingPanel.clientHeight + 1;
    // Rows under the Cook band count as below too.
    const band = ingPanel.querySelector('.col-cook.shown')?.offsetHeight ?? 0;
    const edge = ingPanel.getBoundingClientRect().bottom - 8 - band;
    below = overflowing ? [...ingPanel.querySelectorAll('li')].filter((li) => li.getBoundingClientRect().top > edge - 24).length : 0;
  }
  $effect(() => {
    if (!ingPanel || narrow) {
      measureBelow();
      return;
    }
    asWritten;
    colCookShown;
    measureBelow();
    const ro = new ResizeObserver(measureBelow);
    ro.observe(ingPanel);
    ingPanel.addEventListener('scroll', measureBelow, { passive: true });
    return () => {
      ro.disconnect();
      ingPanel.removeEventListener('scroll', measureBelow);
    };
  });
  const still = () => matchMedia('(prefers-reduced-motion: reduce)').matches;
  function scrollColumn(top) {
    ingPanel.scrollTo({ top, behavior: still() ? 'auto' : 'smooth' });
  }
  const moreBelow = () => scrollColumn(ingPanel.scrollTop + ingPanel.clientHeight * 0.75);
  $effect(() => {
    if (narrow || !ingPanel || !methodPanel || !ingKeys.size) return;
    let last = sectionNow();
    let frame = 0;
    const follow = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const key = sectionNow();
        if (key === last || !overflowing) return (last = key);
        last = key;
        const first = ingPanel.querySelector('[data-sect]');
        const g = ingPanel.querySelector(`[data-sect="${key}"]`);
        scrollColumn(!g || g === first ? 0 : groupTop(g) - 16);
      });
    };
    addEventListener('scroll', follow, { passive: true });
    return () => {
      cancelAnimationFrame(frame);
      removeEventListener('scroll', follow);
    };
  });

  // Keys, as in the library and cook mode. ? lists them.
  let keysDialog = $state();
  const KEYS = $derived([
    ['C', tabbed || !narrow ? 'Cook, from the step you’re reading' : 'Begin cooking'],
    ['E', 'Edit the recipe'],
    ['F', 'Favorite, or not'],
    ['W', 'Want to cook, or not'],
    ['A', 'Show the ingredients as written, or tidied'],
    ...(tabbed ? [['I', 'The Ingredients tab'], ['M', 'The Method tab']] : []),
    ['?', 'This list'],
  ]);
  function onkey(e) {
    if (!recipe || session.asking || e.altKey || e.ctrlKey || e.metaKey) return;
    const el = e.target;
    if (el.closest?.('input, textarea, select, [contenteditable], dialog[open]')) return;
    const k = e.key.toLowerCase();
    if (k === 'c') go(cookFrom(readingStep));
    else if (k === 'e') go(`/r/${id}/edit`);
    else if ((k === 'f' || k === 'w') && !isOffline()) toggle(k === 'f' ? 'favorite' : 'want_to_cook', true);
    else if (k === 'a') asWritten = !asWritten;
    else if (tabbed && (k === 'i' || k === 'm')) show(k === 'i' ? 'ingredients' : 'method', true);
    else if (e.key === '?') keysDialog?.showModal();
    else return;
    e.preventDefault();
  }

  const nutrition = $derived(recipe?.nutrition_text ? labelledFigures(recipe.nutrition_text) : null);
  // A label's worth of precision: 1063.75 calories prints as 1064, 55.25 g as
  // 55.3 g, and units are the short ones a label uses.
  const UNITS = { gram: 'g', grams: 'g', milligram: 'mg', milligrams: 'mg', microgram: 'µg', micrograms: 'µg', kilocalorie: 'kcal', kilocalories: 'kcal', calories: 'kcal' };
  function figure(text) {
    const n = Number(text.replace(/,/g, ''));
    if (!Number.isFinite(n)) return text;
    return n >= 100 ? String(Math.round(n)) : String(Math.round(n * 10) / 10);
  }
  const unit = (u) => UNITS[u.toLowerCase()] ?? u;
  // Calories with no unit given are kilocalories, as on any label.
  const unitFor = (n) => (n.unit ? unit(n.unit) : /^(calories|energy)$/i.test(n.label) ? 'kcal' : '');

  function tabKey(e) {
    const to = { ArrowLeft: 'ingredients', Home: 'ingredients', ArrowRight: 'method', End: 'method' }[e.key];
    if (!to) return;
    e.preventDefault();
    show(to, true);
  }
</script>

<svelte:window onkeydown={onkey} />

{#if error}
  <main class="missing">
    {#if error.code === 'not_on_device'}
      <h1>This recipe isn’t on this device yet.</h1>
      <p>It will be once you’re back online.</p>
    {:else}
      {#if error.status === 404}
        <h1>That recipe isn’t in Sous.</h1>
        <p>The link may be wrong or cut short.</p>
      {:else}
        <h1>This recipe didn’t load.</h1>
        <p>{error.status ? 'Sous had a problem fetching it.' : error.message} Try again in a moment.</p>
      {/if}
    {/if}
    <a class="btn" href="/" use:link>Back to the library</a>
  </main>
{:else if !recipe}
  <main class="page loading" aria-busy="true"><span class="visually-hidden">Loading recipe</span></main>
{:else}
  <main class="page">
    {#if recipe.deleted_at}
      <div class="trashed" role="status">
        <p>This recipe is in the trash, so it’s gone from the library and from search.</p>
        <button class="btn" disabled={restoring || isOffline()} onclick={restore}>Restore</button>
      </div>
    {/if}
    <div class="crumbs">
      <a class="back" href="/" use:link><ArrowLeft size={16} aria-hidden="true" />Library</a>
      <a class="back" href="/r/{id}/edit" use:link aria-keyshortcuts="e" title="Edit recipe (E)"><Pencil size={16} aria-hidden="true" />Edit recipe</a>
    </div>

    <div class="rail" aria-hidden="true"></div>
    <header class="top ticket" class:has-photo={!!recipe.images[0]}>
      <span class="clip" aria-hidden="true"></span>
      <div class="head">
        <h1>{recipe.title}</h1>
        <!-- On a phone, Edit recipe ends the source line (the crumb row is gone). -->
        <p class="src" class:bare={!recipe.source_url && !recipe.tags.length} class:sourced={!!recipe.source_url}>
          {#if recipe.tags.length}<span>{recipe.tags.join(', ')}</span>{#if recipe.source_url}<span class="dot" aria-hidden="true">·</span>{/if}{/if}
          <span class="src-line">
            {#if webLink(recipe.source_url)}<a href={recipe.source_url.trim()} target="_blank" rel="noreferrer">{recipe.source_host ?? recipe.source_url}<ExternalLink size={13} aria-hidden="true" /><span class="visually-hidden">{' (opens in a new tab)'}</span></a>{:else if recipe.source_url}<span>{recipe.source_url}</span>{/if}
            <a class="edit-here" href="/r/{id}/edit" use:link><Pencil size={15} aria-hidden="true" />Edit</a>
          </span>
        </p>
        <div class="facts"><Facts {recipe} size="l" count={false} /></div>
        <div class="acts">
          <a class="btn primary cook" href="/r/{id}/cook" use:link bind:this={headCook} aria-keyshortcuts="c" title="Begin cooking (C)">Begin cooking</a>
          <!-- On a phone these are icon buttons beside Begin cooking; the
               name stays for screen readers and as a tooltip. -->
          <button class="btn mark" aria-pressed={recipe.mine.favorite} disabled={isOffline()} title="Favorite (F)" aria-keyshortcuts="f" onclick={() => toggle('favorite')}>
            <Heart aria-hidden="true" fill={recipe.mine.favorite ? 'currentColor' : 'none'} /><span class="mark-name">Favorite</span>
          </button>
          <button class="btn mark" aria-pressed={recipe.mine.want_to_cook} disabled={isOffline()} title="Want to cook (W)" aria-keyshortcuts="w" onclick={() => toggle('want_to_cook')}>
            <Bookmark aria-hidden="true" fill={recipe.mine.want_to_cook ? 'currentColor' : 'none'} /><span class="mark-name">Want to cook</span>
          </button>
        </div>
        {#if recipe.description}
          <div class="desc-wrap" class:clipped={descClipped && !descOpen}>
            <p class="desc" class:open={descOpen} class:fade-end={descClipped && !descOpen} id="desc" bind:this={descEl}><Markdown text={recipe.description} /></p>
            {#if descLong}<button class="more" aria-expanded={descOpen} aria-controls="desc" onclick={() => (descOpen = !descOpen)}>{descOpen ? 'Show less' : 'Show all'}<span class="visually-hidden">{' of the description'}</span></button>{/if}
          </div>
        {/if}
      </div>
      <div class="photo" class:none={!recipe.images[0]}><Plate hash={recipe.images[0]} title={recipe.title} eager /></div>
    </header>

    <div class="body">
      {#if tabbed}
        <div class="tabs-at" bind:this={tabsAt}></div>
        <div class="tabs" bind:this={tabs}>
          <div class="tablist" role="tablist" aria-label="Recipe">
          <button role="tab" id="tab-ingredients" aria-label="Ingredients, {recipe.ingredients.filter((i) => i.item).length} items" aria-selected={tab === 'ingredients'} aria-controls="panel-ingredients" tabindex={tab === 'ingredients' ? 0 : -1} onkeydown={tabKey} onclick={() => show('ingredients')}>
            Ingredients<span class="count fig" aria-hidden="true">{recipe.ingredients.filter((i) => i.item).length}</span>
          </button>
          <button role="tab" id="tab-method" aria-label="Method, {recipe.steps.length} {recipe.steps.length === 1 ? 'step' : 'steps'}" aria-selected={tab === 'method'} aria-controls="panel-method" tabindex={tab === 'method' ? 0 : -1} onkeydown={tabKey} onclick={() => show('method')}>
            Method<span class="count fig" aria-hidden="true">{recipe.steps.length}</span>
          </button>
          </div>
          <a class="btn primary tab-cook" class:shown={tabCookShown} href={cookFrom(readingStep)} use:link aria-keyshortcuts="c" title={readingStep > 1 ? `Cook from step ${readingStep} (C)` : 'Begin cooking (C)'}>{readingStep > 1 ? `Cook from ${readingStep}` : 'Cook'}</a>
        </div>
      {/if}
      <!-- Keys scroll the column from Show as written, or from its foot. -->
      <section id="panel-ingredients" class="ingredients sheet" class:cooking={colCookShown} role={tabbed ? 'tabpanel' : undefined} aria-labelledby={tabbed ? 'tab-ingredients' : 'ing-h'} hidden={tabbed && tab !== 'ingredients'} bind:this={ingPanel}>
        <div class="col-head">
          <h2 id="ing-h" class="label">Ingredients{#if !tabbed}<span class="steps">{recipe.ingredients.filter((i) => i.item).length}</span>{/if}</h2>
          <button class="as-written" aria-keyshortcuts="a" title="Show as written or tidied (A)" onclick={() => (asWritten = !asWritten)}>{asWritten ? 'Show tidied' : 'Show as written'}</button>
        </div>
        {@render ingredientList()}
        {#if below || !narrow}
          <div class="below">
            {#if below}<button class="below-btn" class:raised={colCookShown} onclick={moreBelow} aria-label="{below} more {below === 1 ? 'ingredient' : 'ingredients'} below; show them">{below} more<ArrowDown size={15} aria-hidden="true" /></button>{/if}
            {#if !narrow}
              <div class="col-cook" class:shown={colCookShown} inert={!colCookShown}>
                <a class="btn primary" href={cookFrom(readingStep)} use:link aria-keyshortcuts="c" title="{readingStep > 1 ? `Cook from step ${readingStep}` : 'Begin cooking'} (C)">{readingStep > 1 ? `Cook from step ${readingStep}` : 'Begin cooking'}</a>
              </div>
            {/if}
          </div>
        {/if}
      </section>

      <section id="panel-method" class="method sheet" role={tabbed ? 'tabpanel' : undefined} aria-labelledby={tabbed ? 'tab-method' : 'method-h'} hidden={tabbed && tab !== 'method'} bind:this={methodPanel}>
        <!-- Notes come before the method: make-ahead and timing advice is
             read before you start, not found after the ingredients. -->
        {#if recipe.notes}
          <div class="col-head">
            <h2 class="label">Notes</h2>
            {#if notesLong}<button class="as-written" aria-expanded={notesOpen} aria-controls="notes" onclick={() => (notesOpen = !notesOpen)}>{notesOpen ? 'Show less' : 'Show all'}<span class="visually-hidden">{' of the notes'}</span></button>{/if}
          </div>
          <div class="notes-block" id="notes">
            {#if notesOpen}
              {#each notesInOrder as para}<p class="notes" class:credit={CREDIT.test(para)}><Markdown text={para} /></p>{/each}
            {:else}
              <p class="notes clamped" class:fade-end={notesClipped} bind:this={notesEl}><Markdown text={notesLead} /></p>
            {/if}
          </div>
        {/if}
        <div class="col-head" class:after-notes={recipe.notes}><h2 id="method-h" class="label">Method</h2>{#if !tabbed}<span class="label steps">{recipe.steps.length} {recipe.steps.length === 1 ? 'step' : 'steps'}</span>{/if}</div>
        {#each groups(recipe.steps) as g}
          {#if g.section}<h3 class="sect" data-sect={sectKey(g.section)}>{g.section}</h3>{/if}
          <ol role="list">
            {#each g.rows as s}
              <li><span class="n fig"><span class="visually-hidden">{'Step '}</span>{recipe.steps.indexOf(s) + 1}</span><p><Markdown text={s.text} /></p></li>
            {/each}
          </ol>
        {/each}

        {#if !endRedundant}
          <div class="method-end" bind:this={endCook}>
            <a class="btn cook" class:primary={!headInView} href="/r/{id}/cook" use:link>Begin cooking</a>
          </div>
        {/if}

        {#if recipe.nutrition_text}
          <h2 class="label aside-h">Nutrition</h2>
          {#if nutrition}
            <dl class="nutrition">
              {#each nutrition as n}
                <div><dt class="label">{n.label}</dt><dd><span class="fig">{figure(n.fig)}</span>{#if unitFor(n)}<span class="unit">{unitFor(n)}</span>{/if}</dd></div>
              {/each}
            </dl>
          {:else}
            <p class="notes"><Markdown text={recipe.nutrition_text} /></p>
          {/if}
        {/if}

        <p class="colophon">
          {#if recipe.created_by === '_sous'}A sample recipe that came with Sous{:else}Added by {person(recipe.created_by)} {ago(recipe.created_at)}{/if}{#if recipe.updated_at !== recipe.created_at}, last edited by {person(recipe.updated_by)} {ago(recipe.updated_at)}{/if}.
        </p>
      </section>
    </div>

    {#if tabbed && tab === 'method'}
      <div class="peek" class:shown={peekShown} inert={!peekShown}>
        <button class="btn peek-btn" aria-haspopup="dialog" onclick={openSheet}><ChevronUp size={18} aria-hidden="true" />Ingredients<span class="count fig" aria-hidden="true">{recipe.ingredients.filter((i) => i.item).length}</span></button>
      </div>
    {/if}
    {#if tabbed}
      <dialog class="ing-sheet" bind:this={sheet} aria-labelledby="sheet-h" class:tall={sheetTall} class:dragging style:transform={drag > 0 ? `translateY(${drag}px)` : null} style:max-height={drag < 0 ? `min(85dvh, ${dragBase - drag}px)` : null} onclick={(e) => e.target === sheet && closeSheet()} onclose={sheetClosed}>
        <!-- Pull the head up for more of the list or down to close, as with
             any sheet; the handle is also a button, and Escape and Back to the
             method close it from the keyboard. -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="sheet-head" onpointerdown={dragStart} onpointermove={dragMove} onpointerup={dragEnd} onpointercancel={dragEnd}>
          <button class="handle" aria-label={sheetTall ? 'Show less of the list' : 'Show more of the list'} aria-expanded={sheetTall} onclick={() => (sheetTall = !sheetTall)}><span aria-hidden="true"></span></button>
          <div class="sheet-title">
            <h2 id="sheet-h" class="label">Ingredients</h2>
            {#if above}<button class="as-written" aria-label="Back up to {above.name}" onclick={toAbove}>{above.name}<ArrowUp size={14} aria-hidden="true" /></button>{/if}
          </div>
        </div>
        <div class="sheet-in" bind:this={sheetList} onscroll={sheetScrolled}>
          {@render ingredientList()}
        </div>
        <!-- In thumb reach, and always in view, so focus lands on Back to the
             method without scrolling the list. Cook from the step you were
             reading hands over to cook mode. -->
        <div class="sheet-foot">
          {#if readingStep > 1}<p class="sheet-at" id="sheet-at"><span class="fig">Step {readingStep}</span> · {stepWords(readingStep)}</p>{/if}
          <!-- svelte-ignore a11y_autofocus -->
          <button class="btn sheet-close" autofocus onclick={closeSheet}>Back to the method</button>
          <a class="btn sheet-cook" href={cookFrom(readingStep)} use:link aria-describedby={readingStep > 1 ? 'sheet-at' : undefined}>{readingStep > 1 ? `Cook from step ${readingStep}` : 'Begin cooking'}</a>
        </div>
      </dialog>
    {/if}
  </main>
{/if}

<dialog class="keys" bind:this={keysDialog} aria-labelledby="keys-h" onclick={(e) => e.target === keysDialog && keysDialog.close()}>
  <h2 id="keys-h" class="label">Keys on a recipe</h2>
  <dl>
    {#each KEYS as [key, what]}<div><dt><kbd>{key}</kbd></dt><dd>{what}</dd></div>{/each}
  </dl>
  <form method="dialog"><button class="btn">Done</button></form>
</dialog>

{#snippet ingredientList()}
  {#each groups(recipe.ingredients) as g}
    {#if g.section}<h3 class="sect" data-sect={sectKey(g.section)}>{g.section}</h3>{/if}
    <ul role="list">
      {#each g.rows as i}
        {#if asWritten}
          <li class="raw">{i.raw}</li>
        {:else}
          <li>
            <span class="amt fig">{amount(i)}</span>
            <span class="item">{i.item}{#if i.note}<span class="note">, {i.note}</span>{/if}</span>
          </li>
        {/if}
      {/each}
    </ul>
  {/each}
{/snippet}

<style>
  .page {
    max-width: 80rem;
    margin: 0 auto;
    padding: var(--space-3) var(--gutter) var(--space-8);
  }
  .loading {
    min-height: 60vh;
  }
  .crumbs {
    display: flex;
    justify-content: space-between;
    margin-bottom: var(--space-3);
  }
  .back {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 40px;
    font-size: var(--ui);
    font-weight: 600;
    text-decoration: none;
    color: var(--ink-2);
  }
  .back:hover {
    color: var(--red);
  }

  .page > .rail {
    position: relative;
    z-index: 1;
  }
  .clip {
    position: absolute;
    top: 0;
    left: 50%;
    width: 44px;
    height: 10px;
    margin-left: -22px;
    border-radius: 0 0 3px 3px;
    background: linear-gradient(var(--steel), var(--steel-lo));
  }
  .top {
    position: relative;
    margin: -4px var(--space-4) 0;
    display: grid;
    grid-template-columns: minmax(0, 7fr) minmax(0, 5fr);
    gap: var(--space-6);
    padding: var(--space-6) var(--space-6) calc(var(--space-5) + var(--tear));
  }
  h1 {
    font-size: var(--title-xl);
    font-weight: 800;
    line-height: 1.04;
    letter-spacing: -0.025em;
  }
  .src {
    margin-top: var(--space-2);
    font-size: var(--ui);
    color: var(--ink-2);
  }
  .src {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    align-self: start;
    gap: 0 var(--space-3);
  }
  .dot {
    color: var(--ink-3);
  }
  /* 44px targets on a text line: the extra height is taken back from the
     margins, so the line keeps its height. */
  .src a {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    min-height: 44px;
    margin: -10px 0;
  }
  .src.bare {
    display: none;
  }
  .src-line {
    display: inline-flex;
    align-items: center;
    gap: var(--space-4);
  }
  .src .edit-here {
    display: none;
    font-weight: 600;
    color: var(--ink-2);
    text-decoration: none;
  }
  .src .edit-here:hover {
    color: var(--red);
  }
  .facts {
    margin-top: var(--space-5);
  }
  /* Without a photo the header is the full width, but the facts' rule spans
     only the figures, not a band of empty paper. */
  @media (min-width: 761px) {
    .top:not(.has-photo) .facts {
      width: fit-content;
      min-width: min(100%, 32rem);
    }
  }
  .desc-wrap {
    margin-top: var(--space-4);
    max-width: 65ch;
  }
  /* Held to a few lines. The last one fades out rather than ending in an
     ellipsis, which landed after a full stop or a hyphen (".…", "co-…"). */
  .desc {
    --lines: 3;
    color: var(--ink-2);
    max-height: calc(var(--lines) * 1lh);
    overflow: hidden;
  }
  .desc.open {
    max-height: none;
  }
  /* Collapsed, Show all sits at the end of the last line, where the text has
     faded out, rather than on a line of its own. */
  .desc-wrap.clipped {
    position: relative;
  }
  .desc-wrap.clipped .more {
    position: absolute;
    right: 0;
    bottom: calc((1lh - 44px) / 2);
    margin: 0;
    padding: 0 0 0 var(--space-4);
    background: var(--ticket);
  }
  .fade-end {
    mask-image: linear-gradient(#000, #000), linear-gradient(to right, #000 55%, transparent 92%);
    mask-size:
      100% calc(100% - 1lh),
      100% 1lh;
    mask-position: top, bottom;
    mask-repeat: no-repeat;
  }
  /* A 44px target around a text-sized link: the extra height is taken back
     from the margins so the line sits where it did. */
  .more {
    min-height: 44px;
    margin: calc(var(--space-1) - 10px) 0 -10px calc(var(--space-3) * -1);
    padding: 0 var(--space-3);
    border: 0;
    background: none;
    font-size: var(--ui);
    font-weight: 600;
    color: var(--ink);
    text-decoration: underline;
    text-decoration-color: var(--ink-3);
    text-decoration-thickness: 1.5px;
    text-underline-offset: 0.2em;
  }
  .more:hover {
    color: var(--red);
    text-decoration-color: var(--red);
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-top: var(--space-5);
  }
  .cook {
    min-height: 52px;
    padding: 0 var(--space-6);
    font-size: 1.0625rem;
  }
  .acts .btn:not(.cook) {
    min-height: 52px;
  }
  .photo {
    align-self: start;
  }
  /* No photo: the header text takes the width, not a grey slot. */
  .top:not(.has-photo) {
    grid-template-columns: minmax(0, 1fr);
  }
  .photo.none {
    display: none;
  }
  /* Held short so the recipe starts in the first screen on a laptop or an
     iPad in landscape. */
  .photo :global(.plate) {
    max-height: min(22rem, 46vh);
  }

  .body {
    display: grid;
    grid-template-columns: minmax(17rem, 23rem) minmax(0, 1fr);
    gap: var(--space-4);
    margin-top: var(--space-4);
    align-items: start;
  }
  .tabs {
    position: sticky;
    top: var(--bar-shown, 0px);
    z-index: 4;
    display: flex;
    margin: 0 calc(var(--gutter) * -1) calc(var(--space-4) * -1);
    padding: 0 var(--gutter);
    background: var(--pass);
    border-bottom: 1px solid var(--perf);
    transition: top 240ms var(--ease-out);
  }
  /* The tabs take their own width, a tab bar's, with Cook at the far end;
     halves would be uneven once Cook claims its space. */
  .tablist {
    display: flex;
    gap: var(--space-6);
  }
  /* Shifted down past the grid gap, so it marks the strip's own top. */
  .tabs-at {
    position: relative;
    top: var(--space-4);
  }
  .tab-cook {
    align-self: center;
    min-height: 44px;
    margin-block: var(--space-1);
    margin-left: auto;
    visibility: hidden;
  }
  .tab-cook.shown {
    visibility: visible;
  }
  .tabs button {
    display: inline-flex;
    justify-content: center;
    align-items: baseline;
    gap: var(--space-2);
    min-height: 52px;
    padding: var(--space-3) var(--space-1) 0;
    border: 0;
    border-bottom: 3px solid transparent;
    background: none;
    font-size: var(--ui);
    font-weight: 700;
    color: var(--ink-2);
  }
  .tabs button[aria-selected='true'] {
    color: var(--ink);
    border-bottom-color: var(--ink);
  }
  .count {
    color: var(--ink-3);
  }
  .sheet {
    background: var(--ticket);
    padding: var(--space-4) var(--space-5) var(--space-6);
  }
  /* Room under the tab strip, so a tab's label doesn't sit against it. */
  .sheet[role='tabpanel'] {
    padding-top: var(--space-5);
  }
  h2 .steps {
    margin-left: var(--space-2);
  }
  @media (min-width: 901px) {
    /* In the column too, a group's heading stays at the top while its rows
       scroll under it (the edge sits inside the panel's padding). */
    .ingredients .sect {
      position: sticky;
      top: calc(var(--space-4) * -1);
      z-index: 1;
      background: var(--ticket);
    }
    /* Paper under a stuck heading, as deep as a row's own top space, then a
       short fade, so a row passing under it goes rather than being cut. */
    .ingredients .sect::after {
      content: '';
      position: absolute;
      inset: 100% 0 auto;
      height: var(--space-3);
      background: linear-gradient(var(--ticket) var(--space-2), transparent);
      pointer-events: none;
    }
    /* Without a photo the ticket is as wide as what's on it, not a band of
       empty paper; it hangs at the middle of the rail. */
    .top:not(.has-photo) {
      max-width: 52rem;
      margin-inline: auto;
    }
  }
  /* A laptop or an iPad in landscape: a shorter header so the recipe starts
     in the first screen. */
  @media (max-width: 1180px) and (min-width: 901px) {
    .top {
      padding-top: var(--space-5);
    }
    .desc {
      --lines: 2;
    }
    .method .col-head.after-notes {
      margin-top: var(--space-5);
    }
  }
  @media (min-width: 1280px) {
    .body {
      grid-template-columns: minmax(17rem, 27rem) minmax(0, 1fr);
    }
  }
  /* Side by side, the ingredients stay on screen the whole way down the
     method: sticky, and scrolling inside themselves when the list is longer
     than the window. Show as written, their one control, is at the top. */
  .ingredients {
    position: sticky;
    top: calc(var(--bar) + var(--space-4));
    max-height: calc(100dvh - var(--bar) - var(--space-6));
    overflow-y: auto;
    overscroll-behavior: contain;
    scrollbar-width: thin;
    scrollbar-color: var(--ink-3) transparent;
  }
  /* The column's foot when the list runs on below it: a fade, and how many
     rows are under it. It takes no room, so the list doesn't move. */
  .below {
    z-index: 2;
    position: sticky;
    /* Sticky edges sit inside the panel's padding; this takes it back. */
    bottom: calc(var(--space-6) * -1);
    height: 0;
  }
  .below-btn {
    position: absolute;
    bottom: 0;
    left: calc(var(--space-5) * -1);
    right: calc(var(--space-5) * -1);
    display: flex;
    justify-content: center;
    align-items: center;
    gap: var(--space-1);
    min-height: 44px;
    padding: var(--space-6) 0 var(--space-2);
    border: 0;
    border-bottom: 1px solid var(--perf);
    background: linear-gradient(to bottom, transparent, var(--ticket) 55%);
    font-size: 0.875rem;
    font-weight: 700;
    color: var(--ink-2);
  }
  .below-btn:hover {
    color: var(--red);
  }
  /* Side by side, mid-method, the foot offers the hand-off to cook mode on a
     band of its own; the list makes room for it, and "N more" sits above. */
  .ingredients {
    --cook-band: 4.5rem;
  }
  /* The room is the foot's own margin, not the panel's padding: a sticky
     foot can't sink into padding, so at the list's end the band would float
     above the panel's edge. */
  .ingredients.cooking {
    padding-bottom: 0;
  }
  .ingredients.cooking .below {
    margin-top: calc(var(--space-6) + var(--cook-band));
    bottom: 0;
  }
  .below-btn.raised {
    bottom: var(--cook-band);
    border-bottom: 0;
  }
  .col-cook {
    position: absolute;
    bottom: 0;
    left: calc(var(--space-5) * -1);
    right: calc(var(--space-5) * -1);
    height: var(--cook-band);
    display: flex;
    align-items: center;
    padding: 0 var(--space-5);
    border-top: 1px solid var(--perf);
    background: var(--ticket);
    opacity: 0;
    visibility: hidden;
    transition:
      opacity 160ms var(--ease-out),
      visibility 0s 160ms;
  }
  .col-cook.shown {
    opacity: 1;
    visibility: visible;
    transition: opacity 160ms var(--ease-out);
  }
  .col-cook .btn {
    flex: 1;
    min-height: 48px;
  }
  .col-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    min-height: 2.25rem;
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
  }
  .steps {
    color: var(--ink-3);
  }
  .as-written {
    min-height: 44px;
    margin: calc(var(--space-2) * -1) 0;
    border: 0;
    background: none;
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--ink-2);
    text-decoration: underline;
    text-decoration-color: var(--ink-3);
    text-decoration-thickness: 1.5px;
    text-underline-offset: 0.2em;
    padding: 0;
  }
  .as-written:hover {
    color: var(--red);
    text-decoration-color: var(--red);
  }
  /* A section opens on the heavy rule, at reading size. The first one sits
     straight under the panel's own rule, so it doesn't draw a second. */
  .sect {
    font-size: 1.0625rem;
    font-weight: 800;
    margin: var(--space-6) 0 0;
    padding: var(--space-2) 0 var(--space-1);
    border-top: 3px solid var(--ink);
  }
  .col-head + .sect {
    margin-top: var(--space-4);
    padding-top: 0;
    border-top: 0;
  }
  .ingredients li,
  .ing-sheet li {
    display: grid;
    grid-template-columns: 5.25rem 1fr;
    gap: var(--space-3);
    padding: var(--space-2) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
    line-height: 1.4;
  }
  .ingredients li.raw,
  .ing-sheet li.raw {
    display: block;
  }
  .amt {
    font-size: 1.1875rem;
    padding-top: 1px;
  }
  .note {
    color: var(--ink-3);
  }

  .method li {
    display: grid;
    /* 58ch of Archivo is about 78 characters: ch is the width of a zero,
       wider than the average letter. */
    grid-template-columns: 3rem minmax(0, 58ch);
    gap: var(--space-3);
    padding: var(--space-4) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
  }
  .n {
    font-size: 2rem;
  }
  .aside-h {
    margin-top: var(--space-7);
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
  }
  .notes {
    margin-top: var(--space-3);
    max-width: 58ch;
    white-space: pre-line;
  }
  .notes-block {
    color: var(--ink-2);
  }
  .notes.credit {
    color: var(--ink-3);
    font-size: 0.9375rem;
  }
  .notes.clamped {
    max-height: 2lh;
    overflow: hidden;
  }
  .col-head.after-notes {
    margin-top: var(--space-6);
  }
  .method-end {
    margin-top: var(--space-6);
  }
  .nutrition {
    display: flex;
    flex-wrap: wrap;
    margin: var(--space-3) 0 0;
  }
  .nutrition > div {
    min-width: 6rem;
    padding: var(--space-2) var(--space-5) var(--space-3) 0;
  }
  .nutrition dt {
    color: var(--ink-3);
    margin-bottom: var(--space-1);
  }
  .nutrition dd {
    margin: 0;
  }
  .nutrition .fig {
    font-size: 1.625rem;
  }
  .nutrition .unit {
    margin-left: 3px;
    font-size: 0.9375rem;
    font-weight: 700;
  }

  /* The ingredients peek: a bar at the foot of the phone screen in the
     Method tab, and the sheet it opens. */
  .edit-here,
  .peek {
    display: none;
  }
  .peek {
    position: fixed;
    inset: auto 0 0;
    z-index: 5;
    padding: var(--space-2) var(--gutter) calc(var(--space-2) + env(safe-area-inset-bottom));
    background: var(--ticket);
    border-top: 1px solid var(--perf);
    transform: translateY(100%);
    transition: transform 200ms var(--ease-out);
  }
  .peek.shown {
    transform: none;
  }
  .peek-btn {
    width: 100%;
    min-height: 52px;
    gap: var(--space-2);
  }
  .ing-sheet {
    position: fixed;
    inset: auto 0 0;
    width: 100%;
    max-width: none;
    max-height: min(58dvh, 100dvh - 6rem);
    transition:
      transform 160ms var(--ease-out),
      max-height 200ms var(--ease-out);
    margin: 0;
    padding: 0;
    border: 0;
    border-top: 3px solid var(--ink);
    background: var(--ticket);
    color: var(--ink);
    flex-direction: column;
  }
  .ing-sheet.tall {
    max-height: 85dvh;
  }
  .ing-sheet[open] {
    display: flex;
  }
  .keys {
    width: min(26rem, calc(100% - 2 * var(--gutter)));
    padding: var(--space-5);
    border: 0;
    border-top: 3px solid var(--ink);
    background: var(--ticket);
    color: var(--ink);
  }
  .keys::backdrop {
    background: rgb(20 24 28 / 0.4);
  }
  .keys h2 {
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
  }
  .keys dl {
    margin: var(--space-2) 0 var(--space-4);
  }
  .keys dl > div {
    display: grid;
    grid-template-columns: 2.5rem 1fr;
    gap: var(--space-3);
    align-items: baseline;
    padding: var(--space-2) 0;
    border-bottom: 1.5px dashed var(--ticket-press);
  }
  .keys dd {
    margin: 0;
  }
  kbd {
    display: inline-block;
    min-width: 1.75rem;
    padding: 1px var(--space-1);
    border: 1.5px solid var(--ink-3);
    border-radius: 3px;
    font-family: inherit;
    font-size: 0.875rem;
    font-weight: 700;
    line-height: 1.4;
    text-align: center;
  }
  .keys form {
    display: flex;
    justify-content: flex-end;
  }
  /* The head is the grab: a handle, and the label under it. The handle is
     a button too, a 44px strip, for more or less of the list. */
  .sheet-head {
    flex: none;
    padding: 0 var(--gutter);
    touch-action: none;
    cursor: grab;
  }
  .handle {
    display: flex;
    justify-content: center;
    align-items: center;
    width: 100%;
    min-height: 32px;
    padding: 0;
    border: 0;
    background: none;
    cursor: inherit;
  }
  .handle span {
    width: 40px;
    height: 4px;
    border-radius: 2px;
    background: var(--perf);
  }
  .handle:hover span {
    background: var(--ink-3);
  }
  .sheet-title {
    display: flex;
    justify-content: space-between;
    align-items: center;
    min-height: 2.25rem;
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
  }
  .sheet-title .as-written {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
  }
  .ing-sheet.dragging {
    transition: none;
  }
  .sheet-in {
    position: relative;
    flex: 1;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: 0 var(--gutter) var(--space-4);
  }
  .sheet-in > .sect:first-child {
    margin-top: var(--space-4);
    padding-top: 0;
    border-top: 0;
  }
  /* A group's heading stays at the top while its rows scroll under it; its
     rule tucks under the sheet's, so only one shows. */
  .sheet-in .sect {
    position: sticky;
    top: -3px;
    z-index: 1;
    background: var(--ticket);
  }
  .sheet-in .sect::after {
    content: '';
    position: absolute;
    inset: 100% 0 auto;
    height: var(--space-3);
    background: linear-gradient(var(--ticket) var(--space-2), transparent);
    pointer-events: none;
  }
  .sheet-in > .sect:first-child {
    top: 0;
  }
  .sheet-foot {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-2);
    flex: none;
    padding: var(--space-3) var(--gutter) calc(var(--space-3) + env(safe-area-inset-bottom));
    border-top: 1px solid var(--perf);
  }
  .ing-sheet::backdrop {
    background: rgb(20 24 28 / 0.4);
  }
  .ing-sheet[open] {
    animation: rise 200ms var(--ease-out);
  }
  @keyframes rise {
    from {
      transform: translateY(24px);
      opacity: 0;
    }
  }
  .sheet-close,
  .sheet-cook {
    width: 100%;
    min-height: 52px;
  }
  /* Which step Cook from means, by its first words: the step itself may be
     under the sheet's top edge. */
  .sheet-at {
    grid-column: 1 / -1;
    margin: 0 0 var(--space-1);
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
    font-size: 0.9375rem;
    color: var(--ink-2);
  }
  .sheet-at .fig {
    font-weight: 700;
    color: var(--ink);
  }
  @media (prefers-reduced-motion: reduce) {
    .peek,
    .ing-sheet[open] {
      transition: none;
      animation: none;
    }
  }
  .colophon {
    margin-top: var(--space-7);
    font-size: 0.875rem;
    color: var(--ink-3);
  }
  .trashed {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3) var(--space-5);
    margin-bottom: var(--space-4);
    padding: var(--space-3) var(--space-4);
    background: var(--ticket);
    border-top: 3px solid var(--ink);
    font-weight: 600;
  }
  .missing {
    max-width: 36rem;
    margin: var(--space-8) auto;
    padding: 0 var(--gutter);
    display: grid;
    gap: var(--space-4);
    justify-items: start;
  }
  .missing h1 {
    font-size: var(--title-l);
  }

  @media (max-width: 900px) {
    .top {
      grid-template-columns: 1fr;
      margin: -4px var(--space-2) 0;
      padding: var(--space-5) var(--space-4) calc(var(--space-5) + var(--tear));
    }
    /* The photo is a thumbnail beside the title, so the first screen reaches
       the ingredients instead of a band of photo: on a phone, and on an iPad
       held upright. */
    .top.has-photo {
      grid-template-columns: minmax(0, 1fr) 112px;
      grid-template-rows: auto 1fr; /* the photo's extra height goes under the source line */
      column-gap: var(--space-5);
      row-gap: 0;
    }
    .has-photo .head {
      display: contents;
    }
    .has-photo .head > :global(*) {
      grid-column: 1 / -1;
    }
    .has-photo .head h1 {
      grid-column: 1;
      grid-row: 1;
    }
    .has-photo .head .src {
      grid-column: 1;
      grid-row: 2;
    }
    .has-photo .photo {
      grid-column: 2;
      grid-row: 1 / 3;
    }
    .photo :global(.plate) {
      aspect-ratio: 1 !important;
      max-height: none;
    }
    /* Show as written is the panel's only visible head here (the tab names
       it), so it sits at the right like a tool, not alone at the start. */
    .as-written,
    .steps {
      margin-left: auto;
    }
    .body {
      grid-template-columns: 1fr;
    }
    .ingredients {
      position: static;
      max-height: none;
      overflow: visible;
    }
    .peek {
      display: block;
    }
    /* Clear of the peek bar at the foot of the screen. */
    .method[role='tabpanel'] {
      padding-bottom: calc(var(--space-6) + 68px);
    }
    .ingredients,
    .method {
      padding-left: var(--space-4);
      padding-right: var(--space-4);
    }
    .method li {
      grid-template-columns: 2.25rem minmax(0, 58ch);
    }
  }
  @media (max-width: 760px) {
    /* Tags and source each take a line, so no separator is left hanging at
       the end of the tags when the source wraps. With no source, Edit ends
       the tags' line instead. */
    .src.sourced {
      flex-direction: column;
      align-items: flex-start;
    }
    .dot {
      display: none;
    }
    .method-end .cook {
      width: 100%;
    }
    /* The masthead already leads back to the library, so the crumb row goes
       and Edit recipe moves into the header, under the title. */
    .crumbs {
      display: none;
    }
    .page {
      padding-top: var(--space-4);
    }
    .src .edit-here {
      display: inline-flex;
    }
    .src.bare {
      display: flex;
    }
    .desc {
      --lines: 2;
    }
    /* Begin cooking and the two marks share one row: the marks as 52px icon
       buttons, named for screen readers. */
    /* A thumb's width between Begin cooking and the marks. */
    .acts {
      flex-wrap: nowrap;
      gap: var(--space-3);
    }
    .acts .cook {
      flex: 1;
      width: auto;
    }
    .acts .mark {
      flex: none;
      width: 52px;
      padding: 0;
    }
    .mark-name {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }
    .top.has-photo {
      grid-template-columns: minmax(0, 1fr) 88px;
      column-gap: var(--space-4);
    }
    /* With no photo and no source line, Edit sits at the end of the title's
       row rather than alone on a line of its own. */
    .top:not(.has-photo) .head:has(.src.bare) {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      column-gap: var(--space-4);
    }
    .top:not(.has-photo) .head > :global(*) {
      grid-column: 1 / -1;
    }
    .top:not(.has-photo) .head:has(.src.bare) h1 {
      grid-column: 1;
      grid-row: 1;
    }
    .top:not(.has-photo) .head .src.bare {
      grid-column: 2;
      grid-row: 1;
      margin-top: 0;
      align-self: start;
    }
  }
  /* An iPad held upright: the Ingredients bar is a button at the corner, not
     a band across the screen, and the steps are set larger for reading at
     arm's length from the counter. */
  @media (min-width: 761px) and (max-width: 900px) {
    .method li p {
      font-size: 1.1875rem;
      line-height: 1.5;
    }
    /* Lines end short of the Ingredients button in the corner. */
    .method[role='tabpanel'] li {
      grid-template-columns: 2.25rem minmax(0, 50ch);
    }
    .method[role='tabpanel'] .notes {
      max-width: 50ch;
    }
    .peek {
      left: auto;
      right: var(--gutter);
      bottom: var(--space-4);
      padding: 0;
      border: 0;
      background: none;
      transform: translateY(calc(100% + var(--space-4)));
    }
    .peek-btn {
      width: auto;
      padding: 0 var(--space-6);
      background: var(--ticket);
    }
  }
</style>
