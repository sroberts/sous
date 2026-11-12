<script>
  // How to connect an AI assistant to the library over MCP (spec.md §9). The
  // address and the header that names who it acts for are filled in from
  // where this page was opened and who's cooking, so each set-up can be
  // copied as it stands.
  import { ChevronDown, Copy, Check } from '@lucide/svelte';
  import { session } from '../lib/session.svelte.js';

  const local = ['127.0.0.1', 'localhost', '[::1]', '::1'].includes(location.hostname);
  // Under Outpost the gateway only lets in browser sign-ins, so assistants
  // connect on loopback, on the machine running Sous (docs/agents.md).
  const url = $derived(session.outpost ? 'http://127.0.0.1:<port>/mcp' : `${location.origin}/mcp`);

  // Who the assistant acts for: whoever's cooking, or anyone in the
  // household when Sous runs on its own.
  let chosen = $state(null);
  const username = $derived(session.outpost ? session.person?.username : (chosen ?? session.person?.username));
  const who = $derived(username ?? '<username>');
  const header = $derived(`X-Sous-Person: ${who}`);

  // mcp-remote refuses plain http except to this machine.
  const plainRemote = $derived(url.startsWith('http:') && !session.outpost && !local);

  const claudeCode = $derived(`claude mcp add --transport http sous ${url} --header "${header}"`);
  const claudeDesktop = $derived(
    JSON.stringify(
      { mcpServers: { sous: { command: 'npx', args: ['-y', 'mcp-remote', url, '--header', `X-Sous-Person:${who}`, ...(plainRemote ? ['--allow-http'] : [])] } } },
      null,
      2,
    ),
  );
  const crush = $derived(
    JSON.stringify({ $schema: 'https://charm.land/crush.json', mcp: { sous: { type: 'http', url, headers: { 'X-Sous-Person': who } } } }, null, 2),
  );

  // Copy where the browser allows it (HTTPS or this machine); elsewhere,
  // select the text so the keyboard's copy takes it.
  let copied = $state(null);
  let said = $state('');
  async function copy(id, text) {
    try {
      await navigator.clipboard.writeText(text);
      copied = id;
      said = 'Copied.';
    } catch {
      const range = document.createRange();
      range.selectNodeContents(document.getElementById(id));
      getSelection().removeAllRanges();
      getSelection().addRange(range);
      copied = `sel:${id}`;
      said = 'Selected. Press Command-C or Control-C to copy.';
    }
    setTimeout(() => {
      if (copied?.endsWith(id)) copied = null;
    }, 2500);
  }
</script>

{#snippet copyBtn(id, text)}
  <button type="button" class="btn quiet copy" onclick={() => copy(id, text)}>
    {#if copied === id}<Check size={16} aria-hidden="true" />Copied{:else if copied === `sel:${id}`}Press ⌘C{:else}<Copy size={16} aria-hidden="true" />Copy{/if}
  </button>
{/snippet}

{#snippet block(id, text)}
  <div class="block">
    <pre id={id}>{text}</pre>
    {@render copyBtn(id, text)}
  </div>
{/snippet}

<main class="assistants" id="main">
  <h1>Connect an AI assistant</h1>
  <p class="lede">
    An assistant that speaks MCP can search the library, read recipes, import links and make changes, with the same rights as the person it acts for. Every change is signed with that name and recorded as made by an assistant, and anything it deletes goes to the trash.
  </p>

  <section class="panel conn" aria-labelledby="conn-h">
    <h2 id="conn-h" class="label">The connection</h2>
    <div class="fact">
      <span class="k">Address</span>
      <code id="mcp-url">{url}</code>
      {@render copyBtn('mcp-url', url)}
    </div>
    <div class="fact">
      <span class="k">Header</span>
      <code id="mcp-header">{header}</code>
      {@render copyBtn('mcp-header', header)}
    </div>
    {#if !session.outpost && session.people.length}
      <label class="fact">
        <span class="k">Acts for</span>
        <select value={username ?? ''} onchange={(e) => (chosen = e.currentTarget.value)}>
          {#if !username}<option value="" disabled>Choose someone</option>{/if}
          {#each session.people as p (p.username)}<option value={p.username}>{p.name}</option>{/each}
        </select>
      </label>
    {/if}
    {#if !username}
      <p class="note">
        The header names who the assistant acts for. Without it, the assistant can only read.
        {#if !session.people.length}<button type="button" class="text-btn" onclick={() => (session.asking = true)}>Add who’s cooking</button> to fill it in.{:else}Choose someone above to fill it in.{/if}
      </p>
    {/if}
    {#if session.outpost}
      <p class="note">
        Under Outpost the login only lets in people with a browser, so an assistant connects on the machine running Sous. There, <code>outpost status sous</code> shows the port to use in place of <code>&lt;port&gt;</code>. If <code>SOUS_REQUIRE_GATEWAY</code> is set, this route is closed.
      </p>
    {:else if local}
      <p class="note">This address only works on this machine. For an assistant on another computer, open Sous at its tailnet address and use the address shown there.</p>
    {:else}
      <p class="note">The assistant’s computer needs to be on your tailnet, the same as a browser.</p>
    {/if}
  </section>

  <section class="setups" aria-labelledby="setups-h">
    <h2 id="setups-h">Set up your assistant</h2>
    <div class="ways">
      <details class="way">
        <summary><h3>Claude Code</h3><span class="what">One command in your terminal</span><ChevronDown class="chev" size={20} aria-hidden="true" /></summary>
        <div class="inner">
          <p>Run this, then type <code>/mcp</code> in Claude Code to see Sous connected.</p>
          {@render block('cfg-claude-code', claudeCode)}
          <p class="hint">It’s added for the folder you run it in. Add <code>--scope user</code> to have Sous in every project.</p>
        </div>
      </details>

      <details class="way">
        <summary><h3>Claude Desktop</h3><span class="what">A bridge in its config file</span><ChevronDown class="chev" size={20} aria-hidden="true" /></summary>
        <div class="inner">
          <p>In Claude, open <b>Settings</b>, then <b>Developer</b>, then <b>Edit Config</b>. Put this in the file (merge it into <code>mcpServers</code> if you already have servers), save, and restart Claude.</p>
          {@render block('cfg-claude-desktop', claudeDesktop)}
          <p class="hint">It needs Node.js installed. Claude’s Connectors setting reaches servers from Anthropic’s cloud, which can’t see your tailnet, so Sous goes in the config file instead.</p>
        </div>
      </details>

      <details class="way">
        <summary><h3>Crush</h3><span class="what">An entry in crush.json</span><ChevronDown class="chev" size={20} aria-hidden="true" /></summary>
        <div class="inner">
          <p>Add this to <code>crush.json</code> in your project, or to <code>~/.config/crush/crush.json</code> to have Sous everywhere. If the file already has an <code>mcp</code> section, add the <code>sous</code> entry to it.</p>
          {@render block('cfg-crush', crush)}
        </div>
      </details>

      <details class="way">
        <summary><h3>ChatGPT</h3><span class="what">Can’t reach Sous yet</span><ChevronDown class="chev" size={20} aria-hidden="true" /></summary>
        <div class="inner">
          <p>ChatGPT connects to MCP servers from OpenAI’s servers, over the public internet, and can’t send the header that says who it acts for. Sous is only on your tailnet, so ChatGPT can’t reach it. The same goes for Claude on the web and on your phone.</p>
          <p>Putting Sous on the public internet to let them in would let anyone read and change the library, since Sous has no passwords yet. Use an assistant on your own computer instead.</p>
        </div>
      </details>
    </div>
    <p class="other">Any other MCP client that can send a header works too: give it the address and header above, with the Streamable HTTP transport.</p>
  </section>
  <p class="visually-hidden" aria-live="polite">{said}</p>
</main>

<style>
  .assistants {
    max-width: 56rem;
    margin: 0 auto;
    padding: var(--space-6) var(--gutter) var(--space-8);
  }
  h1 {
    font-size: var(--title-l);
    font-weight: 800;
  }
  .lede {
    margin-top: var(--space-2);
    color: var(--ink-2);
    max-width: 60ch;
  }
  .panel {
    background: var(--ticket);
    border-top: 3px solid var(--ink);
    padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .conn {
    display: grid;
    gap: var(--space-3);
    margin-top: var(--space-5);
  }
  .conn h2 {
    color: var(--ink-3);
  }
  .fact {
    display: grid;
    grid-template-columns: 5.5rem minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--space-3);
    padding-bottom: var(--space-3);
    border-bottom: 1.5px dashed var(--ticket-press);
  }
  .k {
    font-size: 0.875rem;
    font-weight: 700;
    color: var(--ink-2);
  }
  .fact code {
    font-size: 1rem;
  }
  .fact select {
    justify-self: start;
    min-height: 44px;
    min-width: 12rem;
  }
  .note {
    max-width: 62ch;
    font-size: 0.9375rem;
    color: var(--ink-2);
  }
  .text-btn {
    min-height: 0;
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    font-weight: 700;
    color: var(--ink);
    text-decoration: underline;
    text-decoration-color: var(--perf);
    text-underline-offset: 3px;
  }
  .copy {
    min-height: 40px;
    padding: 0 var(--space-3);
    font-size: 0.875rem;
    white-space: nowrap;
  }

  .setups {
    margin-top: var(--space-7);
  }
  .setups h2 {
    font-size: var(--title-m);
    font-weight: 800;
    padding-bottom: var(--space-2);
    border-bottom: 3px solid var(--ink);
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
  @media (hover: hover) {
    summary:hover {
      background: var(--ticket-press);
    }
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
  .inner {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5) var(--space-5);
  }
  .inner p {
    max-width: 62ch;
  }
  .hint {
    font-size: 0.8125rem;
    color: var(--ink-3);
  }
  .other {
    margin-top: var(--space-4);
    max-width: 62ch;
    font-size: 0.9375rem;
    color: var(--ink-2);
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
  /* A set-up is copied as it stands, so it's set as code: the one place
     Sous uses a monospace. */
  .block {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: start;
    gap: var(--space-2);
    background: var(--pass);
    padding: var(--space-3);
  }
  pre {
    margin: 0;
    overflow-x: auto;
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 0.8125rem;
    line-height: 1.5;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    scrollbar-width: thin;
  }
  .block .copy {
    background: var(--ticket);
  }

  @media (max-width: 560px) {
    .fact {
      grid-template-columns: minmax(0, 1fr) auto;
    }
    .fact .k {
      grid-column: 1 / -1;
    }
    .panel,
    summary,
    .inner {
      padding-left: var(--space-4);
    }
  }
</style>
