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
   - On phones the title, facts and Begin cooking come before the photo.
   - The colophon reads "Added by Sous …", never `_sous`.
   - The notes hold the original text and the source citation.
   - On phones, past the header, Ingredients and Method are tabs that stick
     under the masthead; switching keeps each tab's place. The masthead
     slides away on scroll down and returns on scroll up.
2. Open the French omelette.
   - No image: on desktop the letter tile shows instead of a broken image;
     on phones the photo slot is left out, so Begin cooking moves up.

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
     panel (bookmarklet, iOS Shortcut) show.
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
