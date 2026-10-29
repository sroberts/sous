<script>
  // A recipe's photograph printed as a plate. Without one, the dish's
  // initial is set in the plate instead, so a missing photo reads as a
  // deliberate page rather than a hole.
  import { imageUrl } from '../lib/api.js';

  let { hash = null, title = '', ratio = '4 / 3', size = 'l' } = $props();
  let failed = $state(false);
  const initial = $derived((title.match(/[A-Za-z]/)?.[0] ?? '·').toUpperCase());
</script>

<figure class="plate {size}" style:aspect-ratio={ratio}>
  {#if hash && !failed}
    <img src={imageUrl(hash)} alt="" loading="lazy" decoding="async" onerror={() => (failed = true)} />
  {:else}
    <span class="initial" aria-hidden="true">{initial}</span>
  {/if}
</figure>

<style>
  .plate {
    position: relative;
    overflow: hidden;
    background: var(--paper);
    width: 100%;
  }
  /* The plate mark: an inked edge, and the photograph printed into the
     paper so its whites take the paper's colour. */
  .plate::after {
    content: '';
    position: absolute;
    inset: 0;
    box-shadow: inset 0 0 0 1px var(--ink);
    pointer-events: none;
  }
  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    mix-blend-mode: multiply;
  }
  .plate:has(.initial) {
    background: var(--paper-press);
  }
  .initial {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    font-family: var(--serif);
    font-style: italic;
    color: var(--ink-faint);
    font-size: 5rem;
    line-height: 1;
  }
  .s .initial {
    font-size: 1.75rem;
  }
</style>
