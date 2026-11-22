import { test } from 'node:test';
import assert from 'node:assert/strict';
import { zonedToISO, isoToZoned, byDay, span, runOfShow, remaining, timingSummary, cleanTiming } from '../src/lib/meals.js';

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
  assert.equal(span(90), '1 hr 30');
  assert.equal(span(120), '2 hr');
  assert.equal(span(2880), '2 days');
});

test('the run of show splits live, next and done', () => {
  const now = Date.parse('2026-11-26T16:00:00Z');
  const t = (key, start, mins, extra = {}) => ({
    key, start, end: new Date(Date.parse(start) + mins * 60e3).toISOString(), minutes: mins, status: 'pending', kind: 'step', ...extra,
  });
  const plan = {
    tasks: [
      t('done', '2026-11-26T15:00:00Z', 20, { status: 'done' }),
      t('roasting', '2026-11-26T15:30:00Z', 240, { status: 'active', started_at: '2026-11-26T15:30:00Z' }),
      t('late', '2026-11-26T16:00:00Z', 10, { status: 'late' }),
      t('soon', '2026-11-26T16:03:00Z', 10),
      t('later', '2026-11-26T17:00:00Z', 10),
      t('hot', '2026-11-26T15:00:00Z', 15, { kind: 'preheat' }),
    ],
  };
  const { live, next, done } = runOfShow(plan, now);
  assert.deepEqual(live.map((x) => x.key), ['roasting', 'late', 'soon']);
  assert.deepEqual(next.map((x) => x.key), ['later']);
  assert.deepEqual(done.map((x) => x.key), ['done', 'hot']);
  assert.equal(remaining(plan.tasks[1], now), 210 * 60);
});

test('timing reads in a few words and drops blanks', () => {
  assert.equal(timingSummary({ minutes: 45, equipment: 'oven', oven_temp: 350, hands_on: false }), '45 min · oven 350°F · hands off');
  assert.equal(timingSummary({ minutes: 15, hold_minutes: 2880, hold_in: 'fridge' }), '15 min · can wait 2 days (fridge)');
  assert.deepEqual(cleanTiming({ minutes: 10, oven_temp: 350, equipment: 'stovetop', hold_in: 'fridge' }), { minutes: 10, equipment: 'stovetop' });
  assert.equal(cleanTiming({ minutes: '', equipment: null }), null);
});
