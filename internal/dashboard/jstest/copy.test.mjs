import test from 'node:test';
import assert from 'node:assert/strict';
import { failureSummary, headline, runText, verdictText } from '../web/js/copy.js';

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

test('failureSummary without checks puts the message first', () => {
  const f = failureSummary({ failedChecks: [], message: 'the stored backup does not match the checksum' });
  assert.equal(f.main, 'the stored backup does not match the checksum');
  assert.equal(f.detail, '');
  assert.equal(f.showHint, true);
});

test('failureSummary without checks or message uses a fallback', () => {
  const f = failureSummary({});
  assert.equal(f.main, 'The restore check failed and no reason was recorded.');
  assert.equal(f.showHint, true);
});

test('failureSummary with checks keeps the message as detail', () => {
  const f = failureSummary({ failedChecks: ['row-count'], message: 'row-count: 0 rows' });
  assert.deepEqual(f.checks, ['row-count']);
  assert.equal(f.main, '');
  assert.equal(f.detail, 'row-count: 0 rows');
  assert.equal(f.showHint, false);
});

test('a checksum failure never says the backup was restored', () => {
  for (const reason of ['ChecksumMismatch', 'StorageUnreachable']) {
    assert.doesNotMatch(verdictText('failed', reason), /was restored/);
    assert.doesNotMatch(runText('verify-failed', reason), /and restored/);
  }
  assert.match(verdictText('failed', 'CheckFailed'), /was restored, but a check failed/);
  assert.match(runText('verify-failed', 'RestoreFailed'), /restore failed/);
});

test('unknown or missing reasons use the neutral text', () => {
  assert.equal(verdictText('failed', ''), 'The newest backup failed its restore check. Do not rely on it.');
  assert.equal(verdictText('failed', 'Whatever'), 'The newest backup failed its restore check. Do not rely on it.');
  assert.equal(runText('verify-failed', ''), 'Backed up, but the restore check failed.');
  assert.equal(verdictText('vouched', 'CheckFailed'), 'The newest backup was restored and every check passed.');
});
