// Entry point: load the status, repaint when it changed, keep the last good data on errors.
import { h } from './dom.js';
import { clockSeconds } from './format.js';
import { render } from './render.js';

const POLL_MS = 15000;
const REPAINT_MS = 60000; // relative times ("3 h ago") go stale even when the data does not change

const els = {
  headline: document.getElementById('headline'),
  subline: document.getElementById('subline'),
  notice: document.getElementById('notice'),
  policies: document.getElementById('policies'),
  stamp: document.getElementById('stamp'),
};

let data = null;
let fingerprint = '';
let loadedAt = null;
let lastPaint = 0;
let lastWidth = 0;
let first = true;

function paint() {
  const focused = document.activeElement && document.activeElement.dataset
    ? document.activeElement.dataset.key
    : undefined;
  render(els, data, { animate: first });
  first = false;
  lastPaint = Date.now();
  lastWidth = els.policies.clientWidth;
  if (focused) {
    const again = els.policies.querySelector(`[data-key="${CSS.escape(focused)}"]`);
    if (again) again.focus();
  }
}

function stamp() {
  if (loadedAt) els.stamp.textContent = `Updated ${clockSeconds(loadedAt)}. Refreshes every ${POLL_MS / 1000} seconds.`;
}

function accept(body) {
  data = body;
  loadedAt = new Date();
  els.notice.replaceChildren();
  const next = JSON.stringify({ summary: body.summary, policies: body.policies });
  if (next !== fingerprint || Date.now() - lastPaint > REPAINT_MS) {
    fingerprint = next;
    paint();
  }
  stamp();
}

function fail(err) {
  const what = err instanceof TypeError ? 'The page could not reach the operator.' : err.message;
  if (data) {
    els.notice.replaceChildren(
      h('strong', null, 'The operator did not answer. '),
      `${what} Showing what was loaded at ${clockSeconds(loadedAt)}. Trying again every ${POLL_MS / 1000} seconds.`);
    return;
  }
  els.headline.textContent = 'The status could not be loaded.';
  els.headline.className = 'bad';
  els.subline.textContent = `${what} Check that the operator pod is running and was started with --dashboard-bind-address. This page tries again every ${POLL_MS / 1000} seconds.`;
}

async function load() {
  if (document.hidden) return;
  try {
    const res = await fetch('/api/v1/policies', { headers: { Accept: 'application/json' }, cache: 'no-store' });
    let body = null;
    try {
      body = await res.json();
    } catch {
      body = null; // not JSON: handled below
    }
    if (!res.ok) throw new Error(body && body.error ? body.error : `The server answered ${res.status}.`);
    if (!body || !Array.isArray(body.policies)) throw new Error('The answer was not the status JSON this page expects.');
    accept(body);
  } catch (err) {
    fail(err);
  }
}

// Redraw the tracks when the width changes; they are drawn in pixels.
let resizeTimer = 0;
new ResizeObserver(() => {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    if (data && Math.abs(els.policies.clientWidth - lastWidth) >= 2) paint();
  }, 150);
}).observe(els.policies);

document.addEventListener('visibilitychange', load);
setInterval(load, POLL_MS);
load();
