// Helpers for the Games screen.

const KB = 1024;
const MB = KB * 1024;
const GB = MB * 1024;

export function size(bytes: number, estimate = false): string {
  let s: string;
  if (bytes >= GB) s = (bytes / GB).toFixed(bytes >= 100 * GB ? 0 : 1) + ' GB';
  else if (bytes >= MB) s = (bytes / MB).toFixed(bytes >= 100 * MB ? 0 : 1) + ' MB';
  else s = Math.max(1, Math.round(bytes / KB)) + ' KB';
  return (estimate ? '≈ ' : '') + s;
}

/** The last part of a file path, with either kind of slash. */
export function baseName(path: string): string {
  const parts = path.split('/').join('\\').split('\\');
  return parts[parts.length - 1] || path;
}

export const num =(n: number) => n.toLocaleString('en-US');
export const plural = (n: number, one: string, many = one + 's') => num(n) + ' ' + (n === 1 ? one : many);

export function words(items: string[]): string {
  if (items.length < 2) return items.join('');
  return items.slice(0, -1).join(', ') + ' and ' + items[items.length - 1];
}

export const confidence: Record<string, {icon: string; label: string; cls: string}> = {
  dat_verified: {icon: '✓', label: 'Verified against the DAT', cls: 'ok'},
  heuristic: {icon: '≈', label: 'Identified by file name', cls: 'name'},
  unidentified: {icon: '?', label: 'Unidentified file', cls: 'unk'},
};

/** Splits text into [text, isMatch] pieces for highlighting search words. */
export function highlight(text: string, query: string): [string, boolean][] {
  const fold = (s: string) =>
    s
      .toLowerCase()
      .normalize('NFD')
      .replace(/[̀-ͯ]/g, '');
  const words = fold(query)
    .split(/[^a-z0-9]+/)
    .filter(Boolean);
  if (!words.length) return [[text, false]];
  // Folding can change the length (ß, æ), so only highlight when it did not.
  const folded = fold(text);
  if (folded.length !== text.length) return [[text, false]];
  const hit = new Array(text.length).fill(false);
  for (const w of words) {
    for (let k = folded.indexOf(w); k !== -1; k = folded.indexOf(w, k + 1)) for (let i = k; i < k + w.length; i++) hit[i] = true;
  }
  const out: [string, boolean][] = [];
  for (let i = 0; i < text.length; i++) {
    if (out.length && out[out.length - 1][1] === hit[i]) out[out.length - 1][0] += text[i];
    else out.push([text[i], hit[i]]);
  }
  return out;
}
