package recipes

import (
	"encoding/json"

	"github.com/sroberts/sous/internal/identity"
)

// These are the wire types: the single source for REST JSON, the OpenAPI
// document and the MCP tool schemas. Field names and nullability match
// web/mock/server.js, which the UI was built against. Times are UTC
// RFC 3339 strings with millisecond precision ("2006-01-02T15:04:05.000Z").
// Slices in responses are never nil: an empty list is [], not null.

// Recipe is the full recipe, as GET /v1/recipes/{id} returns it.
type Recipe struct {
	ID            string       `json:"id" jsonschema:"recipe ID, a UUIDv7 string"`
	Title         string       `json:"title"`
	SourceURL     *string      `json:"source_url" jsonschema:"where the recipe came from, if anywhere"`
	Description   *string      `json:"description"`
	Notes         *string      `json:"notes"`
	YieldText     *string      `json:"yield_text" jsonschema:"the yield as written, e.g. 'Serves 4 to 6'"`
	Servings      *int         `json:"servings" jsonschema:"the first number in yield_text; derived, read-only"`
	PrepMinutes   *int         `json:"prep_minutes"`
	CookMinutes   *int         `json:"cook_minutes"`
	TotalMinutes  *int         `json:"total_minutes"`
	TimeText      *string      `json:"time_text" jsonschema:"original time wording when it doesn't parse into minutes"`
	NutritionText *string      `json:"nutrition_text"`
	Tags          []string     `json:"tags" jsonschema:"tag names in order; the first is shown as the course"`
	Images        []string     `json:"images" jsonschema:"image hashes; the first is the cover"`
	Ingredients   []Ingredient `json:"ingredients"`
	Steps         []Step       `json:"steps"`
	Version       int          `json:"version" jsonschema:"incremented on every change; send it back as If-Match or version"`
	DeletedAt     *string      `json:"deleted_at" jsonschema:"set when the recipe is in the trash"`
	CreatedAt     string       `json:"created_at"`
	CreatedBy     string       `json:"created_by" jsonschema:"username"`
	UpdatedAt     string       `json:"updated_at"`
	UpdatedBy     string       `json:"updated_by" jsonschema:"username"`
	SourceHost    *string      `json:"source_host" jsonschema:"hostname of source_url without www.; derived, read-only"`
	Mine          Mine         `json:"mine"`
}

// Ingredient is one parsed ingredient line of a stored recipe.
type Ingredient struct {
	Section       *string  `json:"section" jsonschema:"the heading this line sits under, or null"`
	Raw           string   `json:"raw" jsonschema:"the line exactly as written"`
	Quantity      *float64 `json:"quantity"`
	QuantityMax   *float64 `json:"quantity_max" jsonschema:"upper bound of a range such as '2 to 4'"`
	Unit          *string  `json:"unit" jsonschema:"normalised unit such as tbsp, g or cup"`
	Item          string   `json:"item" jsonschema:"the ingredient itself; the whole line when it can't be parsed"`
	Note          *string  `json:"note" jsonschema:"preparation notes such as 'finely diced'"`
	ParserVersion int      `json:"parser_version"`
}

// Step is one method step of a stored recipe.
type Step struct {
	Section *string `json:"section" jsonschema:"the heading this step sits under, or null"`
	Text    string  `json:"text"`
}

// Mine is the caller's own flags on a recipe. For nobody both are false.
type Mine struct {
	Favorite   bool `json:"favorite"`
	WantToCook bool `json:"want_to_cook"`
}

// MinePatch is the body of PUT /v1/recipes/{id}/mine: a partial merge where
// an absent (or null) field keeps its current value.
type MinePatch struct {
	Favorite   *bool `json:"favorite,omitempty"`
	WantToCook *bool `json:"want_to_cook,omitempty"`
}

// Summary is a recipe as it appears in lists.
type Summary struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	SourceHost      *string  `json:"source_host"`
	Tags            []string `json:"tags"`
	Cover           *string  `json:"cover" jsonschema:"hash of the first image, or null"`
	TotalMinutes    *int     `json:"total_minutes" jsonschema:"total, else prep plus cook when either is set, else null"`
	TimeText        *string  `json:"time_text"`
	IngredientItems []string `json:"ingredient_items" jsonschema:"parsed ingredient items, lower-cased"`
	Mine            Mine     `json:"mine"`
	Version         int      `json:"version"`
	UpdatedAt       string   `json:"updated_at"`
	// DeletedAt is set only on trash listings.
	DeletedAt *string `json:"deleted_at,omitempty"`
}

// Page is one page of a cursor-paged list.
type Page[T any] struct {
	Items      []T     `json:"items"`
	Total      int     `json:"total" jsonschema:"number of matching items across all pages"`
	NextCursor *string `json:"next_cursor" jsonschema:"pass back as cursor for the next page; null on the last page"`
}

// ChangePage is one page of the change log, newest first.
type ChangePage struct {
	Items      []Change `json:"items"`
	NextCursor *string  `json:"next_cursor"`
}

// Items wraps an unpaged list: people, tags, parsed lines.
type Items[T any] struct {
	Items []T `json:"items"`
}

// TagCount is a tag with the number of live recipes carrying it.
type TagCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Change is one change log entry.
type Change struct {
	ID       string  `json:"id"`
	At       string  `json:"at"`
	PersonID *string `json:"person_id" jsonschema:"username of who made the change"`
	Via      string  `json:"via" jsonschema:"ui, api or mcp"`
	Entity   string  `json:"entity" jsonschema:"recipe or person"`
	EntityID string  `json:"entity_id"`
	Action   string  `json:"action" jsonschema:"create, update, delete or restore"`
	Summary  string  `json:"summary"`
}

// Person is a household member.
type Person struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

// NewPerson is the body of POST /v1/people.
type NewPerson struct {
	Name string `json:"name" jsonschema:"display name; the username is derived from it"`
}

// Status is GET /v1/status.
type Status struct {
	Version       string               `json:"version"`
	SchemaVersion int                  `json:"schema_version"`
	Auth          string               `json:"auth" jsonschema:"none or gateway"`
	Recipes       int                  `json:"recipes" jsonschema:"live (not deleted) recipe count"`
	StorageBytes  int64                `json:"storage_bytes"`
	JWKS          *identity.JWKSStatus `json:"jwks" jsonschema:"gateway key cache health; null when not in gateway mode"`
}

// ImageRef is the response to an image upload.
type ImageRef struct {
	Hash string `json:"hash" jsonschema:"lower-case hex SHA-256 of the image bytes"`
}

// RecipeInput is the body of POST /v1/recipes. Ingredients and steps are
// raw lines; a line starting with # is a section heading.
type RecipeInput struct {
	Title         string   `json:"title"`
	SourceURL     *string  `json:"source_url,omitempty"`
	Description   *string  `json:"description,omitempty"`
	Notes         *string  `json:"notes,omitempty"`
	YieldText     *string  `json:"yield_text,omitempty" jsonschema:"the yield as written; servings are derived from it"`
	PrepMinutes   *int     `json:"prep_minutes,omitempty"`
	CookMinutes   *int     `json:"cook_minutes,omitempty"`
	TotalMinutes  *int     `json:"total_minutes,omitempty"`
	TimeText      *string  `json:"time_text,omitempty"`
	NutritionText *string  `json:"nutrition_text,omitempty"`
	Tags          []string `json:"tags,omitempty" jsonschema:"tag names in order; the first is shown as the course"`
	Images        []string `json:"images,omitempty" jsonschema:"hashes from image uploads; the first is the cover"`
	Ingredients   []string `json:"ingredients,omitempty" jsonschema:"raw ingredient lines; '# Heading' starts a section"`
	Steps         []string `json:"steps,omitempty" jsonschema:"raw step lines; '# Heading' starts a section"`
}

// RecipePatch is the body of PATCH /v1/recipes/{id}. Absent fields are left
// alone; null clears a nullable field. Sending ingredients, steps, tags or
// images replaces that whole list.
type RecipePatch struct {
	Title         Optional[string]   `json:"title,omitzero"`
	SourceURL     Optional[string]   `json:"source_url,omitzero"`
	Description   Optional[string]   `json:"description,omitzero"`
	Notes         Optional[string]   `json:"notes,omitzero"`
	YieldText     Optional[string]   `json:"yield_text,omitzero"`
	PrepMinutes   Optional[int]      `json:"prep_minutes,omitzero"`
	CookMinutes   Optional[int]      `json:"cook_minutes,omitzero"`
	TotalMinutes  Optional[int]      `json:"total_minutes,omitzero"`
	TimeText      Optional[string]   `json:"time_text,omitzero"`
	NutritionText Optional[string]   `json:"nutrition_text,omitzero"`
	Tags          Optional[[]string] `json:"tags,omitzero" jsonschema:"replaces all tags"`
	Images        Optional[[]string] `json:"images,omitzero" jsonschema:"replaces all images"`
	Ingredients   Optional[[]string] `json:"ingredients,omitzero" jsonschema:"replaces all ingredients; raw lines, '# Heading' starts a section"`
	Steps         Optional[[]string] `json:"steps,omitzero" jsonschema:"replaces all steps; raw lines, '# Heading' starts a section"`
}

// ListQuery filters GET /v1/recipes. The query tags name the URL
// parameters; repeated parameters fill the slices.
type ListQuery struct {
	Q          string   `query:"q" json:"q,omitempty" jsonschema:"full-text search over title, description, notes, ingredients and tags; words match as prefixes"`
	Tag        []string `query:"tag" json:"tag,omitempty" jsonschema:"only recipes with all of these tags (case-insensitive)"`
	Ingredient []string `query:"ingredient" json:"ingredient,omitempty" jsonschema:"only recipes containing all of these ingredients"`
	Favorite   bool     `query:"favorite" json:"favorite,omitempty" jsonschema:"only the caller's favourites"`
	WantToCook bool     `query:"want_to_cook" json:"want_to_cook,omitempty" jsonschema:"only recipes the caller wants to cook"`
	Sort       string   `query:"sort" json:"sort,omitempty" jsonschema:"title (default), updated or created"`
	Cursor     string   `query:"cursor" json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
	Limit      int      `query:"limit" json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// TrashQuery pages GET /v1/trash.
type TrashQuery struct {
	Cursor string `query:"cursor" json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
	Limit  int    `query:"limit" json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// ChangeQuery filters GET /v1/changes.
type ChangeQuery struct {
	EntityID string `query:"entity_id" json:"entity_id,omitempty" jsonschema:"only changes to this recipe or person"`
	Person   string `query:"person" json:"person,omitempty" jsonschema:"only changes made by this username"`
	Via      string `query:"via" json:"via,omitempty" jsonschema:"only changes made via ui, api or mcp"`
	Cursor   string `query:"cursor" json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
	Limit    int    `query:"limit" json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
}

// ParseRequest is the body of POST /v1/ingredients/parse.
type ParseRequest struct {
	Lines []string `json:"lines" jsonschema:"raw ingredient lines"`
}

// ParsedLine is one line of an ingredient parse preview. A heading line
// (Section non-nil) encodes as {"section","raw"}; any other line encodes as
// an Ingredient without the section key, matching the mock.
type ParsedLine struct {
	Section       *string  `json:"section,omitempty" jsonschema:"present only on heading lines"`
	Raw           string   `json:"raw"`
	Quantity      *float64 `json:"quantity,omitempty"`
	QuantityMax   *float64 `json:"quantity_max,omitempty"`
	Unit          *string  `json:"unit,omitempty"`
	Item          string   `json:"item,omitempty"`
	Note          *string  `json:"note,omitempty"`
	ParserVersion int      `json:"parser_version,omitempty"`
}

// MarshalJSON emits exactly one of the two line shapes.
func (p ParsedLine) MarshalJSON() ([]byte, error) {
	if p.Section != nil {
		return json.Marshal(struct {
			Section string `json:"section"`
			Raw     string `json:"raw"`
		}{*p.Section, p.Raw})
	}
	return json.Marshal(struct {
		Raw           string   `json:"raw"`
		Quantity      *float64 `json:"quantity"`
		QuantityMax   *float64 `json:"quantity_max"`
		Unit          *string  `json:"unit"`
		Item          string   `json:"item"`
		Note          *string  `json:"note"`
		ParserVersion int      `json:"parser_version"`
	}{p.Raw, p.Quantity, p.QuantityMax, p.Unit, p.Item, p.Note, p.ParserVersion})
}
