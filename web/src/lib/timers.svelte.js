// Step timers for cook mode: found in the step's own words, run
// independently of which step is showing, and loud when they finish.

const WORDS = { a: 1, an: 1, one: 1, two: 2, three: 3, four: 4, five: 5, six: 6, seven: 7, eight: 8, nine: 9, ten: 10, fifteen: 15, twenty: 20, thirty: 30, forty: 40, 'forty-five': 45 };
const NUM = String.raw`(\d+(?:\.\d+)?|\d+\s*½|½|a|an|one|two|three|four|five|six|seven|eight|nine|ten|fifteen|twenty|thirty|forty|forty-five)`;
const RE = new RegExp(String.raw`\b${NUM}(?:\s*(?:to|-|–|or)\s*${NUM})?\s*(seconds?|secs?|minutes?|mins?|hours?|hrs?)\b`, 'gi');

function num(s) {
  if (!s) return null;
  s = s.toLowerCase().trim();
  if (s in WORDS) return WORDS[s];
  if (s.includes('½')) return (parseFloat(s) || 0) + 0.5;
  return parseFloat(s);
}

/** Durations mentioned in a step: [{ label, seconds }]. Ranges start at the low end. */
export function findDurations(text) {
  const out = [];
  for (const m of text.matchAll(RE)) {
    const unit = m[3].toLowerCase();
    const mult = unit.startsWith('h') ? 3600 : unit.startsWith('m') ? 60 : 1;
    const low = num(m[1]);
    if (!low) continue;
    out.push({ label: m[0].replace(/\s+/g, ' '), seconds: Math.round(low * mult) });
  }
  return out;
}

export function clock(seconds) {
  const s = Math.max(0, Math.ceil(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, '0');
  return h ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`;
}

export const timers = $state({ list: [], now: Date.now() });

let tick = null;
let audio = null;

function chime() {
  try {
    audio ??= new AudioContext();
    const t = audio.currentTime;
    for (const [i, f] of [880, 660, 880, 660].entries()) {
      const o = audio.createOscillator();
      const g = audio.createGain();
      o.frequency.value = f;
      g.gain.setValueAtTime(0.0001, t + i * 0.28);
      g.gain.exponentialRampToValueAtTime(0.25, t + i * 0.28 + 0.02);
      g.gain.exponentialRampToValueAtTime(0.0001, t + i * 0.28 + 0.25);
      o.connect(g).connect(audio.destination);
      o.start(t + i * 0.28);
      o.stop(t + i * 0.28 + 0.26);
    }
  } catch {}
  navigator.vibrate?.([200, 100, 200, 100, 400]);
}

function loop() {
  timers.now = Date.now();
  for (const t of timers.list) {
    if (!t.done && !t.paused && t.endsAt <= timers.now) {
      t.done = true;
      chime();
    }
  }
  if (!timers.list.some((t) => !t.done && !t.paused)) {
    clearInterval(tick);
    tick = null;
  }
}

export function startTimer(label, seconds, step) {
  if (timers.list.some((t) => t.step === step && t.label === label && !t.done)) return;
  // Unlock audio inside the user's tap so the chime can play later.
  try {
    audio ??= new AudioContext();
    audio.resume();
  } catch {}
  timers.list.push({ id: crypto.randomUUID(), label, step, seconds, endsAt: Date.now() + seconds * 1000, paused: false, left: seconds, done: false });
  tick ??= setInterval(loop, 250);
  loop();
}

export function remaining(t) {
  return t.paused ? t.left : (t.endsAt - timers.now) / 1000;
}

export function togglePause(t) {
  if (t.paused) {
    t.endsAt = Date.now() + t.left * 1000;
    t.paused = false;
    tick ??= setInterval(loop, 250);
  } else {
    t.left = remaining(t);
    t.paused = true;
  }
}

export function dismiss(t) {
  timers.list = timers.list.filter((x) => x.id !== t.id);
}
