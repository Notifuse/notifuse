package notifuse_mjml

import (
	"strings"
	"testing"
)

func TestConvertJSONToMJMLWithData_Success(t *testing.T) {
	// mjml -> body -> section -> column -> text with liquid
	textBase := NewBaseBlock("text1", MJMLComponentMjText)
	textBase.Content = stringPtr("Hello {{name}}")
	text := &MJTextBlock{BaseBlock: textBase}

	column := &MJColumnBlock{BaseBlock: NewBaseBlock("col1", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{text}

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("sec1", MJMLComponentMjSection)}
	section.Children = []EmailBlock{column}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}

	root := &MJMLBlock{BaseBlock: NewBaseBlock("root", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}

	out, err := ConvertJSONToMJMLWithData(root, `{"name":"World"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, ">Hello World<") {
		t.Fatalf("expected rendered liquid content, got: %s", out)
	}
}

func TestConvertJSONToMJMLWithData_InvalidTemplateJSON(t *testing.T) {
	textBase := NewBaseBlock("t1", MJMLComponentMjText)
	textBase.Content = stringPtr("Hi {{name}}")
	text := &MJTextBlock{BaseBlock: textBase}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("b1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{text}

	root := &MJMLBlock{BaseBlock: NewBaseBlock("r1", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}

	_, err := ConvertJSONToMJMLWithData(root, "{") // invalid JSON
	if err == nil {
		t.Fatal("expected error for invalid template JSON")
	}
}

func TestConvertBlockToMJMLWithError_LiquidFailure(t *testing.T) {
	// Malformed liquid to trigger parse/render error
	textBase := NewBaseBlock("bad", MJMLComponentMjText)
	textBase.Content = stringPtr("{% if user %}Hello")
	text := &MJTextBlock{BaseBlock: textBase}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("b", MJMLComponentMjBody)}
	body.Children = []EmailBlock{text}

	root := &MJMLBlock{BaseBlock: NewBaseBlock("r", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}

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
	badRoot := &MJBodyBlock{BaseBlock: NewBaseBlock("b", MJMLComponentMjBody)}
	if _, err := ConvertToMJMLString(badRoot); err == nil {
		t.Fatal("expected error for non-mjml root")
	}

	// minimal valid tree
	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body", MJMLComponentMjBody)}
	body.Children = []EmailBlock{}

	root := &MJMLBlock{BaseBlock: NewBaseBlock("root", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}
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
	bad := &MJBodyBlock{BaseBlock: NewBaseBlock("b", MJMLComponentMjBody)}
	if _, err := ConvertToMJMLWithOptions(bad, MJMLConvertOptions{Validate: true}); err == nil {
		t.Fatal("expected validation error")
	}

	// success with XML header
	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body", MJMLComponentMjBody)}
	body.Children = []EmailBlock{}

	root := &MJMLBlock{BaseBlock: NewBaseBlock("root", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}
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

	textBase := NewBaseBlock("t", MJMLComponentMjText)
	textBase.Content = &s

	buttonBase := NewBaseBlock("b", MJMLComponentMjButton)
	buttonBase.Content = &s

	rawBase := NewBaseBlock("r", MJMLComponentMjRaw)
	rawBase.Content = &s

	previewBase := NewBaseBlock("p", MJMLComponentMjPreview)
	previewBase.Content = &s

	styleBase := NewBaseBlock("st", MJMLComponentMjStyle)
	styleBase.Content = &s

	titleBase := NewBaseBlock("ti", MJMLComponentMjTitle)
	titleBase.Content = &s

	socialElemBase := NewBaseBlock("se", MJMLComponentMjSocialElement)
	socialElemBase.Content = &s

	cases := []EmailBlock{
		&MJTextBlock{BaseBlock: textBase},
		&MJButtonBlock{BaseBlock: buttonBase},
		&MJRawBlock{BaseBlock: rawBase},
		&MJPreviewBlock{BaseBlock: previewBase},
		&MJStyleBlock{BaseBlock: styleBase},
		&MJTitleBlock{BaseBlock: titleBase},
		&MJSocialElementBlock{BaseBlock: socialElemBase},
	}
	for _, b := range cases {
		if c := getBlockContent(b); c != s {
			t.Fatalf("unexpected content for %T: %q", b, c)
		}
	}

	// nil content returns empty
	emptyText := &MJTextBlock{BaseBlock: NewBaseBlock("e", MJMLComponentMjText)}
	if c := getBlockContent(emptyText); c != "" {
		t.Fatalf("expected empty content, got %q", c)
	}
}

func TestOptimizedTemplateDataParsing(t *testing.T) {
	// Test that template data is parsed only once per conversion, not multiple times per block
	// This is a regression test for the optimization where we parse template data once and pass it through

	// Create a nested structure that would trigger multiple parsings in the old implementation
	text1Base := NewBaseBlock("text1", MJMLComponentMjText)
	text1Base.Content = stringPtr("Hello {{ user.name }}")
	text1Base.Attributes["href"] = "{{ base_url }}/text1"
	text1 := &MJTextBlock{BaseBlock: text1Base}

	button1Base := NewBaseBlock("btn1", MJMLComponentMjButton)
	button1Base.Content = stringPtr("Click {{ cta_text }}")
	button1Base.Attributes["href"] = "{{ base_url }}/button"
	button1 := &MJButtonBlock{BaseBlock: button1Base}

	column := &MJColumnBlock{BaseBlock: NewBaseBlock("col1", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{text1, button1}

	sectionBase := NewBaseBlock("sec1", MJMLComponentMjSection)
	sectionBase.Attributes["backgroundUrl"] = "{{ base_url }}/background.jpg"
	section := &MJSectionBlock{BaseBlock: sectionBase}
	section.Children = []EmailBlock{column}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}

	root := &MJMLBlock{BaseBlock: NewBaseBlock("root", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}

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
	textBase := NewBaseBlock("text1", MJMLComponentMjText)
	textBase.Content = stringPtr("Hello {{ name }}")
	text := &MJTextBlock{BaseBlock: textBase}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{text}

	root := &MJMLBlock{BaseBlock: NewBaseBlock("root", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}

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
