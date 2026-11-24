# Review flows

Flows the web-app-review pipeline walks in a browser. Run Sous on a fresh
`--data` directory (it seeds the twelve starter recipes), on its own, with
the UI built: `cd web && npm run build && cd .. && go run ./cmd/sous --port
8099 --data <tmp>`. Check each at desktop (1280×800) and phone (390×844).

## 1. First run: the starter library

1. Open `/`.
   - The library lists 12 recipes; no "who's cooking" prompt yet.
   - No horizontal scroll; no console errors.
2. Search `historic` (search covers tags).
   - 7 recipes.
3. Search `chocolate`.
   - The flourless chocolate torte is listed.

## 2. Read a recipe

1. Open the Peach Melba recipe from the library.
   - URL is `/r/<id>`; title, cover image, ingredients and steps show.
   - On phones the photo is a small square beside the title, so the facts
     and Begin cooking show in the first screen.
   - The colophon reads "A sample recipe that came with Sous.", never
     `_sous`.
   - The notes hold the original text and the source citation.
   - On phones, past the header, Ingredients and Method are tabs that stick
     under the masthead; switching keeps each tab's place. The masthead
     slides away on scroll down and returns on scroll up.
2. Open the French omelette.
   - No image: the photo slot is left out (no broken image, no tile), so
     the facts and Begin cooking move up, on desktop and phones.

## 3. Who's cooking

1. On a recipe, press the favorite control.
   - Sous asks who's cooking. Enter a new name (e.g. "Ada").
   - The favorite sticks; `/?show=fav` lists that recipe.
2. The person menu in the masthead shows "Ada"; "Sous" is never offered as a
   person to pick.

## 4. Cook mode

1. From the boeuf bourguignon, start cooking (`/r/<id>/cook`).
   - Step 1 shows in large type; ingredients can be ticked.
   - A step with a duration offers a timer; starting it counts down.
2. Leave cook mode.
   - Back on the recipe page.

## 5. Edit a sample recipe

1. On the gazpacho, choose Edit (`/r/<id>/edit`).
   - `_sous` appears nowhere in the editor (it shows no author line).
   - Ingredient lines show the live parse preview.
2. Change the title, then Cancel: "Discard your changes?" appears with
   focus on Keep editing (a double click on Cancel doesn't discard).
   Keep editing, then save with Cmd/Ctrl+S.
   - The recipe page shows the new title; the colophon names the editor
     (Ada) as last changed.

## 6. New recipe

1. Choose New recipe (`/new`), enter a title, one ingredient and one step,
   save.
   - Opens the new recipe page; the library now lists 13.

## 7. Import page

1. Open `/import`.
   - The link field, Fetch button and the "Send recipes from anywhere"
     panel (a desktop-browser bookmark, iOS Shortcut) show.
2. Fetch `http://127.0.0.1/` (a loopback link).
   - A clear refusal message, then the paste box. Nothing is saved.

## 8. Delete and restore

1. In the editor for the Victoria sandwich, delete it.
   - It leaves the library; a toast offers Undo for 15 seconds.
2. Press Undo.
   - It's back, still credited to Sous; focus is on the Library heading.

## 9. Source links

1. In the editor, set a source to `javascript:alert(1)` and save.
   - "source_url must be an http or https link"; nothing is saved.

## 10. Connect an AI assistant

1. From the person menu, choose Connect an AI assistant (`/assistants`).
   - The tab reads "Connect an AI assistant · Sous".
   - The address is this Sous's origin plus `/mcp`; Copy copies it (or, on
     plain http away from this machine, selects it and says to press the
     copy key).
2. Pick Ada under Acts for.
   - The header reads `X-Sous-Person: ada`, and every set-up (Claude Code,
     Claude Desktop, Crush) uses it. The Claude Code command single-quotes
     the address and header.
3. Open the ChatGPT row.
   - It explains ChatGPT can't reach Sous and gives no public-exposure steps.
4. Open the Check it works row.
   - It gives a question to ask (naming Ada) and four symptoms with causes.

## 11. Your kitchen

1. Open Meals in the masthead (`/meals`), then Your kitchen (`/kitchen`).
   - One oven with 2 racks, 4 burners, the day 08:00–22:00. The save button
     reads "Saved" and is disabled until something changes.
2. Set burners to 6, add a serving dish "Big platter", save.
   - "The kitchen is saved." toast; the Meals page's kitchen row reads
     "1 oven, 2 racks, 6 burners, 1 serving dishes".

## 12. Plan a meal

1. On `/meals`, name a meal "Sunday lunch", served next Sunday at 1:00 PM,
   Plan it.
   - Opens `/m/<id>`; the menu is empty with a hint; Cook the meal is
     disabled.
2. Add the boeuf bourguignon and the flourless chocolate torte with the
   Add a dish field (type, arrow down, Enter).
   - The plan lists tasks by day with times; preheats appear before oven
     steps; "Times guessed from the words · Set its timing" shows under
     dishes without timing.
   - At 1280 the lanes chart shows Oven, Stovetop and Hands lanes; Tab
     reaches the chart once and arrow keys move between bars. At 390 the
     chart is replaced by a line per oven.
3. Open Change on the torte, set Served to "1 hr after".
   - The plan moves; a toast says how many steps moved, with Undo.
4. Press `?`.
   - A Keys dialog lists C, D, the lane keys and ?.

## 13. A meal running late, and cooking it

1. Plan a meal served two hours from now with the boeuf bourguignon (it
   needs about three).
   - The header leads with "Dinner now lands … late" in red, Planned for in
     grey; the late block names the dish holding it up and what's in its
     way, with one red option first (Move dinner to …, with its outcome).
     Cook the meal is a default button.
2. Move dinner, then Undo from the toast.
   - The meal moves and comes back; nothing else changes.
3. Cook the meal (`/m/<id>/cook`).
   - The screen stays on; the bar shows the meal and the time.
   - A due preheat is the first ticket, "Turn the oven on to 325°F", with
     It's on. Pressing it moves it to a countdown that says "left".
   - A step's ticket has Start (red) and a quiet Already done. Already
     done ticks it off with an Undo toast; Undo brings it back.
   - No horizontal scroll at 820 and 390.

## 14. Fill in step timing

1. Edit the gazpacho (`/r/<id>/edit`).
   - Below the method: "7 steps have no timing for meal plans. Fill them
     from Sous's reading", and "or have an assistant read them".
2. Fill them, then save.
   - The steps show their timing summaries; the meal plan no longer says
     the gazpacho's times are guessed.
3. Follow "or have an assistant read them".
   - `/assistants#timing` opens with Fill in step timing expanded, showing
     `/mcp__sous__fill_step_timing`.
