// Every sentence the page says about status lives here, so it can be read and tested in one place.
import { count, relative } from './format.js';

export const VERDICT_WORD = {
  vouched: 'Vouched',
  failed: 'Failed restore check',
  'backup-failed': 'Backup failed',
  unverified: 'Not yet verified',
};

export const VERDICT_TEXT = {
  vouched: 'The newest backup was restored and every check passed.',
  failed: 'The newest backup was restored, but a check failed. Do not rely on it.',
  'backup-failed': 'The newest backup did not complete, so there is nothing new to trust.',
  unverified: 'The newest backup has not been restored and checked yet.',
};

export const PHASE_TEXT = {
  BackingUp: 'Backing up now',
  Verifying: 'Restoring and checking now',
  Pruning: 'Removing old backups now',
};

export const RUN_TEXT = {
  vouched: 'Backed up, restored and every check passed.',
  'verify-failed': 'Backed up and restored, but a check failed.',
  'backup-failed': 'The backup failed, so nothing was stored.',
  unverified: 'Backed up, not restored and checked yet.',
};

const isBad = (p) => p.verdict === 'failed' || p.verdict === 'backup-failed';

// headline returns the one sentence this page exists to answer, a supporting line, and a tone.
export function headline(data, nowMs) {
  const { summary, policies } = data;
  const { total, vouched, needAttention, unverified } = summary;
  const vouchedOf = `${vouched} of ${count(total, 'policy', 'policies')} vouched for.`;

  if (total === 0) {
    return {
      tone: '',
      text: 'No backup policies yet.',
      sub: 'Create a BackupPolicy and its status appears here. This page updates by itself.',
    };
  }

  if (needAttention > 0) {
    const first = policies.find(isBad);
    let text = `${needAttention} of ${total} policies need attention.`;
    if (needAttention === 1) {
      text = first.verdict === 'failed'
        ? `${first.name} failed its latest restore check.`
        : `${first.name} did not complete its latest backup.`;
    }
    let sub = `${vouchedOf} Policies that need attention are listed first.`;
    if (needAttention === total) {
      sub = total === 1 ? 'Its newest backup is not vouched for.' : 'None of them is vouched for right now.';
    }
    return { tone: 'bad', text, sub };
  }

  if (vouched === total) {
    const times = policies
      .map((p) => (p.lastVerification && p.lastVerification.time ? Date.parse(p.lastVerification.time) : NaN))
      .filter(Number.isFinite);
    const oldest = times.length > 0 ? ` The oldest of those proofs is ${relative(Math.min(...times), nowMs)}.` : '';
    return {
      tone: 'good',
      text: 'Every backup is vouched for.',
      sub: `Each policy's newest backup was restored and passed its checks.${oldest}`,
    };
  }

  const first = policies.find((p) => p.verdict === 'unverified');
  const text = unverified === 1
    ? `${first.name} has a backup that has not been restored yet.`
    : `${unverified} policies have backups that have not been restored yet.`;
  return { tone: '', text, sub: vouchedOf };
}
