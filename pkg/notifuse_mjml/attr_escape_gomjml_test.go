package notifuse_mjml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// attrFromRenderedHTML parses rendered HTML and returns the first value of the given
// attribute, decoded the way a browser reads it.
func attrFromRenderedHTML(t *testing.T, rendered, attr string) (string, bool) {
	t.Helper()
	z := html.NewTokenizer(strings.NewReader(rendered))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return "", false
		case html.StartTagToken, html.SelfClosingTagToken:
			for _, a := range z.Token().Attr {
				if a.Key == attr {
					return a.Val, true
				}
			}
		}
	}
}

// TestAttributeValueRoundTripThroughGomjml is the contract of escapeAttributeValue:
// text typed into an attribute (an image title/alt, a link title, …) must reach the
// final HTML as the same text a browser reads back, even when it contains the markup
// characters " < > & that the MJML renderer would otherwise mangle (issue #438's
// image-title follow-up). Values are read back with a real HTML tokenizer.
func TestAttributeValueRoundTripThroughGomjml(t *testing.T) {
	values := []string{
		`plain title`,
		`27" monitor`,
		`say "hi" now`,
		`a < b and c > d`,
		`<b>bold</b>`,
		`Tom & Jerry`,
		"café \"deluxe\"",
		`it's a "test"`,
		`a&b<c>d"e'f`,
		`100% < 200%`,
		`"`, `<`, `>`, `&`,
		`url?a=1&b=2`,
		`she said "<ok>"`,
	}
	for _, attr := range []string{"title", "alt"} {
		for _, v := range values {
			resp, err := CompileTemplate(CompileTemplateRequest{
				WorkspaceID: "ws", MessageID: "m",
				VisualEditorTree: editorTree(editorImage(attr, v)),
			})
			require.NoError(t, err)
			require.True(t, resp.Success, "[%s=%q] error: %v", attr, v, resp.Error)
			require.NotNil(t, resp.HTML)
			got, ok := attrFromRenderedHTML(t, *resp.HTML, attr)
			require.True(t, ok, "[%s=%q] attribute missing from output", attr, v)
			assert.Equal(t, v, got, "[%s] round-trip of %q", attr, v)
		}
	}
}

// TestAttributeValueWithLiquidDefaultRendersInsideAttribute covers the exact #438
// follow-up: a Liquid default whose fallback contains a double quote used to compile
// to broken HTML; it must now render the fallback as a readable attribute value.
func TestAttributeValueWithLiquidDefaultRendersInsideAttribute(t *testing.T) {
	resp, err := CompileTemplate(CompileTemplateRequest{
		WorkspaceID: "ws", MessageID: "m",
		VisualEditorTree: editorTree(editorImage("title", `{{ contact.first_name | default: 'say "hi"' }}`)),
		TemplateData:     MapOfAny{"contact": map[string]interface{}{"first_name": nil}},
	})
	require.NoError(t, err)
	require.True(t, resp.Success, "error: %v", resp.Error)
	got, ok := attrFromRenderedHTML(t, *resp.HTML, "title")
	require.True(t, ok)
	assert.Equal(t, `say "hi"`, got)
}

// TestEscapeAttributeValue_EntityLikeTextIsTreatedAsAnEntity pins a known, pre-existing
// limitation (unchanged by the double-escape fix): text that is itself an ampersand
// entity, e.g. the six characters &quot;, is interpreted by the renderer as that
// entity rather than surviving verbatim. Documented here so it is a deliberate
// boundary, not a silent surprise.
func TestEscapeAttributeValue_EntityLikeTextIsTreatedAsAnEntity(t *testing.T) {
	resp, err := CompileTemplate(CompileTemplateRequest{
		WorkspaceID: "ws", MessageID: "m",
		VisualEditorTree: editorTree(editorImage("title", `&quot;`)),
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
	got, _ := attrFromRenderedHTML(t, *resp.HTML, "title")
	assert.Equal(t, `"`, got, "literal entity text is decoded, as it has always been")
}
