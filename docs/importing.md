# Getting recipes in

## From a link

In the web UI, choose **Import** in the masthead (or "Import from a link" on
the new-recipe page), paste a link, and Sous reads the recipe into the editor
for you to check before saving. If a site blocks Sous or has no recipe data,
paste the recipe text instead.

The `/import` page also shows how to set up:

- a **bookmarklet** for desktop browsers;
- an **iOS Shortcut** for the Share Sheet.

Both open `/import?url=…`. Agents use the MCP tool `import_recipe`
([AI agents and the API](agents.md)). Details are in
[`spec.md`](../spec.md) §14.

## Moving from Mela

A one-time, throwaway script loads the Mela export through the REST API. The
app itself carries no Mela code. The export is not committed.

```sh
uv run scripts/mela-import/mela_import.py Recipes.melarecipes \
    --url http://127.0.0.1:8080 --person scott --name Scott
```

`--dry-run` reads and maps the export without sending anything. Re-running
skips what's already there, so an interrupted import can just be run again.

Under Outpost, make a short-lived access token on the Outpost machine and
pass it in `SOUS_TOKEN`. The import is then attributed to the token's owner,
whatever `--person` says. Revoke the token when the import is done.

```sh
outpost token create scott --name mela-import --app sous --expires 1d
SOUS_TOKEN=opat_… uv run scripts/mela-import/mela_import.py Recipes.melarecipes \
    --url https://sous.example.ts.net --person scott
```

Details are in [`spec.md`](../spec.md) §11.
