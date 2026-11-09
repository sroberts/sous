// Inline Markdown for free-text fields that Mela (and people) write with it:
// notes and nutrition. Only **bold**, *italic* / _italic_ and [links](url) are
// recognised; everything else stays literal, and line breaks are left to CSS
// (white-space: pre-line). The result is a list of segments for the template
// to render, so nothing here ever becomes HTML.

const pattern =
  /\*\*(.+?)\*\*|__(.+?)__|\*(?!\s)(.+?)(?<!\s)\*|(?<![\p{L}\p{N}])_(?!\s)(.+?)(?<!\s)_(?![\p{L}\p{N}])|\[([^\]\n]+)\]\((\S+?)\)/gu;

// Only web and mail links are followed; anything else (javascript:, data:)
// keeps its text and loses the link.
function safeHref(url) {
  return /^(https?:|mailto:)/i.test(url) ? url : null;
}

/** @returns {{ kind: 'text'|'strong'|'em'|'link', text: string, href?: string }[]} */
export function inlineMarkdown(source) {
  const out = [];
  let last = 0;
  for (const m of source.matchAll(pattern)) {
    if (m.index > last) out.push({ kind: 'text', text: source.slice(last, m.index) });
    const [, b1, b2, i1, i2, label, url] = m;
    if (b1 ?? b2) out.push({ kind: 'strong', text: b1 ?? b2 });
    else if (i1 ?? i2) out.push({ kind: 'em', text: i1 ?? i2 });
    else {
      const href = safeHref(url);
      out.push(href ? { kind: 'link', text: label, href } : { kind: 'text', text: label });
    }
    last = m.index + m[0].length;
  }
  if (last < source.length) out.push({ kind: 'text', text: source.slice(last) });
  return out;
}

/** The text as it reads once rendered: markers and link targets dropped. */
export function plainText(source) {
  return inlineMarkdown(source).map((p) => p.text).join('');
}

/** Lines like "**Calories** 1063.75" or "**Fat** 55.2 grams", as Mela writes
 *  nutrition: [{ label, fig, unit }], or null when any line is something else. */
export function labelledFigures(source) {
  const lines = source.split('\n').map((l) => l.trim()).filter(Boolean);
  if (!lines.length) return null;
  const out = [];
  for (const line of lines) {
    const m = /^\*\*(.+?):?\*\*:?\s+(\d[\d.,]*)\s*(.*)$/.exec(line);
    if (!m) return null;
    out.push({ label: m[1].trim(), fig: m[2], unit: m[3].trim() });
  }
  return out;
}
