<script>
  import { ExternalLink } from '@lucide/svelte';
  import { inlineMarkdown } from '../lib/markdown.js';

  let { text, links = true } = $props();
  let parts = $derived(inlineMarkdown(text));
</script>

{#each parts as p}{#if p.kind === 'strong'}<strong>{p.text}</strong>{:else if p.kind === 'em'}<em>{p.text}</em>{:else if p.kind === 'link' && links}<a href={p.href} target="_blank" rel="noopener noreferrer">{p.text}<ExternalLink size={13} aria-hidden="true" /><span class="visually-hidden">{' (opens in a new tab)'}</span></a>{:else}{p.text}{/if}{/each}

<style>
  /* Links in recipe text: an ink-grey underline that reads as a link at a
     glance, and the same external-link mark as the source link. */
  a {
    text-decoration-color: var(--ink-3);
  }
  a:hover {
    color: var(--red);
    text-decoration-color: var(--red);
  }
  a :global(svg) {
    margin-left: 3px;
    vertical-align: -1px;
  }
</style>
