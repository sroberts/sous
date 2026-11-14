# Changelog

What changed for people running Sous. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Sous uses
[semantic versioning](https://semver.org/): before 1.0, a minor release may
include breaking changes, and they're called out here.

## [Unreleased]

### Added
- A Connect an AI assistant page, in the person menu, shows how to connect
  Claude Code, Claude Desktop or Crush to the library over MCP, with the
  address and your name filled in and each set-up ready to copy. It says why
  ChatGPT can't connect yet.

### Changed
- The recipe page starts the recipe sooner: on a laptop or an iPad in
  landscape the ingredients and method now show in the first screen. The
  description sits under Begin cooking, held to three lines with Show all at
  the end of the last (which fades out rather than ending in "…"), and the
  photo is shorter.
- A recipe's notes come before its method, so make-ahead and timing advice is
  read before you start. The first lines of advice show, with Show all for the
  rest, so the steps aren't pushed down; a credit line ("Inspired by…") waits
  behind Show all rather than taking that place, and comes last when the
  notes are open.
- On a laptop or an iPad in landscape, the ingredients stay beside the method
  all the way down, scrolling on their own when the list is long. A long
  list says how many more are below it, and it follows the method: reading
  into "The garnish" brings "For the garnish" into view. On a phone the
  ingredient sheet opens at the part of the method you're reading.
- A recipe without a photo gives its header the full width instead of a grey
  placeholder.
- Section headings ("For the sauce") are set at reading size on a heavy rule.
- "Show as written" says "Show tidied" while the original lines are showing.
- Links in recipe text are easier to see, carry an external-link mark, and
  tell screen readers they open a new tab. Screen readers now hear each step's
  number ("Step 7").
- On a touch screen (a phone, or an iPad either way up), method steps and
  ingredients are set larger for reading from the counter; on an iPad held upright they're
  also held to a readable line. A laptop keeps the usual size.
- Ingredient notes keep asides where they belong: "cut into 2-inch (5 cm)
  chunks" now reads as written, instead of the "(5 cm)" being moved out
  ahead of it. This applies to recipes added or edited from now on.
- On a phone or an iPad held upright the recipe comes up sooner, with the
  first ingredients in the first screen: the photo is a thumbnail beside the
  title. On a phone, Edit sits at the end of the source line (or the tags'
  line, or the title's), Favorite and Want to cook are icon buttons beside
  Begin cooking, and the times fit on one row. A short
  recipe shows its ingredients and method in one scroll; a long one splits
  them into Ingredients and Method tabs. Switching tabs no longer hides the
  first step under the bar at the top, and once Begin cooking scrolls away a
  Cook button rides along with the tabs.
- In the Method tab on a phone or tablet, an Ingredients bar at the foot of
  the screen pulls the ingredient list up over the step you're reading. Pull
  the sheet down by its handle, or tap Back to the method, to close it. The
  sheet opens at about half the screen so the step stays in view above it;
  tap or pull the handle up for more. Group headings stay at the top of the
  sheet and of the ingredient column as their rows scroll, and the sheet
  offers the group above ("For the stew ↑"). The bar stays through the last steps, stepping
  aside only when the Begin cooking after the last step reaches it, and on an
  iPad held upright it's a button at the corner that stays put.
- One red button at a time: the Begin cooking after the last step turns red
  only once the one at the top has scrolled away, and on a phone the Cook
  button with the tabs hides while it's in view. Tap
  targets on the recipe page are at least 44px.
- Begin cooking also closes the method, so you can start from where you read.
- Partway down the method, the recipe offers "Cook from step 7": cook mode
  opens at the step you were reading. On a phone or tablet it's the Cook
  button with the tabs and the ingredient sheet; on a laptop or an iPad in
  landscape, the foot of the ingredients column. It's called "Cook from step
  7" wherever it's offered. Opening the ingredient sheet lifts the step
  you're reading into view above it, so the number doesn't change, and
  closing it puts the page back where it was. The sheet names it by its first words ("Step 7 · While the beef braises…"). C
  cooks from that step too. Leaving cook mode brings you back to the method
  at the step you'd got to, with the ingredients column at its part of the
  list.
- Side by side, the ingredients column's head stays at its top as the list
  scrolls, so Show as written is always in reach, down to the end of the
  page.
- Reading the method on a phone, a small scroll back up to re-read a line
  no longer brings the masthead down over it.
- A short recipe shows one Begin cooking, not a second one just below it,
  and without a photo its header is as wide as what's on it. The ingredient
  count sits with the Ingredients heading instead of in the times.
- Keys on the recipe page: C begins cooking, E edits, F and W set Favorite
  and Want to cook (with Undo), A shows the ingredients as written, I and M
  switch tabs, and ? lists them.
- A recipe in the trash says so at the top of its page, with Restore. A link
  to a recipe that doesn't exist says the link may be wrong.
- Nutrition written as "**Calories** 1063" lines shows as figures, like the
  times, rounded the way a label is (1064 kcal, 55.3 g) with short units. A
  sample recipe says it came with Sous instead of "Added by Sous". The ingredient column is wider on large screens.
- The source link tells screen readers it opens a new tab, and a recipe that
  fails to load says so plainly instead of showing a raw error.
- On a phone or an iPad in portrait, an Add button in the masthead opens New
  recipe and Import from a link, which were hidden below desktop width.
- Leaving Cook mode while a timer is running asks first, and a cancelled
  timer can be brought back with Undo. Timer clocks are larger on phones,
  with more room between Pause and Cancel.
- Red is kept for the next step forward: New recipe, the favorite and
  want-to-cook marks, and the Using label are now black or grey.
- In the library on a wide screen, a click selects a recipe for the preview
  and a second click opens it.
- The editor says that a recipe's first tag is its course in the library.
- Saving the library to a phone for offline use takes a few requests
  instead of one per recipe: a library of 1,500 recipes is stored in about
  16, so the first visit, and the first after each Sous update, finishes in
  seconds and uses less data. `POST /v1/recipes/batch` returns up to 100
  full recipes at once.

- Cook mode ends on a last ticket, "That's Boeuf Bourguignon.", with how long
  the cook took. Only Finish leaves it, so a stray swipe or key on the last
  step no longer ends the cook and forgets your place.
- Timers say which step they're for in its own words ("Step 2 · Cook the
  bacon"), and a finished timer chimes again every 10 seconds for three
  minutes until you clear it.
- On an iPad or a phone, timers hang in a strip of their own above the step,
  and the next step shows under the current one. The step is larger on an
  iPad in portrait.

- In cook mode, timers stay in view under the rail while the step scrolls,
  and on an iPad in landscape the step is set a little smaller so its timer
  buttons sit above Previous and Next.
- A search result that matched on an ingredient or a tag says so ("Uses
  chicken stock"). Courses that differ only in a plural or capital letters,
  like Side and Sides, are listed once.
- The Import page's set-up guides (bookmarklet, iPhone and iPad Shortcut,
  Siri) are folded away until you open one.
- Cook mode reads better from the counter: a timer says which step it's
  for in larger, darker type, its Pause and Cancel buttons are bigger, Up
  next is larger, and the step grows to fill a tall laptop or iPad screen.
- The Ingredients button in cook mode says what's left ("17 left"), and
  shows a check once everything is gathered.
- The editor takes times as minutes (90) or hours and minutes (1:30), and
  shows them back as 1:30, the way the recipe does.
- Favorites and Want to cook, with no one picked, say they're kept for each
  cook and offer Who's cooking?. The course list counts the view you're on.
- On a phone, the library's Using field folds into a button until you need
  it, so more recipes show in the first screen.
- The end card counts the steps ("11 steps") instead of "11 of 11", and
  the leave prompt there says Stay.

### Fixed
- After the recipe page's list of keys (?) or the ingredient sheet was
  closed, the page's keys stopped working until you clicked the page.
- A cook mode timer for "2 1/2 to 3 hours" ran for 2 hours; written
  fractions (1/2, 2 1/2, 1¼) now count.
- Group headings that stay at the top of an ingredient list no longer show
  the tops of the row passing under them.
- Pulling the ingredient sheet's head with a mouse no longer selects text or
  closes the sheet when you meant to make it taller, and a quick flick of the
  head up or down is no longer missed.
- On the narrowest phones (320px), a recipe page no longer scrolls sideways,
  and a long title isn't squeezed beside the photo.
- On a phone, a short recipe no longer shows its two Begin cooking buttons in
  red at once while the masthead is out of the way.
- On a wide screen, there's no longer a stretch just under the header with
  no Begin cooking or Cook button in view.
- Screen readers hear the description's and the notes' Show all apart, and
  the tabs' names without a stray pause.
- In cook mode, a new step opens at its top; it could open scrolled down
  from the last step, hiding its number.
- A course in the library lists the recipes its count says it has: those
  whose first tag is that course. "Vegetarian 1" could list 4 recipes that
  only had Vegetarian as a later tag.
- On a phone, long timer button labels in cook mode ran off the side of
  the step; they wrap now. A timer that finishes out of sight in the strip
  of timers moves to the front where you can see it.
- A search that finds nothing in a course says how many recipes match in
  the rest of the library, with a button to search them all.
- In cook mode, a step that says the same time twice ("1 minute… then 1
  minute more") gets two timers, each named for its own part of the step;
  starting the second no longer paused the first. Timers are named for what
  they time ("Brown the beef"), not just the step's first words.
- On a laptop, running timers stay in view while a long step scrolls.
- Opening the ingredient list on a phone moves focus into it, and closing
  it puts focus back on the Ingredients button.
- On touch screens a tapped button no longer stays in its hover colour.
- On a phone, a search with few or no results no longer pushes the heading
  down the screen.
- In cook mode on a phone or an iPad in portrait, the ingredient list
  couldn't be scrolled, hiding everything past the first ten or so items. It
  scrolls now, with a Done button at the top.
- On an iPad in portrait the masthead ran off the side of the screen,
  cutting off "Who's cooking?".
- Starting a timer in cook mode keeps keyboard focus on its Pause button.
- Searching the library no longer freezes on a phone. The library draws its
  tickets a page at a time as you scroll, rather than every recipe at once,
  and search waits for a pause in typing instead of re-sorting the whole
  library on every keystroke.
- Back from a recipe, or between library views, returns to the same place
  in the library, with the ticket you left on still selected and focused.
- Pressing Enter on a link in the library (the menu, a course, another
  recipe) followed that link instead of opening the selected recipe.
- On a wide screen, clicking a recipe in the library opened it instead of
  showing it in the preview.
- Keyboard and screen-reader access: a "Skip to recipes" link, arrow keys
  that move focus through the library, headings in order, each Cook mode step
  announced as it changes, and form fields with edges that meet WCAG contrast.
  The library grid is one stop in the tab order, so Tab reaches the preview
  pane; Home and End jump through the grid; and a ticket's focus ring is no
  longer hidden by its torn edge.
- Cook mode's error page has a heading and a way back to the library.
- A recipe's description, steps, notes and nutrition render their Markdown
  (bold, italics and links), as Mela writes them, instead of showing the raw
  asterisks. Steps render it in Cook mode too.

### Removed
- The footer with the version and GitHub links.

## [0.3.0] - 2026-11-02

### Added
- A footer shows the running version of Sous, linked to its release notes,
  and a link to the source on GitHub.
- Sous installs as an app and works offline. The library, recipes and Cook
  mode keep working without a connection, with the library saved to the device
  and photos saved as you view them. Saving changes needs a connection. Under
  the Outpost gateway, a lapsed sign-in still opens the saved library
  read-only. See "On your phone or tablet" in `docs/running.md`.
- `POST /v1/recipes/import` reads a recipe link and saves it in one call,
  returning a short `summary` to speak. A link already in Sous returns the
  existing recipe, restoring it from the trash if needed.
- A "Save to Sous" Siri Shortcut, with setup steps on the `/import` page, to
  save a recipe from your phone without opening Sous.
- A new Sous comes with twelve recipes, credited to "Sous": seven historic
  ones (Glasse 1751 to Escoffier 1907), each with a modern adaptation, the
  original text and a period illustration, and five classics written for
  Sous, four inspired by Julia Child, Paul Bocuse, Anthony Bourdain and
  Jacques Pépin, plus a flourless chocolate torte. They're added only to a library that has
  never been used, and deleting them is permanent. Set `SOUS_SAMPLES=false`
  to skip them.
- `SECURITY.md`, `SUPPORT.md`, and issue forms for feedback, import
  failures, bugs and feature requests.

### Changed
- The database upgrades to schema version 2 on start, so Sous can be named as
  the author of its own changes. Migrations now run with foreign keys checked
  before they commit. The username `_sous` is reserved: it isn't listed, can't
  be chosen, and a gateway sign-in with it is refused.
- A recipe's source must be an http or https link when it's added or
  changed, over the REST API and MCP alike. A source saved earlier that isn't
  one (a book title from Mela, say) can still be saved back unchanged, and
  shows as plain text. The Mela import script now puts such a source at the
  end of the notes.
- The editor asks before you leave it with unsaved changes ("Discard your
  changes?"), and Cmd/Ctrl+S saves.
- On a phone, the masthead slides away while you scroll down and comes back
  when you scroll up, and a recipe's ingredients and method are two tabs, each
  keeping its place.
- Undo after moving a recipe to the trash stays on screen for 15 seconds.

### Fixed
- A long ingredient list on a recipe no longer runs past the bottom of the
  screen, and Move to trash in the editor can always be reached.
- A pre-existing person named `_sous` could still make changes credited to
  "Sous".
- The Mela import script won't send your token on to another site if the
  server redirects there.

## [0.2.0] - 2026-10-30

### Added
- Under Outpost's login gateway, Sous uses your Outpost sign-in: no
  "who's cooking?" prompt, and your name links back to Outpost for your
  account, signing out and other apps.
- A notice when your Outpost sign-in ends while a page is open, with a way
  to sign in again.
- `--data DIR` to run Sous on its own without Outpost's environment
  variables, plus `--help` and `--version`.
- The Outpost portal lists Sous as "Sous · Household recipes".

### Fixed
- A long website name on a recipe card made the library scroll sideways on
  phones.

## [0.1.0] - 2026-10-29

First release.

### Added
- The recipe library, recipe pages, cook mode with step timers, and the
  editor, in the Ticket Rail design.
- Import a recipe from a link, with a bookmarklet and an iOS Shortcut, and a
  paste fallback for sites that block it.
- REST API with an OpenAPI description, and MCP tools for AI agents.
- Running under Outpost, with its login gateway.
- A script to import a Mela export.

[Unreleased]: https://github.com/sroberts/sous/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/sroberts/sous/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/sroberts/sous/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/sroberts/sous/releases/tag/v0.1.0
