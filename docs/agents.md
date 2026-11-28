# AI agents and the API

## MCP

The quickest way in is the **Connect an AI assistant** page (`/assistants`,
in the person menu): it shows the address and header for this Sous, filled
in, and set-ups to copy for Claude Code, Claude Desktop and Crush.

MCP clients connect to `/mcp` on the same port, with the same tools as the
REST API, including `import_recipe`. Every change is attributed to a person:

- **On its own:** `http://127.0.0.1:8080/mcp`, with an `X-Sous-Person:
  <username>` header naming who the agent acts for.
- **Under Outpost:** connect on loopback (Outpost assigns the port; see
  `outpost status sous`) with the same header. The gateway itself only lets
  in people signed in with a browser, and `SOUS_REQUIRE_GATEWAY=true` closes
  the loopback route too, so leave it unset if agents need MCP.
- **Access tokens:** the gateway also accepts an Outpost access token
  (`Authorization: Bearer opat_…`) and passes Sous the same signed identity as
  a login. Scripts and the Siri Shortcut use this to reach Sous from outside.
  MCP still connects on loopback, as above.

Assistants that connect from their maker's cloud (ChatGPT, Claude on the web
and phone) can't reach Sous: it's only on your tailnet, and they can't send
the `X-Sous-Person` header. Use an assistant that runs on your own computer.

The tools are listed in [`spec.md`](../spec.md) §9, and the meal-planning
tools (`get_kitchen`, `update_kitchen`, `list_meals`, `get_meal`,
`create_meal`, `update_meal`, `delete_meal`, `restore_meal`, `plan_meal`,
`update_meal_task`) in §15, and the step graph tools in §16. An assistant can plan a whole meal: "Plan
Thanksgiving for 5pm Thursday with the turkey, stuffing, pie and cranberry
sauce, and tell me when the turkey goes in." `plan_meal` answers in UTC;
assistants should give times in the meal's timezone.

### Filling in step timing

Recipes saved before meal plans don't say how long each step takes, whether
it needs the cook, or what can be made ahead. Sous guesses from the words,
but an assistant reads them properly. Three tools do it, without touching a
step's words:

- `list_recipes_needing_timing` lists recipes with steps that have no
  timing, in batches (pass `meal_id` for one meal's dishes).
- `get_step_timing` shows a recipe's steps, each with its timing and Sous's
  guess.
- `set_step_timing` saves one timing entry per step (null for none).

The `fill_step_timing` prompt walks an assistant through all of it, for the
whole library or one meal. In Claude Code it's
`/mcp__sous__fill_step_timing`, or `/mcp__sous__fill_step_timing
Thanksgiving` for one meal; any assistant can also just be asked, "Fill in
the step timing for my recipes in Sous." Every change is signed and shows in
the recipe's history. The Assistants page in Sous has the same, under **Fill
in step timing**.

### Step graphs

A step graph describes a recipe's method more finely than step timing: each
step split into single actions (whisk, rub in, chill), joined by what they
make and use, so it's clear which work can happen side by side. Each action
says how long it takes as a range, how much of the cook it needs (all of
them, a check every so often, or none), what it occupies (the oven at a
temperature, a burner, a sheet pan) and how long what it makes can wait.
Each of those is labelled with where it came from and how sure that is, so
review can go where it's needed.

- `get_recipe_graph` reads a recipe's graph, what the checks find in it, and
  the step timing it comes to.
- `validate_recipe_graph` checks a draft without saving it: every input
  comes from the ingredient list or another action, nothing is made twice,
  there are no loops, every bake has a preheat before it.
- `set_recipe_graph` saves a graph. Saving one as verified marks every label
  reviewed by you.
- `delete_recipe_graph` takes it away.

A graph points at the words each action comes from, so when a step's words
change, the actions from it show as stale. Meal plans don't read step graphs
yet; they still plan from step timing.

#### Drafting them with an assistant

An assistant writes a graph's structure and Sous labels it, using a small
decision model on your own machine (see
[Running Sous](running.md#step-graphs-with-local-models)):

- `list_recipes_without_graph` lists the recipes still to do.
- `draft_recipe_graph` takes the structure: the actions, the words each
  comes from, what joins them. Sous checks it (and sends back anything to
  fix), then labels every action with how sure it is. It's saved as a draft
  for review; `mode: apply` saves it ready for review, and `force` redoes a
  recipe that has one.
- If Sous has a graph model of its own, `upgrade_recipe` does the drafting
  too, and `upgrade_recipes` queues many to run in the background
  (`get_upgrade_job`, `cancel_upgrade_job`).
- `list_review_items` lists what the models weren't sure of, most
  consequential first, each with the words, the value, how sure the model
  was and the other answers it weighed. `resolve_review_items` confirms or
  corrects them, and can accept a recipe's graph as verified.

The `upgrade_step_graphs` prompt drafts the library, or one meal's dishes
(`/mcp__sous__upgrade_step_graphs Thanksgiving` in Claude Code), and reviews
them with you; `review_session` goes through the queue ten at a time.
Every correction is kept, to make the models' answers better over time.

## REST API

The REST API is described at `/v1/openapi.json` on a running Sous, and in
[`spec.md`](../spec.md) §8. Writes name a person the same way as MCP.
