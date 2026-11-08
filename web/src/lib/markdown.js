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
