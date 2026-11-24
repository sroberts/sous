import { test } from 'node:test';
import assert from 'node:assert/strict';
import { zonedToISO, isoToZoned, byDay, span, runOfShow, remaining, timingSummary, cleanTiming, lanes, mealWord, withItem, withoutItem, restoreItem, moveItemBy } from '../src/lib/meals.js';

test('a wall time in the kitchen’s zone round-trips', () => {
  assert.equal(zonedToISO('2026-11-26T17:00', 'America/New_York'), '2026-11-26T22:00:00.000Z');
  assert.equal(zonedToISO('2026-07-04T12:30', 'America/New_York'), '2026-07-04T16:30:00.000Z');
  assert.equal(zonedToISO('2026-12-25T13:00', 'Europe/London'), '2026-12-25T13:00:00.000Z');
  assert.equal(isoToZoned('2026-11-26T22:00:00.000Z', 'America/New_York'), '2026-11-26T17:00');
  assert.equal(zonedToISO('nope', 'UTC'), null);
});

test('tasks group by the kitchen’s day, not UTC’s', () => {
  const tasks = [
    { start: '2026-11-26T03:00:00.000Z' }, // 10pm Wednesday in New York
    { start: '2026-11-26T15:00:00.000Z' },
    { start: '2026-11-26T22:00:00.000Z' },
  ];
  const days = byDay(tasks, 'America/New_York');
  assert.equal(days.length, 2);
  assert.equal(days[0].tasks.length, 1);
  assert.equal(days[1].tasks.length, 2);
});

test('spans read as the plan says them', () => {
  assert.equal(span(45), '45 min');
  assert.equal(span(90), '1 hr 30 min');
  assert.equal(span(120), '2 hr');
  assert.equal(span(2880), '2 days');
});

test('the run of show puts one ticket per cook on the rail', () => {
  const now = Date.parse('2026-11-26T16:00:00Z');
  const t = (key, item, start, mins, extra = {}) => ({
    key, item_id: item, recipe_title: item, start, end: new Date(Date.parse(start) + mins * 60e3).toISOString(),
    minutes: mins, status: 'pending', kind: 'step', hands_on: true, ...extra,
  });
  const plan = {
    tasks: [
      t('pie:0', 'pie', '2026-11-25T14:00:00Z', 30, { status: 'missed', make_ahead: true }),
      t('pie:1', 'pie', '2026-11-25T14:30:00Z', 50, { status: 'missed', make_ahead: true }),
      t('done', 'beans', '2026-11-26T15:00:00Z', 20, { status: 'done' }),
      t('roast', 'turkey', '2026-11-26T15:30:00Z', 240, { status: 'active', started_at: '2026-11-26T15:30:00Z', hands_on: false }),
      t('cran:0', 'cran', '2026-11-26T15:50:00Z', 10, { status: 'late' }),
      t('cran:1', 'cran', '2026-11-26T16:00:00Z', 10),
      t('mash:0', 'mash', '2026-11-26T16:03:00Z', 10),
      t('later', 'mash2', '2026-11-26T17:00:00Z', 10),
      t('hot', null, '2026-11-26T15:50:00Z', 15, { kind: 'preheat', item_id: null, status: 'active', started_at: '2026-11-26T15:50:00Z' }),
    ],
  };
  const one = runOfShow(plan, now);
  assert.deepEqual(one.focus.map((x) => x.key), ['cran:0']); // late, and moved to start now
  assert.deepEqual(one.waiting.map((x) => x.key), ['mash:0']);
  assert.deepEqual(one.timers.map((x) => x.key), ['roast', 'hot']);
  assert.deepEqual(one.missed.map((m) => [m.item_id, m.tasks.length]), [['pie', 2]]);
  assert.ok(!one.next.some((x) => x.key === 'pie:1'), 'missed work stays in the strip');
  assert.ok(one.next.some((x) => x.key === 'cran:1'), 'a later step waits for the earlier one');
  assert.deepEqual(one.done.map((x) => x.key), ['done']);
  const two = runOfShow(plan, now, { cooks: 2 });
  assert.deepEqual(two.focus.map((x) => x.key), ['cran:0', 'mash:0']);

  // Hands-off work that's due goes on the rail whatever the cooks are doing:
  // starting it takes a moment.
  plan.tasks.push(t('beans', 'beans2', '2026-11-26T16:02:00Z', 30, { hands_on: false }));
  assert.deepEqual(runOfShow(plan, now).focus.map((x) => x.key), ['cran:0', 'beans']);
  plan.tasks.pop();

  // An oven nobody has said is on is a ticket, not a timer.
  plan.tasks.push(t('cold', null, '2026-11-26T16:01:00Z', 15, { kind: 'preheat', item_id: null, hands_on: false }));
  assert.equal(runOfShow(plan, now).focus[0].key, 'cold', 'turning the oven on comes first');
  plan.tasks.pop();

  // Once someone takes up a missed dish, its steps come up as ordinary work.
  plan.tasks[0] = { ...plan.tasks[0], status: 'done', done_at: '2026-11-26T15:59:00Z' };
  const after = runOfShow(plan, now, { cooks: 2 });
  assert.equal(after.missed.length, 0);
  assert.ok(after.focus.some((x) => x.key === 'pie:1') || after.waiting.some((x) => x.key === 'pie:1'));
  assert.equal(remaining(plan.tasks[3], now), 210 * 60);
});

test('the day’s lanes stack what overlaps', () => {
  const t = (key, start, mins, extra = {}) => ({ key, start, end: new Date(Date.parse(start) + mins * 60e3).toISOString(), minutes: mins, kind: 'step', ...extra });
  const plan = {
    serve_at: '2026-11-26T22:00:00.000Z',
    tasks: [
      t('ahead', '2026-11-25T14:00:00Z', 30, { hands_on: true, make_ahead: true }),
      t('roast', '2026-11-26T17:00:00Z', 240, { oven: 'Oven', equipment: 'oven' }),
      t('beans', '2026-11-26T20:00:00Z', 30, { oven: 'Oven', equipment: 'oven' }),
      t('mash', '2026-11-26T21:00:00Z', 10, { hands_on: true }),
    ],
  };
  const l = lanes(plan, 'America/New_York', [{ name: 'Oven', racks: 2 }], Date.parse('2026-11-20T12:00:00Z'));
  assert.deepEqual(l.lanes.map((x) => [x.name, x.rows.length]), [['Oven', 2], ['Hands', 1]]);
  assert.equal(l.ticks[0].iso, '2026-11-26T17:00:00.000Z');
});

test('timing reads in a few words and drops blanks', () => {
  assert.equal(timingSummary({ minutes: 45, equipment: 'oven', oven_temp: 350, hands_on: false }), '45 min · oven 350°F · hands off');
  assert.equal(timingSummary({ minutes: 15, hold_minutes: 2880, hold_in: 'fridge' }), '15 min · can wait 2 days in the fridge');
  assert.deepEqual(cleanTiming({ minutes: 10, oven_temp: 350, equipment: 'stovetop', hold_in: 'fridge' }), { minutes: 10, equipment: 'stovetop' });
  assert.equal(cleanTiming({ minutes: '', equipment: null }), null);
});

test('a meal is named by the hour it is served, in its own zone', () => {
  assert.equal(mealWord('2026-11-26T14:00:00Z', 'America/New_York'), 'Breakfast');
  assert.equal(mealWord('2026-11-26T18:00:00Z', 'America/New_York'), 'Lunch');
  assert.equal(mealWord('2026-11-26T22:00:00Z', 'America/New_York'), 'Dinner');
  assert.equal(mealWord('2026-11-27T05:30:00Z', 'America/New_York'), 'Dinner', 'a dinner run late past midnight');
});

test('menu changes apply to whatever version of the meal they meet', () => {
  const it = (id, extra = {}) => ({ id, recipe_id: `r${id}`, serve_offset_minutes: 0, dish: null, title: id, ...extra });
  const mine = { items: [it('a'), it('b')] };
  // Someone else added c and moved b later meanwhile: a change to a keeps both.
  const theirs = { items: [it('a'), it('b', { serve_offset_minutes: 30 }), it('c')] };
  assert.deepEqual(withItem('a', { dish: 'Platter' })(theirs).items.map((x) => [x.id, x.dish, x.serve_offset_minutes]), [['a', 'Platter', 0], ['b', null, 30], ['c', null, 0]]);
  assert.deepEqual(withoutItem('a')(theirs).items.map((x) => x.id), ['b', 'c']);
  assert.deepEqual(restoreItem(mine.items[0], 0)(withoutItem('a')(theirs)).items.map((x) => x.id), ['a', 'b', 'c']);
  assert.deepEqual(moveItemBy('c', -1)(theirs).items.map((x) => x.id), ['a', 'c', 'b']);
  assert.deepEqual(moveItemBy('a', -1)(theirs).items.map((x) => x.id), ['a', 'b', 'c']);
});
