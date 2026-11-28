package recipes

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// A step graph (ontology 2.0) describes a recipe's method as atomic actions
// joined by what they make and use: a node depends on every node that
// produces one of its inputs, not on the order the steps are written in.
// Each classified field is a Label carrying where it came from and how sure
// that source is, so review can go where the labels are unsure. The
// recipe's own steps stay the record; nodes point back at the words they
// come from.

// OntologyVersion is the step graph shape this build reads and writes.
const OntologyVersion = "2.0"

// Graph statuses as stored. A graph whose nodes no longer match its
// recipe's words reads as GraphStale.
const (
	GraphDraft       = "draft"
	GraphNeedsReview = "needs_review"
	GraphVerified    = "verified"
	GraphStale       = "stale"
)

// Actions a node can be.
var graphActions = []string{"prep", "combine", "heat", "cool", "rest", "ferment", "transfer", "assemble", "plate", "setup"}

// How much of the cook a node takes.
const (
	AttentionActive       = "active"
	AttentionIntermittent = "intermittent"
	AttentionPassive      = "passive"
)

var (
	graphAttention = []string{AttentionActive, AttentionIntermittent, AttentionPassive}
	graphAnchors   = []string{"asap_after_inputs", "flexible", "just_before_serve"}
	graphItemKinds = []string{"ingredient", "intermediate", "final"}
	graphScopes    = []string{"whole_node", "start_only", "end_only"}
	graphSources   = []string{"author", "recipe_text", "model", "jev", "user"}
	graphHoldIn    = []string{HoldFridge, HoldCounter, HoldWarm}
)

// Equipment resources. Appliances and vessels are named after a colon, as
// the kitchen lists them: appliance:slow cooker, vessel:dutch oven.
const (
	ResOven       = "oven"
	ResBurner     = "burner"
	ResCounter    = "counter"
	ResFridge     = "fridge"
	resAppliance  = "appliance:"
	resVessel     = "vessel:"
	maxGraphNodes = 300
	maxGraphItems = 500
	maxNodeLinks  = 50
	maxHoldMin    = 30 * 24 * 60
	maxCheckEvery = 24 * 60
	maxClaimCount = maxSpace
	graphIDChars  = 64
)

var graphID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Label is one classified field with its provenance. reviewed_by and
// reviewed_at are set by Sous, not the caller: a label is reviewed when a
// graph is saved as verified, and keeps that while its value is unchanged.
type Label[T any] struct {
	Value      T        `json:"value"`
	Source     string   `json:"source,omitempty" jsonschema:"where the value came from: author, recipe_text (read from the words), model, jev or user; unset is user"`
	Confidence *float64 `json:"confidence,omitempty" jsonschema:"0 to 1: how sure the source is. Unset is 1 for author, recipe_text and user; model and jev must say"`
	ReviewedBy *string  `json:"reviewed_by,omitempty" jsonschema:"set by Sous: who confirmed it, by saving the graph as verified"`
	ReviewedAt *string  `json:"reviewed_at,omitempty" jsonschema:"set by Sous"`
	// Alternatives are the next answers a model weighed, for review to
	// offer beside the value.
	Alternatives []Alternative[T] `json:"alternatives,omitempty" jsonschema:"other answers the source weighed, most likely first, with their probabilities"`
}

// Alternative is an answer a model weighed and didn't pick.
type Alternative[T any] struct {
	Value       T       `json:"value"`
	Probability float64 `json:"probability"`
}

// Duration is a node's time in minutes, as a range: the planner picks
// within it.
type Duration struct {
	Min     int `json:"min"`
	Typical int `json:"typical"`
	Max     int `json:"max"`
}

// Hold is how long a node's output can wait before whatever uses it next.
type Hold struct {
	MaxMin      int    `json:"max_min" jsonschema:"minutes it keeps"`
	Where       string `json:"where" jsonschema:"fridge, counter or warm"`
	ReheatMin   *int   `json:"reheat_min,omitempty" jsonschema:"minutes to bring it back before it's used"`
	ReheatTempF *int   `json:"reheat_temp_f,omitempty" jsonschema:"oven temperature to reheat at, °F; unset brings it back out of the oven"`
}

// EquipmentClaim is a resource a node occupies.
type EquipmentClaim struct {
	Resource string `json:"resource" jsonschema:"oven, burner, counter, fridge, appliance:<name> or vessel:<name>, named as in the kitchen"`
	Count    int    `json:"count,omitempty" jsonschema:"racks, burners, units or places taken at once; unset is 1"`
	TempF    *int   `json:"temp_f,omitempty" jsonschema:"oven temperature, °F"`
	Scope    string `json:"scope,omitempty" jsonschema:"whole_node (unset), start_only or end_only"`
}

// SourceSpan points a node at the words it comes from: characters start to
// end of step number step. An implied node, which the words never state,
// names the step it serves and may leave the span empty.
type SourceSpan struct {
	Step  int    `json:"step" jsonschema:"the step's number, from 1, as get_step_timing numbers them"`
	Start int    `json:"start" jsonschema:"first character of the span, from 0"`
	End   int    `json:"end" jsonschema:"character after the span's last"`
	Quote string `json:"quote,omitempty" jsonschema:"set by Sous: the words the span covered when the graph was saved"`
}

// ActionNode is one atomic action.
type ActionNode struct {
	ID            string                   `json:"id" jsonschema:"letters, digits, - or _; kept across re-saves so labels keep their review"`
	Source        SourceSpan               `json:"source"`
	Text          string                   `json:"text" jsonschema:"the action restated on its own, for display"`
	Action        Label[string]            `json:"action" jsonschema:"prep, combine, heat, cool, rest, ferment, transfer, assemble, plate or setup (preheat, boil water)"`
	Technique     *string                  `json:"technique,omitempty" jsonschema:"what kind of action: dice, sear, fold, knead"`
	Inputs        []string                 `json:"inputs,omitempty" jsonschema:"ids of the items it uses"`
	Outputs       []string                 `json:"outputs" jsonschema:"ids of the items it makes"`
	Duration      Label[Duration]          `json:"duration" jsonschema:"minutes; times stated in the recipe win over estimates"`
	Attention     Label[string]            `json:"attention" jsonschema:"active (the cook is busy throughout), intermittent (check-ins) or passive (looks after itself)"`
	CheckInterval *Label[int]              `json:"check_interval_min,omitempty" jsonschema:"minutes between check-ins; needed when attention is intermittent"`
	Interruptible *Label[bool]             `json:"interruptible,omitempty" jsonschema:"whether the cook can step away mid-action without harm"`
	Equipment     *Label[[]EquipmentClaim] `json:"equipment,omitempty" jsonschema:"what it occupies; unset or an empty list is nothing"`
	Hold          *Label[Hold]             `json:"hold,omitempty" jsonschema:"how long its output can wait before it's used next"`
	TimingAnchor  *Label[string]           `json:"timing_anchor,omitempty" jsonschema:"asap_after_inputs, flexible or just_before_serve"`
	Implied       bool                     `json:"implied,omitempty" jsonschema:"true for an action the recipe never states, such as a preheat or boiling water"`
	Stale         bool                     `json:"stale,omitempty" jsonschema:"set by Sous: the words its span covered have changed since the graph was saved"`
}

// GraphItem is anything a node uses or makes.
type GraphItem struct {
	ID         string  `json:"id" jsonschema:"letters, digits, - or _"`
	Kind       string  `json:"kind" jsonschema:"ingredient (from the ingredient list), intermediate (made along the way) or final"`
	Label      string  `json:"label" jsonschema:"what it is: butter, the dough, reserved pasta water"`
	Ingredient *int    `json:"ingredient,omitempty" jsonschema:"for an ingredient, its line in the ingredient list, from 1"`
	Quantity   *string `json:"quantity,omitempty" jsonschema:"how much, as written"`
}

// GraphInput is a step graph as a client sends it.
type GraphInput struct {
	Status      string       `json:"status,omitempty" jsonschema:"draft (unset), needs_review or verified; verified needs a graph with no errors and marks every label reviewed"`
	ServeItemID string       `json:"serve_item_id" jsonschema:"the final item that goes to the table"`
	Nodes       []ActionNode `json:"nodes"`
	Items       []GraphItem  `json:"items"`
}

// RecipeGraph is a recipe's step graph as stored.
type RecipeGraph struct {
	RecipeID        string       `json:"recipe_id"`
	RecipeVersion   int          `json:"recipe_version" jsonschema:"the recipe's version the graph was saved against"`
	Version         int          `json:"version" jsonschema:"the graph's own version; send it back to change or delete it"`
	OntologyVersion string       `json:"ontology_version"`
	Status          string       `json:"status" jsonschema:"draft, needs_review, verified, or stale when the recipe's words have changed under some nodes"`
	ServeItemID     string       `json:"serve_item_id"`
	Nodes           []ActionNode `json:"nodes"`
	Items           []GraphItem  `json:"items"`
	CreatedAt       string       `json:"created_at"`
	CreatedBy       string       `json:"created_by"`
	UpdatedAt       string       `json:"updated_at"`
	UpdatedBy       string       `json:"updated_by"`
}

// GraphProblem is one thing the checks found.
type GraphProblem struct {
	Code    string  `json:"code" jsonschema:"stable: what kind of problem"`
	Message string  `json:"message"`
	Node    *string `json:"node,omitempty" jsonschema:"the node it's about"`
	Item    *string `json:"item,omitempty" jsonschema:"the item it's about"`
}

// GraphCheck is what the deterministic checks found. Errors make a graph
// unfit to verify; warnings don't (most are about the kitchen, which can
// change after a graph is saved).
type GraphCheck struct {
	Valid    bool           `json:"valid"`
	Errors   []GraphProblem `json:"errors"`
	Warnings []GraphProblem `json:"warnings"`
}

// GraphView is GET /v1/recipes/{id}/graph: the graph, what the checks find
// in it now, and the step timing it comes to for planners that read
// timing by step.
type GraphView struct {
	Graph       RecipeGraph   `json:"graph"`
	ReviewCount int           `json:"review_count" jsonschema:"labels to confirm or correct: those below their field's threshold, and implied actions, not yet reviewed"`
	Check       GraphCheck    `json:"check"`
	Projection  []*StepTiming `json:"projection" jsonschema:"one entry per step (null for a step without nodes): minutes summed, hands-on when most of it is active, the longest-running oven, stovetop or appliance, and the shortest hold"`
}

// graphLabelRef names one label of a graph: a node and a field.
type graphLabelRef struct{ node, field string }

// problems collects errors and warnings.
type problems struct{ errs, warns []GraphProblem }

func (p *problems) err(code, node, item, format string, args ...any) {
	p.errs = append(p.errs, problem(code, node, item, format, args...))
}

func (p *problems) warn(code, node, item, format string, args ...any) {
	p.warns = append(p.warns, problem(code, node, item, format, args...))
}

func problem(code, node, item, format string, args ...any) GraphProblem {
	g := GraphProblem{Code: code, Message: fmt.Sprintf(format, args...)}
	if node != "" {
		g.Node = &node
	}
	if item != "" {
		g.Item = &item
	}
	return g
}

// normaliseGraph tidies a graph as sent (defaults, trimmed text, quotes
// from the recipe's words) and refuses what can't be stored at all: a bad
// enum, id or range is CodeInvalid. Whether the graph hangs together is
// checkGraph's job, so a draft with loose ends can still be saved.
func normaliseGraph(g *GraphInput, r Recipe) error {
	if g.Status == "" {
		g.Status = GraphDraft
	}
	if !slices.Contains([]string{GraphDraft, GraphNeedsReview, GraphVerified}, g.Status) {
		return Errorf(CodeInvalid, "status must be draft, needs_review or verified")
	}
	if len(g.Nodes) == 0 {
		return Errorf(CodeInvalid, "nodes: a graph needs at least one")
	}
	if len(g.Nodes) > maxGraphNodes {
		return Errorf(CodeInvalid, "nodes: at most %d", maxGraphNodes)
	}
	if len(g.Items) > maxGraphItems {
		return Errorf(CodeInvalid, "items: at most %d", maxGraphItems)
	}
	g.ServeItemID = strings.TrimSpace(g.ServeItemID)
	seen := map[string]bool{}
	for i := range g.Items {
		it := &g.Items[i]
		it.ID = strings.TrimSpace(it.ID)
		if err := checkGraphID("items", it.ID, seen); err != nil {
			return err
		}
		if !slices.Contains(graphItemKinds, it.Kind) {
			return Errorf(CodeInvalid, "items: %s: kind must be ingredient, intermediate or final", it.ID)
		}
		it.Label = strings.Join(strings.Fields(it.Label), " ")
		if it.Label == "" || len(it.Label) > 200 {
			return Errorf(CodeInvalid, "items: %s: label must be 1 to 200 characters", it.ID)
		}
		if it.Ingredient != nil {
			if it.Kind != "ingredient" {
				return Errorf(CodeInvalid, "items: %s: only an ingredient has an ingredient line", it.ID)
			}
			if *it.Ingredient < 1 || *it.Ingredient > len(r.Ingredients) {
				return Errorf(CodeInvalid, "items: %s: ingredient %d isn't a line of the recipe's %d", it.ID, *it.Ingredient, len(r.Ingredients))
			}
		}
		if it.Quantity != nil {
			q := strings.TrimSpace(*it.Quantity)
			if len(q) > 100 {
				return Errorf(CodeInvalid, "items: %s: quantity is at most 100 characters", it.ID)
			}
			it.Quantity = &q
			if q == "" {
				it.Quantity = nil
			}
		}
	}
	seen = map[string]bool{}
	for i := range g.Nodes {
		if err := normaliseNode(&g.Nodes[i], r, seen); err != nil {
			return err
		}
	}
	return nil
}

func checkGraphID(what, id string, seen map[string]bool) error {
	if id == "" || len(id) > graphIDChars || !graphID.MatchString(id) {
		return Errorf(CodeInvalid, "%s: id %q must be 1 to %d letters, digits, - or _", what, id, graphIDChars)
	}
	if seen[id] {
		return Errorf(CodeInvalid, "%s: id %s is used twice", what, id)
	}
	seen[id] = true
	return nil
}

func normaliseNode(n *ActionNode, r Recipe, seen map[string]bool) error {
	n.ID = strings.TrimSpace(n.ID)
	if err := checkGraphID("nodes", n.ID, seen); err != nil {
		return err
	}
	at := func(format string, args ...any) error {
		return Errorf(CodeInvalid, "nodes: %s: %s", n.ID, fmt.Sprintf(format, args...))
	}
	n.Stale = false
	n.Text = strings.Join(strings.Fields(n.Text), " ")
	if n.Text == "" || len(n.Text) > 500 {
		return at("text must be 1 to 500 characters")
	}
	if n.Technique != nil {
		t := strings.ToLower(strings.Join(strings.Fields(*n.Technique), " "))
		if len(t) > 60 {
			return at("technique is at most 60 characters")
		}
		n.Technique = &t
		if t == "" {
			n.Technique = nil
		}
	}
	// The span, against the recipe's words as they are now.
	s := &n.Source
	if s.Step < 1 || s.Step > len(r.Steps) {
		return at("source step %d isn't one of the recipe's %d steps", s.Step, len(r.Steps))
	}
	words := []rune(r.Steps[s.Step-1].Text)
	if s.Start < 0 || s.End > len(words) || s.Start > s.End {
		return at("source span %d to %d is outside step %d, which is %d characters", s.Start, s.End, s.Step, len(words))
	}
	if s.Start == s.End && !n.Implied {
		return at("source span is empty; only an implied node may leave it empty")
	}
	s.Quote = string(words[s.Start:s.End])

	if n.Inputs == nil {
		n.Inputs = []string{}
	}
	if n.Outputs == nil {
		n.Outputs = []string{}
	}
	if len(n.Inputs) > maxNodeLinks || len(n.Outputs) > maxNodeLinks {
		return at("at most %d inputs and %d outputs", maxNodeLinks, maxNodeLinks)
	}

	if err := n.Action.check(); err != nil {
		return at("action: %v", err)
	}
	if !slices.Contains(graphActions, n.Action.Value) {
		return at("action must be one of %s", strings.Join(graphActions, ", "))
	}
	if err := n.Duration.check(); err != nil {
		return at("duration: %v", err)
	}
	d := n.Duration.Value
	if d.Min < 0 || d.Min > d.Typical || d.Typical > d.Max || d.Max > maxStepMinutes {
		return at("duration needs 0 ≤ min ≤ typical ≤ max ≤ %d minutes", maxStepMinutes)
	}
	if err := n.Attention.check(); err != nil {
		return at("attention: %v", err)
	}
	if !slices.Contains(graphAttention, n.Attention.Value) {
		return at("attention must be active, intermittent or passive")
	}
	if c := n.CheckInterval; c != nil {
		if err := c.check(); err != nil {
			return at("check_interval_min: %v", err)
		}
		if c.Value < 1 || c.Value > maxCheckEvery {
			return at("check_interval_min must be 1 to %d", maxCheckEvery)
		}
	}
	if c := n.Interruptible; c != nil {
		if err := c.check(); err != nil {
			return at("interruptible: %v", err)
		}
	}
	if a := n.TimingAnchor; a != nil {
		if err := a.check(); err != nil {
			return at("timing_anchor: %v", err)
		}
		if !slices.Contains(graphAnchors, a.Value) {
			return at("timing_anchor must be asap_after_inputs, flexible or just_before_serve")
		}
	}
	if h := n.Hold; h != nil {
		if err := h.check(); err != nil {
			return at("hold: %v", err)
		}
		v := &h.Value
		v.Where = strings.ToLower(strings.TrimSpace(v.Where))
		if v.MaxMin < 1 || v.MaxMin > maxHoldMin {
			return at("hold max_min must be 1 to %d", maxHoldMin)
		}
		if !slices.Contains(graphHoldIn, v.Where) {
			return at("hold where must be fridge, counter or warm")
		}
		if v.ReheatMin != nil && (*v.ReheatMin < 0 || *v.ReheatMin > maxStepMinutes) {
			return at("hold reheat_min must be 0 to %d", maxStepMinutes)
		}
		if v.ReheatTempF != nil && (*v.ReheatTempF < minOvenTemp || *v.ReheatTempF > maxOvenTemp) {
			return at("hold reheat_temp_f must be %d to %d", minOvenTemp, maxOvenTemp)
		}
	}
	if e := n.Equipment; e != nil {
		if err := e.check(); err != nil {
			return at("equipment: %v", err)
		}
		if e.Value == nil {
			e.Value = []EquipmentClaim{}
		}
		if len(e.Value) > 10 {
			return at("equipment: at most 10 claims")
		}
		for i := range e.Value {
			if err := normaliseClaim(&e.Value[i]); err != nil {
				return at("equipment: %v", err)
			}
		}
	}
	return nil
}

func normaliseClaim(c *EquipmentClaim) error {
	r := strings.ToLower(strings.Join(strings.Fields(c.Resource), " "))
	if kind, name, ok := strings.Cut(r, ":"); ok {
		name = strings.TrimSpace(name)
		r = kind + ":" + name
		if (kind+":" != resAppliance && kind+":" != resVessel) || name == "" || len(name) > 40 {
			return Errorf(CodeInvalid, "resource %q must be oven, burner, counter, fridge, appliance:<name> or vessel:<name>", c.Resource)
		}
	} else if !slices.Contains([]string{ResOven, ResBurner, ResCounter, ResFridge}, r) {
		return Errorf(CodeInvalid, "resource %q must be oven, burner, counter, fridge, appliance:<name> or vessel:<name>", c.Resource)
	}
	c.Resource = r
	if c.Count == 0 {
		c.Count = 1
	}
	if c.Count < 1 || c.Count > maxClaimCount {
		return Errorf(CodeInvalid, "%s: count must be 1 to %d", r, maxClaimCount)
	}
	if c.TempF != nil {
		if r != ResOven {
			return Errorf(CodeInvalid, "%s: only the oven takes temp_f", r)
		}
		if *c.TempF < minOvenTemp || *c.TempF > maxOvenTemp {
			return Errorf(CodeInvalid, "oven temp_f must be %d to %d", minOvenTemp, maxOvenTemp)
		}
	}
	if c.Scope == "" {
		c.Scope = "whole_node"
	}
	if !slices.Contains(graphScopes, c.Scope) {
		return Errorf(CodeInvalid, "%s: scope must be whole_node, start_only or end_only", r)
	}
	return nil
}

// check fills a label's defaults and checks its source and confidence.
// Review stamps are Sous's to set, so whatever was sent is dropped.
func (l *Label[T]) check() error {
	l.ReviewedBy, l.ReviewedAt = nil, nil
	if l.Source == "" {
		l.Source = "user"
	}
	if !slices.Contains(graphSources, l.Source) {
		return fmt.Errorf("source must be one of %s", strings.Join(graphSources, ", "))
	}
	if l.Confidence == nil {
		if l.Source == "model" || l.Source == "jev" {
			return fmt.Errorf("a %s label needs a confidence", l.Source)
		}
		one := 1.0
		l.Confidence = &one
	}
	if c := *l.Confidence; c < 0 || c > 1 || c != c {
		return fmt.Errorf("confidence must be 0 to 1")
	}
	if len(l.Alternatives) > 5 {
		return fmt.Errorf("at most 5 alternatives")
	}
	for _, a := range l.Alternatives {
		if a.Probability < 0 || a.Probability > 1 || a.Probability != a.Probability {
			return fmt.Errorf("an alternative's probability must be 0 to 1")
		}
	}
	return nil
}

// checkGraph runs the deterministic checks on a normalised graph: every
// input resolves, nothing is made twice, there are no cycles, every output
// is used or served, every bake has a preheat before it, and intermittent
// work says how often to check. Kitchen fit is only warned about. It also
// marks nodes whose words have changed in the recipe since.
func checkGraph(g *RecipeGraph, r Recipe, k KitchenProfile) GraphCheck {
	var p problems
	items := map[string]GraphItem{}
	for _, it := range g.Items {
		items[it.ID] = it
	}
	producer := map[string]string{}
	consumers := map[string][]string{}
	for _, n := range g.Nodes {
		for _, id := range n.Outputs {
			it, ok := items[id]
			switch {
			case !ok:
				p.err("unknown_item", n.ID, id, "%s makes %s, which isn't an item", n.ID, id)
			case it.Kind == "ingredient":
				p.err("makes_ingredient", n.ID, id, "%s makes %s, but ingredients come from the list, not from a step", n.ID, it.Label)
			case producer[id] != "":
				p.err("made_twice", n.ID, id, "%s is made by both %s and %s", it.Label, producer[id], n.ID)
			default:
				producer[id] = n.ID
			}
		}
	}
	for _, n := range g.Nodes {
		for _, id := range n.Inputs {
			it, ok := items[id]
			switch {
			case !ok:
				p.err("unknown_item", n.ID, id, "%s uses %s, which isn't an item", n.ID, id)
			case it.Kind != "ingredient" && producer[id] == "":
				p.err("dangling_input", n.ID, id, "%s uses %s, but no node makes it", n.ID, it.Label)
			}
			if slices.Contains(n.Outputs, id) {
				p.err("cycle", n.ID, id, "%s both uses and makes %s", n.ID, id)
			}
			consumers[id] = append(consumers[id], n.ID)
		}
	}

	serve, ok := items[g.ServeItemID]
	switch {
	case g.ServeItemID == "":
		p.err("no_serve_item", "", "", "serve_item_id names no item")
	case !ok:
		p.err("unknown_item", "", g.ServeItemID, "serve_item_id %s isn't an item", g.ServeItemID)
	case serve.Kind != "final":
		p.err("serve_not_final", "", serve.ID, "what's served, %s, should be a final item", serve.Label)
	case producer[serve.ID] == "":
		p.err("dangling_input", "", serve.ID, "no node makes %s, which is served", serve.Label)
	}
	for _, it := range g.Items {
		used := len(consumers[it.ID]) > 0
		switch {
		case it.Kind == "ingredient" && !used:
			p.warn("unused_item", "", it.ID, "nothing uses %s", it.Label)
		case it.Kind != "ingredient" && producer[it.ID] != "" && !used && it.ID != g.ServeItemID:
			p.err("unused_output", producer[it.ID], it.ID, "%s makes %s, but nothing uses it and it isn't served", producer[it.ID], it.Label)
		case it.Kind != "ingredient" && producer[it.ID] == "" && !used:
			p.warn("unused_item", "", it.ID, "nothing makes or uses %s", it.Label)
		}
		if it.Kind == "ingredient" && it.Ingredient == nil {
			p.warn("unlisted_ingredient", "", it.ID, "%s isn't tied to a line of the ingredient list", it.Label)
		}
	}
	listed := map[int]bool{}
	for _, it := range g.Items {
		if it.Ingredient != nil {
			listed[*it.Ingredient] = true
		}
	}
	for i, ing := range r.Ingredients {
		if !listed[i+1] {
			p.warn("unused_ingredient", "", "", "ingredient line %d (%s) isn't an item", i+1, strings.TrimSpace(ing.Raw))
		}
	}

	// Edges and cycles. parents[n] are the nodes making n's inputs.
	idx := map[string]int{}
	for i, n := range g.Nodes {
		idx[n.ID] = i
	}
	parents := make([][]int, len(g.Nodes))
	for i, n := range g.Nodes {
		for _, id := range n.Inputs {
			if pr, ok := producer[id]; ok && pr != n.ID {
				parents[i] = append(parents[i], idx[pr])
			}
		}
	}
	order, cyclic := topoOrder(parents)
	if cyclic != nil {
		names := make([]string, len(cyclic))
		for i, c := range cyclic {
			names[i] = g.Nodes[c].ID
		}
		p.err("cycle", names[0], "", "these nodes wait on each other in a circle: %s", strings.Join(names, ", "))
	}

	// Preheats: an oven claim needs a setup node claiming the oven at a
	// temperature within 25°F somewhere before it.
	if cyclic == nil {
		anc := ancestors(parents, order)
		for i, n := range g.Nodes {
			if n.Action.Value == "setup" {
				continue
			}
			for _, c := range claims(n) {
				if c.Resource != ResOven {
					continue
				}
				found := false
				for _, a := range anc[i] {
					if g.Nodes[a].Action.Value == "setup" && heatsFor(g.Nodes[a], c) {
						found = true
						break
					}
				}
				if !found {
					p.err("no_preheat", n.ID, "", "%s uses the oven, but no preheat comes before it", n.ID)
				}
			}
		}
	}

	covered := map[int]bool{}
	for _, n := range g.Nodes {
		covered[n.Source.Step] = true
		if n.Attention.Value == AttentionIntermittent && n.CheckInterval == nil {
			p.err("no_check_interval", n.ID, "", "%s is intermittent but doesn't say how often to check", n.ID)
		}
		for _, c := range claims(n) {
			if c.Resource == ResOven && c.TempF == nil && c.Scope == "whole_node" {
				p.warn("no_oven_temp", n.ID, "", "%s uses the oven without a temperature", n.ID)
			}
			kitchenFit(&p, n.ID, c, k)
		}
		// Stale: the words under the span have changed since.
		if n.Source.Step <= len(r.Steps) {
			words := []rune(r.Steps[n.Source.Step-1].Text)
			s := n.Source
			if s.End > len(words) || string(words[s.Start:s.End]) != s.Quote {
				p.warn("stale", n.ID, "", "the words %s came from in step %d have changed", n.ID, s.Step)
			}
		} else {
			p.warn("stale", n.ID, "", "step %d, which %s came from, is gone", n.Source.Step, n.ID)
		}
	}
	for i := range r.Steps {
		if !covered[i+1] {
			p.warn("step_uncovered", "", "", "step %d has no nodes", i+1)
		}
	}
	return GraphCheck{Valid: len(p.errs) == 0, Errors: nonNil(p.errs), Warnings: nonNil(p.warns)}
}

func nonNil(ps []GraphProblem) []GraphProblem {
	if ps == nil {
		return []GraphProblem{}
	}
	return ps
}

// markStale sets Stale on nodes whose words have changed and reports
// whether any did.
func markStale(g *RecipeGraph, r Recipe) bool {
	some := false
	for i := range g.Nodes {
		n := &g.Nodes[i]
		n.Stale = true
		if n.Source.Step <= len(r.Steps) {
			words := []rune(r.Steps[n.Source.Step-1].Text)
			s := n.Source
			n.Stale = s.End > len(words) || string(words[s.Start:s.End]) != s.Quote
		}
		some = some || n.Stale
	}
	return some
}

func claims(n ActionNode) []EquipmentClaim {
	if n.Equipment == nil {
		return nil
	}
	return n.Equipment.Value
}

// heatsFor reports whether setup node s heats the oven for claim c.
func heatsFor(s ActionNode, c EquipmentClaim) bool {
	for _, h := range claims(s) {
		if h.Resource != ResOven {
			continue
		}
		if h.TempF == nil || c.TempF == nil || abs(*h.TempF-*c.TempF) <= 25 {
			return true
		}
	}
	return false
}

func kitchenFit(p *problems, node string, c EquipmentClaim, k KitchenProfile) {
	switch {
	case c.Resource == ResOven:
		most := 0
		for _, o := range k.Ovens {
			most = max(most, o.Racks)
		}
		if len(k.Ovens) == 0 {
			p.warn("not_in_kitchen", node, "", "%s uses an oven, and the kitchen has none", node)
		} else if c.Count > most {
			p.warn("too_big_for_kitchen", node, "", "%s takes %d racks; the biggest oven has %d", node, c.Count, most)
		}
	case c.Resource == ResBurner:
		if c.Count > k.Burners {
			p.warn("too_big_for_kitchen", node, "", "%s takes %d burners; the kitchen has %d", node, c.Count, k.Burners)
		}
	case c.Resource == ResCounter:
		if k.CounterZones != nil && *k.CounterZones > 0 && c.Count > *k.CounterZones {
			z := *k.CounterZones
			p.warn("too_big_for_kitchen", node, "", "%s takes %d places on the counter; the kitchen has %d", node, c.Count, z)
		}
	case strings.HasPrefix(c.Resource, resAppliance):
		name := strings.TrimPrefix(c.Resource, resAppliance)
		i := slices.IndexFunc(k.Appliances, func(a Appliance) bool { return a.Name == name })
		if i < 0 {
			p.warn("not_in_kitchen", node, "", "%s uses a %s, which isn't in the kitchen", node, name)
		} else if c.Count > k.Appliances[i].Count {
			p.warn("too_big_for_kitchen", node, "", "%s takes %d of the kitchen's %d %s", node, c.Count, k.Appliances[i].Count, name)
		}
	case strings.HasPrefix(c.Resource, resVessel):
		name := strings.TrimPrefix(c.Resource, resVessel)
		i := slices.IndexFunc(k.Vessels, func(v Vessel) bool { return v.Name == name })
		if i < 0 {
			p.warn("not_in_kitchen", node, "", "%s uses a %s, which isn't in the kitchen's pans and containers", node, name)
		} else if c.Count > k.Vessels[i].Count {
			p.warn("too_big_for_kitchen", node, "", "%s takes %d of the kitchen's %d %s", node, c.Count, k.Vessels[i].Count, name)
		}
	}
}

// topoOrder orders nodes so each comes after its parents. When there's a
// cycle it returns the nodes caught in one instead.
func topoOrder(parents [][]int) (order, cyclic []int) {
	n := len(parents)
	children := make([][]int, n)
	indeg := make([]int, n)
	for i, ps := range parents {
		for _, pr := range ps {
			children[pr] = append(children[pr], i)
			indeg[i]++
		}
	}
	var queue []int
	for i := range n {
		if indeg[i] == 0 {
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		order = append(order, v)
		for _, c := range children[v] {
			if indeg[c]--; indeg[c] == 0 {
				queue = append(queue, c)
			}
		}
	}
	if len(order) == n {
		return order, nil
	}
	for i := range n {
		if indeg[i] > 0 {
			cyclic = append(cyclic, i)
		}
	}
	return nil, cyclic
}

// ancestors lists every node before each, given a topological order.
func ancestors(parents [][]int, order []int) [][]int {
	sets := make([]map[int]bool, len(parents))
	for _, v := range order {
		sets[v] = map[int]bool{}
		for _, pr := range parents[v] {
			sets[v][pr] = true
			for a := range sets[pr] {
				sets[v][a] = true
			}
		}
	}
	out := make([][]int, len(parents))
	for v, s := range sets {
		for a := range s {
			out[v] = append(out[v], a)
		}
		slices.Sort(out[v])
	}
	return out
}

// project turns a graph into step timing, one entry per step, for planners
// that schedule steps rather than nodes. Preheats are left out: the
// planner adds its own. Per step: minutes are the nodes' typical minutes
// summed; hands-on when active work is at least half of them; equipment
// is the oven, stovetop or appliance held longest (vessels, the counter
// and the fridge have no step timing equivalent); the hold is the
// shortest of any node's in the step.
func project(g RecipeGraph, steps int) []*StepTiming {
	out := make([]*StepTiming, steps)
	type acc struct {
		total, active, held int
		equip               *string
		temp, space         *int
		hold                *Hold
	}
	per := make([]*acc, steps)
	for _, n := range g.Nodes {
		if n.Action.Value == "setup" && slices.ContainsFunc(claims(n), func(c EquipmentClaim) bool { return c.Resource == ResOven }) {
			continue
		}
		i := n.Source.Step - 1
		if i < 0 || i >= steps {
			continue
		}
		a := per[i]
		if a == nil {
			a = &acc{}
			per[i] = a
		}
		m := n.Duration.Value.Typical
		a.total += m
		if n.Attention.Value == AttentionActive {
			a.active += m
		}
		for _, c := range claims(n) {
			var e string
			switch {
			case c.Resource == ResOven:
				e = EquipOven
			case c.Resource == ResBurner:
				e = EquipStovetop
			case strings.HasPrefix(c.Resource, resAppliance):
				e = strings.TrimPrefix(c.Resource, resAppliance)
			default:
				continue
			}
			if a.equip == nil || m > a.held {
				a.equip, a.held, a.temp = &e, m, c.TempF
				space := c.Count
				a.space = &space
				if space == 1 {
					a.space = nil
				}
			}
		}
		if n.Hold != nil && (a.hold == nil || n.Hold.Value.MaxMin < a.hold.MaxMin) {
			h := n.Hold.Value
			a.hold = &h
		}
	}
	for i, a := range per {
		if a == nil {
			continue
		}
		t := &StepTiming{Minutes: &a.total, Equipment: a.equip, OvenTemp: a.temp, Space: a.space}
		handsOn := a.active*2 >= a.total
		t.HandsOn = &handsOn
		if a.hold != nil {
			t.HoldMinutes, t.HoldIn = &a.hold.MaxMin, &a.hold.Where
			t.ReheatMinutes, t.ReheatTemp = a.hold.ReheatMin, a.hold.ReheatTempF
		}
		out[i] = t
	}
	return out
}
