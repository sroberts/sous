// Meal planning helpers: times shown in the kitchen's own timezone (the
// meal's), whatever the device's, and the plan's tasks sorted for the day.

/** The device's IANA timezone, for a new meal. */
export function deviceZone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

const partsCache = new Map();
function partsFormat(tz) {
  let f = partsCache.get(tz);
  if (!f) {
    f = new Intl.DateTimeFormat('en-US', {
      timeZone: tz, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit',
    });
    partsCache.set(tz, f);
  }
  return f;
}

/** The wall-clock fields of an instant in tz. */
export function zoned(ms, tz) {
  const out = {};
  for (const p of partsFormat(tz).formatToParts(new Date(ms))) if (p.type !== 'literal') out[p.type] = +p.value;
  return out; // { year, month, day, hour, minute, second }
}

/** A datetime-local value ("2026-11-26T17:00") read as a wall time in tz, to
 *  an ISO instant. Twice round, so a time near a DST change settles. */
export function zonedToISO(local, tz) {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(local ?? '');
  if (!m) return null;
  const [y, mo, d, h, mi] = m.slice(1).map(Number);
  const wall = Date.UTC(y, mo - 1, d, h, mi);
  let utc = wall;
  for (let i = 0; i < 2; i++) {
    const z = zoned(utc, tz);
    const shown = Date.UTC(z.year, z.month - 1, z.day, z.hour, z.minute);
    utc += wall - shown;
  }
  return new Date(utc).toISOString();
}

/** An ISO instant as a datetime-local value in tz. */
export function isoToZoned(iso, tz) {
  const z = zoned(Date.parse(iso), tz);
  const p = (n) => String(n).padStart(2, '0');
  return `${z.year}-${p(z.month)}-${p(z.day)}T${p(z.hour)}:${p(z.minute)}`;
}

/** "4:15 PM" (as the device writes times) for an instant in tz. */
export function clockAt(iso, tz) {
  return new Date(iso).toLocaleTimeString(undefined, { timeZone: tz, hour: 'numeric', minute: '2-digit' });
}

/** "Wednesday, November 25" for an instant in tz. */
export function dayAt(iso, tz) {
  return new Date(iso).toLocaleDateString(undefined, { timeZone: tz, weekday: 'long', month: 'long', day: 'numeric' });
}

/** A key that changes with the day in tz, for grouping. */
export function dayKey(iso, tz) {
  const z = zoned(Date.parse(iso), tz);
  return `${z.year}-${z.month}-${z.day}`;
}

/** "Thursday 5:00 PM", or "Today 5:00 PM" / "Tomorrow" when it is. */
export function whenAt(iso, tz, now = Date.now()) {
  const k = dayKey(iso, tz);
  const label = k === dayKey(new Date(now).toISOString(), tz)
    ? 'Today'
    : k === dayKey(new Date(now + 864e5).toISOString(), tz)
      ? 'Tomorrow'
      : new Date(iso).toLocaleDateString(undefined, { timeZone: tz, weekday: 'long', month: 'short', day: 'numeric' });
  return `${label} ${clockAt(iso, tz)}`;
}

/** Tasks grouped by day in tz: [{ key, label, tasks }], in order. */
export function byDay(tasks, tz) {
  const out = [];
  for (const t of tasks) {
    const key = dayKey(t.start, tz);
    let g = out.at(-1);
    if (!g || g.key !== key) out.push((g = { key, label: dayAt(t.start, tz), tasks: [] }));
    g.tasks.push(t);
  }
  return out;
}

/** Minutes as the plan says them: 45 → "45 min", 90 → "1 hr 30 min", 2880 → "2 days". */
export function span(min) {
  if (min == null) return '';
  if (min >= 1440 && min % 1440 === 0) return min === 1440 ? '1 day' : `${min / 1440} days`;
  if (min < 60) return `${min} min`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  return m ? `${h} hr ${m} min` : `${h} hr`;
}

/** The day an instant falls on, said one way everywhere: "Today",
 *  "Tomorrow", else the short weekday ("Wed"). */
export function dayWord(iso, tz, now = Date.now()) {
  const k = dayKey(iso, tz);
  if (k === dayKey(new Date(now).toISOString(), tz)) return 'Today';
  if (k === dayKey(new Date(now + 864e5).toISOString(), tz)) return 'Tomorrow';
  return new Date(iso).toLocaleDateString(undefined, { timeZone: tz, weekday: 'short' });
}

/** What the day-of screen shows, so one glance answers "what now?":
 *  - focus: what each cook should be doing, at most one hands-on ticket per
 *    cook (work under way first, then what's due, earliest first), plus any
 *    hands-off work that's due, which only needs a moment to start;
 *  - timers: hands-off work under way (a roast), and ovens heating;
 *  - waiting: due work beyond what the cooks can take on, first in line;
 *  - missed: make-ahead work whose time passed before anyone started it,
 *    by dish, for the cook to do now or call done;
 *  - next: everything later; done: what's finished.
 *  A dish's later step never comes up while an earlier one is still open. */
export function runOfShow(plan, now = Date.now(), { cooks = 1, soonMin = 5 } = {}) {
  const soon = now + soonMin * 60e3;
  const out = { focus: [], timers: [], waiting: [], missed: [], next: [], done: [] };
  // The first open task of each dish; the rest wait for it.
  const first = new Map();
  // Dishes whose make-ahead work someone has taken up: their missed steps
  // come up as ordinary work rather than in the missed strip.
  const takenUp = new Set();
  for (const t of plan.tasks) {
    if (!t.item_id || t.kind === 'preheat') continue;
    if (t.make_ahead && (t.status === 'active' || t.status === 'done')) takenUp.add(t.item_id);
    if (t.status !== 'done' && !first.has(t.item_id)) first.set(t.item_id, t.key);
  }
  const missed = new Map();
  const due = [];
  const handsOnActive = [];
  for (const t of plan.tasks) {
    const s = Date.parse(t.start);
    const e = Date.parse(t.end);
    if (t.kind === 'preheat') {
      if (e <= now) continue;
      (s <= now ? out.timers : out.next).push(t);
    } else if (t.status === 'done') out.done.push(t);
    else if (t.status === 'missed' && !takenUp.has(t.item_id)) {
      if (!missed.has(t.item_id)) missed.set(t.item_id, { item_id: t.item_id, recipe_title: t.recipe_title, tasks: [] });
      missed.get(t.item_id).tasks.push(t);
    } else if (t.status === 'active') (t.hands_on ? handsOnActive : out.timers).push(t);
    // Sous has already moved late work to start from now, so what's due is
    // simply what starts within the next few minutes.
    else if (first.get(t.item_id) === t.key && s <= soon) due.push(t);
    else out.next.push(t);
  }
  out.missed = [...missed.values()];
  const slots = Math.max(0, cooks - handsOnActive.length);
  const dueHands = due.filter((t) => t.hands_on);
  out.focus = [...handsOnActive, ...dueHands.slice(0, slots), ...due.filter((t) => !t.hands_on)];
  out.waiting = dueHands.slice(slots);
  return out;
}

/** The time from the first start to the last tick, in minutes, for the
 *  end of the meal; null when nothing was timed. */
export function cookedMinutes(plan) {
  const starts = plan.tasks.map((t) => t.started_at ?? t.done_at).filter(Boolean).map(Date.parse);
  const ends = plan.tasks.map((t) => t.done_at).filter(Boolean).map(Date.parse);
  if (!starts.length || !ends.length) return null;
  return Math.round((Math.max(...ends) - Math.min(...starts)) / 60e3);
}

/** Where a held dish waits, in words: "in the fridge", "on the counter". */
export function holdPlace(where) {
  return { fridge: 'in the fridge', counter: 'on the counter', warm: 'somewhere warm' }[where] ?? `in the ${where}`;
}

/** Bars for the day's lane chart: one lane per oven, the stovetop, each
 *  appliance, and the cooks' hands, with overlapping bars stacked in rows.
 *  Only the meal's own day is drawn; make-ahead days are in the list. */
export function lanes(plan, tz, ovens = [], now = Date.now()) {
  // The run-up to the meal: day-of work (make-ahead has its own days in
  // the list), and once that's under way, from half an hour before now.
  const tasks = plan.tasks.filter((t) => t.minutes > 0 && !t.make_ahead && t.status !== 'missed');
  if (!tasks.length) return null;
  let from = Math.min(...tasks.map((t) => Date.parse(t.start)));
  const to = Math.max(...tasks.map((t) => Date.parse(t.end)), Date.parse(plan.serve_at));
  if (now > from - 6 * 3600e3 && now < to) from = Math.min(from, now - 30 * 60e3);
  const hour = 3600e3;
  const start = Math.floor(from / hour) * hour;
  const end = Math.ceil(to / hour) * hour;
  const pos = (ms) => ((ms - start) / (end - start)) * 100;
  const pack = (items) => {
    const rows = [];
    for (const t of items) {
      const s = Date.parse(t.start);
      let row = rows.find((r) => r.at(-1).e <= s);
      const bar = { ...t, s, e: Date.parse(t.end), left: pos(Date.parse(t.start)), width: pos(Date.parse(t.end)) - pos(Date.parse(t.start)) };
      if (row) row.push(bar);
      else rows.push([bar]);
    }
    return rows;
  };
  const out = [];
  for (const o of ovens) {
    const mine = tasks.filter((t) => t.oven === o.name);
    if (mine.length) out.push({ name: o.name, rows: pack(mine) });
  }
  const stove = tasks.filter((t) => t.equipment === 'stovetop');
  if (stove.length) out.push({ name: 'Stovetop', rows: pack(stove) });
  for (const name of new Set(tasks.map((t) => t.equipment).filter((e) => e && e !== 'oven' && e !== 'stovetop'))) {
    out.push({ name: name[0].toUpperCase() + name.slice(1), rows: pack(tasks.filter((t) => t.equipment === name)) });
  }
  const hands = tasks.filter((t) => t.hands_on && t.kind !== 'preheat');
  if (hands.length) out.push({ name: 'Hands', rows: pack(hands), hands: true });
  // Hour labels at least 12% of the width apart, so they never collide.
  const every = Math.max(1, Math.ceil(12 / (100 / Math.max(1, (end - start) / hour))));
  const ticks = [];
  for (let h = start, n = 0; h <= end; h += hour, n++) ticks.push({ at: pos(h), iso: new Date(h).toISOString(), label: n % every === 0 });
  return { lanes: out, ticks, pos, start, end };
}

/** Seconds left on a started task's own clock (negative once it's over). */
export function remaining(task, now = Date.now()) {
  const from = Date.parse(task.started_at ?? task.start);
  return (from + task.minutes * 60e3 - now) / 1000;
}

/** A step's timing in a few words: "45 min · oven 350°F · hands off". */
export function timingSummary(t) {
  if (!t) return '';
  const bits = [];
  if (t.minutes != null) bits.push(span(t.minutes));
  if (t.equipment === 'oven') bits.push(t.oven_temp ? `oven ${t.oven_temp}°F` : 'oven');
  else if (t.equipment) bits.push(t.equipment);
  if (t.space > 1) bits.push(t.equipment === 'stovetop' ? `${t.space} burners` : `${t.space} racks`);
  if (t.hands_on === false) bits.push('hands off');
  if (t.hold_minutes) bits.push(`can wait ${span(t.hold_minutes)}${t.hold_in ? ` ${holdPlace(t.hold_in)}` : ''}`);
  if (t.reheat_minutes) bits.push(`reheat ${span(t.reheat_minutes)}${t.reheat_temp ? ` at ${t.reheat_temp}°F` : ''}`);
  return bits.join(' · ');
}

/** A timing with blanks dropped, or null when nothing is set. */
export function cleanTiming(t) {
  if (!t) return null;
  const out = {};
  for (const [k, v] of Object.entries(t)) {
    if (v === '' || v == null || Number.isNaN(v)) continue;
    out[k] = v;
  }
  if (out.equipment !== 'oven') delete out.oven_temp;
  if (!out.hold_minutes) {
    delete out.hold_in;
    delete out.reheat_minutes;
    delete out.reheat_temp;
  }
  if (!out.reheat_minutes) delete out.reheat_temp;
  return Object.keys(out).length ? out : null;
}
