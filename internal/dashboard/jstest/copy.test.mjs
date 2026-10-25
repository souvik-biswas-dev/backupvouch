import test from 'node:test';
import assert from 'node:assert/strict';
import { headline } from '../web/js/copy.js';

const NOW = Date.UTC(2026, 9, 25, 12, 0, 0);

function data(...verdicts) {
  const policies = verdicts.map((verdict, i) => ({
    name: `p${i}`, verdict,
    lastVerification: verdict === 'vouched' ? { time: new Date(NOW - (i + 1) * 3600e3).toISOString() } : null,
  }));
  const summary = {
    total: policies.length,
    vouched: verdicts.filter((v) => v === 'vouched').length,
    needAttention: verdicts.filter((v) => v === 'failed' || v === 'backup-failed').length,
    unverified: verdicts.filter((v) => v === 'unverified').length,
  };
  return { summary, policies };
}

test('no policies', () => {
  const h = headline(data(), NOW);
  assert.equal(h.text, 'No backup policies yet.');
  assert.equal(h.tone, '');
});

test('everything vouched', () => {
  const h = headline(data('vouched', 'vouched'), NOW);
  assert.equal(h.tone, 'good');
  assert.equal(h.text, 'Every backup is vouched for.');
  assert.match(h.sub, /oldest of those proofs is 2 h ago/);
});

test('one failed restore check names the policy', () => {
  const h = headline(data('failed', 'vouched'), NOW);
  assert.equal(h.tone, 'bad');
  assert.equal(h.text, 'p0 failed its latest restore check.');
  assert.match(h.sub, /1 of 2 policies vouched for/);
});

test('one failed backup names the policy', () => {
  const h = headline(data('backup-failed'), NOW);
  assert.equal(h.text, 'p0 did not complete its latest backup.');
  assert.equal(h.sub, 'Its newest backup is not vouched for.');
});

test('several policies need attention', () => {
  const h = headline(data('failed', 'backup-failed', 'vouched'), NOW);
  assert.equal(h.text, '2 of 3 policies need attention.');
});

test('nothing vouched and several failing', () => {
  const h = headline(data('failed', 'backup-failed'), NOW);
  assert.equal(h.sub, 'None of them is vouched for right now.');
});

test('unverified backups are not a failure', () => {
  const one = headline(data('unverified', 'vouched'), NOW);
  assert.equal(one.tone, '');
  assert.equal(one.text, 'p0 has a backup that has not been restored yet.');
  const many = headline(data('unverified', 'unverified'), NOW);
  assert.equal(many.text, '2 policies have backups that have not been restored yet.');
});
