# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

Delegated: **Svelte 5 + Vite**, built as a static single-page app into
`internal/ui/dist` and embedded in the Go binary with `go:embed`. Chosen
because it compiles to small static assets (good for embedding and for an
offline service worker), handles a highly interactive app (recipe editor, cook
mode timers, scaling, offline sync) without hand-rolled state, and stays far
lighter than React. The UI is a plain client of the `/v1` REST API; it has no
private endpoints (`spec.md` §4).

## Users

A household of a few people who share one recipe library on their tailnet.

- **Managing** (adding, editing, organising, planning) happens mostly at a
  desk, on a Mac.
- **Cooking** happens in the kitchen, on an iPad or phone propped up on the
  counter, often with wet or messy hands and at arm's length.

Each person has their own favorites, want-to-cook list, and (later) ratings,
notes, and cook history, on top of the shared library.

## Product Purpose

Sous is the household's recipe box and kitchen companion: one place to keep
every recipe worth keeping, find the right one quickly, and cook from it
without fighting the screen. It replaces Mela, whose library (696 recipes,
mostly with photos) is imported once and then left behind.

Success: recipes are found in seconds, and cooking from Sous on a propped-up
tablet is calmer than cooking from a cookbook.

## Positioning

Self-hosted and household-owned: it runs as a service under Outpost on the
household's own machine, reachable only over their tailnet. The same library
is fully available to AI agents over MCP, with every change attributed to
the person or agent who made it.

## Operating Context

- Accessed in a browser over Tailscale, or through the Outpost login gateway.
  Installable to the home screen as a PWA.
- Works offline for recipes already opened and for the shopping list.
- Two distinct modes of use: **managing** (dense, keyboard-friendly, desk) and
  **cooking** (glanceable from a distance, large touch targets, screen kept
  awake, one step at a time).
- Who is using it comes from the Outpost gateway, or from a "who's cooking?"
  picker on each device when the gateway is off.

## Capabilities and Constraints

Basics (`spec.md`):

- Recipes with title, source link, yield and servings, prep/cook/total times,
  notes, nutrition text, and photos.
- Ingredients are parsed into quantity, unit, item, and note, with the
  original line always kept. Steps are stored individually. Both can be
  grouped under section headings ("For the sauce").
- Free-form tags; per-person favorite and want-to-cook flags.
- Search by text, tag, and ingredient.
- Soft delete with a trash and restore. A change log records who changed
  what, and whether through the UI, the API, or MCP.
- Edit conflicts are detected: a save based on a stale version is rejected
  with the current recipe.

Planned features (later specs): cook mode (step view, wake lock, ingredient
check-offs, step timers); scaling and US/metric conversion; a weekly meal plan
and a combined shopping list grouped by aisle; import from URL; collections;
per-person ratings, notes, and cook history.

## Brand Commitments

- The product name is **Sous**.

## Evidence on Hand

- The Mela export (`Recipes.melarecipes`, not committed): 696 real recipes,
  mostly from NYT Cooking and similar sources, with photos. Use it as the
  reference for realistic content: long titles, multi-section ingredient
  lists, vague times ("about 3 hours"), yields written as prose.
- No logo, testimonials, or other brand assets exist. None should be invented.

## Product Principles

1. **The kitchen is the hardest room.** Anything used while cooking must work
   at arm's length, with one hand, and at a glance.
2. **The original is never lost.** Parsed or derived data sits alongside what
   the recipe actually said, and the original is always one tap away.
3. **One library, many cooks.** The library is shared. Personal views
   (favorites, plans, history) are layered on top, never forked.
4. **Agents are household members, not superusers.** Their changes are
   attributed, logged, and as reversible as anyone's.
5. **Fast to find, calm to use.** Search and browse should feel instant; the
   interface stays out of the way of the food.

## Accessibility & Inclusion

WCAG 2.2 AA. No household-specific needs beyond that.
