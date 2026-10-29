<script>
  // A recipe's photograph, shown as shot. Without one, the photo slot prints
  // the dish's initial on the pass, so a missing photo reads as deliberate.
  import { imageUrl } from '../lib/api.js';

  let { hash = null, title = '', ratio = '4 / 3', size = 'l' } = $props();
  let failed = $state(false);
  const initial = $derived((title.match(/[A-Za-z]/)?.[0] ?? '·').toUpperCase());
</script>

<figure class="plate {size}" style:aspect-ratio={ratio}>
  {#if hash && !failed}
    <img src={imageUrl(hash)} alt="" loading="lazy" decoding="async" onerror={() => (failed = true)} />
  {:else}
    <span class="initial fig" aria-hidden="true">{initial}</span>
  {/if}
</figure>

<style>
  .plate {
    position: relative;
    overflow: hidden;
    width: 100%;
    background: var(--pass-deep);
  }
  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .initial {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    color: var(--ticket);
    font-size: 6rem;
  }
  .s .initial {
    font-size: 2.5rem;
  }
</style>
