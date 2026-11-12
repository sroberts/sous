<script>
  // Ways to send the page you're reading to /import without copying links:
  // a bookmarklet for desktop browsers and a Shortcut for the iOS Share sheet, and a Siri Shortcut that saves without opening Sous.
  import { Bookmark, ChevronDown } from '@lucide/svelte';
  import SaveToSous from './SaveToSous.svelte';
  import { link } from '../lib/router.svelte.js';

  const origin = location.origin;

  // Svelte refuses javascript: URLs in markup, so the href is set here.
  function bookmarklet(node) {
    node.setAttribute('href', `javascript:location.href='${origin}/import?url='+encodeURIComponent(location.href)`);
  }
</script>

<section class="send" aria-labelledby="send-h">
  <h2 id="send-h">Send recipes from anywhere</h2>
  <p class="lede">Import the page you’re reading in one step, without copying its link.</p>

  <!-- Three set-ups, each done once: closed until someone asks for one, so
       the page's own job (the link field above) stays the first thing. -->
  <div class="ways">
    <details class="way">
      <summary><h3>Desktop browser</h3><span class="what">A bookmark that sends the page you’re on</span><ChevronDown class="chev" size={20} aria-hidden="true" /></summary>
      <div class="inner">
      <p>Drag this button to your bookmarks bar. On a recipe page, click the bookmark and Sous opens here with the recipe read in.</p>
      <a class="btn bookmarklet" href="/import" use:bookmarklet draggable="true" onclick={(e) => e.preventDefault()} title="Drag to your bookmarks bar">
        <Bookmark aria-hidden="true" />Send to Sous
      </a>
      <p class="hint">Clicking it on this page does nothing. It works from the recipe’s page.</p>
      </div>
    </details>

    <details class="way">
      <summary><h3>iPhone and iPad</h3><span class="what">A Share-sheet shortcut that opens the recipe here</span><ChevronDown class="chev" size={20} aria-hidden="true" /></summary>
      <div class="inner">
      <p>Build a shortcut once in the Shortcuts app, then send any page from Safari’s Share button.</p>
      <ol class="steps">
        <li><span class="n fig" aria-hidden="true">1</span><span>New shortcut. In its details, turn on <b>Show in Share Sheet</b> and have it receive <b>URLs</b> and <b>Safari web pages</b>.</span></li>
        <li><span class="n fig" aria-hidden="true">2</span><span>Add <b>Get URLs from Input</b>.</span></li>
        <li><span class="n fig" aria-hidden="true">3</span><span>Add <b>URL Encode</b>, set to encode the URLs.</span></li>
        <li><span class="n fig" aria-hidden="true">4</span><span>Add <b>Text</b>: <code>{origin}/import?url=</code> followed by the <b>URL Encoded Text</b> variable.</span></li>
        <li><span class="n fig" aria-hidden="true">5</span><span>Add <b>Open URLs</b>, and name the shortcut “Send to Sous”.</span></li>
      </ol>
      <p class="hint">Tailscale has to be connected, and Safari signed in to Sous through the gateway. To save without opening Sous, use the Siri set-up below.</p>
      </div>
    </details>

    <details class="way">
      <summary><h3>Siri</h3><span class="what">A shortcut that saves the recipe without opening Sous</span><ChevronDown class="chev" size={20} aria-hidden="true" /></summary>
      <SaveToSous />
    </details>
  </div>
  <p class="more">An AI assistant can import links too, and search and edit the library: <a href="/assistants" use:link>connect one</a>.</p>
</section>

<style>
  .send {
    margin-top: var(--space-7);
  }
  h2 {
    font-size: var(--title-m);
    font-weight: 800;
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
  }
  .lede {
    margin-top: var(--space-3);
    color: var(--ink-2);
    max-width: 60ch;
  }
  .ways {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-5);
  }
  .way {
    background: var(--ticket);
  }
  summary {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-1) var(--space-3);
    min-height: 56px;
    padding: var(--space-3) calc(var(--space-5) + 28px) var(--space-3) var(--space-5);
    position: relative;
    cursor: pointer;
    list-style: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  summary:hover {
    background: var(--ticket-press);
  }
  summary h3 {
    font-size: var(--ui);
    font-weight: 800;
  }
  .what {
    font-size: 0.9375rem;
    color: var(--ink-2);
  }
  summary :global(.chev) {
    position: absolute;
    right: var(--space-4);
    top: 50%;
    margin-top: -10px;
    color: var(--ink-2);
    transition: transform 200ms var(--ease-out);
  }
  .way[open] summary :global(.chev) {
    transform: rotate(180deg);
  }
  .way[open] summary {
    border-bottom: 1.5px dashed var(--perf);
  }
  .way > :global(.inner) {
    padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .inner {
    display: grid;
    gap: var(--space-3);
    justify-items: start;
  }
  .way p {
    max-width: 60ch;
  }
  .more {
    margin-top: var(--space-4);
    font-size: 0.9375rem;
    color: var(--ink-2);
  }
  .more a {
    font-weight: 700;
    color: var(--ink);
  }
  .bookmarklet {
    cursor: grab;
  }
  .hint {
    font-size: 0.8125rem;
    color: var(--ink-3);
  }
  .steps {
    display: grid;
    gap: var(--space-2);
    justify-self: stretch;
  }
  .steps li {
    display: grid;
    grid-template-columns: 1.75rem minmax(0, 1fr);
    gap: var(--space-2);
    align-items: baseline;
    padding-bottom: var(--space-2);
    border-bottom: 1.5px dashed var(--ticket-press);
  }
  .steps .n {
    font-size: 1.375rem;
  }
  b {
    font-weight: 700;
  }
  code {
    font: inherit;
    font-weight: 600;
    overflow-wrap: anywhere;
    background: var(--ticket-press);
    padding: 1px 4px;
    border-radius: 2px;
  }
</style>
