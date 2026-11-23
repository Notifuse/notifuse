package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// FreeTextLimit is the most characters a free-text field of a tool result
// carries. Editable content (MJML, a subject, a blog body, a segment tree) is
// returned whole and never goes through CapText.
const FreeTextLimit = 2000

// successResult builds the result of a tool that succeeded: the object as
// structuredContent, and its serialized JSON as the text content, so a client
// that reads only text sees the same thing.
func successResult(out any) *sdk.CallToolResult {
	data, err := json.Marshal(out)
	if err != nil {
		return errorResult(internalError())
	}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: string(data)}},
		StructuredContent: json.RawMessage(data),
	}
}

// errorResult builds an isError result carrying {"error": {...}} in both
// structuredContent and the text content.
func errorResult(toolErr *ToolError) *sdk.CallToolResult {
	data, err := json.Marshal(map[string]*ToolError{"error": toolErr})
	if err != nil {
		data = []byte(`{"error":{"code":"internal","message":"internal error"}}`)
	}
	return &sdk.CallToolResult{
		IsError:           true,
		Content:           []sdk.Content{&sdk.TextContent{Text: string(data)}},
		StructuredContent: json.RawMessage(data),
	}
}

// CapText cleans a free-text value for a tool result: invalid UTF-8 and
// control characters other than newline and tab are removed, then the text is
// cut at FreeTextLimit characters. cut reports the second step.
func CapText(value string) (capped string, cut bool) {
	value = strings.ToValidUTF8(value, "")
	var b strings.Builder
	b.Grow(min(len(value), FreeTextLimit*utf8.UTFMax))
	count := 0
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			continue
		}
		if count == FreeTextLimit {
			return b.String(), true
		}
		b.WriteRune(r)
		count++
	}
	return b.String(), false
}

// Truncation is embedded in an object of a tool result that holds free text:
// it says which of the object's fields CapText cut.
type Truncation struct {
	Truncated       bool     `json:"truncated,omitempty"`
	TruncatedFields []string `json:"truncated_fields,omitempty"`
}

// Text caps one free-text field of the object, recording its JSON path when
// it was cut.
func (t *Truncation) Text(path, value string) string {
	capped, cut := CapText(value)
	if cut {
		t.Truncated = true
		t.TruncatedFields = append(t.TruncatedFields, path)
	}
	return capped
}

// OptionalText caps a free-text field that may be absent: nil stays nil.
func (t *Truncation) OptionalText(path string, value *string) *string {
	if value == nil {
		return nil
	}
	capped := t.Text(path, *value)
	return &capped
}

// JSON caps a free-form JSON field (a custom_json value, a timeline entry's
// changes): a value whose serialization is longer than FreeTextLimit
// characters is replaced by nil and its path recorded, since cutting it would
// leave invalid or misleading JSON. A shorter value is returned with control
// characters removed from its strings and keys.
func (t *Truncation) JSON(path string, value any) any {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil || utf8.RuneCount(bytes.TrimSpace(buf.Bytes())) > FreeTextLimit {
		t.Truncated = true
		t.TruncatedFields = append(t.TruncatedFields, path)
		return nil
	}
	return cleanJSON(value)
}

// cleanJSON removes control characters from the strings of a decoded JSON
// value. Its strings are already within FreeTextLimit, so nothing is cut.
func cleanJSON(value any) any {
	switch v := value.(type) {
	case string:
		capped, _ := CapText(v)
		return capped
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			cleanKey, _ := CapText(key)
			out[cleanKey] = cleanJSON(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cleanJSON(item)
		}
		return out
	default:
		return v
	}
}
