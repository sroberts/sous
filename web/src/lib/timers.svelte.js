// Step timers for cook mode: found in the step's own words, run
// independently of which step is showing, and loud when they finish.

const WORDS = { a: 1, an: 1, one: 1, two: 2, three: 3, four: 4, five: 5, six: 6, seven: 7, eight: 8, nine: 9, ten: 10, fifteen: 15, twenty: 20, thirty: 30, forty: 40, 'forty-five': 45 };
const NUM = String.raw`(\d+\s+\d\/\d|\d\/\d|\d+(?:\.\d+)?\s*[¼½¾]|[¼½¾]|\d+(?:\.\d+)?|a|an|one|two|three|four|five|six|seven|eight|nine|ten|fifteen|twenty|thirty|forty|forty-five)`;
const RE = new RegExp(String.raw`\b${NUM}(?:\s*(?:to|-|–|or)\s*${NUM})?\s*(seconds?|secs?|minutes?|mins?|hours?|hrs?)\b`, 'gi');

function num(s) {
  if (!s) return null;
  s = s.toLowerCase().trim();
  if (s in WORDS) return WORDS[s];
  // "2 1/2", "1/2", "2½": a whole number and a fraction.
  const frac = s.match(/^(?:(\d+)\s+)?(\d)\/(\d)$/);
  if (frac) return (Number(frac[1]) || 0) + frac[2] / frac[3];
  const glyph = { '¼': 0.25, '½': 0.5, '¾': 0.75 }[s.at(-1)];
  if (glyph) return (parseFloat(s) || 0) + glyph;
  return parseFloat(s);
}

/** What a duration times, in the step's own words: the start of the clause
 * it ends ("Brown the beef in batches…, 8 minutes" → "Brown the beef"). A
 * clause starts a sentence or follows "then", so "…for 1 minute, then
 * sprinkle in the flour and stir for 1 minute" names each minute apart. */
export function actionOf(before) {
  // Collapsed first: a long run of spaces would make the split below slow.
  before = before.replace(/\s+/g, ' ');
  const clause = before.split(/[.;:!?]\s+|,?\s+then\s+/i).at(-1) ?? '';
  let first = clause.split(/[,(]|\s(?:and|until|while|for|over|about)\s/i)[0].trim();
  // "Cook the bacon in a large pot" → "Cook the bacon", but "Pour in the
  // wine" keeps its "in": a cut needs two words before it.
  const short = first.split(/\s(?:in|into|on|onto|with|to|at|under|from)\s/i)[0];
  if (short.split(/\s+/).length >= 2) first = short;
  if (!first || /^\d/.test(first)) return '';
  first = first[0].toUpperCase() + first.slice(1);
  return first.length > 28 ? `${first.slice(0, 28).replace(/\s+\S*$/, '')}…` : first;
}

/** Durations mentioned in a step: [{ label, seconds, what }], in order.
 * Ranges start at the low end. */
export function findDurations(text) {
  const out = [];
  for (const m of text.matchAll(RE)) {
    const unit = m[3].toLowerCase();
    const mult = unit.startsWith('h') ? 3600 : unit.startsWith('m') ? 60 : 1;
    const low = num(m[1]);
    if (!low) continue;
    const label = m[0].replace(/\s+/g, ' ');
    out.push({ label, seconds: Math.round(low * mult), what: actionOf(text.slice(Math.max(0, m.index - 200), m.index)) || label });
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

// A finished timer chimes again every 10 seconds until it's cleared, for
// three minutes: one chime is easy to miss over a fan or a running tap.
const AGAIN = 10_000;
const RING_FOR = 180_000;
const ringing = (t) => t.done && timers.now - t.endsAt < RING_FOR;

function loop() {
  timers.now = Date.now();
  let ring = false;
  for (const t of timers.list) {
    if (!t.done && !t.paused && t.endsAt <= timers.now) {
      t.done = true;
      t.rang = timers.now;
      ring = true;
    } else if (ringing(t) && timers.now - t.rang >= AGAIN) {
      t.rang = timers.now;
      ring = true;
    }
  }
  if (ring) chime();
  if (!timers.list.some((t) => (!t.done && !t.paused) || ringing(t))) {
    clearInterval(tick);
    tick = null;
  }
}

/** Start the timer for one duration in a step. `key` names it ("2:0", the
 * step's first duration), so two "1 minute"s in a step are two timers. */
export function startTimer(label, seconds, step, key = `${step}:${label}`, what = label) {
  if (timers.list.some((t) => t.key === key && !t.done)) return;
  // Unlock audio inside the user's tap so the chime can play later.
  try {
    audio ??= new AudioContext();
    audio.resume();
  } catch {}
  timers.list.push({ id: crypto.randomUUID(), key, what, label, step, seconds, endsAt: Date.now() + seconds * 1000, paused: false, left: seconds, done: false });
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

/** Put back a timer that was cancelled by mistake. It kept counting while it
 * was gone, so it may come back finished. */
export function restore(t) {
  if (timers.list.some((x) => x.id === t.id)) return;
  timers.list.push(t);
  if (t.done) t.rang = Date.now(); // it rang just now; the next chime comes in its turn
  if (!t.paused) tick ??= setInterval(loop, 250);
  loop();
}

/** Timers still counting down (running or paused), not finished ones. */
export function counting() {
  return timers.list.filter((t) => !t.done).length;
}

export function clearAll() {
  timers.list = [];
}
