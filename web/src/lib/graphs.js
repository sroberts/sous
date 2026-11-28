// Words for step graphs: what each field and value means to a cook, so the
// review never shows an enum.

export const STATUS = {
  none: 'No step graph',
  draft: 'Draft',
  needs_review: 'To review',
  verified: 'Verified',
  stale: 'Words changed',
};

// The question each field answers, as review asks it.
export const ASK = {
  action: 'What is it?',
  attention: 'How hands-on?',
  check_interval_min: 'How often to check?',
  equipment: 'What does it use?',
  hold: 'Can it wait?',
  timing_anchor: 'When should it happen?',
  duration: 'How long?',
  interruptible: 'Can it be left?',
};

export const ACTIONS = {
  prep: 'Prep',
  combine: 'Mix',
  heat: 'Cook',
  cool: 'Cool',
  rest: 'Rest',
  ferment: 'Rise',
  transfer: 'Move',
  assemble: 'Put together',
  plate: 'Serve',
  setup: 'Get ready',
};

export const ATTENTION = {
  active: 'Busy the whole time',
  intermittent: 'A check now and then',
  passive: 'Looks after itself',
};

export const ANCHORS = {
  asap_after_inputs: 'As soon as it can',
  flexible: 'Any time before',
  just_before_serve: 'Just before serving',
};

export const HOLD_IN = { fridge: 'in the fridge', counter: 'on the counter', warm: 'kept warm' };

const ENUMS = { action: ACTIONS, attention: ATTENTION, timing_anchor: ANCHORS };

// The choices "Something else" offers for a field with a fixed set.
export function choicesFor(field) {
  return ENUMS[field] ? Object.entries(ENUMS[field]) : null;
}

export function minutes(m) {
  if (m < 60) return `${m} min`;
  if (m < 24 * 60) {
    const h = Math.floor(m / 60);
    const r = m % 60;
    return r ? `${h} hr ${r} min` : `${h} hr`;
  }
  const d = Math.round(m / (24 * 60));
  return d === 1 ? '1 day' : `${d} days`;
}

function claim(c) {
  const r = c.resource;
  let s;
  if (r === 'oven') s = c.temp_f ? `Oven at ${c.temp_f}°F` : 'Oven';
  else if (r === 'burner') s = 'A burner';
  else if (r === 'fridge') s = 'Fridge';
  else if (r === 'counter') s = 'Counter space';
  else s = r.slice(r.indexOf(':') + 1).replace(/^./, (x) => x.toUpperCase());
  return c.count > 1 ? `${s} ×${c.count}` : s;
}

// show is a value in words.
export function show(field, v) {
  if (v === null || v === undefined) return field === 'hold' ? 'Use at once' : '—';
  switch (field) {
    case 'action':
    case 'attention':
    case 'timing_anchor':
      return ENUMS[field][v] ?? v;
    case 'duration':
      return v.min === v.max ? minutes(v.max) : `${minutes(v.min)} to ${minutes(v.max)}`;
    case 'check_interval_min':
      return `Every ${minutes(v)}`;
    case 'interruptible':
      return v ? 'Can be left' : 'Must be watched';
    case 'equipment':
      return v.length ? v.map(claim).join(', ') : 'Nothing';
    case 'hold':
      return `Up to ${minutes(v.max_min)} ${HOLD_IN[v.where] ?? v.where}`;
  }
  return String(v);
}

export const pct = (p) => `${Math.round(p * 100)}%`;

export const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);

// ofStep groups a graph's nodes under the step they come from, and marks
// each step's text into plain runs and the runs nodes quote.
export function bySteps(recipe, graph) {
  return recipe.steps.map((st, i) => {
    const nodes = graph.nodes.filter((n) => n.source.step === i + 1);
    const chars = [...st.text];
    const spans = nodes.filter((n) => !n.implied && n.source.end > n.source.start).sort((a, b) => a.source.start - b.source.start);
    const runs = [];
    let at = 0;
    for (const n of spans) {
      if (n.source.start < at) continue; // overlapping spans: the first wins
      if (n.source.start > at) runs.push({ text: chars.slice(at, n.source.start).join('') });
      runs.push({ text: chars.slice(n.source.start, n.source.end).join(''), node: n.id });
      at = n.source.end;
    }
    if (at < chars.length) runs.push({ text: chars.slice(at).join('') });
    return { number: i + 1, section: st.section, runs, nodes };
  });
}
