// Ingredient and step parsing for the mock /v1 API. The Go service owns
// parsing in production (spec.md §7); this mirrors its contract so the UI
// can be built first.

export const PARSER_VERSION = 1;

const FRACTIONS = { '¼': 0.25, '½': 0.5, '¾': 0.75, '⅓': 1 / 3, '⅔': 2 / 3, '⅛': 0.125, '⅜': 0.375, '⅝': 0.625, '⅞': 0.875 };

const UNIT_ALIASES = {
  tsp: ['teaspoons', 'teaspoon', 'tsps', 'tsp', 't'],
  tbsp: ['tablespoons', 'tablespoon', 'tbsps', 'tbsp', 'tbs', 'tbl', 'T'],
  cup: ['cups', 'cup', 'c', 'C'],
  'fl oz': ['fluid ounces', 'fluid ounce', 'fl oz', 'fl. oz.'],
  oz: ['ounces', 'ounce', 'oz'],
  lb: ['pounds', 'pound', 'lbs', 'lb'],
  g: ['grams', 'gram', 'g'],
  kg: ['kilograms', 'kilogram', 'kg'],
  ml: ['milliliters', 'milliliter', 'millilitres', 'millilitre', 'ml'],
  l: ['liters', 'liter', 'litres', 'litre', 'l'],
  qt: ['quarts', 'quart', 'qt'],
  pt: ['pints', 'pint', 'pt'],
  pinch: ['pinches', 'pinch'],
  dash: ['dashes', 'dash'],
  clove: ['cloves', 'clove'],
  can: ['cans', 'can'],
  stick: ['sticks', 'stick'],
  sprig: ['sprigs', 'sprig'],
  bunch: ['bunches', 'bunch'],
  slice: ['slices', 'slice'],
  package: ['packages', 'package', 'pkg'],
  head: ['heads', 'head'],
};

// Longest alias first, so "fluid ounces" wins over "ounces".
const UNITS = Object.entries(UNIT_ALIASES)
  .flatMap(([unit, aliases]) => aliases.map((a) => [a, unit]))
  .sort((a, b) => b[0].length - a[0].length);

const NUM = String.raw`(?:\d+\s+\d+\/\d+|\d+\s*[¼½¾⅓⅔⅛⅜⅝⅞]|\d+\/\d+|\d*\.\d+|\d+|[¼½¾⅓⅔⅛⅜⅝⅞])`;
const QTY = new RegExp(String.raw`^(${NUM})(?:\s*(?:-|–|—|to|or)\s*(${NUM}))?\s*`, 'i');

function toNumber(s) {
  s = s.trim();
  let m;
  if ((m = s.match(/^(\d+)\s+(\d+)\/(\d+)$/))) return +m[1] + m[2] / m[3];
  if ((m = s.match(/^(\d+)\s*([¼½¾⅓⅔⅛⅜⅝⅞])$/))) return +m[1] + FRACTIONS[m[2]];
  if ((m = s.match(/^(\d+)\/(\d+)$/))) return m[1] / m[2];
  if (FRACTIONS[s] !== undefined) return FRACTIONS[s];
  return parseFloat(s);
}

const PREP = /^(?:finely|thinly|roughly|coarsely|freshly|lightly|very|well|about|plus|divided|to taste|as needed|optional|for\b|at room|room temperature|cut|chopped|diced|sliced|minced|grated|peeled|trimmed|halved|quartered|crushed|smashed|softened|melted|cubed|torn|rinsed|drained|beaten|shredded|julienned|seeded|stemmed|cored|zested|juiced|toasted|warmed|cooled|chilled|packed|sifted|separated|whisked|patted|ground|cooked|uncooked|thawed|picked|pitted|deveined|scrubbed|in\b|into\b|such as|or more|preferably|if\b|leaves|white and|torn|broken|removed|reserved|squeezed|scraped|split|sliced)/i;

function isSection(text) {
  return /:$/.test(text) || (text.length <= 40 && !/[.!?]$/.test(text));
}

/** Parse one ingredient line. Lines starting with "#" are section headings. */
export function parseIngredient(raw) {
  raw = raw.trim();
  const base = { raw, quantity: null, quantity_max: null, unit: null, item: raw, note: null, parser_version: PARSER_VERSION };
  let rest = raw;
  const notes = [];
  const q = rest.match(QTY);
  if (q) {
    base.quantity = toNumber(q[1]);
    if (q[2]) base.quantity_max = toNumber(q[2]);
    // "1 (14-ounce) can": the size note sits between quantity and unit.
    rest = rest.slice(q[0].length).replace(/^\(([^)]*)\)\s*/, (_, inner) => {
      notes.push(inner.trim());
      return '';
    });
    const lower = rest.toLowerCase();
    for (const [alias, unit] of UNITS) {
      // Single-letter aliases are case-sensitive ("T" is tablespoon, "t" teaspoon).
      const hit = alias.length === 1 ? rest.startsWith(alias) : lower.startsWith(alias.toLowerCase());
      if (hit && /^[\s.,)]|^$/.test(rest.slice(alias.length))) {
        base.unit = unit;
        rest = rest.slice(alias.length).replace(/^\.?\s*/, '').replace(/^of\s+/i, '');
        break;
      }
    }
  }
  rest = rest.replace(/\s*\(([^)]*)\)\s*/g, (_, inner) => {
    notes.push(inner.trim());
    return ' ';
  }).trim();
  // "8 bone-in, skin-on chicken thighs" keeps its comma; "garlic, minced" splits.
  const comma = [...rest.matchAll(/,\s*/g)].find((m) => PREP.test(rest.slice(m.index + m[0].length)));
  if (comma) {
    notes.push(rest.slice(comma.index + comma[0].length).trim());
    rest = rest.slice(0, comma.index).trim();
  }
  base.item = rest.replace(/[\s,;]+$/, '') || raw;
  base.note = notes.filter(Boolean).join('; ') || null;
  return base;
}

/** Turn raw lines into ingredient records, carrying "# Heading" lines as sections. */
export function parseIngredientLines(lines) {
  let section = null;
  const out = [];
  for (const line of lines) {
    const text = line.trim();
    if (!text) continue;
    if (text.startsWith('#')) {
      section = text.replace(/^#+\s*/, '').replace(/:$/, '').trim() || null;
      continue;
    }
    out.push({ section, ...parseIngredient(text) });
  }
  return out;
}

/** Turn raw step lines into step records, stripping "1." numbering. */
export function parseStepLines(lines) {
  let section = null;
  const out = [];
  for (const line of lines) {
    let text = line.trim();
    if (!text) continue;
    if (text.startsWith('#')) {
      text = text.replace(/^#+\s*/, '').trim();
      if (isSection(text)) {
        section = text.replace(/:$/, '').trim();
        continue;
      }
    }
    out.push({ section, text: text.replace(/^(?:step\s*)?\d+[.)]\s+/i, '') });
  }
  return out;
}

/** The inverse, for editing: records back to raw lines with "# " headings. */
export function toLines(records, field) {
  const lines = [];
  let section = null;
  for (const r of records) {
    if (r.section !== section) {
      if (r.section) lines.push(`# ${r.section}`);
      section = r.section;
    }
    lines.push(r[field]);
  }
  return lines;
}
