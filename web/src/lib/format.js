const VULGAR = [
  [1 / 8, '⅛'], [1 / 4, '¼'], [1 / 3, '⅓'], [3 / 8, '⅜'], [1 / 2, '½'],
  [5 / 8, '⅝'], [2 / 3, '⅔'], [3 / 4, '¾'], [7 / 8, '⅞'],
];

/** 1.5 → "1½", 0.333 → "⅓", 2.4 → "2.4". */
export function quantity(n) {
  if (n == null) return '';
  const whole = Math.floor(n);
  const frac = n - whole;
  if (frac < 0.01) return String(whole);
  const hit = VULGAR.find(([v]) => Math.abs(v - frac) < 0.02);
  if (hit) return `${whole || ''}${hit[1]}`;
  return String(Math.round(n * 100) / 100);
}

const PLURAL = { cup: 'cups', pinch: 'pinches', dash: 'dashes', clove: 'cloves', can: 'cans', stick: 'sticks', sprig: 'sprigs', bunch: 'bunches', slice: 'slices', package: 'packages', head: 'heads' };

export function amount(i) {
  if (i.quantity == null) return '';
  const q = i.quantity_max != null ? `${quantity(i.quantity)}–${quantity(i.quantity_max)}` : quantity(i.quantity);
  if (!i.unit) return q;
  const many = (i.quantity_max ?? i.quantity) > 1;
  return `${q} ${many && PLURAL[i.unit] ? PLURAL[i.unit] : i.unit}`;
}

/** 55 → "55 min", 80 → "1 hr 20", 180 → "3 hr". */
export function duration(min) {
  if (!min) return '';
  if (min < 60) return `${min} min`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  return m ? `${h} hr ${m}` : `${h} hr`;
}

/** True for an http or https link, the only kind shown as a link: a stored
 *  source can be anything (a book title from Mela), and a javascript: link
 *  would run when clicked. */
export function webLink(u) {
  return /^https?:\/\/[^/\s]/i.test(u?.trim() ?? '');
}

export function recipeTime(r) {
  return duration(r.total_minutes) || r.time_text || '';
}

export function dateline(d = new Date()) {
  return d.toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' });
}

export function ago(iso) {
  const s = (Date.now() - new Date(iso)) / 1000;
  if (s < 90) return 'just now';
  if (s < 3600) return `${Math.round(s / 60)} minutes ago`;
  if (s < 86400 * 2) return s < 86400 ? `${Math.round(s / 3600)} hours ago` : 'yesterday';
  if (s < 86400 * 30) return `${Math.round(s / 86400)} days ago`;
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
}

/** A time as a ticket prints it: 45 → { fig: '45', unit: 'min' },
 *  165 → { fig: '2:45', unit: 'hr' }, each with `said`, the same time in
 *  words for a screen reader ('2 hours 45 minutes'). Wording that never
 *  parsed comes back as { text }; no time at all as null. */
export function ticketTime(min, text) {
  if (min) {
    const said = spokenTime(min);
    if (min < 60) return { fig: String(min), unit: 'min', said };
    const m = min % 60;
    return { fig: `${Math.floor(min / 60)}:${String(m).padStart(2, '0')}`, unit: 'hr', said };
  }
  return text ? { text } : null;
}

function spokenTime(min) {
  const count = (n, word) => `${n} ${word}${n === 1 ? '' : 's'}`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  return [h && count(h, 'hour'), m && count(m, 'minute')].filter(Boolean).join(' ');
}

/** The display name for a username. The built-in author of the sample
 *  recipes, _sous, isn't in the household list, so it is named here. */
export function personName(username, people) {
  if (username === '_sous') return 'Sous';
  return people.find((p) => p.username === username)?.name ?? username;
}
