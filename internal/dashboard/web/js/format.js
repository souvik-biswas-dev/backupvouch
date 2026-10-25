// Plain formatting helpers. No DOM, so they can be tested with node.
const MIN = 60e3;
const HOUR = 3600e3;
const DAY = 86400e3;

const dateTime = new Intl.DateTimeFormat(undefined, {
  day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit',
});
const dateOnly = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
const timeOnly = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' });
const timeWithSeconds = new Intl.DateTimeFormat(undefined, {
  hour: '2-digit', minute: '2-digit', second: '2-digit',
});

export const absolute = (ms) => dateTime.format(ms);
export const shortDate = (ms) => dateOnly.format(ms);
export const clock = (ms) => timeOnly.format(ms);
export const clockSeconds = (ms) => timeWithSeconds.format(ms);

export function count(n, one, many) {
  return `${n} ${n === 1 ? one : many}`;
}

// relative(ms, nowMs): "just now", "5 min ago", "3 h ago", "2 days ago", "in 4 h".
export function relative(ms, nowMs) {
  const diff = nowMs - ms;
  const abs = Math.abs(diff);
  if (abs < 45e3) return 'just now';
  let text;
  if (abs < 90 * MIN) text = `${Math.max(1, Math.round(abs / MIN))} min`;
  else if (abs < 36 * HOUR) text = `${Math.round(abs / HOUR)} h`;
  else text = count(Math.round(abs / DAY), 'day', 'days');
  return diff >= 0 ? `${text} ago` : `in ${text}`;
}

// duration(seconds): "38 s", "2 min 5 s", "1 h 4 min". Empty string if the input is not usable.
export function duration(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) return '';
  const total = Math.round(seconds);
  if (total < 60) return `${total} s`;
  const minutes = Math.floor(total / 60);
  const rest = total % 60;
  if (minutes < 60) return rest ? `${minutes} min ${rest} s` : `${minutes} min`;
  const hours = Math.floor(minutes / 60);
  const mins = minutes % 60;
  return mins ? `${hours} h ${mins} min` : `${hours} h`;
}

// bytes(n): binary units, "512 B", "1.5 KiB", "120 MiB".
export function bytes(n) {
  if (!Number.isFinite(n) || n < 0) return '';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let value = n;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i += 1;
  }
  const text = i === 0 || value >= 100 ? String(Math.round(value)) : value.toFixed(1);
  return `${text} ${units[i]}`;
}
