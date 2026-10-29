---
version: 1
slug: "web-src-app-svelte"
primary_target: "web/src/App.svelte"
related_targets: ["web/src"]
---

# Sous web app — surface brief

Scope: the Sous web UI — Library, Recipe page, Cook mode, Recipe editor. Mode: Operate.
Audience: a household; manage at a Mac desk, cook from an iPad or phone propped on the counter.
Task: find a recipe in seconds among ~700, read it, cook from it at arm's length, edit it with parsed ingredients shown live.
Constraints: Svelte 5 + Vite SPA, client of /v1 only (mock until the Go service exists); installable PWA later; WCAG 2.2 AA.
Must not feel like: a food blog, a generic SaaS app, slow photo-grid browsing, or fussy controls in the kitchen.

## Direction contract

THESIS: The library is the house menu. Recipes are set as a typographic list in two inks and scanned like a restaurant menu, not browsed as a photo feed. Refuses the category's photo-card grid.

OWN-WORLD: Cotton-paper ground, letterpress ink and one brick-red second ink; every lighter value is a tint of the ink, never a neutral grey. Libre Caslon Text carries titles and reading text; Libre Franklin in small caps carries section heads, controls and figures, always tabular. Structure is made of menu printing's own rules: double rules under the masthead, dotted leaders, centred section heads. No cards, no drop shadows, no rounded tiles; photographs appear as printed plates on the recipe page only.

STORY: The cook sees the whole menu at once, narrows it by typing or by naming ingredients on hand, opens a recipe to read, and begins cooking into a course-card view that prints one step at a time large enough to read from across the counter. At the desk, the editor shows how each ingredient line was understood as it is typed.

FIRST VIEWPORT: Library at 1440: a 72px masthead (Sous wordmark left, nav, search field right) closed by a double rule. Left two thirds: a centred dateline, the "Using" ingredient filter in brick, then the menu: tag section heads centred in spaced caps, dish lines with dotted leaders running to the cook time in the price column and the source in italic beneath. Right third: the selected recipe as a preview with a photo plate, yield and times, and the primary "Begin cooking" action in brick.

FORM: Letterpress Menu (two-colour letterpress restaurant menus); my own pick, position 1 of my ordered list; seed key 2d86a0ba.

SIGNATURE: dotted leaders run from every dish to its time, the menu's price column; adding an ingredient to "Using" reranks dishes in place (rows move, never vanish); cook mode prints the current step at menu-card scale under a brick step numeral.

RAISES: one reserved second ink, brick, marks only what is active or actionable (from the orienteering map); rerank in place preserving row identity (from the gate board); one reading size and measure for all recipe text (from the acetate manual); tabular figures for every quantity and time (from Datamatics).

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
