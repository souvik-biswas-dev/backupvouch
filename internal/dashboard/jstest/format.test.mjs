import test from 'node:test';
import assert from 'node:assert/strict';
import { bytes, count, duration, relative } from '../web/js/format.js';

const NOW = Date.UTC(2026, 9, 25, 12, 0, 0);
const MIN = 60e3;
const HOUR = 3600e3;
const DAY = 86400e3;

test('relative', () => {
  assert.equal(relative(NOW - 10e3, NOW), 'just now');
  assert.equal(relative(NOW - 5 * MIN, NOW), '5 min ago');
  assert.equal(relative(NOW - 3 * HOUR, NOW), '3 h ago');
  assert.equal(relative(NOW - 3 * DAY, NOW), '3 days ago');
  assert.equal(relative(NOW - 36 * HOUR, NOW), '2 days ago');
  assert.equal(relative(NOW + 4 * HOUR, NOW), 'in 4 h');
});

test('duration', () => {
  assert.equal(duration(0), '0 s');
  assert.equal(duration(38), '38 s');
  assert.equal(duration(119.6), '2 min');
  assert.equal(duration(125), '2 min 5 s');
  assert.equal(duration(3840), '1 h 4 min');
  assert.equal(duration(-1), '');
  assert.equal(duration(Number.NaN), '');
});

test('bytes', () => {
  assert.equal(bytes(512), '512 B');
  assert.equal(bytes(1536), '1.5 KiB');
  assert.equal(bytes(120 * 1024 * 1024), '120 MiB');
  assert.equal(bytes(-1), '');
});

test('count', () => {
  assert.equal(count(1, 'policy', 'policies'), '1 policy');
  assert.equal(count(2, 'policy', 'policies'), '2 policies');
  assert.equal(count(0, 'policy', 'policies'), '0 policies');
});
