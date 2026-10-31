<script>
  // Setup card for a Siri Shortcut that saves a recipe link without opening
  // Sous: it posts the link to /v1/recipes/import and speaks the reply.
  import { session } from '../lib/session.svelte.js';

  const origin = location.origin;
  const phoneCannotReach = ['127.0.0.1', 'localhost', '[::1]', '::1'].includes(location.hostname);
  const username = $derived(session.person?.username);
  const who = $derived(username ?? '<username>');
</script>

<div class="way">
  <h3 class="label">Save without opening Sous (Siri)</h3>
  <p>A second shortcut saves the recipe for you and Siri tells you what happened. Say “Hey Siri, Save to Sous” or use the Share button.</p>
  {#if phoneCannotReach}
    <p class="warn">You're on <code>{location.hostname}</code>, which your phone can't reach. Open Sous at the address your phone uses, then come back to this page.</p>
  {/if}
  <ol class="steps">
    <li><span class="n fig" aria-hidden="true">1</span><span>New shortcut named “Save to Sous”. Turn on <b>Show in Share Sheet</b> and have it receive <b>URLs</b> and <b>Safari web pages</b>. If there's no input, <b>Ask For</b> <b>URLs</b>.</span></li>
    <li><span class="n fig" aria-hidden="true">2</span><span>Add <b>Get URLs from Input</b>.</span></li>
    <li><span class="n fig" aria-hidden="true">3</span><span>Add <b>Get Contents of URL</b>: <code>{origin}/v1/recipes/import</code>, Method <b>POST</b>, Request Body <b>JSON</b> with a Text key <code>url</code> set to the <b>URLs</b> variable. Add the header below.</span></li>
    <li><span class="n fig" aria-hidden="true">4</span><span>Add <b>Get Dictionary Value</b> for <code>summary</code>.</span></li>
    <li><span class="n fig" aria-hidden="true">5</span><span><b>If</b> it <b>has any value</b>, <b>Speak Text</b> it. <b>Otherwise</b>, speak “Sous said: ” followed by <b>Contents of URL</b>.</span></li>
  </ol>

  {#if session.outpost}
    <p>Header: <code>Authorization</code> = <code>Bearer</code> and your token. On the machine running Outpost, make a token:</p>
    <p><code>outpost token create {who} --name siri --app sous</code></p>
    <p class="hint">Add <code>--expires 90d</code> to have it lapse. The token is shown once. List yours with <code>outpost token list {who}</code> and cancel one with <code>outpost token revoke &lt;id&gt;</code>.</p>
    <p class="hint">The token can do anything you can do in Sous, so revoke it if the phone is lost.</p>
  {:else}
    <p>Header: <code>X-Sous-Person</code> = <code>{who}</code>.{#if !username}{' '}Pick who's cooking first.{/if}</p>
    <p class="hint">This names who is saving; it isn't a password. Your phone has to reach Sous through <code>tailscale serve</code> or a reverse proxy, because Sous only listens on 127.0.0.1.</p>
  {/if}
</div>

<style>
  .way {
    grid-column: 1 / -1;
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
  .hint {
    font-size: 0.8125rem;
    color: var(--ink-3);
  }
  .warn {
    font-weight: 700;
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
