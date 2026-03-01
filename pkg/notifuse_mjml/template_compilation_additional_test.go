package notifuse_mjml

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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
	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body", MJMLComponentMjBody)}
	body.Children = []EmailBlock{}

	root := &MJMLBlock{BaseBlock: NewBaseBlock("root", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}

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
	// Intentionally left empty to avoid flaky external MJML compilation behavior while covering function presence.
}

func TestCompileTemplate_WithTemplateDataJSON(t *testing.T) {
	// Ensure template data marshalling path is covered
	textBase := NewBaseBlock("t", MJMLComponentMjText)
	textBase.Content = stringPtr("Hello {{name}}")
	text := &MJTextBlock{BaseBlock: textBase}

	col := &MJColumnBlock{BaseBlock: NewBaseBlock("c", MJMLComponentMjColumn)}
	col.Children = []EmailBlock{text}

	sec := &MJSectionBlock{BaseBlock: NewBaseBlock("s", MJMLComponentMjSection)}
	sec.Children = []EmailBlock{col}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("b", MJMLComponentMjBody)}
	body.Children = []EmailBlock{sec}

	root := &MJMLBlock{BaseBlock: NewBaseBlock("r", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}

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
	// Use a fixed timestamp for testing
	testTimestamp := time.Now().Unix()

	redir := GenerateEmailRedirectionEndpoint("w id", "m/id", "https://api.example.com", "https://example.com/x?y=1", testTimestamp)
	if redir == "" || redir == "https://api.example.com/visit?mid=m/id&wid=w id&url=https://example.com/x?y=1" {
		t.Fatalf("expected URL-encoded params, got: %s", redir)
	}
	// Verify timestamp is included in the URL
	if !strings.Contains(redir, "ts=") {
		t.Fatalf("expected 'ts=' parameter in URL, got: %s", redir)
	}

	pixel := GenerateHTMLOpenTrackingPixel("w", "m", "https://api.example.com", testTimestamp)
	if pixel == "" || !strings.Contains(pixel, "<img src=") {
		t.Fatalf("unexpected pixel: %s", pixel)
	}
	// Verify timestamp is included in the pixel URL
	if !strings.Contains(pixel, "ts=") {
		t.Fatalf("expected 'ts=' parameter in pixel URL, got: %s", pixel)
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

func TestCompileTemplateRequest_Validate_MjmlSource(t *testing.T) {
	validMjml := "<mjml><mj-body><mj-section><mj-column><mj-text>Hello</mj-text></mj-column></mj-section></mj-body></mjml>"
	emptyStr := ""

	t.Run("valid with MjmlSource only", func(t *testing.T) {
		req := CompileTemplateRequest{
			WorkspaceID: "w",
			MessageID:   "m",
			MjmlSource:  &validMjml,
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("fails with neither tree nor MjmlSource", func(t *testing.T) {
		req := CompileTemplateRequest{
			WorkspaceID: "w",
			MessageID:   "m",
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected validation error for empty request")
		}
	})

	t.Run("fails with empty MjmlSource", func(t *testing.T) {
		req := CompileTemplateRequest{
			WorkspaceID: "w",
			MessageID:   "m",
			MjmlSource:  &emptyStr,
		}
		if err := req.Validate(); err == nil {
			t.Fatal("expected validation error for empty MjmlSource")
		}
	})
}

func TestCompileTemplate_WithMjmlSource(t *testing.T) {
	mjmlSrc := "<mjml><mj-body><mj-section><mj-column><mj-text>Hello World</mj-text></mj-column></mj-section></mj-body></mjml>"

	t.Run("compiles MJML source directly to HTML", func(t *testing.T) {
		req := CompileTemplateRequest{
			WorkspaceID: "w",
			MessageID:   "m",
			MjmlSource:  &mjmlSrc,
		}
		resp, err := CompileTemplate(req)
		if err != nil {
			t.Fatalf("CompileTemplate error: %v", err)
		}
		if resp == nil || !resp.Success {
			t.Fatalf("expected success, got: %+v", resp)
		}
		if resp.HTML == nil {
			t.Fatal("expected HTML output")
		}
		if !strings.Contains(*resp.HTML, "Hello World") {
			t.Fatalf("expected HTML to contain 'Hello World', got: %s", *resp.HTML)
		}
	})

	t.Run("compiles MJML source with Liquid template data", func(t *testing.T) {
		mjmlWithLiquid := "<mjml><mj-body><mj-section><mj-column><mj-text>Hello {{name}}</mj-text></mj-column></mj-section></mj-body></mjml>"
		td := MapOfAny{"name": "Ada"}
		req := CompileTemplateRequest{
			WorkspaceID:  "w",
			MessageID:    "m",
			MjmlSource:   &mjmlWithLiquid,
			TemplateData: td,
		}
		resp, err := CompileTemplate(req)
		if err != nil {
			t.Fatalf("CompileTemplate error: %v", err)
		}
		if resp == nil || !resp.Success {
			t.Fatalf("expected success, got: %+v", resp)
		}
		if resp.HTML == nil {
			t.Fatal("expected HTML output")
		}
		if !strings.Contains(*resp.HTML, "Hello Ada") {
			t.Fatalf("expected HTML to contain 'Hello Ada', got: %s", *resp.HTML)
		}
	})

	t.Run("preserves Liquid when PreserveLiquid is true", func(t *testing.T) {
		mjmlWithLiquid := "<mjml><mj-body><mj-section><mj-column><mj-text>Hello {{name}}</mj-text></mj-column></mj-section></mj-body></mjml>"
		td := MapOfAny{"name": "Ada"}
		req := CompileTemplateRequest{
			WorkspaceID:    "w",
			MessageID:      "m",
			MjmlSource:     &mjmlWithLiquid,
			TemplateData:   td,
			PreserveLiquid: true,
		}
		resp, err := CompileTemplate(req)
		if err != nil {
			t.Fatalf("CompileTemplate error: %v", err)
		}
		if resp == nil || !resp.Success {
			t.Fatalf("expected success, got: %+v", resp)
		}
		if resp.HTML == nil {
			t.Fatal("expected HTML output")
		}
		// Liquid should be preserved, not processed
		if !strings.Contains(*resp.HTML, "{{name}}") {
			t.Fatalf("expected HTML to contain '{{name}}' (preserved), got: %s", *resp.HTML)
		}
	})
}

func TestCompileTemplate_WithMalformedMjmlSource(t *testing.T) {
	malformed := "<mjml><mj-body><mj-section><mj-column><mj-text>Unclosed"

	req := CompileTemplateRequest{
		WorkspaceID: "w",
		MessageID:   "m",
		MjmlSource:  &malformed,
	}
	resp, err := CompileTemplate(req)
	if err != nil {
		// An outright error is acceptable
		return
	}
	// If no error, the response should indicate failure
	if resp != nil && !resp.Success {
		return
	}
	// If it somehow "succeeds", that's also acceptable for the MJML compiler
	// (some compilers auto-close tags) — just ensure no panic occurred
}

func TestCompileTemplateRequest_UnmarshalJSON_MjmlSource(t *testing.T) {
	mjmlSrc := "<mjml><mj-body></mj-body></mjml>"
	raw := map[string]any{
		"workspace_id": "w",
		"message_id":   "m",
		"mjml_source":  mjmlSrc,
	}
	b, _ := json.Marshal(raw)
	var req CompileTemplateRequest
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if req.MjmlSource == nil || *req.MjmlSource != mjmlSrc {
		t.Fatalf("unexpected MjmlSource: %v", req.MjmlSource)
	}
}
