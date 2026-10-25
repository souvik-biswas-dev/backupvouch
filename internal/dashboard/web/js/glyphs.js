// The brand check mark (docs/BRAND.md, 64 by 64 grid) in five states. Shape carries the meaning,
// colour only reinforces it: a failure is a broken check or a flat bar, never just a red check.
import { s } from './dom.js';

const CHECK = 'M8 23 L21 36 L56 9';

const SHAPES = {
  vouched: [[CHECK, 'g-vouched']],
  unverified: [[CHECK, 'g-stored']],
  // The long arm is cut and the far piece slips down: restored, but a check broke.
  'verify-failed': [
    ['M8 23 L21 36 L32.9 26.8', 'g-failed'],
    ['M44.3 28.3 L59 17', 'g-failed'],
  ],
  // A flat bar: nothing was stored.
  'backup-failed': [['M6 40 L58 40', 'g-failed']],
  next: [[CHECK, 'g-next']],
};

// glyph returns an SVG <g> of the given pixel size. strokeWidth is in grid units.
export function glyph(state, size, strokeWidth = 9) {
  const shapes = SHAPES[state] || SHAPES.unverified;
  return s('g', { transform: `scale(${size / 64})` },
    shapes.map(([d, cls]) => s('path', {
      d,
      class: `g ${cls}`,
      // pathLength lets the entrance animation use a dash of length 1; the dashed "next" shape keeps real units.
      pathLength: state === 'next' ? null : 1,
      'stroke-width': strokeWidth,
    })));
}

// The verdict uses the same shapes as the runs.
export const VERDICT_GLYPH = {
  vouched: 'vouched',
  failed: 'verify-failed',
  'backup-failed': 'backup-failed',
  unverified: 'unverified',
};
