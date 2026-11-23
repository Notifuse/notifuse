package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapText(t *testing.T) {
	capped, cut := CapText("line one\nline\ttwo")
	assert.Equal(t, "line one\nline\ttwo", capped, "newline and tab are kept")
	assert.False(t, cut)

	capped, cut = CapText("a\x00b\x1bc\rd\u0085e\x7f")
	assert.Equal(t, "abcde", capped, "other control characters are removed")
	assert.False(t, cut, "removing control characters is not a cut")

	capped, cut = CapText(strings.Repeat("é", 2000))
	assert.Equal(t, 2000, len([]rune(capped)))
	assert.False(t, cut, "exactly 2000 characters is not cut")

	capped, cut = CapText(strings.Repeat("é", 2001))
	assert.Equal(t, strings.Repeat("é", 2000), capped, "the cut counts characters, not bytes")
	assert.True(t, cut)

	capped, cut = CapText(strings.Repeat("\x01", 2500) + "ok")
	assert.Equal(t, "ok", capped, "removed characters do not count toward the cap")
	assert.False(t, cut)

	capped, _ = CapText("bad\xffbyte")
	assert.Equal(t, "badbyte", capped)
}

func TestTruncation_NamesTheCutFields(t *testing.T) {
	var truncation Truncation
	assert.Equal(t, "short", truncation.Text("first_name", "short"))
	assert.False(t, truncation.Truncated)

	truncation.Text("last_name", strings.Repeat("x", 2001))
	truncation.Text("lists[0].list_name", strings.Repeat("y", 3000))
	assert.True(t, truncation.Truncated)
	assert.Equal(t, []string{"last_name", "lists[0].list_name"}, truncation.TruncatedFields)
}

func TestSuccessResult_TextIsTheStructuredContent(t *testing.T) {
	result := successResult(map[string]any{"workspace": map[string]string{"id": "ws1", "name": "Acme"}, "count": 2})
	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	text := result.Content[0].(*sdk.TextContent).Text
	structured, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	assert.JSONEq(t, text, string(structured))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"count":2}`, text)
}

func TestErrorResult_CarriesTheErrorObjectTwice(t *testing.T) {
	result := errorResult(&ToolError{Code: CodeInvalidState, Message: "cannot pause", Status: "draft"})
	assert.True(t, result.IsError)
	text := result.Content[0].(*sdk.TextContent).Text
	structured, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	assert.JSONEq(t, `{"error":{"code":"invalid_state","message":"cannot pause","status":"draft"}}`, text)
	assert.JSONEq(t, text, string(structured))
}

func TestTruncation_JSON(t *testing.T) {
	var truncation Truncation
	value := map[string]any{"plan": "pro\x1b", "seats": float64(3), "tags": []any{"a\x00", "b"}, "k\x07ey": "v"}
	assert.Equal(t, map[string]any{"plan": "pro", "seats": float64(3), "tags": []any{"a", "b"}, "key": "v"}, truncation.JSON("custom_json_1", value),
		"control characters are removed from strings and keys")
	assert.False(t, truncation.Truncated)

	// {"note":"…"} is 11 characters plus the string, so 1989 fit and 1990 do not.
	assert.NotNil(t, truncation.JSON("changes", map[string]any{"note": strings.Repeat("é", 1989)}))
	assert.False(t, truncation.Truncated, "a serialization of exactly 2000 characters is kept")
	assert.Nil(t, truncation.JSON("changes", map[string]any{"note": strings.Repeat("é", 1990)}))
	assert.True(t, truncation.Truncated)
	assert.Equal(t, []string{"changes"}, truncation.TruncatedFields)

	var html Truncation
	assert.NotNil(t, html.JSON("custom_json_2", strings.Repeat("<", 1998)), "HTML characters count once, not as \\u003c")
	assert.False(t, html.Truncated)
}

func TestTruncation_OptionalText(t *testing.T) {
	var truncation Truncation
	assert.Nil(t, truncation.OptionalText("absent", nil))
	long := strings.Repeat("o", FreeTextLimit+1)
	got := truncation.OptionalText("present", &long)
	if assert.NotNil(t, got) {
		assert.Equal(t, strings.Repeat("o", FreeTextLimit), *got)
	}
	assert.Equal(t, []string{"present"}, truncation.TruncatedFields)
}
