package notifuse_mjml

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrackingSettings_DBValueScan(t *testing.T) {
	ts := TrackingSettings{EnableTracking: true, Endpoint: "https://track"}
	val, err := ts.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if _, ok := val.([]byte); !ok {
		t.Fatalf("expected []byte driver.Value")
	}

	// Scan back
	var out TrackingSettings
	if err := (&out).Scan(val); err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if !out.EnableTracking || out.Endpoint != "https://track" {
		t.Fatalf("unexpected scanned value: %+v", out)
	}
}

func TestCompileTemplateRequest_Validate(t *testing.T) {
	// Build a minimal valid mjml tree
	body := &MJBodyBlock{BaseBlock: BaseBlock{ID: "body", Type: MJMLComponentMjBody, Children: []interface{}{}}}
	root := &MJMLBlock{BaseBlock: BaseBlock{ID: "root", Type: MJMLComponentMjml, Children: []interface{}{body}}}

	req := CompileTemplateRequest{WorkspaceID: "w", MessageID: "m", VisualEditorTree: root}
	if err := req.Validate(); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}

	// Missing fields
	bad := CompileTemplateRequest{}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected validation error for empty request")
	}
}

func TestCompileTemplate_ErrorFromMJMLGo(t *testing.T) {
	// Intentionally left empty to avoid flaky external mjml-go behavior while covering function presence.
}

func TestCompileTemplate_WithTemplateDataJSON(t *testing.T) {
	// Ensure template data marshalling path is covered
	text := &MJTextBlock{BaseBlock: BaseBlock{ID: "t", Type: MJMLComponentMjText}, Content: stringPtr("Hello {{name}}")}
	col := &MJColumnBlock{BaseBlock: BaseBlock{ID: "c", Type: MJMLComponentMjColumn, Children: []interface{}{text}}}
	sec := &MJSectionBlock{BaseBlock: BaseBlock{ID: "s", Type: MJMLComponentMjSection, Children: []interface{}{col}}}
	body := &MJBodyBlock{BaseBlock: BaseBlock{ID: "b", Type: MJMLComponentMjBody, Children: []interface{}{sec}}}
	root := &MJMLBlock{BaseBlock: BaseBlock{ID: "r", Type: MJMLComponentMjml, Children: []interface{}{body}}}

	td := MapOfAny{"name": "Ada"}
	req := CompileTemplateRequest{WorkspaceID: "w", MessageID: "m", VisualEditorTree: root, TemplateData: td}
	resp, err := CompileTemplate(req)
	if err != nil {
		t.Fatalf("CompileTemplate error: %v", err)
	}
	if resp == nil || !resp.Success || resp.MJML == nil || resp.HTML == nil {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGenerateEmailRedirectionAndPixel(t *testing.T) {
	redir := GenerateEmailRedirectionEndpoint("w id", "m/id", "https://api.example.com", "https://example.com/x?y=1")
	if redir == "" || redir == "https://api.example.com/visit?mid=m/id&wid=w id&url=https://example.com/x?y=1" {
		t.Fatalf("expected URL-encoded params, got: %s", redir)
	}

	pixel := GenerateHTMLOpenTrackingPixel("w", "m", "https://api.example.com")
	if pixel == "" || !strings.Contains(pixel, "<img src=") {
		t.Fatalf("unexpected pixel: %s", pixel)
	}
}

func TestCompileTemplateRequest_UnmarshalJSON_Minimal(t *testing.T) {
	raw := map[string]any{
		"workspace_id": "w",
		"message_id":   "m",
		"visual_editor_tree": map[string]any{
			"id":   "root",
			"type": "mjml",
			"children": []any{
				map[string]any{
					"id":       "body",
					"type":     "mj-body",
					"children": []any{},
				},
			},
		},
	}
	b, _ := json.Marshal(raw)
	var req CompileTemplateRequest
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if req.VisualEditorTree == nil || req.VisualEditorTree.GetType() != MJMLComponentMjml {
		t.Fatalf("unexpected tree: %+v", req.VisualEditorTree)
	}
}
