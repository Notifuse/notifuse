package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The annotation half of the OpenAPI drift guard. It lives in this package for
// the same reason TestOpenAPIWebTrackBoundsMatchConstants does: openapi.json is
// generated and read by nobody in this repository except customers generating a
// client from it, so a constant changed here and never re-bundled ships a spec
// that quietly disagrees with the server.
//
// A failure is never fixed by editing openapi.json. Edit
// openapi/components/schemas/annotations.yaml (or paths/annotations.yaml) and
// re-run `make openapi-bundle`.

// TestOpenAPIAnnotationBoundsMatchConstants pins the documented lengths, colour
// pattern and list limits to the constants the server enforces. Both request
// schemas are checked as well as the response one: a caller sizes their input
// field from the request schema, and only the response schema being right would
// still let them build a title the server rejects.
func TestOpenAPIAnnotationBoundsMatchConstants(t *testing.T) {
	spec := loadOpenAPIBundle(t)

	cases := []struct {
		constant string
		path     []string
		expected float64
	}{
		{"AnnotationMaxTitleLength", []string{"components", "schemas", "Annotation", "properties", "title", "maxLength"}, AnnotationMaxTitleLength},
		{"AnnotationMaxDescriptionLength", []string{"components", "schemas", "Annotation", "properties", "description", "maxLength"}, AnnotationMaxDescriptionLength},
		{"AnnotationMaxTitleLength", []string{"components", "schemas", "CreateAnnotationRequest", "properties", "title", "maxLength"}, AnnotationMaxTitleLength},
		{"AnnotationMaxDescriptionLength", []string{"components", "schemas", "CreateAnnotationRequest", "properties", "description", "maxLength"}, AnnotationMaxDescriptionLength},
		{"AnnotationMaxTitleLength", []string{"components", "schemas", "UpdateAnnotationRequest", "properties", "title", "maxLength"}, AnnotationMaxTitleLength},
		{"AnnotationMaxDescriptionLength", []string{"components", "schemas", "UpdateAnnotationRequest", "properties", "description", "maxLength"}, AnnotationMaxDescriptionLength},
	}

	for _, tc := range cases {
		t.Run(tc.constant+"/"+tc.path[2], func(t *testing.T) {
			documented := openAPINumberAt(t, spec, tc.path)
			require.Equal(t, tc.expected, documented,
				"openapi.json documents %v at %s, but the server enforces %s = %v; re-run `make openapi-bundle` after editing openapi/components/schemas/annotations.yaml",
				documented, strings.Join(tc.path, "."), tc.constant, tc.expected)
		})
	}

	t.Run("color pattern", func(t *testing.T) {
		// The Go regexp's own source string, so a widened pattern here cannot pass
		// against a spec still publishing the old one.
		expected := annotationColorRegex.String()
		for _, schema := range []string{"Annotation", "CreateAnnotationRequest", "UpdateAnnotationRequest"} {
			path := []string{"components", "schemas", schema, "properties", "color", "pattern"}
			require.Equal(t, expected, openAPIStringAt(t, spec, path),
				"%s documents a colour pattern the server does not enforce", strings.Join(path, "."))
		}
	})

	t.Run("default color", func(t *testing.T) {
		path := []string{"components", "schemas", "CreateAnnotationRequest", "properties", "color", "default"}
		require.Equal(t, AnnotationDefaultColor, openAPIStringAt(t, spec, path))
	})

	t.Run("source enum", func(t *testing.T) {
		path := []string{"components", "schemas", "Annotation", "properties", "source", "enum"}
		require.Equal(t, ValidAnnotationSources, openAPIStringsAt(t, spec, path),
			"%s must list exactly the sources the server accepts", strings.Join(path, "."))
	})

	t.Run("list limit", func(t *testing.T) {
		limit := openAPIParameter(t, spec, "/api/annotations.list", "get", "limit")
		require.Equal(t, float64(AnnotationDefaultListLimit), openAPINumberAt(t, limit, []string{"schema", "default"}))
		require.Equal(t, float64(AnnotationMaxListLimit), openAPINumberAt(t, limit, []string{"schema", "maximum"}))
	})
}

// TestOpenAPIAnnotationEndpointsArePublished pins the five routes and their
// methods to what annotation_handler.go registers. The methods are half the
// contract: .list and .get are GETs with query parameters, and documenting them
// as POSTs would send every generated client into a 405.
func TestOpenAPIAnnotationEndpointsArePublished(t *testing.T) {
	spec := loadOpenAPIBundle(t)

	endpoints := map[string]string{
		"/api/annotations.list":   "get",
		"/api/annotations.get":    "get",
		"/api/annotations.create": "post",
		"/api/annotations.update": "post",
		"/api/annotations.delete": "post",
	}

	for endpoint, method := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			item, ok := openAPIValueAt(t, spec, []string{"paths", endpoint}).(map[string]interface{})
			require.Truef(t, ok, "openapi.json publishes no path item for %s", endpoint)

			_, documented := item[method]
			require.Truef(t, documented, "openapi.json documents %s but not as a %s; the handler only accepts %s", endpoint, strings.ToUpper(method), strings.ToUpper(method))

			for other := range item {
				require.Equalf(t, method, other, "openapi.json documents %s on %s, which the handler answers with 405", strings.ToUpper(other), endpoint)
			}
		})
	}
}

// TestOpenAPIAnnotationCreateDoesNotPromiseIdempotency guards a deliberate
// out-of-scope decision rather than a constant: the API forces source=manual, so
// a retried call writes a duplicate. If an api source with a caller-supplied key
// is ever added, this test is the reminder that the description has to change
// with it.
func TestOpenAPIAnnotationCreateDoesNotPromiseIdempotency(t *testing.T) {
	spec := loadOpenAPIBundle(t)

	description := openAPIStringAt(t, spec, []string{"paths", "/api/annotations.create", "post", "description"})
	require.Contains(t, strings.ToLower(description), "not idempotent",
		"annotations.create must say outright that a retry writes a second annotation")
}

// openAPIValueAt walks a key path and returns whatever sits there. The sibling
// openAPINumberAt walks its own path rather than sharing this one; keeping both
// is cheaper than reworking a test that is already pinning production behaviour.
func openAPIValueAt(t *testing.T, spec map[string]interface{}, path []string) interface{} {
	t.Helper()

	var current interface{} = spec
	for i, key := range path {
		object, ok := current.(map[string]interface{})
		require.Truef(t, ok, "%s: %s is not an object in openapi.json", strings.Join(path, "."), strings.Join(path[:i], "."))

		current, ok = object[key]
		require.Truef(t, ok, "%s: openapi.json has no %q — the field was renamed or dropped from the spec", strings.Join(path, "."), key)
	}
	return current
}

func openAPIStringAt(t *testing.T, spec map[string]interface{}, path []string) string {
	t.Helper()

	value, ok := openAPIValueAt(t, spec, path).(string)
	require.Truef(t, ok, "%s: expected a string in openapi.json", strings.Join(path, "."))
	return value
}

func openAPIStringsAt(t *testing.T, spec map[string]interface{}, path []string) []string {
	t.Helper()

	raw, ok := openAPIValueAt(t, spec, path).([]interface{})
	require.Truef(t, ok, "%s: expected an array in openapi.json", strings.Join(path, "."))

	values := make([]string, 0, len(raw))
	for _, item := range raw {
		value, ok := item.(string)
		require.Truef(t, ok, "%s: expected an array of strings in openapi.json, got a %T", strings.Join(path, "."), item)
		values = append(values, value)
	}
	return values
}

// openAPIParameter returns a named query parameter of an operation. Parameters
// are an array, so they cannot be reached by key path.
func openAPIParameter(t *testing.T, spec map[string]interface{}, endpoint, method, name string) map[string]interface{} {
	t.Helper()

	raw, ok := openAPIValueAt(t, spec, []string{"paths", endpoint, method, "parameters"}).([]interface{})
	require.Truef(t, ok, "%s %s: parameters is not an array in openapi.json", strings.ToUpper(method), endpoint)

	for _, item := range raw {
		parameter, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if parameter["name"] == name {
			return parameter
		}
	}

	t.Fatalf("%s %s: openapi.json documents no %q parameter", strings.ToUpper(method), endpoint, name)
	return nil
}
