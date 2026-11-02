<script>
  import { session } from '../lib/session.svelte.js';

  const REPO = 'https://github.com/sroberts/sous';

  // A release build links its release notes; a dev build has none.
  let release = $derived(/^v\d/.test(session.version ?? '') ? `${REPO}/releases/tag/${session.version}` : null);
</script>

<footer>
  {#if session.version}
    {#if release}
      <a href={release}>Sous {session.version}</a>
    {:else}
      <span>Sous {session.version}</span>
    {/if}
    <span aria-hidden="true">·</span>
  {/if}
  <a href={REPO}>Source on GitHub</a>
</footer>

<style>
  footer {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: var(--space-2);
    padding: var(--space-7) var(--gutter) max(var(--space-6), env(safe-area-inset-bottom));
    font-size: var(--label);
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--ink-3);
  }
</style>
