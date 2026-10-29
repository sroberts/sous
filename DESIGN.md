---
name: Sous
description: The household's recipe box, printed as a two-ink letterpress menu.
colors:
  paper: "#f4efe6"
  paper-press: "#ebe4d6"
  paper-deep: "#e2d9c8"
  ink: "#2a2420"
  ink-soft: "#5c534c"
  ink-faint: "#8f857c"
  rule: "#d4c9b7"
  brick: "#a83b27"
  brick-press: "#8e2f1e"
  brick-wash: "#f1ddd3"
typography:
  step:
    fontFamily: "Libre Caslon Text, Iowan Old Style, Georgia, serif"
    fontSize: "clamp(1.625rem, 0.9rem + 2.1vw, 2.75rem)"
    fontWeight: 400
    lineHeight: 1.4
  headline:
    fontFamily: "Libre Caslon Text, Iowan Old Style, Georgia, serif"
    fontSize: "clamp(2.125rem, 1.6rem + 1.6vw, 3rem)"
    fontWeight: 400
    lineHeight: 1.08
  title:
    fontFamily: "Libre Caslon Text, Iowan Old Style, Georgia, serif"
    fontSize: "1.875rem"
    fontWeight: 400
    lineHeight: 1.15
  dish:
    fontFamily: "Libre Caslon Text, Iowan Old Style, Georgia, serif"
    fontSize: "1.25rem"
    fontWeight: 400
    lineHeight: 1.3
  body:
    fontFamily: "Libre Caslon Text, Iowan Old Style, Georgia, serif"
    fontSize: "1.1875rem"
    fontWeight: 400
    lineHeight: 1.6
  label:
    fontFamily: "Libre Franklin Variable, Libre Franklin, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 600
    letterSpacing: "0.16em"
  control:
    fontFamily: "Libre Franklin Variable, Libre Franklin, system-ui, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 650
    letterSpacing: "0.1em"
  figure:
    fontFamily: "Libre Franklin Variable, Libre Franklin, system-ui, sans-serif"
    fontSize: "0.9375rem"
    fontWeight: 500
    fontFeature: "\"tnum\", \"lnum\""
rounded:
  none: "0px"
  print: "2px"
spacing:
  "1": "4px"
  "2": "8px"
  "3": "12px"
  "4": "16px"
  "5": "24px"
  "6": "32px"
  "7": "48px"
  "8": "64px"
  gutter: "clamp(16px, 4vw, 56px)"
  masthead: "72px"
components:
  button:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    typography: "{typography.control}"
    rounded: "{rounded.print}"
    padding: "0 16px"
    height: "44px"
  button-hover:
    backgroundColor: "{colors.paper-press}"
  button-active:
    backgroundColor: "{colors.paper-deep}"
  button-primary:
    backgroundColor: "{colors.brick}"
    textColor: "{colors.paper}"
    typography: "{typography.control}"
    rounded: "{rounded.print}"
    padding: "0 16px"
    height: "44px"
  button-primary-hover:
    backgroundColor: "{colors.brick-press}"
  button-quiet:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    typography: "{typography.control}"
    rounded: "{rounded.print}"
    height: "44px"
  button-cook-pager:
    typography: "{typography.control}"
    rounded: "{rounded.print}"
    height: "72px"
  nav-link:
    textColor: "{colors.ink-soft}"
    typography: "{typography.label}"
    padding: "6px 0 4px"
  nav-link-active:
    textColor: "{colors.ink}"
  field-underline:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.none}"
    padding: "8px 0"
  using-ticket:
    textColor: "{colors.brick}"
    rounded: "{rounded.print}"
    padding: "2px 2px 2px 10px"
  tag-ticket:
    textColor: "{colors.ink}"
    rounded: "{rounded.print}"
    padding: "0 0 0 12px"
  timer:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.print}"
    padding: "0 0 0 16px"
  timer-done:
    backgroundColor: "{colors.brick}"
    textColor: "{colors.paper}"
  toast:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
    rounded: "{rounded.none}"
    padding: "12px 12px 12px 24px"
  dialog:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.none}"
    padding: "32px"
    width: "420px"
---

# Design System: Sous

## Overview

**Creative North Star: "The Letterpress Menu"**

Sous is set like a two-colour letterpress restaurant menu printed on cotton paper. The library reads as the house menu: dishes listed in a serif face, dotted leaders running to cook times in a price column, centred spaced-capital course heads, a double rule closing the masthead. Every surface is paper and ink. Structure comes from printers' rules and type, never from boxes floating above the page.

There are two inks and only two. Dark ink carries everything; its lighter values are tints of that same warm ink, never a neutral grey. Brick is the second ink, and it is spent on what is active, selected, actionable, or asking for the cook's action right now. Density follows the two modes of use: the desk screens (library, recipe, editor) are dense and calm at one reading size; cook mode prints one step at a time at menu-card scale for reading across a counter.

Photographs are printed, not displayed: they sit in the paper as plates with an inked edge, their whites taking the paper's colour.

**Key Characteristics:**
- Warm paper ground with one warm ink and its tints; brick as the reserved second ink.
- Libre Caslon Text for titles and all reading text; Libre Franklin in tracked capitals for labels, controls and figures.
- Hairline rules, ink rules, dotted leaders and double rules do all structural work.
- Flat: no shadows, no cards, no rounded tiles; corners are square or barely softened (2px).
- Every quantity and time is set in tabular lining figures.

## Colors

A cream paper, a warm near-black ink with its own tints, and one brick-red second ink.

### Primary
- **Letterpress Brick** (brick): the second ink. Primary buttons ("Begin cooking", "Finish"), the selected dish in the menu and its leader, the active nav underline, the "Using" label and its ingredient tickets, ingredients you have on hand, a full ingredient match, a running timer, the cook-mode step numeral, focus rings, text carets, link hover, and validation or conflict messages that ask the cook to act.
- **Pressed Brick** (brick-press): hover and press on brick fills only.
- **Brick Wash** (brick-wash): brick tinted into paper. Text selection, hover behind a ticket's remove control, the ground of a conflict notice.

### Neutral
- **Cotton Paper** (paper): the page, the masthead, dialogs, menus, and the field ground. Nothing sits on a white.
- **Pressed Paper** (paper-press): ink at about 6% on paper. Hover on buttons and list rows, the focused editor line, the empty-photo plate, loading bars.
- **Deep Paper** (paper-deep): ink at about 11%. The pressed (active) state of a button.
- **Letterpress Ink** (ink): all primary text, button outlines, the masthead's double rule, column-head rules, plate edges, and the toast ground.
- **Soft Ink** (ink-soft): secondary text: sources in italic, figure labels, idle nav, placeholders, struck-through gathered ingredients.
- **Faint Ink** (ink-faint): dotted leaders, link underlines, disabled controls, the plate initial. Never body text.
- **Hairline** (rule): the 1px rules between list rows, the column divider, the "Using" band's top and bottom rules.

### Named Rules
**The Second Ink Rule.** Brick marks only what is active, selected, actionable, or waiting on the cook. If a brick element is neither live nor asking for action, it goes back to ink.

**The Tint Rule.** Every light value is the ink printed thinner on paper. Don't introduce a neutral grey or a cool tone.

## Typography

**Display Font:** Libre Caslon Text (with Iowan Old Style, Georgia, serif)
**Body Font:** Libre Caslon Text (with Iowan Old Style, Georgia, serif)
**Label/Mono Font:** Libre Franklin Variable (with Libre Franklin, system-ui, sans-serif)

**Character:** A bookish Caslon carries every title and every line a cook reads; a sturdy Franklin, tracked into small capitals, does the menu's printed furniture: section heads, controls, figures. Headings stay at regular weight; hierarchy comes from size and italic, not bold. The wordmark is the only bold serif.

### Hierarchy
- **Step** (400, clamp 1.625–2.75rem, 1.4): cook mode's current step, held to 34ch, under a brick Franklin numeral at about twice the step size (700) with a soft "of N".
- **Headline** (400, clamp 2.125–3rem, 1.08): the recipe title and the editor's title field.
- **Title** (400, 1.875rem, 1.15): the preview's recipe name, dialog heads.
- **Dish** (400, 1.25rem, 1.3; 1.125rem under 760px): a dish line in the menu, held to 44ch, with its source in italic beneath at 0.9375rem.
- **Body** (400, 1.1875rem, 1.6): one reading size for all recipe text: description, method (max 65ch), notes. Ingredient lines run slightly smaller (1.0625rem, 1.4).
- **Italic voice**: sources, the library dateline, ingredient section names, notes, hints, empty states. Italic Caslon is the system's quiet secondary register.
- **Label** (Franklin 600, 0.75rem, 0.16em, uppercase): column and section heads, form labels, fact labels (some at 0.6875rem), nav. Course heads in the menu open up to 0.24em and centre.
- **Control** (Franklin 650, 0.8125rem, 0.1em, uppercase): button text.
- **Figure** (Franklin 500, 0.875–1.0625rem, tabular lining): quantities, cook times, yields, timer clocks (650, 1.5rem).

### Named Rules
**The Tabular Rule.** Every quantity, time and count is Franklin in tabular lining figures, so the price column aligns down the menu.

**The One Measure Rule.** Recipe reading text uses one size (1.1875rem / 1.6) and one measure (65ch). Don't add a second reading size for "long" recipes.

## Layout

Gutters scale with the viewport (clamp 16–56px); internal rhythm runs on a 4px-based scale (4, 8, 12, 16, 24, 32, 48, 64). The masthead is a sticky 72px band closed by a double rule; at 760px and below it wraps into wordmark and person, then nav, then a full-width search.

- **Library**: a centred menu column (max 52rem). From 1100px the selected recipe previews in a sticky right column (min 380px, 30vw) behind a hairline divider; below that, selecting a dish opens the recipe. Courses are grouped under centred spaced-capital heads.
- **Recipe**: max 76rem. A 5:7 head (text, then plate); below, a sticky ingredient column (16–22rem) beside the method. Both column heads share one height so their ink rules sit level. Single column under 900px.
- **Cook**: full viewport, no masthead. A bar closed by a double rule, a timer strip, an ingredient column (18–24rem) beside the step stage, and a pager whose Next/Finish is twice the width of Previous. Under 900px the ingredients become a sheet over the stage.
- **Editor**: max 76rem, a sticky toolbar under the masthead, the sheet beside a 20rem side column (which moves above it under 960px).

**The Price Column Rule.** A dotted leader always ends at a figure. A dish with no time and no match count has no leader; never draw a leader into empty space.

## Elevation & Depth

Flat by doctrine. Nothing casts a shadow; the page is one sheet of paper. Separation is printed: hairline rules between rows, an ink rule under column heads, a double rule under the masthead and cook bar, and tinted paper (paper-press) for hover and focus. Overlays (menus, the who's-cooking dialog, the ingredient sheet) are paper with an ink border, never a lift; the dialog adds a double-ruled outline 4px outside its border, and its backdrop dims the page with ink. The toast is a solid ink slug.

### Named Rules
**The Printed Sheet Rule.** No drop shadows. If something must stand apart, rule it or tint the paper under it.

## Shapes

Square, printed forms. Controls, tickets, tags and timers have a barely softened 2px corner, the bite of a letterpress impression; plates, dialogs, menus, notices and the toast are fully square. Outlines are drawn in ink at 1.5px (buttons, tickets, timers, dialog, underlined fields at rest) and hairlines at 1px. Fields are a single underline, never a box. Photographs are rectangles with a 1px inked edge, multiplied into the paper; a recipe without a photo shows its initial in faint italic on pressed paper.

## Components

### Buttons
Printed, square, inked; never floating.
- **Shape:** 2px corner, 1.5px ink outline, 44px minimum height, tracked capital Franklin labels.
- **Default:** transparent on paper, ink text. Hover tints to pressed paper; active to deep paper.
- **Primary:** brick fill, paper text; hover darkens to pressed brick. One primary per view (Begin cooking, Finish, Save).
- **Quiet:** borderless; hover tints to pressed paper.
- **Toggled (aria-pressed):** brick outline and brick text (Favorite, Want to cook, a running timer).
- **Disabled:** hairline outline, faint ink text.
- **Cook scale:** 52px on the recipe's Begin cooking, 60px for step timers, 72px for the cook pager.
- **Focus:** 2px brick outline, 2px offset, everywhere.

### Tickets (chips)
- **Using tickets:** brick 1.5px outline, brick Franklin text, 2px corner, 28px remove target that washes brick on hover.
- **Tags (editor):** ink 1px outline, Franklin text, 30px remove target that turns brick on hover.

### Inputs / Fields
- **Style:** a single 1.5px ink underline (search, title, dialog) or a hairline underline (editor fields); no box, no fill.
- **Focus:** the underline turns brick; the caret is brick.
- **Error:** underline stays brick, with a brick Franklin message below.

### Navigation
Masthead nav in tracked capitals, soft ink at rest, ink on hover; the current section takes ink text over a 2px brick underline. Bold serif "Sous" wordmark at left; search at right with a small ⌘K key hint.

### Menu Line (signature)
The dish name in Caslon, a 2px dotted faint-ink leader, then the time in tabular Franklin right-aligned in the price column; the source in italic beneath. Favorite and want-to-cook marks are small brick line icons after the name. The selected dish turns brick, leader included. Under an active "Using" filter a Franklin "n of m" count joins the column, brick when every ingredient matches; dishes rerank in place rather than disappearing.

### Plate
A photograph printed into the paper: full-bleed in its frame, multiply-blended so its whites take the paper's colour, with a 1px inked edge. Without a photo, the dish's initial in large faint italic on pressed paper.

### Ingredient Line
A two-column row (about 5.5em figure column, then the item) with a hairline beneath. Notes in soft italic. In cook mode the whole line is a 52px tap target that strikes through in soft ink when gathered.

### Cook Step
The step at menu-card scale under a brick numeral; the ingredient-section name sits above in soft italic. Steps enter with a short rise (280ms, ease-out). Finished timers fill brick and ring three times with a slight scale pulse.

### Toast and Dialog
The toast is an ink slug with paper text and a paper-outlined action, rising 12px over 320ms. The who's-cooking dialog is paper inside a 1.5px ink border plus a double-ruled outline, people listed as hairline-ruled serif rows that turn brick on hover.

## Do's and Don'ts

### Do:
- **Do** close every top band (masthead, cook bar) with a 3px double ink rule.
- **Do** run a dotted leader from a dish to its time, and only when there is a figure to reach.
- **Do** spend brick only on active, selected, actionable or attention-needing states, and keep exactly one brick-filled primary per view.
- **Do** set every quantity, time and count in tabular lining Franklin.
- **Do** keep recipe reading text at 1.1875rem / 1.6 with a 65ch measure.
- **Do** separate with rules and paper tints: hairline between rows, ink rule under column heads.
- **Do** print photographs as plates: multiplied into the paper with a 1px inked edge.
- **Do** keep cook-mode targets large (52px lines, 56px icon buttons, 72px pager).

### Don't:
- **Don't** use drop shadows or lift anything off the page.
- **Don't** set recipes in cards or rounded tiles; the library is a typographic list, not a photo grid.
- **Don't** round corners past 2px.
- **Don't** introduce a third ink, a neutral grey, or pure white.
- **Don't** use faint ink for body text.
- **Don't** bold Caslon headings; build hierarchy from size and italic.
- **Don't** box form fields; a field is an underline.

## Known Gaps

State of the build at the finish review, recorded as open targets, not rules:

- Library density falls one short of target at both reference sizes: 11 visible dishes at 1440x900 (target 12) and 7 at 390x844 (target 8).
