import test from 'node:test';
import assert from 'node:assert/strict';
import { MINUTE, HOUR, DAY, linear, pickStep, spread, tickTimes, timeWindow } from '../web/js/scale.js';

test('linear maps the domain ends to the range ends', () => {
  const x = linear(100, 200, 30, 330);
  assert.equal(x(100), 30);
  assert.equal(x(200), 330);
  assert.equal(x(150), 180);
});

test('linear does not divide by zero for an empty domain', () => {
  assert.equal(linear(5, 5, 0, 100)(5), 0);
});

test('timeWindow is at least 30 minutes wide and ends at the latest time', () => {
  assert.deepEqual(timeWindow([1000 * HOUR]), [1000 * HOUR - 30 * MINUTE, 1000 * HOUR]);
  assert.deepEqual(timeWindow([0, 10 * HOUR]), [0, 10 * HOUR]);
  assert.deepEqual(timeWindow([100 * MINUTE, 110 * MINUTE]), [80 * MINUTE, 110 * MINUTE]);
});

test('timeWindow accepts a different minimum span', () => {
  assert.deepEqual(timeWindow([10 * HOUR], 6 * HOUR), [4 * HOUR, 10 * HOUR]);
});

test('pickStep leaves room for a label every 88 px', () => {
  assert.equal(pickStep(5 * HOUR, 700), HOUR);
  assert.equal(pickStep(10 * DAY, 700), 2 * DAY);
  assert.equal(pickStep(1000 * DAY, 700), 30 * DAY);
});

test('pickStep uses minute steps for a short window', () => {
  assert.equal(pickStep(30 * MINUTE, 700), 5 * MINUTE);
  assert.equal(pickStep(30 * MINUTE, 300), 10 * MINUTE);
  assert.equal(pickStep(60 * MINUTE, 300), 30 * MINUTE);
  assert.equal(pickStep(2 * HOUR, 700), 30 * MINUTE);
});

test('tickTimes aligns minute ticks to the step', () => {
  process.env.TZ = 'UTC';
  const lo = Date.UTC(2026, 9, 25, 10, 3);
  const hi = Date.UTC(2026, 9, 25, 10, 33);
  const ticks = tickTimes(lo, hi, 10 * MINUTE).map((t) => new Date(t).getUTCMinutes());
  assert.deepEqual(ticks, [10, 20, 30]);
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
