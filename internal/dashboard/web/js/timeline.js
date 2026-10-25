// The proof track: one SVG per policy. Runs sit at their real time, so a missed run shows up as an
// empty stretch. Each run is the brand check mark; its shape says what is known about the run.
import { s } from './dom.js';
import { glyph } from './glyphs.js';
import { DAY, linear, pickStep, spread, tickTimes, timeWindow } from './scale.js';
import { clock, relative, shortDate } from './format.js';

const BASE = 64; // y of the time axis
const HEIGHT = 100;
const PAD = 30; // room at both ends so the larger glyph and the labels fit
const SIZE = 22;
const SIZE_LATEST = 30;
const GAP = 28; // least distance between two run centres
const FAR = 3; // a next run further away than this many window widths is drawn as an arrow
const TAIL = 100; // room kept at the right for the arrow and its label

function tickLabel(t, step) {
  if (step >= DAY) return shortDate(t);
  return new Date(t).getHours() === 0 ? shortDate(t) : clock(t);
}

// drawTrack renders the track into wrap and returns { select(runID) }.
// opts: nowMs, selectedID, animate, index (policy position, staggers the entrance),
//       describe(run) -> string for screen readers, onSelect(run, byPerson).
export function drawTrack(wrap, policy, opts) {
  const width = Math.max(300, Math.floor(wrap.clientWidth));
  const left = PAD;
  const right = width - PAD;
  const runs = policy.history.map((r) => ({ ...r, t: Date.parse(r.at) }));
  const nextMs = policy.nextRun ? Date.parse(policy.nextRun) : null;

  // The window is built from "now" and the runs only. A next run far outside it does not stretch
  // the axis; it is drawn as an arrow at the right edge instead.
  const base = [opts.nowMs, ...runs.map((r) => r.t)];
  let [lo, hi] = timeWindow(base);
  const far = nextMs !== null && nextMs - hi > FAR * (hi - lo);
  if (nextMs !== null && !far) [lo, hi] = timeWindow([...base, nextMs]);
  const x = linear(lo, hi, left, far ? right - TAIL : right);
  const step = pickStep(hi - lo, (far ? right - TAIL : right) - left);

  const svg = s('svg', {
    class: opts.animate ? 'track enter' : 'track',
    width,
    height: HEIGHT,
    viewBox: `0 0 ${width} ${HEIGHT}`,
    role: 'group',
    'aria-label': `Runs of ${policy.name} over time, oldest on the left`,
  });

  svg.append(s('line', { class: 'axis', x1: left - 12, x2: right + 12, y1: BASE, y2: BASE }));

  // "now" and "next" are labelled in the tick row, so ticks that would collide with them are dropped.
  const xNow = x(opts.nowMs);
  const xNext = nextMs !== null && !far ? x(nextMs) : null;
  const crowded = (px) => Math.abs(px - xNow) < 60 || (xNext !== null && Math.abs(px - xNext) < 60);
  for (const t of tickTimes(lo, hi, step)) {
    const px = x(t);
    svg.append(s('line', { class: 'tick', x1: px, x2: px, y1: BASE, y2: BASE + 6 }));
    if (!crowded(px)) {
      svg.append(s('text', { class: 'tick-label', x: px, y: BASE + 24, 'text-anchor': 'middle' }, tickLabel(t, step)));
    }
  }

  const close = xNext !== null && Math.abs(xNext - xNow) < 56;
  svg.append(s('line', { class: 'now-line', x1: xNow, x2: xNow, y1: BASE - 4, y2: BASE + 8 }));
  svg.append(s('text', {
    class: 'marker-label', x: xNow, y: BASE + 24, 'text-anchor': close ? 'end' : 'middle',
  }, 'now'));

  if (xNext !== null) {
    svg.append(s('g', { 'aria-hidden': 'true', transform: `translate(${xNext - SIZE / 2} ${BASE - 8 - SIZE})` }, glyph('next', SIZE)));
    svg.append(s('text', {
      class: 'marker-label', x: xNext, y: BASE + 24, 'text-anchor': close ? 'start' : 'middle',
    }, 'next'));
  } else if (far) {
    const edge = right + 8;
    svg.append(s('path', { class: 'next-arrow', d: `M${edge - 8} ${BASE - 6} L${edge} ${BASE} L${edge - 8} ${BASE + 6}` }));
    svg.append(s('g', { 'aria-hidden': 'true', transform: `translate(${right - SIZE / 2} ${BASE - 8 - SIZE})` }, glyph('next', SIZE)));
    svg.append(s('text', {
      class: 'marker-label', x: right + 12, y: BASE + 24, 'text-anchor': 'end',
    }, `next ${relative(nextMs, opts.nowMs)}`));
  }

  if (runs.length === 0) {
    svg.append(s('text', { class: 'tick-label', x: left, y: BASE - 20 }, 'No runs recorded yet.'));
  }

  const xs = spread(runs.map((r) => x(r.t)), GAP, left, right);
  const els = runs.map((run, i) => {
    const size = i === runs.length - 1 ? SIZE_LATEST : SIZE;
    const cx = xs[i];
    const top = BASE - 8 - size;
    const box = { x: cx - size / 2 - 4, y: top - 4, width: size + 8, height: size + 16 };
    const g = s('g', {
      class: 'run',
      role: 'button',
      tabindex: -1,
      'aria-pressed': 'false',
      'aria-label': opts.describe(run),
      'data-key': `${policy.namespace}/${policy.name}#${run.runID}`,
    },
    s('rect', { class: 'hit', ...box }),
    s('g', { transform: `translate(${cx - size / 2} ${top})` }, glyph(run.state, size)),
    s('rect', { class: 'sel', x: cx - size / 2, y: BASE - 1.5, width: size, height: 3 }),
    s('rect', { class: 'ring', ...box }));
    g.style.setProperty('--i', String(i));
    g.style.setProperty('--p', String(opts.index));
    svg.append(g);
    return g;
  });

  function select(index, byPerson) {
    els.forEach((el, i) => {
      el.setAttribute('aria-pressed', i === index ? 'true' : 'false');
      el.setAttribute('tabindex', i === index ? '0' : '-1');
    });
    opts.onSelect(runs[index], byPerson);
  }

  els.forEach((el, i) => {
    el.addEventListener('click', () => select(i, true));
    el.addEventListener('keydown', (event) => {
      const moves = { ArrowLeft: i - 1, ArrowRight: i + 1, Home: 0, End: els.length - 1 };
      if (event.key in moves) {
        const target = Math.min(els.length - 1, Math.max(0, moves[event.key]));
        event.preventDefault();
        select(target, true);
        els[target].focus();
      } else if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        select(i, true);
      }
    });
  });

  wrap.replaceChildren(svg);

  if (runs.length > 0) {
    const found = runs.findIndex((r) => r.runID === opts.selectedID);
    select(found >= 0 ? found : runs.length - 1, false);
  }
  return { select: (runID) => select(runs.findIndex((r) => r.runID === runID), true) };
}
