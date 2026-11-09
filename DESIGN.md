---
name: Sous
description: The household's recipes as tickets on a kitchen pass, clipped to a steel rail and read time first.
colors:
  red: "#bb1c14"
  red-press: "#981610"
  red-wash: "#fbe5e2"
  pass: "#dfe3e6"
  pass-deep: "#d0d6da"
  ticket: "#fafbfc"
  ticket-press: "#eef1f3"
  ink: "#141516"
  ink-2: "#404447"
  ink-3: "#5f6367"
  perf: "#adb4ba"
  field: "#787e84"
  steel-hi: "#d2d6d9"
  steel: "#959ba0"
  steel-lo: "#5d6368"
typography:
  display:
    fontFamily: "Archivo Variable, Archivo, system-ui, sans-serif"
    fontSize: "clamp(2rem, 1.4rem + 1.8vw, 3rem)"
    fontWeight: 800
    lineHeight: 1.04
    letterSpacing: "-0.025em"
  headline:
    fontFamily: "Archivo Variable, Archivo, system-ui, sans-serif"
    fontSize: "1.625rem"
    fontWeight: 800
    lineHeight: 1.1
    letterSpacing: "-0.015em"
  title:
    fontFamily: "Archivo Variable, Archivo, system-ui, sans-serif"
    fontSize: "1.1875rem"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "-0.01em"
  step:
    fontFamily: "Archivo Variable, Archivo, system-ui, sans-serif"
    fontSize: "clamp(1.5rem, 0.95rem + 1.5vw, 2.25rem)"
    fontWeight: 500
    lineHeight: 1.32
    letterSpacing: "-0.01em"
  body:
    fontFamily: "Archivo Variable, Archivo, system-ui, sans-serif"
    fontSize: "1.0625rem"
    fontWeight: 400
    lineHeight: 1.55
    fontFeature: "\"tnum\", \"lnum\""
  ui:
    fontFamily: "Archivo Variable, Archivo, system-ui, sans-serif"
    fontSize: "0.9375rem"
    fontWeight: 600
    lineHeight: 1
  label:
    fontFamily: "Archivo Variable, Archivo, system-ui, sans-serif"
    fontSize: "0.6875rem"
    fontWeight: 700
    lineHeight: 1.3
    letterSpacing: "0.12em"
  figure:
    fontFamily: "Archivo Variable, Archivo, system-ui, sans-serif"
    fontWeight: 800
    lineHeight: 1
    letterSpacing: "-0.01em"
    fontFeature: "\"tnum\", \"lnum\""
    fontVariation: "\"wdth\" 62"
  wordmark:
    fontFamily: "Archivo Variable, Archivo, system-ui, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 850
    lineHeight: 1
    letterSpacing: "-0.03em"
    fontVariation: "\"wdth\" 112"
rounded:
  ticket: "0px"
  badge: "2px"
  control: "3px"
  rail: "5px"
  round: "50%"
spacing:
  space-1: "4px"
  space-2: "8px"
  space-3: "12px"
  space-4: "16px"
  space-5: "24px"
  space-6: "32px"
  space-7: "48px"
  space-8: "64px"
  gutter: "clamp(16px, 2.5vw, 32px)"
  bar: "56px"
  tear: "7px"
components:
  button:
    backgroundColor: "{colors.ticket}"
    textColor: "{colors.ink}"
    typography: "{typography.ui}"
    rounded: "{rounded.control}"
    padding: "0 16px"
    height: "44px"
  button-hover:
    backgroundColor: "{colors.ticket-press}"
  button-active:
    backgroundColor: "{colors.pass-deep}"
  button-primary:
    backgroundColor: "{colors.red}"
    textColor: "{colors.ticket}"
    typography: "{typography.ui}"
    rounded: "{rounded.control}"
    padding: "0 16px"
    height: "44px"
  button-primary-hover:
    backgroundColor: "{colors.red-press}"
  button-quiet:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    rounded: "{rounded.control}"
  button-quiet-hover:
    backgroundColor: "{colors.pass-deep}"
  button-pressed:
    backgroundColor: "{colors.ticket}"
    textColor: "{colors.red}"
  search-field:
    backgroundColor: "{colors.ticket}"
    textColor: "{colors.ink}"
    typography: "{typography.ui}"
    rounded: "{rounded.control}"
    height: "40px"
    padding: "0 12px"
  chip:
    backgroundColor: "{colors.ticket}"
    textColor: "{colors.ink}"
    rounded: "{rounded.control}"
    padding: "0 0 0 8px"
  ticket-card:
    backgroundColor: "{colors.ticket}"
    textColor: "{colors.ink}"
    rounded: "{rounded.ticket}"
  ticket-card-hover:
    backgroundColor: "{colors.ticket-press}"
  rail-ticket:
    backgroundColor: "{colors.ticket}"
    textColor: "{colors.ink}"
    rounded: "{rounded.ticket}"
    padding: "16px 16px 23px"
    width: "15rem"
  panel:
    backgroundColor: "{colors.ticket}"
    textColor: "{colors.ink}"
    rounded: "{rounded.ticket}"
    padding: "16px 24px 32px"
  course-link:
    backgroundColor: "transparent"
    textColor: "{colors.ink-2}"
    typography: "{typography.ui}"
    padding: "6px 16px"
  course-link-active:
    backgroundColor: "{colors.ticket}"
    textColor: "{colors.ink}"
  masthead:
    backgroundColor: "{colors.ticket}"
    textColor: "{colors.ink}"
    height: "{spacing.bar}"
  toast:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.ticket}"
    rounded: "{rounded.control}"
    width: "min(32rem, calc(100vw - 32px))"
---

# Design System: Sous

## Overview

**Creative North Star: "The Ticket Rail"**

Sous is the pass in a working kitchen. Recipes are thermal-white tickets laid on a cool grey counter and clipped to a steel rail, and they are scanned the way an expo reads orders: time first, then what it takes, then the name. The system is printed in two inks, black and one red, and the red is spent only on what is live, selected or actionable, so a glance across any screen finds the next thing to do.

Density is a working density. The library is a tight grid of small tickets with a figure foot; the recipe is one large ticket clipped under a rail with straight-edged panels below it; cook mode fires the current step as a big ticket on the rail with the queue and running timers hanging beside it, legible from across the counter. One family, Archivo, carries everything: condensed and heavy for figures, regular width for titles and reading text, small tracked capitals for ticket headers.

The world refuses the cookbook page and the rounded photo-card grid. There are no drop shadows, no soft radii on tickets, and no filters on photographs.

**Key Characteristics:**
- Two inks on thermal paper: black for everything, red only for live, selected or actionable.
- Thermal-white tickets with a torn bottom edge on a cool grey pass.
- One steel rail with clips as the only ornament, and it always holds a ticket.
- Archivo throughout; times and counts in condensed heavy tabular figures in fixed columns.
- Square ticket corners, dashed perforations between a ticket's head and body, flat surfaces.
- Photos as shot.

## Colors

A cool, near-monochrome kitchen palette (grey counter, white paper, black print) with a single red second ink.

### Primary
- **Expo Red** (`red`): the second ink. Primary actions (Begin cooking, Next step, Save), the selected ticket's top edge and figures, the active course count, the fired step number, running timer clocks, ingredients the cook has on hand, focus rings, caret, field-focus underlines, hover on text links, field errors. Contrast 6.2:1 on ticket, 5.0:1 on pass.
- **Pressed Red** (`red-press`): hover and pressed state of red buttons, and hover inside a done timer.
- **Red Wash** (`red-wash`): text selection highlight only.

### Neutral
- **Pass Grey** (`pass`): the counter. Page background behind every ticket and panel; the active masthead nav item.
- **Deep Pass** (`pass-deep`): hover on the pass, pressed buttons, loading ghosts, the empty photo slot.
- **Thermal White** (`ticket`): ticket and panel paper, the masthead and cook bars, the selected course row.
- **Ticket Press** (`ticket-press`): hover and pressed on a ticket; the faint dashed rules between ingredient and method rows.
- **Print Black** (`ink`): all primary text, button strokes, the heavy 3px rules that head a section or a facts block, the avatar, the toast.
- **Second Grey** (`ink-2`): secondary text and inactive navigation. 7.6:1 on pass.
- **Third Grey** (`ink-3`): tertiary text, labels inside facts and timers, units, counts. 5.9:1 on ticket, 4.7:1 on pass.
- **Perforation** (`perf`): perforations, hairline dividers and quiet strokes. Never text, and never the edge of a field (2.0:1 on ticket).
- **Field Edge** (`field`): the edge of anything you type into (editor and import underlines, the Using box), so a field can be found without guessing. 4.0:1 on ticket, 3.2:1 on pass, above WCAG's 3:1 for controls.
- **Steel** (`steel-hi`, `steel`, `steel-lo`): the rail's three-stop gradient and the clips (`steel` to `steel-lo`). Used nowhere else except the scrollbar thumb.

### Named Rules
**The Second Ink Rule.** Red marks only what is live, selected or actionable. If the cook could not act on it, has not selected it, and it is not running, it prints in black or grey. Status marks (a card's favorite heart and want-to-cook bookmark), field labels and menu icons are grey; New recipe and Import are default buttons, so the one red button on a page is the step forward (Begin cooking, Next step, Save).

**The Thermal Paper Rule.** Every reading surface is `ticket` white on the `pass` grey. There are no tinted cards, gradients or overlays on paper; only the rail and its clips carry a gradient.

## Typography

**Display Font:** Archivo Variable (with Archivo, system-ui)
**Body Font:** Archivo Variable
**Label/Mono Font:** Archivo Variable, as tracked capitals and as condensed figures

**Character:** One grotesque doing every job by width and weight: a ticket printer's heavy condensed numerals beside plain, sturdy reading text.

### Hierarchy
- **Display** (800, clamp(2rem, 1.4rem + 1.8vw, 3rem), 1.04): the recipe title on its ticket and the editor's title field.
- **Headline** (800, 1.625rem, 1.1): the library heading, the preview pane's title, empty and error page headings.
- **Title** (700, 1.1875rem, 1.2): titles on rail tickets, the cook bar's recipe name (800).
- **Step** (500, clamp(1.5rem, 0.95rem + 1.5vw, 2.25rem), 1.32, max 36ch): the fired step in cook mode. Steps over 280 characters drop to clamp(1.25rem, 0.85rem + 1.1vw, 1.75rem) at 44ch.
- **Body** (400, 1.0625rem, 1.55, max 65ch): all recipe reading text: ingredients, method, notes, descriptions.
- **UI** (600 to 700, 0.9375rem): buttons, navigation, course list, search.
- **Label** (700, 0.6875rem, 0.12em, uppercase): ticket headers and field labels: Ingredients, Method, Total, Up next, Step 3 of 5.
- **Figure** (800, width 62%, tabular lining numerals, line-height 1): every time, count, quantity and step number. Sized by context: 1.625rem in a card foot, 2.25rem in facts (3rem large, back to 2.25rem at 1180px and below and 1.875rem on a phone, so the facts stay on one row), 2.75rem on rail tickets, up to clamp(4rem, 2.6rem + 4vw, 6.5rem) for the fired step number. Units follow in small uppercase at 0.4em, regular width.
- **Wordmark** (850, width 112%, 1.5rem): "Sous" in the masthead only.

### Named Rules
**The One Reading Size Rule.** Recipe text reads at one size (1.0625rem / 1.55) everywhere outside cook mode. Hierarchy comes from figures, weight and labels, not from a ladder of body sizes.

**The Printed Figure Rule.** A number the cook scans (minutes, hours, ingredient count, quantity, step, timer) is set as a condensed heavy tabular figure, never in body weight.

## Layout

A sticky 56px masthead over full-bleed grids on the pass. Below 760px it slides up out of view while the page scrolls down and back on any scroll up, at the top of the page, or when focus is in it; `--bar-shown` carries how much of it is showing, for anything sticky beneath. Gutter is clamp(16px, 2.5vw, 32px); spacing follows a 4px base (4, 8, 12, 16, 24, 32, 48, 64).

- **Library:** a 200px course list with figure counts, then the pass. When there are want-to-cook recipes, the rail runs across the top with 15rem tickets hanging from it; below, the library is an auto-fill grid of tickets (min 176px; 16px row and 12px column gaps). From the wide breakpoint a third column, minmax(360px, 26vw), holds the selected ticket's preview as a sticky panel: a click selects a ticket and a second click on the selected one opens it. Arrow keys move through the grid (moving focus when a ticket has it, the selection from the search field), and Enter opens the selected ticket only from the search field; on any other link it follows that link. A "Skip to recipes" link, first in the tab order, jumps past the masthead and course list to the first ticket. The rail is shown above the library heading but follows it in the page, so headings read in order. A recipe's course is its first tag, which the editor says beside the tags. Below 860px the course list becomes a horizontal strip of ticket chips.
- **Recipe:** a rail across the content width with the recipe's header ticket clipped beneath it (7fr text, 5fr photo, the photo no taller than min(22rem, 46vh)). The header reads title, source, facts, then the actions, then the description held to three lines (two from 901 to 1180px) with a More link, so the recipe body starts in the first screen on a laptop or an iPad in landscape. Below, two straight-edged panels: the ingredients panel (17 to 23rem, up to 27rem from 1280px) and the method panel, which opens with the Notes in `ink-2` (make-ahead and timing advice is read before the steps) and closes with Begin cooking and then Nutrition. Method steps are held to 58ch, about 78 characters of Archivo. Nutrition written as labelled values ("**Calories** 1063") is set like the facts, label over figure with a small lowercase unit (g, mg, kcal), rounded as a label would be. The ingredients panel is sticky only while the whole list fits under the masthead; a longer list scrolls with the page rather than inside the panel. Collapses to one column at 900px in the same order, with the photo as a band no taller than 40vh (omitted when the recipe has no photo); the facts drop the ingredient count. On a phone (760px and below) the photo is an 88px square beside the title, the crumb row goes (the masthead leads back to the library) and Edit recipe sits under the title. A recipe whose two panels fit in about a screen and a half stays stacked. A longer one turns Ingredients and Method into two tabs on a `pass` strip that sticks under the masthead (700, `ink-2` with an `ink-3` figure count; the selected tab `ink` with a 3px `ink` underline); each tab keeps its own scroll position, and switching tabs never brings the hidden masthead back. Once the strip sticks, a red Cook button (44px) appears at its end, since Begin cooking has scrolled away, and hides again while the Begin cooking after the last step is in view: one red button at a time. On a phone the Method tab also gets an Ingredients bar fixed to the foot of the screen (ticket white, `perf` hairline above), which opens the ingredient list as a bottom sheet with a 3px `ink` top rule over a dimmed page; Done at its top and Back to the method at its foot close it, as does Escape.
- **Cook:** full-height. A ticket-white bar, then a 18 to 24rem ingredients panel beside the stage. The stage is a sticky rail with the fired step (up to 50rem), then up to two queued steps and a lane of timers, all hanging from it. The second queued ticket drops away at 1439px. At 1180px and below the timers hang from the rail in a strip of their own above the step (scrolling sideways when more than fit), never over its words, and one Up next ticket (two lines) sits under the step; at 900px ingredients become a sheet. On a portrait tablet the step text is set larger (2.125rem, 1.625rem for a long step). A timer is labelled with its step and the step's first words ("Step 2 · Cook the bacon"), so a finished one says what to check; it chimes when it finishes and again every 10 seconds, for three minutes or until it's cleared. A pager of Previous (1fr) and Next step (2fr), 72px tall, closes the screen; on the last step Next step reads Done. Past the last step comes the end card, the last ticket on the rail: a red check that draws itself in as the ticket fires, "That's <recipe>." at 800, the time from the first step to the last as a figure, and a quiet line in `ink-3`; the pager's primary becomes Finish. Swipes and keys stop at the end card, so only Finish ends the cook and forgets its place. Leaving (the X, Esc or Finish) while a timer is still counting asks first in a strip under the bar: "A timer is still running. Leaving stops it." with Stop the timer and leave, and Keep cooking, focused; Esc keeps cooking. Cancelling a running timer offers Undo in a toast, which sits above the pager in cook mode. Each new step is announced to screen readers from one quiet live region.
- **Editor:** a sticky toolbar (back, unsaved state, Cancel, Save) over a main panel and a 20rem side panel, sticky only while all of it fits, so Move to trash at its foot is always in reach. Single column at 960px, the main panel first and the side panel (ending in Move to trash) after it. Below 760px Cancel and Save move to a ticket-white bar fixed along the bottom with a `perf` top hairline. Leaving with unsaved changes asks in place of those actions: "Discard your changes?" with Keep editing, focused, in Save's place and a quiet Discard to its left; Esc keeps editing.

### Named Rules
**The Fixed Column Rule.** Figures sit in fixed columns so they align down a list: 5 to 5.25rem for quantities, 3rem for step numbers in the method, 2.25rem in the editor, 4.5rem minimum for each facts cell, 4.2ch for a timer clock.

## Elevation & Depth

The system is flat. There are no drop shadows anywhere; depth comes from paper on counter (white `ticket` on grey `pass`), from the steel rail and its clips (gradients, not shadows), and from tickets overlapping the rail by 4px so they visibly hang from it. Where `box-shadow` appears in code it is used as a stroke (a 3px inset red underline on the active mobile course chip) or to bleed a hover fill sideways, never to lift a surface.

### Named Rules
**The Flat Pass Rule.** Nothing floats. A surface is either paper on the pass or a ticket on the rail; the rail clip is the only thing with modelled depth.

## Shapes

Tickets are square-cornered thermal paper with a torn bottom edge: a repeating 7px zigzag cut by mask, with matching bottom padding. A dashed 1.5px perforation in `perf` separates a ticket's head from its body. Panels (the recipe's ingredient and method sheets, the preview pane, cook ingredients, editor panels) are the same paper with straight edges. Section heads and facts blocks open on a heavy 3px black rule. A section head inside a panel ("For the sauce") is 800 at reading size; the first one under the panel's own rule draws no second rule.

Controls take a small, almost-square 3px radius: buttons, inputs, chips, menus, the toast. The match badge is 2px. The rail is a 10px bar with a 5px radius; clips are 36 to 44px wide, 9 to 10px tall, rounded only at the bottom (0 0 3px 3px). The avatar is the one circle.

### Named Rules
**The Torn Edge Rule.** The torn edge belongs only to tickets that stand for a recipe, a step or a timer (library cards and their loading ghosts, rail tickets, the recipe header, the fired and queued steps, timers). Panels, bars, dialogs and controls have straight edges.

**The Rail Rule.** The steel rail with clips is the one ornament, and it always holds something. It appears only where tickets hang from it: the library's want-to-cook rail (omitted when empty), above the recipe header, and in cook mode, where the fired step, the queue and the timers all hang from it.

## Components

### Buttons
Ticket-printed and square-shouldered.
- **Shape:** near-square corners (3px), 2px stroke, 44px minimum height (52px on the recipe header, 60px for timer buttons, 72px for the cook pager).
- **Default:** thermal white with a black stroke and black UI text; 18px line icons at stroke 2.
- **Primary:** solid red with a red stroke and white text; one per view, for the action that moves the cook forward.
- **Hover / Active / Focus:** hover fills `ticket-press` (red buttons go to `red-press`), active fills `pass-deep`, 120ms colour transitions. Focus is a 2px red outline offset 2px.
- **Pressed toggles:** `aria-pressed` buttons (Favorite, Want to cook, a running timer's control) turn stroke and text red.
- **Quiet:** no stroke, transparent; `pass-deep` on hover.
- **Disabled:** `perf` stroke, `ink-3` text, transparent.

### Chips
- **Style:** thermal white, 2px black stroke, 3px corners, 0.875rem bold text, a 26 to 30px remove button that turns red on hover. Used for "Using" ingredients and editor tags. The Using box around them has a 2px `field` edge that turns red on focus, and its label is `ink-3`.
- **Match badge:** a 2px-corner outlined figure (matched/asked); red stroke and figure when every asked ingredient is in the recipe.

### Cards / Containers
- **Library ticket:** thermal white, square corners, torn bottom edge, photo inset 8px at 4:3, title and source, a perforation, then the foot: time figure left, ingredient count right. A 3px top edge in paper colour sits ready for selection.
- **Selected ticket:** the top edge turns red and the time figure turns red. Nothing else changes.
- **Panel:** thermal white, straight edges, 16 to 24px padding, opened by a 3px black rule under its label header.
- **Shadow Strategy:** none (see Elevation & Depth).

### Inputs / Fields
- **Search:** 40px, 2px black stroke, 3px corners, white; stroke turns red on focus. Shows a ⌘K hint key on desktop.
- **Editor fields:** borderless with a 1.5px `field` underline that turns red on focus; the title field is a display-size line over a 2px black underline. The caret is red. Ingredient and step lines grow to fit their text rather than cut it off. Placeholders are instructions or start "e.g." in general words, never a plausible value.
- **Error:** red underline and red 0.875rem/600 message beneath.

### Navigation
- **Masthead:** 56px ticket-white bar with a `perf` bottom hairline: wordmark, text nav (600, `ink-2`; hover `ticket-press`; current item on a `pass` fill), search, Import and New recipe as default buttons, and the cook's name with a black initial avatar. At 1080px and below, Import and New recipe fold into one default Add button with a two-item menu (New recipe, Import from a link); on a phone it sits in the top row beside the cook's name. Menus close on Esc or a click elsewhere, and their icons are `ink-2`. From 761 to 1080px the bar stays one 56px row: the ⌘K hint goes, the gaps close up (tighter again below 860px, where search may narrow to 8rem with its hint ending in …), and the connection note keeps only its icon. Below 760px the bar wraps to three rows and hides on scroll down (see Layout).
- **Course list:** UI text with figure counts in `ink-3`; hover on `pass-deep`; the active course sits on thermal white in bold with its count in red.

### Facts
The recipe's figures read across the top of a ticket in fixed cells: label over figure with a small unit, between a 3px black top rule and a dashed perforation below. Wordy values (a yield like "4 to 6 servings") set in body weight.

### The Rail (signature)
A 10px steel bar (`steel-hi` to `steel` to `steel-lo`) with tickets overlapping it by 4px, each held by a centred steel clip. Rail tickets show a big time figure and ingredient count, a perforation, then the title and source; on hover a ticket swings 1deg from its clip (240ms). In cook mode a new step fires onto the rail with a short slide and settle (420ms); queued steps hang beside it with their text in `ink-2`, then `ink-3`, so they recede without dropping below 4.5:1; timers hang in their own lane with a red clock (2.25rem, on a phone too), grey when paused, and turn solid red and swing when done. A timer's Pause and Cancel sit 12px apart, Cancel at the far edge.

### Import page
`/import` is a plain page on paper, not a panel. With no link it holds one field and a primary Fetch button, with the "Send recipes from anywhere" panel below (bookmarklet link, iOS Shortcut steps, and a full-width Siri card that saves without opening Sous). While a link is read it says "Reading <host>…" in a polite live region, and adds a slow-site note after 5 seconds. On success the banner sits above the ordinary editor; on failure it shows a ticket-voiced message for the error code, then the paste box.

### Import banner
A thermal-white strip under a 3px black rule, above the editor. The headline reads "Read from <host>" with an external-link icon; beneath it, one line on how the page was read. Counts (ingredients, steps, sections) sit to the right as label-over-figure cells under 3px black rules, like Facts. Missing items and warnings follow a dashed perforation as icon rows (dashed circle for missing, triangle for warnings), 18px line icons in `ink-2`. A duplicate adds another perforated block: the existing recipe's title, then default buttons (Open it, Update it with this version, Save as new; Restore it if trashed), with chosen options `aria-pressed`. The recipe stays hidden until one is chosen. No colour is added; red stays with the primary action.

### Paste box
A panel (3px black rule under its label header) titled "Paste the recipe", with a Title field and a ten-row text area in the editor-field style (borderless, `perf` underline turning red on focus), an intro line saying why the link failed, and a "Read the recipe" button. It is primary on the error view, where it is the next step, and default on the blank form. Errors are the standard red message beneath the field.

### Offline indicator
A quiet "Offline" (or "Can’t reach Sous") in the masthead before the cook's name: a cloud-off line icon and UI text at 600 in `ink-2`, a polite status region. It never turns red, and stays in the top row on phones. Offline reading and cooking are normal, so nothing else on the page changes.

### Offline notices
Writes that need a connection are disabled, not hidden: Save, Move to trash, photo upload, Favorite, Want to cook, Fetch and Read, Add a person. Beside the main one sits one plain line, "You’re offline. Saving needs a connection.", in `ink-2`. The form stays editable. A recipe that was never saved to the device says so, with a Back to the library button.

### App icon
A chef's toque in solid ink `#141516` on ticket white `#fafbfc`: three overlapping circles for the puff with three pleats cut up into it, a gap, then a plain band. Home-screen icons are full-bleed squares with the toque inside the maskable safe zone; the browser-tab icon fills a rounded tile and drops the pleats. `scripts/icons/icons.py` draws every size from the same shapes. The masthead keeps the "Sous" wordmark; the icon is for the home screen and browser tab.

### Toast
Black slip with white text and a white-stroked button, 3px corners, rising 12px into place over 320ms. "Sous has been updated." with Reload waits until cook mode is closed.

## Do's and Don'ts

### Do:
- **Do** print every scanned number (time, count, quantity, step, timer) as a condensed heavy tabular figure in a fixed-width column.
- **Do** give red to one primary action per view, and otherwise only to live, selected or actionable things: the fired step number, running timers, on-hand ingredients, focus.
- **Do** show selection as a red top edge plus red figures, and nothing more.
- **Do** hang tickets from the rail only when they stand for a recipe, a step or a timer, and only when there is at least one to hang.
- **Do** keep recipe reading text at 1.0625rem / 1.55 within 65ch.
- **Do** separate a ticket's head from its body with the 1.5px dashed perforation.
- **Do** show photos as shot: cropped to the slot with object-fit, never filtered or tinted. A missing photo prints the dish's initial as a figure on `pass-deep`, except in the recipe header's one-column layout, where the slot is left out.

### Don't:
- **Don't** add drop shadows, glows or lifted cards; the rail clip is the only modelled depth.
- **Don't** put a torn edge on panels, bars, dialogs or controls.
- **Don't** add a second ornament or an empty rail.
- **Don't** round ticket corners or use pill shapes; controls stop at 3px.
- **Don't** use red for decoration, static headings or inert labels.
- **Don't** introduce a second typeface; Archivo's width and weight axes carry every role.
- **Don't** place kickers or eyebrow labels above titles; tracked capitals are for ticket headers and field labels only.
