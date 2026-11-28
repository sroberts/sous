package recipes

import (
	"slices"
	"strings"
)

// Planning a dish from its step graph: each action is a task, waiting for
// the actions that make its inputs rather than for the step before, so a
// recipe's independent work (the rice boiling while the curry simmers)
// overlaps. An action takes as much of a cook as its attention says, and
// what its equipment claims (an oven at a temperature, burners, an
// appliance, a pan or container, counter space).

// link makes b wait for a.
func link(a, b *ptask) {
	b.prevs = append(b.prevs, a)
	a.succs = append(a.succs, b)
}

// after is the earliest t can start: floor, or once every task it waits
// for that's already placed has ended.
func after(t *ptask, floor int) int {
	for _, u := range t.prevs {
		if u.placed {
			floor = max(floor, u.end)
		}
	}
	return floor
}

// longest is, for each task of a dish's list (in an order where every task
// comes after those it waits for), the longest run of work from its start
// to the end of the list: its own time and the longest of what follows.
// For a chain it's the time left; for a graph, the critical path.
func longest(tasks []*ptask) map[*ptask]int {
	in := map[*ptask]bool{}
	for _, t := range tasks {
		in[t] = true
	}
	out := map[*ptask]int{}
	for j := len(tasks) - 1; j >= 0; j-- {
		t := tasks[j]
		next := 0
		for _, u := range t.succs {
			if in[u] {
				next = max(next, out[u])
			}
		}
		out[t] = t.dur + next
	}
	return out
}

// graph is dish i's step graph when it plans from one.
func (p *planner) graph(i int) *RecipeGraph {
	if i < len(p.in.graphs) {
		return p.in.graphs[i]
	}
	return nil
}

// attentionHands is how much of a cook an action takes, in tenths: all of
// one for active work; for check-ins, a minute and a half every so often
// (every 2 minutes is most of a cook, every 30 almost none, at least a
// tenth); nothing for work that looks after itself.
func attentionHands(n ActionNode) int {
	switch n.Attention.Value {
	case AttentionActive:
		return 10
	case AttentionIntermittent:
		every := 10
		if n.CheckInterval != nil && n.CheckInterval.Value > 0 {
			every = n.CheckInterval.Value
		}
		return min(10, max(1, (15+every-1)/every))
	}
	return 0
}

// graphTasks turns dish i's graph into tasks: the made-ahead part (every
// action that leads up to one whose output can wait, with those actions)
// and the day-of rest, each in an order where a task follows those it
// waits for. Setup actions that heat the oven are left out, as the planner
// adds and shares preheats itself; whatever waited for one waits for what
// it waited for.
func (p *planner) graphTasks(it MealItem, i int, r Recipe, g *RecipeGraph) (all, ahead, dayOf []*ptask) {
	idx := map[string]int{}
	for j, n := range g.Nodes {
		idx[n.ID] = j
	}
	producer := map[string]int{}
	for j, n := range g.Nodes {
		for _, id := range n.Outputs {
			producer[id] = j
		}
	}
	parents := make([][]int, len(g.Nodes))
	for j, n := range g.Nodes {
		for _, id := range n.Inputs {
			if pr, ok := producer[id]; ok && pr != j && !slices.Contains(parents[j], pr) {
				parents[j] = append(parents[j], pr)
			}
		}
	}
	order, cyclic := topoOrder(parents)
	if cyclic != nil {
		// Checked when the graph was chosen; a cycle here plans in the
		// order the actions were written.
		order = make([]int, len(g.Nodes))
		for j := range order {
			order[j] = j
		}
	}
	preheat := func(n ActionNode) bool {
		return n.Action.Value == "setup" && slices.ContainsFunc(claims(n), func(c EquipmentClaim) bool { return c.Resource == ResOven })
	}

	tasks := make([]*ptask, len(g.Nodes))
	for _, j := range order {
		n := g.Nodes[j]
		if preheat(n) {
			continue
		}
		t := &ptask{
			key: it.ID + ":g:" + n.ID, kind: KindStep, item: i, step: n.Source.Step - 1,
			text: cmpOr(n.Source.Quote, n.Text), dur: n.Duration.Value.Typical, space: 1, oven: -1,
			hands: attentionHands(n),
		}
		t.handsOn = t.hands == 10
		p.claimFor(t, n)
		if n.Hold != nil {
			t.holdMax, t.holdIn = n.Hold.Value.MaxMin, n.Hold.Value.Where
		}
		t.prog = p.in.progress[t.key]
		tasks[j] = t
	}
	// What each action waits for, looking through the preheats left out.
	var waitsOn func(j int) []*ptask
	memo := map[int][]*ptask{}
	waitsOn = func(j int) []*ptask {
		if w, ok := memo[j]; ok {
			return w
		}
		var out []*ptask
		for _, pr := range parents[j] {
			if tasks[pr] != nil {
				out = append(out, tasks[pr])
			} else {
				out = append(out, waitsOn(pr)...)
			}
		}
		memo[j] = out
		return out
	}
	var list []*ptask
	for _, j := range order {
		t := tasks[j]
		if t == nil {
			continue
		}
		for _, u := range waitsOn(j) {
			if !slices.Contains(t.prevs, u) {
				link(u, t)
			}
		}
		list = append(list, t)
	}

	// The made-ahead part: every task that leads up to one that can wait.
	made := map[*ptask]bool{}
	var mark func(t *ptask)
	mark = func(t *ptask) {
		if made[t] {
			return
		}
		made[t] = true
		for _, u := range t.prevs {
			mark(u)
		}
	}
	var last *ptask // the latest action that can wait: a reheat follows it
	for _, t := range list {
		if t.holdMax > 0 {
			mark(t)
			last = t
			if p.holdMin[i] == 0 || t.holdMax < p.holdMin[i] {
				p.holdMin[i] = t.holdMax
			}
		}
	}
	for _, t := range list {
		if made[t] {
			t.makeAhead = true
			ahead = append(ahead, t)
		} else {
			dayOf = append(dayOf, t)
		}
	}
	all = list
	if last != nil && len(dayOf) > 0 {
		if h := g.Nodes[idx[strings.TrimPrefix(last.key, it.ID+":g:")]].Hold; h != nil && h.Value.ReheatMin != nil && *h.Value.ReheatMin > 0 {
			rh := &ptask{key: it.ID + ":reheat", kind: KindReheat, item: i, step: -1, dur: *h.Value.ReheatMin, space: 1, oven: -1}
			if h.Value.ReheatTempF != nil {
				rh.equip, rh.temp = EquipOven, *h.Value.ReheatTempF
				rh.text = "Reheat " + r.Title + " in the oven"
			} else {
				rh.text = "Bring " + r.Title + " back to temperature"
			}
			rh.prog = p.in.progress[rh.key]
			// What waited for the held action now waits for its reheat.
			for _, u := range slices.Clone(last.succs) {
				if made[u] {
					continue
				}
				u.prevs = slices.DeleteFunc(u.prevs, func(x *ptask) bool { return x == last })
				last.succs = slices.DeleteFunc(last.succs, func(x *ptask) bool { return x == u })
				link(rh, u)
			}
			link(last, rh)
			dayOf = append([]*ptask{rh}, dayOf...)
			all = append(all, rh)
		}
	}
	return all, ahead, dayOf
}

// claimFor sets a task's equipment from its action's claims: the first the
// planner can hold it to. The fridge isn't counted, and counter space only
// when the kitchen says how much there is.
func (p *planner) claimFor(t *ptask, n ActionNode) {
	for _, c := range claims(n) {
		switch {
		case c.Resource == ResOven:
			t.equip, t.space = EquipOven, c.Count
			if c.TempF != nil {
				t.temp = *c.TempF
			}
		case c.Resource == ResBurner:
			t.equip, t.space = EquipStovetop, c.Count
		case strings.HasPrefix(c.Resource, resAppliance):
			t.equip, t.space = strings.TrimPrefix(c.Resource, resAppliance), c.Count
		case strings.HasPrefix(c.Resource, resVessel):
			t.equip, t.space = c.Resource, c.Count
		case c.Resource == ResCounter && p.in.kitchen.CounterZones != nil && *p.in.kitchen.CounterZones > 0:
			t.equip, t.space = ResCounter, c.Count
		default:
			continue
		}
		return
	}
}

// planGraph is a recipe's step graph when a meal plan can use it: not a
// draft, its words unchanged and without errors. why says, when it can't,
// for the warning on a meal that plans other dishes from graphs.
func planGraph(g *RecipeGraph, r Recipe, k KitchenProfile) (use *RecipeGraph, why string) {
	switch {
	case g == nil:
		return nil, "it has no step graph"
	case g.Status == GraphDraft:
		return nil, "its step graph is still a draft"
	case markStale(g, r):
		return nil, "its words have changed since its step graph was made"
	case !checkGraph(g, r, k).Valid:
		return nil, "its step graph has problems to fix"
	}
	return g, ""
}
