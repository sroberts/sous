import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { mockApi } from './mock/server.js';
import { pwa } from './build/pwa.js';

// Builds into internal/ui/dist, which the Go binary embeds (spec.md §10).
// The mock /v1 API exists only in the dev and preview servers; the service
// worker only in a build.
export default defineConfig({
  plugins: [svelte(), mockApi(), pwa()],
  build: {
    outDir: '../internal/ui/dist',
    emptyOutDir: true,
  },
});
