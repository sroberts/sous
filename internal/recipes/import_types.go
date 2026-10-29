package recipes

// Wire types for importing a recipe from a link (issue #7). A preview is a
// draft for a person to review; saving it goes through POST /v1/recipes.

// ImportRequest is the body of POST /v1/recipes/import/preview. URL alone
// reads that page. Text is split without fetching anything; a URL sent with
// it is only the link the recipe is credited to, and is never fetched.
type ImportRequest struct {
	URL   string `json:"url,omitempty" jsonschema:"the recipe page to read; with text, only the link the recipe is credited to, never fetched"`
	Text  string `json:"text,omitempty" jsonschema:"recipe text pasted by a person; split into ingredients and steps without fetching anything"`
	Title string `json:"title,omitempty" jsonschema:"with text: a title carried over from a page that had no recipe data"`
}

// ImportPreview is a draft recipe read from a page, not yet saved.
type ImportPreview struct {
	Draft       RecipeInput      `json:"draft" jsonschema:"the recipe as read; POST it to /v1/recipes to save"`
	Ingredients []ParsedLine     `json:"ingredients" jsonschema:"draft.ingredients as the server would store them"`
	Images      []ImageCandidate `json:"images" jsonschema:"photos found on the page, best first"`
	Report      ImportReport     `json:"report"`
	Duplicates  []DuplicateRef   `json:"duplicates" jsonschema:"recipes already in Sous from the same link"`
}

// ImageCandidate is a photo found on the page. Hash is set once it has been
// downloaded and stored; Problem says why it wasn't.
type ImageCandidate struct {
	URL     string  `json:"url"`
	Width   *int    `json:"width"`
	Height  *int    `json:"height"`
	Hash    *string `json:"hash" jsonschema:"set when the photo was stored in Sous"`
	Problem *string `json:"problem" jsonschema:"why the photo wasn't stored"`
}

// ImportReport says how the page was read and what is missing.
type ImportReport struct {
	Method     string   `json:"method" jsonschema:"json-ld, microdata, h-recipe or text"`
	SourceURL  string   `json:"source_url" jsonschema:"the link the recipe is credited to"`
	FetchedURL *string  `json:"fetched_url" jsonschema:"the URL actually fetched, when it differs (an archive copy, a redirect)"`
	Archived   bool     `json:"archived" jsonschema:"read from a web.archive.org copy"`
	Found      []string `json:"found"`
	Missing    []string `json:"missing"`
	Warnings   []string `json:"warnings"`
}

// DuplicateRef is an existing recipe with the same source link.
type DuplicateRef struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Version int    `json:"version"`
	Deleted bool   `json:"deleted" jsonschema:"the existing copy is in the trash"`
}

// ImportPage is what could still be read from a page with no usable recipe:
// enough to start a pasted one.
type ImportPage struct {
	SourceURL string          `json:"source_url"`
	Title     *string         `json:"title"`
	Image     *ImageCandidate `json:"image"`
}

// ImportResult is the MCP import_recipe output: the preview, and the saved
// recipe when save was requested.
type ImportResult struct {
	ImportPreview
	Saved *Recipe `json:"saved" jsonschema:"the created recipe, when save was true"`
}

// Import error codes. They travel as 422 responses, separate from
// recipes.Error, because not_found and too_large already mean other
// statuses there.
const (
	CodeBlocked        = "blocked"
	CodePaywalled      = "paywalled"
	CodePageNotFound   = "not_found"
	CodeNoRecipe       = "no_recipe_found"
	CodeTimeout        = "timeout"
	CodePageTooLarge   = "too_large"
	CodeUnsupportedURL = "unsupported_url"
	CodeSiteError      = "site_error"
)

// ImportError is a failed import, with whatever the page still offered.
type ImportError struct {
	Code    string
	Message string
	Page    *ImportPage
}

func (e *ImportError) Error() string { return e.Code + ": " + e.Message }
