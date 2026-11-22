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

/** Minutes as the plan says them: 45 → "45 min", 90 → "1 hr 30", 2880 → "2 days". */
export function span(min) {
  if (min == null) return '';
  if (min >= 1440 && min % 1440 === 0) return min === 1440 ? '1 day' : `${min / 1440} days`;
  if (min < 60) return `${min} min`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  return m ? `${h} hr ${m}` : `${h} hr`;
}

/** What the day-of screen shows: tasks happening or due now, those coming
 *  up, and what's finished. Due means it should have started by now (or
 *  starts in the next few minutes), so it's what to do next. */
export function runOfShow(plan, now = Date.now(), soonMin = 5) {
  const soon = now + soonMin * 60e3;
  const live = [];
  const next = [];
  const done = [];
  for (const t of plan.tasks) {
    const s = Date.parse(t.start);
    const e = Date.parse(t.end);
    if (t.status === 'done' || (t.kind === 'preheat' && e <= now)) done.push(t);
    else if (t.status === 'active' || t.status === 'late' || (t.kind === 'preheat' ? s <= now : s <= soon)) live.push(t);
    else next.push(t);
  }
  return { live, next, done };
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
  if (t.hold_minutes) bits.push(`can wait ${span(t.hold_minutes)}${t.hold_in ? ` (${t.hold_in})` : ''}`);
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
