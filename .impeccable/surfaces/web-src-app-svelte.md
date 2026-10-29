---
version: 1
slug: "web-src-app-svelte"
primary_target: "web/src/App.svelte"
related_targets: ["web/src"]
---

# Sous web app — surface brief

Scope: the Sous web UI — Library, Recipe page, Cook mode, Recipe editor, plus the who's-cooking dialog and toast. Mode: Operate.
Audience: a household; manage at a Mac desk, cook from an iPad or phone propped on the counter, at arm's length.
Task: find a recipe in seconds among ~700, read it, cook from it, edit it with parsed ingredients shown live.
Constraints: Svelte 5 + Vite SPA served by the Go binary, client of /v1 only; WCAG 2.2 AA; fonts self-hosted.
Redesign (Oct 2026): replaces the letterpress-menu world, which read as too precious and too slow to scan. Features, routes, shortcuts and API unchanged.
Must not feel like: letterpress/cream/serif again, a costume (barcodes, printer chrome, receipt kitsch everywhere), a generic rounded-card SaaS grid, slow to find things.

## Direction contract

THESIS: Sous is the pass in a working kitchen. Recipes are tickets clipped to a steel rail and scanned like an expo reads orders: time first, then what it takes. Refuses both the cookbook page and the rounded photo-card grid.

OWN-WORLD: Thermal-white tickets on a cool grey pass, printed in one black and one red second ink; red marks only what is live, selected or actionable. A steel rail with clips is the one ornament and it always holds something. Archivo carries everything: condensed heavy for times and counts (tabular), regular width for titles and reading text; small tracked caps for ticket headers. Tickets have a torn bottom edge, square corners, hairline dashed perforations between header and body. No shadows beyond the rail's own clip; photos as shot.

STORY: The cook glances at the rail for what they meant to cook, scans the ticket grid by time and ingredient count, narrows by typing or naming ingredients on hand, opens a ticket to read it, and fires it into cook mode where the current step is the ticket on the rail, read across the counter.

FIRST VIEWPORT: Library at 1440: a 56px top bar (Sous wordmark, nav, search with ⌘K, New recipe in red). Left 200px course list with counts. Main: "Want to cook" rail across the top with tickets hanging from clips (big time figure, ingredient count, title); below it the library as a dense ticket grid (photo, title, source, then a perforation and time + ingredient count in the ticket foot). Right pane from 1180px: the selected ticket's preview with Begin cooking in red.

FORM: Ticket Rail (restaurant expo pass); my own pick, position 1 of my ordered list, chosen by the user over the roll's Shelf Edge; seed key c4a5777d.

SIGNATURE: the rail — want-to-cook tickets hang from a steel rail in the library; in cook mode the current step is the ticket fired on the rail with the next steps queued beside it and running timers clipped on as small tickets. Kept raises: photos as shot; one reading size; fixed figure columns; only the live step and timers in red during cooking.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
