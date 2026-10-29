<script>
  import { session } from '../lib/session.svelte.js';

  async function act() {
    const { action } = session.toast;
    session.toast = null;
    await action.run();
  }
</script>

<div class="region" role="status" aria-live="polite">
  {#if session.toast}
    {#key session.toast.id}
      <div class="toast">
        <p>{session.toast.message}</p>
        {#if session.toast.action}<button class="btn" onclick={act}>{session.toast.action.label}</button>{/if}
      </div>
    {/key}
  {/if}
</div>

<style>
  .region {
    position: fixed;
    left: 50%;
    bottom: max(var(--space-5), env(safe-area-inset-bottom));
    transform: translateX(-50%);
    z-index: 20;
    width: min(32rem, calc(100vw - 32px));
  }
  .toast {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-3) var(--space-3) var(--space-5);
    background: var(--ink);
    color: var(--ticket);
    border-radius: 3px;
    animation: rise 320ms var(--ease-out);
  }
  p {
    flex: 1;
    font-size: var(--ui);
    font-weight: 500;
  }
  .btn {
    background: var(--ink);
    border-color: var(--ticket);
    color: var(--ticket);
  }
  .btn:hover {
    background: var(--ink-2);
  }
  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(12px);
    }
  }
</style>
