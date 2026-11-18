package notifuse_mjml

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compileEditorTree compiles a visual editor tree sent as the console sends it.
func compileEditorTree(t *testing.T, tree string) string {
	t.Helper()
	var req CompileTemplateRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"workspace_id": "ws-1",
		"message_id": "msg-1",
		"visual_editor_tree": `+tree+`
	}`), &req))
	resp, err := CompileTemplate(req)
	require.NoError(t, err)
	require.True(t, resp.Success, "compile error: %v", resp.Error)
	require.NotNil(t, resp.HTML)
	return *resp.HTML
}

// The attributes MJML 5 adds and the editor writes reach safe-mjml through the converter.
func TestCompileTemplate_MJML5EditorAttributes(t *testing.T) {
	t.Run("a section gutter spaces its columns", func(t *testing.T) {
		html := compileEditorTree(t, `{"id": "mjml-1", "type": "mjml", "children": [
			{"id": "body-1", "type": "mj-body", "children": [
				{"id": "section-1", "type": "mj-section", "attributes": {"gutter": "20px"}, "children": [
					{"id": "column-1", "type": "mj-column", "children": [{"id": "t1", "type": "mj-text", "content": "a"}]},
					{"id": "column-2", "type": "mj-column", "children": [{"id": "t2", "type": "mj-text", "content": "b"}]}
				]}
			]}
		]}`)
		// The two 50% columns lose half the gutter each, and get it as padding on their inner side
		assert.Contains(t, html, ".mj-column-per-48-333333")
		assert.Contains(t, html, ".mj-column-gutter-2-1-per-3-333333 { padding: 0% 1.666667% 0% 0% !important; }")
		assert.Contains(t, html, ".mj-column-gutter-2-2-per-3-333333 { padding: 0% 0% 0% 1.666667% !important; }")
	})

	t.Run("a social icon border is drawn on the icon", func(t *testing.T) {
		html := compileEditorTree(t, `{"id": "mjml-1", "type": "mjml", "children": [
			{"id": "body-1", "type": "mj-body", "children": [
				{"id": "section-1", "type": "mj-section", "children": [
					{"id": "column-1", "type": "mj-column", "children": [
						{"id": "social-1", "type": "mj-social", "attributes": {"border": "3px dashed #00ff00"}, "children": [
							{"id": "el-1", "type": "mj-social-element", "attributes": {"name": "facebook", "href": "https://facebook.com/notifuse", "border": "2px solid #ff0000"}},
							{"id": "el-2", "type": "mj-social-element", "attributes": {"name": "x", "href": "https://x.com/notifuse"}}
						]}
					]}
				]}
			]}
		]}`)
		// The icon's own border, then the one mj-social passes to the other icon
		assert.Contains(t, html, "border:2px solid #ff0000")
		assert.Contains(t, html, "border:3px dashed #00ff00")
	})

	t.Run("the body CSS class is written on the body tag", func(t *testing.T) {
		html := compileEditorTree(t, `{"id": "mjml-1", "type": "mjml", "children": [
			{"id": "body-1", "type": "mj-body", "attributes": {"cssClass": "brand"}, "children": [
				{"id": "section-1", "type": "mj-section", "children": [
					{"id": "column-1", "type": "mj-column", "children": [{"id": "t1", "type": "mj-text", "content": "a"}]}
				]}
			]}
		]}`)
		assert.Contains(t, html, `<body class="brand"`)
	})
}
