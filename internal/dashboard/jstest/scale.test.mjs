import test from 'node:test';
import assert from 'node:assert/strict';
import { HOUR, DAY, linear, pickStep, spread, tickTimes, timeWindow } from '../web/js/scale.js';

test('linear maps the domain ends to the range ends', () => {
  const x = linear(100, 200, 30, 330);
  assert.equal(x(100), 30);
  assert.equal(x(200), 330);
  assert.equal(x(150), 180);
});

test('linear does not divide by zero for an empty domain', () => {
  assert.equal(linear(5, 5, 0, 100)(5), 0);
});

test('timeWindow is at least six hours wide and ends at the latest time', () => {
  assert.deepEqual(timeWindow([1000 * HOUR]), [994 * HOUR, 1000 * HOUR]);
  assert.deepEqual(timeWindow([0, 10 * HOUR]), [0, 10 * HOUR]);
});

test('pickStep leaves room for a label every 88 px', () => {
  assert.equal(pickStep(5 * HOUR, 700), HOUR);
  assert.equal(pickStep(10 * DAY, 700), 2 * DAY);
  assert.equal(pickStep(1000 * DAY, 700), 30 * DAY);
});

test('tickTimes aligns hourly ticks to the hour', () => {
  process.env.TZ = 'UTC';
  const lo = Date.UTC(2026, 9, 25, 1, 30);
  const hi = Date.UTC(2026, 9, 25, 6, 10);
  const ticks = tickTimes(lo, hi, HOUR).map((t) => new Date(t).getUTCHours());
  assert.deepEqual(ticks, [2, 3, 4, 5, 6]);
});

test('tickTimes with a day step returns one tick per step inside the window', () => {
  process.env.TZ = 'UTC';
  const lo = Date.UTC(2026, 9, 20, 12);
  const hi = Date.UTC(2026, 9, 27, 12);
  const ticks = tickTimes(lo, hi, 2 * DAY).map((t) => new Date(t).getUTCDate());
  assert.deepEqual(ticks, [22, 24, 26]);
});

test('spread keeps positions apart and inside the limit', () => {
  assert.deepEqual(spread([10, 12, 14], 28, 0, 500), [10, 38, 66]);
  assert.deepEqual(spread([450, 460], 28, 0, 470), [442, 470]);
  assert.deepEqual(spread([], 28, 0, 470), []);
});
