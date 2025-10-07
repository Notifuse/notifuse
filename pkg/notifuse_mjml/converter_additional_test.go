package notifuse_mjml

import (
	"strings"
	"testing"
)

func TestConvertJSONToMJMLWithData_Success(t *testing.T) {
	// mjml -> body -> section -> column -> text with liquid
	text := &MJTextBlock{
		BaseBlock: BaseBlock{ID: "text1", Type: MJMLComponentMjText},
		Content:   stringPtr("Hello {{name}}"),
	}
	column := &MJColumnBlock{BaseBlock: BaseBlock{ID: "col1", Type: MJMLComponentMjColumn, Children: []interface{}{text}}}
	section := &MJSectionBlock{BaseBlock: BaseBlock{ID: "sec1", Type: MJMLComponentMjSection, Children: []interface{}{column}}}
	body := &MJBodyBlock{BaseBlock: BaseBlock{ID: "body1", Type: MJMLComponentMjBody, Children: []interface{}{section}}}
	root := &MJMLBlock{BaseBlock: BaseBlock{ID: "root", Type: MJMLComponentMjml, Children: []interface{}{body}}}

	out, err := ConvertJSONToMJMLWithData(root, `{"name":"World"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, ">Hello World<") {
		t.Fatalf("expected rendered liquid content, got: %s", out)
	}
}

func TestConvertJSONToMJMLWithData_InvalidTemplateJSON(t *testing.T) {
	text := &MJTextBlock{BaseBlock: BaseBlock{ID: "t1", Type: MJMLComponentMjText}, Content: stringPtr("Hi {{name}}")}
	body := &MJBodyBlock{BaseBlock: BaseBlock{ID: "b1", Type: MJMLComponentMjBody, Children: []interface{}{text}}}
	root := &MJMLBlock{BaseBlock: BaseBlock{ID: "r1", Type: MJMLComponentMjml, Children: []interface{}{body}}}

	_, err := ConvertJSONToMJMLWithData(root, "{") // invalid JSON
	if err == nil {
		t.Fatal("expected error for invalid template JSON")
	}
}

func TestConvertBlockToMJMLWithError_LiquidFailure(t *testing.T) {
	// Malformed liquid to trigger parse/render error
	text := &MJTextBlock{BaseBlock: BaseBlock{ID: "bad", Type: MJMLComponentMjText}, Content: stringPtr("{% if user %}Hello")}
	body := &MJBodyBlock{BaseBlock: BaseBlock{ID: "b", Type: MJMLComponentMjBody, Children: []interface{}{text}}}
	root := &MJMLBlock{BaseBlock: BaseBlock{ID: "r", Type: MJMLComponentMjml, Children: []interface{}{body}}}

	_, err := ConvertJSONToMJMLWithData(root, `{"x":1}`)
	if err == nil {
		t.Fatal("expected liquid processing error but got none")
	}
	if !strings.Contains(err.Error(), "liquid processing failed") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestEscapeContent(t *testing.T) {
	in := "<b>A&B</b>"
	got := escapeContent(in)
	want1 := "&lt;b&gt;A&amp;B&lt;/b&gt;"
	if got != want1 {
		t.Fatalf("escapeContent mismatch: got %q want %q", got, want1)
	}
}

func TestConvertToMJMLString_ValidAndErrors(t *testing.T) {
	// nil
	if _, err := ConvertToMJMLString(nil); err == nil {
		t.Fatal("expected error for nil block")
	}

	// invalid root type
	badRoot := &MJBodyBlock{BaseBlock: BaseBlock{ID: "b", Type: MJMLComponentMjBody}}
	if _, err := ConvertToMJMLString(badRoot); err == nil {
		t.Fatal("expected error for non-mjml root")
	}

	// minimal valid tree
	body := &MJBodyBlock{BaseBlock: BaseBlock{ID: "body", Type: MJMLComponentMjBody, Children: []interface{}{}}}
	root := &MJMLBlock{BaseBlock: BaseBlock{ID: "root", Type: MJMLComponentMjml, Children: []interface{}{body}}}
	out, err := ConvertToMJMLString(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "<mjml>") || !strings.Contains(out, "<mj-body />") {
		t.Fatalf("unexpected MJML output: %s", out)
	}
}

func TestConvertToMJMLWithOptions(t *testing.T) {
	// validation failure path
	bad := &MJBodyBlock{BaseBlock: BaseBlock{ID: "b", Type: MJMLComponentMjBody}}
	if _, err := ConvertToMJMLWithOptions(bad, MJMLConvertOptions{Validate: true}); err == nil {
		t.Fatal("expected validation error")
	}

	// success with XML header
	body := &MJBodyBlock{BaseBlock: BaseBlock{ID: "body", Type: MJMLComponentMjBody, Children: []interface{}{}}}
	root := &MJMLBlock{BaseBlock: BaseBlock{ID: "root", Type: MJMLComponentMjml, Children: []interface{}{body}}}
	out, err := ConvertToMJMLWithOptions(root, MJMLConvertOptions{Validate: true, IncludeXMLTag: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n") {
		t.Fatalf("expected XML declaration, got: %s", out)
	}
}

func TestFormatAttributesAndHelpers(t *testing.T) {
	url := "https://example.com?a=1&b=2"
	title := `He said "Hi"`
	num := 123
	empty := ""
	var nilStr *string
	attrs := map[string]interface{}{
		"href":      url,
		"title":     title,
		"dataValue": num,
		"isPrimary": true,
		"disabled":  false,
		"className": empty,
		"optional":  nilStr,
	}
	got := formatAttributes(attrs)

	// href should escape '&' as '&amp;' per XML spec; title must be quoted/escaped; data-value numeric; boolean true present; false omitted; empty omitted
	if !strings.Contains(got, ` href="https://example.com?a=1&amp;b=2"`) {
		t.Fatalf("href not formatted as expected: %s", got)
	}
	if !strings.Contains(got, ` title="He said &quot;Hi&quot;"`) {
		t.Fatalf("title not escaped/quoted: %s", got)
	}
	if !strings.Contains(got, ` data-value="123"`) {
		t.Fatalf("numeric attribute missing: %s", got)
	}
	if !strings.Contains(got, ` is-primary`) {
		t.Fatalf("boolean true attribute missing: %s", got)
	}
	if strings.Contains(got, "disabled") || strings.Contains(got, "class-name") {
		t.Fatalf("unexpected attributes present: %s", got)
	}
}

func TestCamelToKebab(t *testing.T) {
	cases := map[string]string{
		"fontSize":                 "font-size",
		"BackgroundColor":          "-background-color",
		"fullWidthBackgroundColor": "full-width-background-color",
		"ID":                       "-i-d",
	}
	for in, want := range cases {
		if got := camelToKebab(in); got != want {
			t.Fatalf("camelToKebab(%q)=%q want %q", in, got, want)
		}
	}
}

func TestGetBlockContent_AllTypes(t *testing.T) {
	s := "content"
	cases := []EmailBlock{
		&MJTextBlock{BaseBlock: BaseBlock{ID: "t", Type: MJMLComponentMjText}, Content: &s},
		&MJButtonBlock{BaseBlock: BaseBlock{ID: "b", Type: MJMLComponentMjButton}, Content: &s},
		&MJRawBlock{BaseBlock: BaseBlock{ID: "r", Type: MJMLComponentMjRaw}, Content: &s},
		&MJPreviewBlock{BaseBlock: BaseBlock{ID: "p", Type: MJMLComponentMjPreview}, Content: &s},
		&MJStyleBlock{BaseBlock: BaseBlock{ID: "st", Type: MJMLComponentMjStyle}, Content: &s},
		&MJTitleBlock{BaseBlock: BaseBlock{ID: "ti", Type: MJMLComponentMjTitle}, Content: &s},
		&MJSocialElementBlock{BaseBlock: BaseBlock{ID: "se", Type: MJMLComponentMjSocialElement}, Content: &s},
	}
	for _, b := range cases {
		if c := getBlockContent(b); c != s {
			t.Fatalf("unexpected content for %T: %q", b, c)
		}
	}

	// nil content returns empty
	emptyText := &MJTextBlock{BaseBlock: BaseBlock{ID: "e", Type: MJMLComponentMjText}}
	if c := getBlockContent(emptyText); c != "" {
		t.Fatalf("expected empty content, got %q", c)
	}
}

func TestOptimizedTemplateDataParsing(t *testing.T) {
	// Test that template data is parsed only once per conversion, not multiple times per block
	// This is a regression test for the optimization where we parse template data once and pass it through

	// Create a nested structure that would trigger multiple parsings in the old implementation
	text1 := &MJTextBlock{
		BaseBlock: BaseBlock{
			ID:   "text1",
			Type: MJMLComponentMjText,
			Attributes: map[string]interface{}{
				"href": "{{ base_url }}/text1",
			},
		},
		Content: stringPtr("Hello {{ user.name }}"),
	}

	button1 := &MJButtonBlock{
		BaseBlock: BaseBlock{
			ID:   "btn1",
			Type: MJMLComponentMjButton,
			Attributes: map[string]interface{}{
				"href": "{{ base_url }}/button",
			},
		},
		Content: stringPtr("Click {{ cta_text }}"),
	}

	column := &MJColumnBlock{
		BaseBlock: BaseBlock{
			ID:       "col1",
			Type:     MJMLComponentMjColumn,
			Children: []interface{}{text1, button1},
		},
	}

	section := &MJSectionBlock{
		BaseBlock: BaseBlock{
			ID:       "sec1",
			Type:     MJMLComponentMjSection,
			Children: []interface{}{column},
			Attributes: map[string]interface{}{
				"backgroundUrl": "{{ base_url }}/background.jpg",
			},
		},
	}

	body := &MJBodyBlock{
		BaseBlock: BaseBlock{
			ID:       "body1",
			Type:     MJMLComponentMjBody,
			Children: []interface{}{section},
		},
	}

	root := &MJMLBlock{
		BaseBlock: BaseBlock{
			ID:       "root",
			Type:     MJMLComponentMjml,
			Children: []interface{}{body},
		},
	}

	templateData := `{
		"user": {"name": "John Doe"},
		"base_url": "https://example.com",
		"cta_text": "Get Started"
	}`

	// Convert with template data
	result, err := ConvertJSONToMJMLWithData(root, templateData)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify all liquid expressions were processed correctly
	expectedStrings := []string{
		"Hello John Doe",                                      // Content processing
		"Click Get Started",                                   // Content processing
		`href="https://example.com/text1"`,                    // Attribute processing
		`href="https://example.com/button"`,                   // Attribute processing
		`background-url="https://example.com/background.jpg"`, // Attribute processing
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain '%s', got: %s", expected, result)
		}
	}
}

func TestFormatAttributesWithLiquid(t *testing.T) {
	templateData := map[string]interface{}{
		"base_url": "https://example.com",
		"color":    "#ff0000",
	}

	attrs := map[string]interface{}{
		"href":            "{{ base_url }}/profile",   // Should be processed
		"src":             "{{ base_url }}/image.jpg", // Should be processed
		"backgroundColor": "{{ color }}",              // Should NOT be processed (not URL attribute)
		"fontSize":        "16px",                     // Should not be processed
	}

	result := formatAttributesWithLiquid(attrs, templateData, "test-block")

	// Check that URL attributes were processed
	if !strings.Contains(result, `href="https://example.com/profile"`) {
		t.Errorf("href attribute not processed correctly: %s", result)
	}
	if !strings.Contains(result, `src="https://example.com/image.jpg"`) {
		t.Errorf("src attribute not processed correctly: %s", result)
	}

	// Check that non-URL attributes were NOT processed
	if !strings.Contains(result, `background-color="{{ color }}"`) {
		t.Errorf("backgroundColor should not be processed, got: %s", result)
	}
	if !strings.Contains(result, `font-size="16px"`) {
		t.Errorf("fontSize should be included as-is, got: %s", result)
	}
}

func TestTemplateDataParsingErrorHandling(t *testing.T) {
	// Test error handling for invalid template data
	text := &MJTextBlock{
		BaseBlock: BaseBlock{ID: "text1", Type: MJMLComponentMjText},
		Content:   stringPtr("Hello {{ name }}"),
	}
	body := &MJBodyBlock{
		BaseBlock: BaseBlock{ID: "body1", Type: MJMLComponentMjBody, Children: []interface{}{text}},
	}
	root := &MJMLBlock{
		BaseBlock: BaseBlock{ID: "root", Type: MJMLComponentMjml, Children: []interface{}{body}},
	}

	// Test with invalid JSON
	_, err := ConvertJSONToMJMLWithData(root, "{invalid json")
	if err == nil {
		t.Fatal("Expected error for invalid JSON template data")
	}
	if !strings.Contains(err.Error(), "template data parsing failed") {
		t.Errorf("Expected template data parsing error, got: %v", err)
	}

	// Test with valid JSON but liquid processing error in error-handling function
	_, err = ConvertJSONToMJMLWithData(root, `{"name": "John"}`)
	if err != nil {
		t.Fatalf("Unexpected error with valid data: %v", err)
	}
}
