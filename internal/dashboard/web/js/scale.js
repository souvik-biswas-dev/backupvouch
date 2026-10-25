// A linear time scale and tick placement for the proof track. No DOM, so it can be tested with node.
export const HOUR = 3600e3;
export const DAY = 24 * HOUR;

const STEPS = [1, 2, 3, 6, 12].map((n) => n * HOUR).concat([1, 2, 7, 14, 30].map((n) => n * DAY));

export function linear(d0, d1, r0, r1) {
  const k = d1 === d0 ? 0 : (r1 - r0) / (d1 - d0);
  return (v) => r0 + (v - d0) * k;
}

// timeWindow returns [lo, hi] covering all times, at least minSpan wide.
export function timeWindow(times, minSpan = 6 * HOUR) {
  let lo = Math.min(...times);
  const hi = Math.max(...times);
  if (hi - lo < minSpan) lo = hi - minSpan;
  return [lo, hi];
}

// pickStep returns the smallest tick step that leaves room for a label every minLabelPx.
export function pickStep(span, widthPx, minLabelPx = 88) {
  const most = Math.max(2, Math.floor(widthPx / minLabelPx));
  for (const step of STEPS) {
    if (span / step <= most) return step;
  }
  return STEPS[STEPS.length - 1];
}

// tickTimes returns tick times inside [lo, hi], aligned to local midnight.
export function tickTimes(lo, hi, step) {
  const out = [];
  const start = new Date(lo);
  start.setHours(0, 0, 0, 0);
  if (step < DAY) {
    for (let t = start.getTime(); t <= hi; t += step) {
      if (t >= lo) out.push(t);
    }
    return out;
  }
  const days = Math.round(step / DAY);
  for (const d = new Date(start); d.getTime() <= hi; d.setDate(d.getDate() + days)) {
    if (d.getTime() >= lo) out.push(d.getTime());
  }
  return out;
}

// spread keeps sorted positions at least `gap` apart, pushing right first and then back from `hi`.
export function spread(xs, gap, lo, hi) {
  const out = xs.slice();
  for (let i = 1; i < out.length; i += 1) {
    if (out[i] < out[i - 1] + gap) out[i] = out[i - 1] + gap;
  }
  if (out.length > 0 && out[out.length - 1] > hi) {
    out[out.length - 1] = hi;
    for (let i = out.length - 2; i >= 0; i -= 1) {
      if (out[i] > out[i + 1] - gap) out[i] = out[i + 1] - gap;
    }
  }
  if (out.length > 0 && out[0] < lo) out[0] = lo;
  return out;
}
