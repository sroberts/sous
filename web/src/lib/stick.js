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
