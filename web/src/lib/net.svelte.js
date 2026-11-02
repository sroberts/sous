// Whether Sous can be reached. The browser knows when the device is offline;
// a device that is online but can't reach Sous (the server down, the tailnet
// away) shows up as failed requests, and then /up is tried every 15 seconds
// until it answers.

export const net = $state({
  online: navigator.onLine,
  reachable: true,
});

export const isOffline = () => !net.online || !net.reachable;

/** The notice for a view that needs a connection right now. */
export const offlineMessage = () => (net.online ? 'Sous isn’t answering.' : 'You’re offline.');

export const OFFLINE_SAVE = 'You’re offline, so this can’t be saved yet.';
export const NOT_ON_DEVICE = 'This recipe isn’t on this device yet. It will be once you’re back online.';

let probe = null;

/** A request failed for want of a connection. */
export function unreachable() {
  net.reachable = false;
  if (!probe) probe = setInterval(check, 15000);
}

/** A request got an answer from Sous. */
export function reached() {
  net.reachable = true;
  clearInterval(probe);
  probe = null;
}

async function check() {
  if (!navigator.onLine) return;
  try {
    // Any answer will do: even the gateway's redirect means the server is there.
    await fetch('/up', { cache: 'no-store', redirect: 'manual' });
    reached();
  } catch {}
}

addEventListener('online', () => {
  net.online = true;
  if (!net.reachable) check();
});
addEventListener('offline', () => (net.online = false));
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'visible' && !net.reachable) check();
});
