// Builds the page from the status JSON. All text goes in as text nodes (see dom.js).
import { h, s } from './dom.js';
import { glyph, VERDICT_GLYPH } from './glyphs.js';
import { drawTrack } from './timeline.js';
import { absolute, bytes, count, duration, relative } from './format.js';
import { failureSummary, headline, PHASE_TEXT, runText, verdictText, VERDICT_WORD } from './copy.js';

// The run a person picked on each track, so a refresh does not lose their place.
const selection = new Map();

function fact(label, value, sub) {
  return h('div', null,
    h('dt', null, label),
    h('dd', null, value, sub ? h('span', { class: 'sub' }, sub) : null));
}

function whenText(iso) {
  return iso ? absolute(Date.parse(iso)) : '';
}

function facts(p, nowMs) {
  const lb = p.lastBackup;
  const lv = p.lastVerification;
  const ago = (iso) => (iso ? relative(Date.parse(iso), nowMs) : 'None yet');
  const backupSub = lb && lb.result === 'Failed'
    ? 'Failed'
    : [lb && whenText(lb.time), lb && lb.sizeBytes ? bytes(lb.sizeBytes) : ''].filter(Boolean).join(', ');
  const checkSub = lv ? [whenText(lv.time), lv.result === 'Passed' ? 'passed' : 'failed'].filter(Boolean).join(', ') : '';
  return h('dl', { class: 'facts' },
    fact('Last backup', ago(lb && lb.time), backupSub),
    fact('Last restore check', ago(lv && lv.time), checkSub),
    fact('Restore took', lv && lv.restoreDurationSeconds ? duration(lv.restoreDurationSeconds) : 'Not recorded'),
    p.nextRun ? fact('Next run', relative(Date.parse(p.nextRun), nowMs), absolute(Date.parse(p.nextRun))) : null);
}

function describeRun(p, run) {
  const latest = p.history.length > 0 && run.runID === p.history[p.history.length - 1].runID;
  const lv = p.lastVerification;
  const reason = latest && lv && run.objectKey === lv.objectKey ? lv.reason || '' : '';
  const parts = [h('span', { class: 'when' }, `Run finished ${absolute(Date.parse(run.at))}. `), runText(run.state, reason)];
  if (latest && lv && lv.restoreDurationSeconds && run.objectKey === lv.objectKey) {
    parts.push(` The restore took ${duration(lv.restoreDurationSeconds)}.`);
  }
  if (run.objectKey) parts.push(' Backup object ', h('code', null, run.objectKey), '.');
  return parts;
}

function whyBlock(p) {
  const lv = p.lastVerification;
  const lb = p.lastBackup;
  const hint = h('p', null, 'To see the events for this policy, run ',
    h('code', null, `kubectl describe backuppolicy ${p.name} -n ${p.namespace}`), '.');
  if (p.verdict === 'failed' && lv) {
    const f = failureSummary(lv);
    return h('div', { class: 'why' },
      h('h3', null, 'What failed'),
      f.checks.length > 0
        ? [h('p', null, `${count(f.checks.length, 'check', 'checks')} did not pass:`),
          h('ul', null, f.checks.map((c) => h('li', null, h('code', null, c))))]
        : h('p', null, f.main),
      f.detail ? h('p', null, f.detail) : null,
      f.showHint ? hint : null);
  }
  if (p.verdict === 'backup-failed' && lb) {
    return h('div', { class: 'why' },
      h('h3', null, 'What failed'),
      h('p', null, lb.message || 'The backup did not complete and no reason was recorded.'),
      hint);
  }
  return null;
}

function historyTable(p) {
  if (p.history.length === 0) return null;
  const rows = [...p.history].reverse();
  return h('details', null,
    h('summary', null, 'Run history as a table'),
    h('div', { class: 'table-scroll' },
      h('table', null,
        h('caption', { class: 'sr' }, `Run history of ${p.name}, newest first`),
        h('thead', null, h('tr', null,
          h('th', { scope: 'col' }, 'Finished'),
          h('th', { scope: 'col' }, 'Backup'),
          h('th', { scope: 'col' }, 'Restore check'),
          h('th', { scope: 'col' }, 'Backup object'))),
        h('tbody', null, rows.map((r) => h('tr', null,
          h('td', { class: 'num' }, absolute(Date.parse(r.at))),
          h('td', null, r.backupResult || '—'),
          h('td', null, r.backupResult === 'Failed' ? 'Not run' : (r.verificationResult || 'Not run')),
          h('td', null, r.objectKey ? h('code', null, r.objectKey) : '—')))))));
}

function policySection(p, index, nowMs) {
  const key = `${p.namespace}/${p.name}`;
  const wrap = h('div', { class: 'track-wrap' });
  const detail = h('p', { class: 'run-detail', 'aria-live': 'polite' });

  const verdict = h('div', { class: `verdict is-${p.verdict}` },
    s('svg', { class: 'big', viewBox: '0 0 56 40', width: 56, height: 40, 'aria-hidden': 'true' },
      glyph(VERDICT_GLYPH[p.verdict], 56, 6)),
    h('p', { class: 'verdict-word' }, VERDICT_WORD[p.verdict]),
    h('p', { class: 'verdict-text' }, verdictText(p.verdict, p.lastVerification && p.lastVerification.reason)),
    p.phase !== 'Idle' ? h('p', { class: 'verdict-phase' }, PHASE_TEXT[p.phase] || p.phase) : null);

  const meta = h('p', { class: 'meta' },
    'Runs ', h('code', null, p.schedule), ` ${p.timeZone || 'UTC'} in namespace `, h('code', null, p.namespace), '.',
    p.suspended ? ' Paused: no new runs are scheduled.' : null);

  const node = h('section', { class: 'policy', 'aria-labelledby': `policy-${index}` },
    verdict,
    h('div', null,
      h('h2', { class: 'name', id: `policy-${index}` }, p.name),
      meta,
      facts(p, nowMs),
      wrap,
      detail,
      whyBlock(p),
      historyTable(p)));

  const showRun = (run, byPerson) => {
    detail.replaceChildren(...describeRun(p, run));
    if (byPerson) {
      detail.classList.remove('swap');
      void detail.offsetWidth; // restart the short fade
      detail.classList.add('swap');
    }
  };

  // Drawing needs the width of the attached element, so it runs after the node is in the page.
  const draw = (animate) => drawTrack(wrap, p, {
    nowMs,
    index,
    animate,
    selectedID: selection.get(key),
    describe: (run) => `${absolute(Date.parse(run.at))}: ${runText(run.state, '')}`,
    onSelect: (run, byPerson) => {
      if (byPerson) selection.set(key, run.runID);
      showRun(run, byPerson);
    },
  });

  return { node, draw };
}

function emptyState() {
  return h('section', { class: 'empty' },
    h('h2', null, 'Nothing to show yet'),
    h('p', null, 'There are no BackupPolicy objects in the cluster. Create one and it appears here within a few seconds.'),
    h('p', null, 'For the steps, see ', h('code', null, 'docs/INSTALL.md'), ', section "Create your first BackupPolicy".'));
}

// render paints the headline and every policy. animate is true only for the first paint.
export function render(els, data, { animate }) {
  const nowMs = Date.parse(data.generatedAt);
  const head = headline(data, nowMs);
  els.headline.textContent = head.text;
  els.headline.className = head.tone;
  els.subline.textContent = head.sub;
  document.title = `${head.text} BackupVouch`;

  const sections = data.policies.map((p, i) => policySection(p, i, nowMs));
  els.policies.replaceChildren(...(sections.length > 0 ? sections.map((x) => x.node) : [emptyState()]));
  sections.forEach((x) => x.draw(animate));
}
