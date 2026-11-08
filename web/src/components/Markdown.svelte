<script>
  import { inlineMarkdown } from '../lib/markdown.js';

  let { text } = $props();
  let parts = $derived(inlineMarkdown(text));
</script>

{#each parts as p}{#if p.kind === 'strong'}<strong>{p.text}</strong>{:else if p.kind === 'em'}<em>{p.text}</em>{:else if p.kind === 'link'}<a href={p.href} target="_blank" rel="noopener noreferrer">{p.text}</a>{:else}{p.text}{/if}{/each}
