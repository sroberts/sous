// The service worker (web/sw/sw.js): it lets Sous open without a connection
// and be installed to a home screen. A build has one; the dev server doesn't,
// and removes any left over from a preview so it can't serve a stale build.

import { route } from './router.svelte.js';
import { toast } from './session.svelte.js';

const HOUR = 60 * 60 * 1000;

export async function registerWorker() {
  if (!('serviceWorker' in navigator)) return;
  if (import.meta.env.DEV) {
    for (const reg of await navigator.serviceWorker.getRegistrations()) reg.unregister();
    return;
  }

  // Whether a worker was already in charge when the page loaded: on a first
  // visit the worker that activates is simply the first one, not an update.
  const controlled = !!navigator.serviceWorker.controller;
  const build = document.querySelector('meta[name="sous-build"]')?.content;

  let reg;
  try {
    reg = await navigator.serviceWorker.register('/sw.js', { scope: '/', updateViaCache: 'none' });
  } catch {
    return; // plain http other than localhost, say: Sous works, online only
  }

  // A page left open for days still hears about new builds.
  let checked = Date.now();
  const check = () => {
    if (navigator.onLine && Date.now() - checked > HOUR) {
      checked = Date.now();
      reg.update().catch(() => {});
    }
  };
  addEventListener('online', check);
  document.addEventListener('visibilitychange', () => document.visibilityState === 'visible' && check());

  // The new worker activates on its own. This page still runs the build it
  // loaded, so offer a reload, but never in the middle of cooking.
  let offered = false;
  const offer = () => {
    if (offered) return;
    if (route.name === 'cook') return void setTimeout(offer, 5000);
    offered = true;
    toast('Sous has been updated.', { label: 'Reload', run: () => location.reload() });
  };
  navigator.serviceWorker.addEventListener('message', (e) => {
    if (e.data?.type === 'sous:activated' && controlled && e.data.build !== build) offer();
  });
}
