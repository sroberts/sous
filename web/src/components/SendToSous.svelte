<script>
  // Ways to send the page you're reading to /import without copying links:
  // a bookmarklet for desktop browsers and a Shortcut for the iOS Share sheet.
  import { Bookmark } from '@lucide/svelte';

  const host = location.host;

  // Svelte refuses javascript: URLs in markup, so the href is set here.
  function bookmarklet(node) {
    node.setAttribute('href', `javascript:location.href='${location.origin}/import?url='+encodeURIComponent(location.href)`);
  }
</script>

<section class="send" aria-labelledby="send-h">
  <h2 id="send-h">Send recipes from anywhere</h2>
  <p class="lede">Import the page you're reading in one step, without copying its link.</p>

  <div class="ways">
    <div class="way">
      <h3 class="label">Desktop browser</h3>
      <p>Drag this button to your bookmarks bar. On a recipe page, click the bookmark and Sous opens here with the recipe read in.</p>
      <a class="btn bookmarklet" href="/import" use:bookmarklet draggable="true" onclick={(e) => e.preventDefault()} title="Drag to your bookmarks bar">
        <Bookmark aria-hidden="true" />Send to Sous
      </a>
      <p class="hint">Clicking it on this page does nothing. It works from the recipe's page.</p>
    </div>

    <div class="way">
      <h3 class="label">iPhone and iPad</h3>
      <p>Build a shortcut once in the Shortcuts app, then send any page from Safari's Share button.</p>
      <ol class="steps">
        <li><span class="n fig" aria-hidden="true">1</span><span>New shortcut. In its details, turn on <b>Show in Share Sheet</b> and have it receive <b>URLs</b> and <b>Safari web pages</b>.</span></li>
        <li><span class="n fig" aria-hidden="true">2</span><span>Add <b>Get URLs from Input</b>.</span></li>
        <li><span class="n fig" aria-hidden="true">3</span><span>Add <b>URL Encode</b>, set to encode the URLs.</span></li>
        <li><span class="n fig" aria-hidden="true">4</span><span>Add <b>Text</b>: <code>https://{host}/import?url=</code> followed by the <b>URL Encoded Text</b> variable.</span></li>
        <li><span class="n fig" aria-hidden="true">5</span><span>Add <b>Open URLs</b>, and name the shortcut “Send to Sous”.</span></li>
      </ol>
      <p class="hint">Tailscale has to be connected, and Safari signed in to Sous through the gateway.</p>
    </div>
  </div>
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
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 20rem), 1fr));
    gap: var(--space-4);
    margin-top: var(--space-5);
    align-items: start;
  }
  .way {
    display: grid;
    gap: var(--space-3);
    justify-items: start;
    background: var(--ticket);
    padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .way .label {
    color: var(--ink-3);
    justify-self: stretch;
    padding-bottom: var(--space-2);
    border-bottom: 1.5px dashed var(--perf);
  }
  .way p {
    max-width: 60ch;
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
