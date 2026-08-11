package domain

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// openAPIBundlePath is the generated spec, relative to this package's
// directory (Go runs a test binary with its own package dir as cwd).
const openAPIBundlePath = "../../openapi.json"

// TestOpenAPIWebTrackBoundsMatchConstants pins the /track bounds published in
// openapi.json to the constants the server actually enforces.
//
// It exists because nothing else in this repository reads openapi.json: it is a
// generated artifact whose only consumers are customers writing integrations
// against it. Without this test, changing one of the constants below and
// forgetting `make openapi-bundle` ships a spec that quietly disagrees with the
// server, and the drift is discovered by whoever generated a client from it.
//
// A failure here is not a reason to edit openapi.json — it is generated. Change
// the bound in openapi/components/schemas/web-analytics.yaml if the spec is
// wrong, then re-run `make openapi-bundle`.
func TestOpenAPIWebTrackBoundsMatchConstants(t *testing.T) {
	spec := loadOpenAPIBundle(t)

	// The expected values are float64 because that is what JSON numbers decode
	// to; every bound here is a whole number well inside float64's exact
	// integer range (including 1e12), so equality is exact rather than
	// approximate.
	cases := []struct {
		constant string   // the Go constant that must be mirrored
		path     []string // where the bundled spec publishes it
		expected float64
	}{
		{
			constant: "WebTrackMaxEmailLength",
			path:     []string{"components", "schemas", "WebTrackPayload", "properties", "contact_email", "maxLength"},
			expected: WebTrackMaxEmailLength,
		},
		{
			constant: "WebTrackMaxHMACLength",
			path:     []string{"components", "schemas", "WebTrackPayload", "properties", "contact_email_hmac", "maxLength"},
			expected: WebTrackMaxHMACLength,
		},
		{
			constant: "WebTrackMaxIdentifyTokenLength",
			path:     []string{"components", "schemas", "WebTrackPayload", "properties", "identify_token", "maxLength"},
			expected: WebTrackMaxIdentifyTokenLength,
		},
		{
			constant: "WebTrackMaxActions",
			path:     []string{"components", "schemas", "WebTrackPayload", "properties", "actions", "maxItems"},
			expected: WebTrackMaxActions,
		},
		{
			constant: "WebTrackMaxDurationMs",
			path:     []string{"components", "schemas", "WebPageviewAction", "properties", "duration", "maximum"},
			expected: WebTrackMaxDurationMs,
		},
		{
			constant: "WebTrackMaxGoalValue",
			path:     []string{"components", "schemas", "WebGoalAction", "properties", "value", "maximum"},
			expected: WebTrackMaxGoalValue,
		},
	}

	for _, tc := range cases {
		t.Run(tc.constant, func(t *testing.T) {
			documented := openAPINumberAt(t, spec, tc.path)
			require.Equal(t, tc.expected, documented,
				"openapi.json documents %s = %v at %s, but the server enforces %s = %v; re-run `make openapi-bundle` after editing openapi/components/schemas/web-analytics.yaml",
				tc.constant, documented, strings.Join(tc.path, "."), tc.constant, tc.expected)
		})
	}
}

// loadOpenAPIBundle decodes the generated spec with UseNumber so no bound is
// silently reshaped on the way in: a large integer left to the default decoder
// becomes a float64, which would make a mismatch read as a type problem instead
// of the drift it is.
func loadOpenAPIBundle(t *testing.T) map[string]interface{} {
	t.Helper()

	raw, err := os.ReadFile(openAPIBundlePath)
	require.NoError(t, err, "cannot read the generated OpenAPI bundle at %s; run `make openapi-bundle`", openAPIBundlePath)

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var spec map[string]interface{}
	require.NoError(t, decoder.Decode(&spec), "%s is not valid JSON; it is generated, so re-run `make openapi-bundle`", openAPIBundlePath)
	return spec
}

// openAPINumberAt walks a key path and returns the numeric value there,
// accepting json.Number as well as float64 so the assertion never depends on
// how the decoder happened to type the literal.
func openAPINumberAt(t *testing.T, spec map[string]interface{}, path []string) float64 {
	t.Helper()

	var current interface{} = spec
	for i, key := range path {
		object, ok := current.(map[string]interface{})
		require.Truef(t, ok, "%s: %s is not an object in openapi.json", strings.Join(path, "."), strings.Join(path[:i], "."))

		current, ok = object[key]
		require.Truef(t, ok, "%s: openapi.json has no %q — the field was renamed or the bound was dropped from the spec", strings.Join(path, "."), key)
	}

	switch value := current.(type) {
	case json.Number:
		parsed, err := value.Float64()
		require.NoErrorf(t, err, "%s: %q is not a number", strings.Join(path, "."), value.String())
		return parsed
	case float64:
		return value
	default:
		t.Fatalf("%s: expected a number in openapi.json, got %T", strings.Join(path, "."), current)
		return 0
	}
}
