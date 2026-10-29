// Prints the parser golden file: the output of web/mock/parse.js (and the
// small helpers in web/mock/server.js) for a corpus of inputs.
//
//   node scripts/parser-golden/gen.mjs > internal/recipes/testdata/parse_golden.json
//   node scripts/parser-golden/gen.mjs --local web/mock-local/recipes.json > internal/recipes/testdata/parse_golden.local.json
//
// The committed corpus is every ingredient and step in web/mock/fixtures.json
// plus the hand-written cases in corpus.json beside this script. --local
// uses the recipes in the given file instead; that is personal data and its
// output must never be committed.
import { readFileSync } from 'node:fs';
import { parseIngredientLines, parseStepLines, parseIngredient } from '../../web/mock/parse.js';

const here = (p) => new URL(p, import.meta.url).pathname;
const read = (p) => JSON.parse(readFileSync(p, 'utf8'));

// These two mirror web/mock/server.js.
function servingsFrom(text) {
  const m = (text || '').match(/(\d+)/);
  return m ? +m[1] : null;
}
function hostOf(url) {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return null;
  }
}
const preview = (lines) => lines.map((l) => (l.trim().startsWith('#') ? { section: l.replace(/^#+\s*/, ''), raw: l } : parseIngredient(l)));

const args = process.argv.slice(2);
const li = args.indexOf('--local');
const corpus = { ingredients: [], steps: [], preview: [], yields: [], urls: [] };
if (li >= 0) {
  for (const r of read(args[li + 1])) {
    if (r.ingredients?.length) corpus.ingredients.push(r.ingredients);
    if (r.steps?.length) corpus.steps.push(r.steps);
    if (r.ingredients?.length) corpus.preview.push(r.ingredients);
    if (r.yield_text) corpus.yields.push(r.yield_text);
    if (r.source_url) corpus.urls.push(r.source_url);
  }
} else {
  for (const r of read(here('../../web/mock/fixtures.json'))) {
    corpus.ingredients.push(r.ingredients ?? []);
    corpus.steps.push(r.steps ?? []);
    corpus.preview.push(r.ingredients ?? []);
    if (r.yield_text) corpus.yields.push(r.yield_text);
    if (r.source_url) corpus.urls.push(r.source_url);
  }
  const hand = read(here('corpus.json'));
  for (const k of Object.keys(corpus)) corpus[k].push(...(hand[k] ?? []));
}

const golden = {
  ingredients: corpus.ingredients.map((lines) => ({ lines, out: parseIngredientLines(lines) })),
  steps: corpus.steps.map((lines) => ({ lines, out: parseStepLines(lines) })),
  preview: corpus.preview.map((lines) => ({ lines, out: preview(lines) })),
  servings: corpus.yields.map((text) => ({ text, out: servingsFrom(text) })),
  hosts: corpus.urls.map((url) => ({ url, out: hostOf(url) })),
};
process.stdout.write(JSON.stringify(golden) + '\n');
