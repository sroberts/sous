import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { mockApi } from './mock/server.js';

// Builds into internal/ui/dist, which the Go binary embeds (spec.md §10).
// The mock /v1 API exists only in the dev server.
export default defineConfig({
  plugins: [svelte(), mockApi()],
  build: {
    outDir: '../internal/ui/dist',
    emptyOutDir: true,
  },
});
