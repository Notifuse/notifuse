package mcp

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCursor_RoundTrip(t *testing.T) {
	filters := map[string]any{"search": "jane", "segment_ids": []string{"vip", "beta"}}
	cursor, err := EncodeCursor("search_contacts", "ws1", filters, "inner~position")
	require.NoError(t, err)

	var inner string
	require.Nil(t, DecodeCursor(cursor, "search_contacts", "ws1", map[string]any{"segment_ids": []string{"vip", "beta"}, "search": "jane"}, &inner))
	assert.Equal(t, "inner~position", inner)

	var offset int
	cursor, err = EncodeCursor("list_lists", "ws1", nil, 40)
	require.NoError(t, err)
	require.Nil(t, DecodeCursor(cursor, "list_lists", "ws1", map[string]any{}, &offset))
	assert.Equal(t, 40, offset)
}

func TestCursor_Envelope(t *testing.T) {
	cursor, err := EncodeCursor("search_contacts", "ws1", map[string]any{"search": "jane"}, "k1")
	require.NoError(t, err)
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	require.NoError(t, err)

	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &envelope))
	assert.ElementsMatch(t, []string{"v", "t", "h", "k"}, mapKeys(envelope))
	assert.JSONEq(t, `1`, string(envelope["v"]))
	assert.JSONEq(t, `"search_contacts"`, string(envelope["t"]))
	assert.JSONEq(t, `"k1"`, string(envelope["k"]))
	var hash string
	require.NoError(t, json.Unmarshal(envelope["h"], &hash))
	assert.Regexp(t, `^[0-9a-f]{12}$`, hash)
}

// A cursor from another tool, another workspace or other filters, and a
// cursor that does not decode, are validation errors.
func TestCursor_Mismatches(t *testing.T) {
	filters := map[string]any{"search": "jane"}
	cursor, err := EncodeCursor("search_contacts", "ws1", filters, "k1")
	require.NoError(t, err)

	otherVersion, err := json.Marshal(map[string]any{"v": 2, "t": "search_contacts", "h": filterHash("ws1", filters), "k": "k1"})
	require.NoError(t, err)

	cases := map[string]struct {
		cursor, tool, workspace string
		filters                 map[string]any
	}{
		"another tool":      {cursor, "list_lists", "ws1", filters},
		"another workspace": {cursor, "search_contacts", "ws2", filters},
		"other filters":     {cursor, "search_contacts", "ws1", map[string]any{"search": "john"}},
		"a filter dropped":  {cursor, "search_contacts", "ws1", map[string]any{}},
		"garbage":           {"not a cursor!", "search_contacts", "ws1", filters},
		"base64 of junk":    {base64.RawURLEncoding.EncodeToString([]byte("junk")), "search_contacts", "ws1", filters},
		"another version":   {base64.RawURLEncoding.EncodeToString(otherVersion), "search_contacts", "ws1", filters},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var inner string
			toolErr := DecodeCursor(tc.cursor, tc.tool, tc.workspace, tc.filters, &inner)
			require.NotNil(t, toolErr)
			assert.Equal(t, CodeValidation, toolErr.Code)
		})
	}

	var wrongType int
	toolErr := DecodeCursor(cursor, "search_contacts", "ws1", filters, &wrongType)
	require.NotNil(t, toolErr, "an inner value of the wrong type")
	assert.Equal(t, CodeValidation, toolErr.Code)
}

func TestPageLimit(t *testing.T) {
	limit, toolErr := PageLimit(nil)
	require.Nil(t, toolErr)
	assert.Equal(t, 20, limit)

	for _, value := range []int{1, 50} {
		limit, toolErr = PageLimit(&value)
		require.Nil(t, toolErr)
		assert.Equal(t, value, limit)
	}
	for _, value := range []int{0, -1, 51} {
		_, toolErr = PageLimit(&value)
		require.NotNil(t, toolErr, value)
		assert.Equal(t, CodeValidation, toolErr.Code)
	}
}
