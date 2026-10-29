// The mock's version of the importer's text mode (spec.md §14): split a
// pasted recipe into a title, ingredient lines and step lines. No fetching.

const ING_HEAD = /^ingredients?\s*:?$/i;
const STEP_HEAD = /^(directions|instructions|method|steps|preparation)\s*:?$/i;
const QTY = /^(\d|[½⅓⅔¼¾⅕⅛⅜⅝⅞]|(a|an|one|two|three|half|pinch|handful|juice|zest)\b)/i;
const BULLET = /^[▢•·*\-–]\s*/;

const short = (l) => l.length <= 60 && !/[.!?]$/.test(l);

// A line ending in a colon inside a section is a sub-heading: "For the sauce:".
const headings = (lines) => lines.map((l) => (/^[^:]{2,40}:$/.test(l) && !QTY.test(l) ? `# ${l.slice(0, -1)}` : l));

// Mela's rule: strip step numbers only when every step is numbered 1, 2, 3…
function unnumber(steps) {
  const NUM = /^(?:step\s*)?(\d+)\s*[.):]\s*/i;
  const body = steps.filter((s) => !s.startsWith('#'));
  if (!body.length || !body.every((s, i) => +(NUM.exec(s)?.[1] ?? -1) === i + 1)) return steps;
  return steps.map((s) => (s.startsWith('#') ? s : s.replace(NUM, '')));
}

export function splitText(text) {
  const lines = String(text)
    .split(/\r?\n/)
    .map((l) => l.trim().replace(BULLET, '').trim())
    .filter(Boolean);
  let title = null;
  let ingredients = [];
  let steps = [];

  const ingAt = lines.findIndex((l) => ING_HEAD.test(l));
  const stepAt = lines.findIndex((l) => STEP_HEAD.test(l));
  if (ingAt >= 0 || stepAt >= 0) {
    const first = Math.min(...[ingAt, stepAt].filter((i) => i >= 0));
    title = lines.slice(0, first).find((l) => short(l) && !QTY.test(l)) ?? null;
    if (ingAt >= 0) ingredients = lines.slice(ingAt + 1, stepAt > ingAt ? stepAt : undefined);
    if (stepAt >= 0) steps = lines.slice(stepAt + 1, ingAt > stepAt ? ingAt : undefined);
  } else {
    let i = 0;
    if (lines[0] && short(lines[0]) && !QTY.test(lines[0])) title = lines[i++];
    while (i < lines.length && (QTY.test(lines[i]) || short(lines[i]))) ingredients.push(lines[i++]);
    steps = lines.slice(i);
  }
  return { title, ingredients: headings(ingredients), steps: unnumber(headings(steps)) };
}
