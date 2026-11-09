/** A Svelte action: the node sticks (gets class "stuck") only while all of it
 *  fits in the window below `top` pixels, so a panel taller than the window
 *  scrolls with the page and nothing at its foot is out of reach. */
export function stickIfFits(node, top = 0) {
  const check = () => {
    const bar = parseFloat(getComputedStyle(node).getPropertyValue('--bar')) || 56;
    node.classList.toggle('stuck', node.offsetHeight <= innerHeight - bar - top);
  };
  const ro = new ResizeObserver(check);
  ro.observe(node);
  addEventListener('resize', check);
  return {
    update(next) {
      top = next;
      check();
    },
    destroy() {
      ro.disconnect();
      removeEventListener('resize', check);
    },
  };
}

/** Scroll the page without the masthead taking it for the reader scrolling
 *  up and sliding back in over whatever the scroll was meant to show. */
export function scrollQuietly(top) {
  const root = document.documentElement;
  root.dataset.quietScroll = '';
  scrollTo({ top });
  requestAnimationFrame(() => requestAnimationFrame(() => delete root.dataset.quietScroll));
}
