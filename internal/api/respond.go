package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/sroberts/sous/internal/recipes"
)

// errorBody is every error response: {"error":{"code","message"}}.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code" jsonschema:"stable code to branch on, such as who_are_you, not_found or stale"`
	Message string `json:"message" jsonschema:"for people; may change"`
}

// importErrorBody is a 422: the error plus what the page still offered, to
// start a pasted recipe from.
type importErrorBody struct {
	Error errorDetail         `json:"error"`
	Page  *recipes.ImportPage `json:"page" jsonschema:"the page's link, title and photo, when it was reached"`
}

// staleBody is a 412: the error plus the recipe as it now stands.
type staleBody struct {
	Error   errorDetail    `json:"error"`
	Current recipes.Recipe `json:"current" jsonschema:"the recipe as it now stands; its version is the one to send next"`
}

const codeTooLarge = "too_large"

var statusOf = map[string]int{
	recipes.CodeInvalid:              http.StatusBadRequest,
	recipes.CodeUnknownImage:         http.StatusBadRequest,
	recipes.CodeInvalidCursor:        http.StatusBadRequest,
	recipes.CodeWhoAreYou:            http.StatusUnauthorized,
	recipes.CodeNotFound:             http.StatusNotFound,
	recipes.CodeExists:               http.StatusConflict,
	recipes.CodeDeleted:              http.StatusConflict,
	recipes.CodeStale:                http.StatusPreconditionFailed,
	recipes.CodeUnsupportedMediaType: http.StatusUnsupportedMediaType,
	recipes.CodeVersionRequired:      http.StatusPreconditionRequired,
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(append(b, '\n'))
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, errorBody{errorDetail{code, message}})
}

// writeRecipe sends a full recipe with its version as the ETag.
func writeRecipe(w http.ResponseWriter, status int, r recipes.Recipe) {
	w.Header().Set("ETag", etag(r.Version))
	writeJSON(w, status, r)
}

func etag(version int) string { return `"` + strconv.Itoa(version) + `"` }

// fail writes err as a JSON error. Anything that isn't a domain error is a
// storage failure: logged, and reported as 503 (spec §4.1) without its cause.
func (a *api) fail(w http.ResponseWriter, r *http.Request, err error) {
	var re *recipes.Error
	var ie *recipes.ImportError
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &ie):
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusUnprocessableEntity, importErrorBody{errorDetail{ie.Code, ie.Message}, ie.Page})
		return
	case errors.As(err, &re):
		status, ok := statusOf[re.Code]
		if !ok {
			break
		}
		if re.Code == recipes.CodeStale && re.Current != nil {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("ETag", etag(re.Current.Version))
			writeJSON(w, status, staleBody{errorDetail{re.Code, re.Message}, *re.Current})
			return
		}
		writeError(w, status, re.Code, re.Message)
		return
	case errors.As(err, &tooBig):
		writeError(w, http.StatusRequestEntityTooLarge, codeTooLarge,
			fmt.Sprintf("the body is larger than the %d byte limit", tooBig.Limit))
		return
	}
	a.d.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusServiceUnavailable, "unavailable", "Sous can't reach its storage right now")
}

// ifMatch reads the recipe version from If-Match, accepting "3", W/"3" or 3.
func ifMatch(r *http.Request) (int, error) {
	v := strings.TrimSpace(r.Header.Get("If-Match"))
	if v == "" {
		return 0, recipes.Errorf(recipes.CodeVersionRequired, `send If-Match with the version you edited, e.g. If-Match: "3"`)
	}
	v = strings.Trim(strings.TrimPrefix(v, "W/"), `"`)
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, recipes.Errorf(recipes.CodeInvalid, `If-Match must be a recipe version such as "3"`)
	}
	return n, nil
}

// decodeJSON decodes exactly one JSON value into v, refusing unknown fields.
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return bodyError(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return err
		}
		return recipes.Errorf(recipes.CodeInvalid, "the body must be a single JSON value")
	}
	return nil
}

func bodyError(err error) error {
	var tooBig *http.MaxBytesError
	var syntax *json.SyntaxError
	var typ *json.UnmarshalTypeError
	switch {
	case errors.As(err, &tooBig):
		return err
	case errors.Is(err, io.EOF):
		return recipes.Errorf(recipes.CodeInvalid, "a JSON body is required")
	case errors.As(err, &syntax), errors.Is(err, io.ErrUnexpectedEOF):
		return recipes.Errorf(recipes.CodeInvalid, "the body is not valid JSON")
	case errors.As(err, &typ):
		if typ.Field == "" {
			return recipes.Errorf(recipes.CodeInvalid, "the body must be %s", jsonType(typ.Type))
		}
		return recipes.Errorf(recipes.CodeInvalid, "%s must be %s", typ.Field, jsonType(typ.Type))
	}
	// Unknown fields have no error type; the message names the field.
	return recipes.Errorf(recipes.CodeInvalid, "%s", strings.TrimPrefix(err.Error(), "json: "))
}

func jsonType(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "an integer"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Slice, reflect.Array:
		return "an array"
	}
	return "an object"
}
