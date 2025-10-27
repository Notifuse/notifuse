package notifuse_mjml

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrackLinks(t *testing.T) {
	tests := []struct {
		name                string
		htmlInput           string
		trackingSettings    TrackingSettings
		expectedContains    []string
		expectedNotContains []string
		shouldError         bool
	}{
		{
			name: "Basic HTML anchor tag with UTM parameters",
			htmlInput: `<!DOCTYPE html>
<html>
<body>
	<a href="https://example.com">Click me</a>
</body>
</html>`,
			trackingSettings: TrackingSettings{
				EnableTracking: false,
				UTMSource:      "email",
				UTMMedium:      "newsletter",
				UTMCampaign:    "summer2024",
			},
			expectedContains: []string{
				"utm_source=email",
				"utm_medium=newsletter",
				"utm_campaign=summer2024",
				"https://example.com?",
			},
			shouldError: false,
		},
		{
			name: "Multiple anchor tags with different URLs",
			htmlInput: `<!DOCTYPE html>
<html>
<body>
	<a href="https://example.com/page1">Link 1</a>
	<a href="https://example.com/page2">Link 2</a>
</body>
</html>`,
			trackingSettings: TrackingSettings{
				EnableTracking: true,
				Endpoint:       "https://track.example.com/redirect",
				UTMSource:      "email",
				UTMMedium:      "newsletter",
				WorkspaceID:    "test-workspace",
				MessageID:      "test-message",
			},
			expectedContains: []string{
				"https://track.example.com/redirect/visit?mid=test-message&wid=test-workspace&url=",
			},
			shouldError: false,
		},
		{
			name:      "Anchor tags with existing UTM parameters should not be modified",
			htmlInput: `<a href="https://example.com?utm_source=existing">Link</a>`,
			trackingSettings: TrackingSettings{
				EnableTracking: false,
				UTMSource:      "email",
				UTMMedium:      "newsletter",
			},
			expectedContains: []string{
				"utm_source=existing",
			},
			expectedNotContains: []string{
				"utm_source=email",
				"utm_medium=newsletter",
			},
			shouldError: false,
		},
		{
			name: "Skip mailto and tel links",
			htmlInput: `<a href="mailto:test@example.com">Email</a>
<a href="tel:+1234567890">Call</a>`,
			trackingSettings: TrackingSettings{
				EnableTracking: false,
				UTMSource:      "email",
			},
			expectedContains: []string{
				"mailto:test@example.com",
				"tel:+1234567890",
			},
			expectedNotContains: []string{
				"utm_source=email",
			},
			shouldError: false,
		},
		{
			name: "Skip Liquid template URLs",
			htmlInput: `<a href="https://example.com/{{ user.id }}">Dynamic Link</a>
<a href="{% if user.premium %}https://premium.com{% endif %}">Conditional Link</a>`,
			trackingSettings: TrackingSettings{
				EnableTracking: false,
				UTMSource:      "email",
			},
			expectedContains: []string{
				"{{ user.id }}",
				"{% if user.premium %}",
			},
			expectedNotContains: []string{
				"utm_source=email",
			},
			shouldError: false,
		},
		{
			name:      "No tracking when disabled and no UTM",
			htmlInput: `<a href="https://example.com">Link</a>`,
			trackingSettings: TrackingSettings{
				EnableTracking: false,
			},
			expectedContains: []string{
				"https://example.com",
			},
			expectedNotContains: []string{
				"utm_",
				"track.example.com",
			},
			shouldError: false,
		},
		{
			name:      "Full tracking with endpoint and UTM parameters",
			htmlInput: `<a href="https://example.com/product">Buy Now</a>`,
			trackingSettings: TrackingSettings{
				EnableTracking: true,
				Endpoint:       "https://track.example.com/redirect",
				UTMSource:      "email",
				UTMMedium:      "newsletter",
				UTMCampaign:    "black-friday",
				UTMContent:     "buy-button",
				UTMTerm:        "product-sale",
				WorkspaceID:    "test-workspace",
				MessageID:      "test-message",
			},
			expectedContains: []string{
				"https://track.example.com/redirect/visit?mid=test-message&wid=test-workspace&url=",
			},
			shouldError: false,
		},
		{
			name:      "Handle single quotes in href",
			htmlInput: `<a href='https://example.com/single-quotes'>Link</a>`,
			trackingSettings: TrackingSettings{
				EnableTracking: false,
				UTMSource:      "email",
			},
			expectedContains: []string{
				"utm_source=email",
				"single-quotes",
			},
			shouldError: false,
		},
		{
			name: "Complex HTML with nested elements",
			htmlInput: `<table>
<tr>
	<td>
		<a href="https://example.com" class="button" style="color: blue;">
			<span>Click Here</span>
		</a>
	</td>
</tr>
</table>`,
			trackingSettings: TrackingSettings{
				EnableTracking: true,
				Endpoint:       "https://track.example.com",
				UTMSource:      "email",
				WorkspaceID:    "test-workspace",
				MessageID:      "test-message",
			},
			expectedContains: []string{
				"https://track.example.com/visit?mid=test-message&wid=test-workspace&url=",
				"class=\"button\"",
				"<span>Click Here</span>",
			},
			shouldError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := TrackLinks(test.htmlInput, test.trackingSettings)

			if test.shouldError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !test.shouldError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check expected contains
			for _, expected := range test.expectedContains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected result to contain %q, but it didn't. Result: %s", expected, result)
				}
			}

			// Check expected not contains
			for _, notExpected := range test.expectedNotContains {
				if strings.Contains(result, notExpected) {
					t.Errorf("Expected result NOT to contain %q, but it did. Result: %s", notExpected, result)
				}
			}
		})
	}
}

func TestTrackLinksInvalidHTML(t *testing.T) {
	// Test with malformed HTML - should still work with regex approach
	invalidHTML := `<a href="https://example.com">Link without closing tag`
	trackingSettings := TrackingSettings{
		EnableTracking: true,
		Endpoint:       "https://track.example.com",
		UTMSource:      "email",
		WorkspaceID:    "test-workspace",
		MessageID:      "test-message",
	}

	result, err := TrackLinks(invalidHTML, trackingSettings)
	if err != nil {
		t.Errorf("TrackLinks should handle malformed HTML gracefully, got error: %v", err)
	}

	// Should still process the href attribute
	if !strings.Contains(result, "track.example.com") {
		t.Error("Expected tracking URL to be added even with malformed HTML")
	}
}

func TestGetTrackingURL(t *testing.T) {
	trackingSettings := TrackingSettings{
		EnableTracking: true,
		Endpoint:       "https://track.example.com/redirect",
		UTMSource:      "email",
		UTMMedium:      "newsletter",
		UTMCampaign:    "test-campaign",
		WorkspaceID:    "test-workspace",
		MessageID:      "test-message",
	}

	tests := []struct {
		name     string
		inputURL string
		expected string
	}{
		{
			name:     "Basic URL with UTM parameters",
			inputURL: "https://example.com",
			expected: "https://track.example.com/redirect?url=https%3A%2F%2Fexample.com%3Futm_campaign%3Dtest-campaign%26utm_medium%3Dnewsletter%26utm_source%3Demail",
		},
		{
			name:     "URL with existing UTM parameters",
			inputURL: "https://example.com?utm_source=existing",
			expected: "https://track.example.com/redirect?url=https%3A%2F%2Fexample.com%3Futm_source%3Dexisting",
		},
		{
			name:     "Mailto URL should not be modified",
			inputURL: "mailto:test@example.com",
			expected: "mailto:test@example.com",
		},
		{
			name:     "Tel URL should not be modified",
			inputURL: "tel:+1234567890",
			expected: "tel:+1234567890",
		},
		{
			name:     "Liquid template URL should not be modified",
			inputURL: "https://example.com/{{ user.id }}",
			expected: "https://example.com/{{ user.id }}",
		},
		{
			name:     "Empty URL should not be modified",
			inputURL: "",
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := trackingSettings.GetTrackingURL(test.inputURL)
			if result != test.expected {
				t.Errorf("Expected %q, got %q", test.expected, result)
			}
		})
	}
}

func TestCompileTemplateWithTracking(t *testing.T) {
	// Create a simple email with button
	textBlock := &MJTextBlock{
		BaseBlock: BaseBlock{
			ID:   "text-1",
			Type: MJMLComponentMjText,
		},
		Content: stringPtr("Check out our latest offers!"),
	}

	buttonBlock := &MJButtonBlock{
		BaseBlock: BaseBlock{
			ID:   "button-1",
			Type: MJMLComponentMjButton,
			Attributes: map[string]interface{}{
				"href": "https://shop.example.com/offers",
			},
		},
		Content: stringPtr("Shop Now"),
	}

	// Create MJML structure
	mjml := &MJMLBlock{
		BaseBlock: BaseBlock{
			ID:   "mjml-1",
			Type: MJMLComponentMjml,
			Children: []interface{}{
				&MJBodyBlock{
					BaseBlock: BaseBlock{
						ID:   "body-1",
						Type: MJMLComponentMjBody,
						Children: []interface{}{
							&MJSectionBlock{
								BaseBlock: BaseBlock{
									ID:   "section-1",
									Type: MJMLComponentMjSection,
									Children: []interface{}{
										&MJColumnBlock{
											BaseBlock: BaseBlock{
												ID:       "column-1",
												Type:     MJMLComponentMjColumn,
												Children: []interface{}{textBlock, buttonBlock},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Test CompileTemplate with tracking
	req := CompileTemplateRequest{
		WorkspaceID:      "test-workspace",
		MessageID:        "test-message",
		VisualEditorTree: mjml,
		TrackingSettings: TrackingSettings{
			EnableTracking: true,
			Endpoint:       "https://track.example.com/redirect",
			UTMSource:      "email",
			UTMMedium:      "newsletter",
			WorkspaceID:    "test-workspace",
			MessageID:      "test-message",
		},
	}

	resp, err := CompileTemplate(req)
	if err != nil {
		t.Fatalf("CompileTemplate failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected successful compilation")
	}

	if resp.MJML == nil {
		t.Error("Expected MJML in response")
	}

	if resp.HTML == nil {
		t.Error("Expected HTML in response")
	}

	// Check that HTML contains tracking (now HTML-based tracking)
	if !strings.Contains(*resp.HTML, "track.example.com") {
		t.Error("Expected HTML to contain tracking URL")
	}

	t.Logf("Generated MJML:\n%s", *resp.MJML)
	t.Logf("Generated HTML with tracking length: %d bytes", len(*resp.HTML))
}

func TestCompileTemplateRequest_UnmarshalJSON(t *testing.T) {
	// Test JSON that should unmarshal correctly
	jsonData := `{
		"workspace_id": "test-workspace", 
		"message_id": "test-message",
		"visual_editor_tree": {
			"id": "mjml-1",
			"type": "mjml",
			"children": [
				{
					"id": "body-1",
					"type": "mj-body",
					"children": [
						{
							"id": "section-1",
							"type": "mj-section",
							"children": [
								{
									"id": "column-1",
									"type": "mj-column",
									"children": [
										{
											"id": "text-1",
											"type": "mj-text",
											"content": "Hello World"
										}
									]
								}
							]
						}
					]
				}
			]
		},
		"test_data": {"name": "John"},
		"tracking_settings": {
			"enable_tracking": true,
			"utm_source": "email"
		}
	}`

	var req CompileTemplateRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	if err != nil {
		t.Fatalf("Failed to unmarshal CompileTemplateRequest: %v", err)
	}

	// Verify that the fields were unmarshaled correctly
	if req.WorkspaceID != "test-workspace" {
		t.Errorf("Expected WorkspaceID to be 'test-workspace', got %s", req.WorkspaceID)
	}
	if req.MessageID != "test-message" {
		t.Errorf("Expected MessageID to be 'test-message', got %s", req.MessageID)
	}
	if req.VisualEditorTree == nil {
		t.Error("Expected VisualEditorTree to be set")
	} else {
		if req.VisualEditorTree.GetType() != MJMLComponentMjml {
			t.Errorf("Expected VisualEditorTree type to be 'mjml', got %s", req.VisualEditorTree.GetType())
		}
		if req.VisualEditorTree.GetID() != "mjml-1" {
			t.Errorf("Expected VisualEditorTree ID to be 'mjml-1', got %s", req.VisualEditorTree.GetID())
		}
	}
	if req.TemplateData["name"] != "John" {
		t.Errorf("Expected TemplateData name to be 'John', got %v", req.TemplateData["name"])
	}
	if !req.TrackingSettings.EnableTracking {
		t.Error("Expected EnableTracking to be true")
	}
	if req.TrackingSettings.UTMSource != "email" {
		t.Errorf("Expected UTMSource to be 'email', got %s", req.TrackingSettings.UTMSource)
	}
}

func TestTrackingPixelPlacement(t *testing.T) {
	htmlString := `<!DOCTYPE html>
<html>
<head>
    <title>Test Email</title>
</head>
<body>
    <h1>Hello World</h1>
    <p>This is a test email.</p>
</body>
</html>`

	trackingSettings := TrackingSettings{
		EnableTracking: true,
		Endpoint:       "https://track.example.com",
		WorkspaceID:    "test-workspace",
		MessageID:      "test-message",
	}

	result, err := TrackLinks(htmlString, trackingSettings)
	if err != nil {
		t.Fatalf("TrackLinks failed: %v", err)
	}

	// Check that the tracking pixel is inserted before the closing body tag
	expectedPixel := `<img src="https://track.example.com/opens?mid=test-message&wid=test-workspace" alt="" width="1" height="1">`
	if !strings.Contains(result, expectedPixel) {
		t.Errorf("Expected tracking pixel to be present in the HTML. Result: %s", result)
	}

	// Check that the pixel is placed before the closing body tag
	bodyCloseIndex := strings.Index(result, "</body>")
	pixelIndex := strings.Index(result, expectedPixel)

	if bodyCloseIndex == -1 {
		t.Error("Expected closing body tag to be present")
	}

	if pixelIndex == -1 {
		t.Error("Expected tracking pixel to be present")
	}

	if pixelIndex >= bodyCloseIndex {
		t.Error("Expected tracking pixel to be placed before the closing body tag")
	}
}

func TestTrackingPixelWithoutBodyTag(t *testing.T) {
	// Test fallback behavior when there's no body tag
	htmlString := `<h1>Hello World</h1><p>This is a test without body tag.</p>`

	trackingSettings := TrackingSettings{
		EnableTracking: true,
		Endpoint:       "https://track.example.com",
		WorkspaceID:    "test-workspace",
		MessageID:      "test-message",
	}

	result, err := TrackLinks(htmlString, trackingSettings)
	if err != nil {
		t.Fatalf("TrackLinks failed: %v", err)
	}

	// Check that the tracking pixel is appended to the end as fallback
	expectedPixel := `<img src="https://track.example.com/opens?mid=test-message&wid=test-workspace" alt="" width="1" height="1">`
	if !strings.Contains(result, expectedPixel) {
		t.Error("Expected tracking pixel to be present in the HTML")
	}

	// Check that the pixel is at the end
	if !strings.HasSuffix(result, expectedPixel) {
		t.Error("Expected tracking pixel to be at the end when no body tag is present")
	}
}

func TestDecodeHTMLEntitiesInURLAttributes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "href with query parameters containing &amp;",
			input:    `<a href="https://example.com/confirm?action=confirm&amp;email=test@example.com&amp;token=abc123">Link</a>`,
			expected: `<a href="https://example.com/confirm?action=confirm&email=test@example.com&token=abc123">Link</a>`,
		},
		{
			name:     "button href with multiple &amp; entities",
			input:    `<a href="https://mailing.example.com/notification-center?action=confirm&amp;email=mymail%40gmail.com&amp;email_hmac=fd6&amp;lid=mylist&amp;lname=MyList&amp;mid=fb9&amp;wid=myworkspace">Confirm</a>`,
			expected: `<a href="https://mailing.example.com/notification-center?action=confirm&email=mymail%40gmail.com&email_hmac=fd6&lid=mylist&lname=MyList&mid=fb9&wid=myworkspace">Confirm</a>`,
		},
		{
			name:     "src attribute with &amp;",
			input:    `<img src="https://example.com/image.png?w=100&amp;h=200" alt="test">`,
			expected: `<img src="https://example.com/image.png?w=100&h=200" alt="test">`,
		},
		{
			name:     "action attribute with &amp;",
			input:    `<form action="https://example.com/submit?id=1&amp;type=2">`,
			expected: `<form action="https://example.com/submit?id=1&type=2">`,
		},
		{
			name:     "multiple attributes in same tag",
			input:    `<a href="https://example.com?a=1&amp;b=2" class="btn" id="link">Text</a>`,
			expected: `<a href="https://example.com?a=1&b=2" class="btn" id="link">Text</a>`,
		},
		{
			name:     "href with other HTML entities",
			input:    `<a href="https://example.com?name=&quot;John&quot;&amp;age=30">Link</a>`,
			expected: `<a href="https://example.com?name="John"&age=30">Link</a>`,
		},
		{
			name:     "no entities to decode",
			input:    `<a href="https://example.com/simple">Link</a>`,
			expected: `<a href="https://example.com/simple">Link</a>`,
		},
		{
			name:     "single quotes in attribute",
			input:    `<a href='https://example.com?a=1&amp;b=2'>Link</a>`,
			expected: `<a href='https://example.com?a=1&b=2'>Link</a>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decodeHTMLEntitiesInURLAttributes(tt.input)
			if result != tt.expected {
				t.Errorf("Expected:\n%s\nGot:\n%s", tt.expected, result)
			}
		})
	}
}

func TestCompileTemplateWithButtonQueryParameters(t *testing.T) {
	// Test the complete flow: button with confirm_subscription_url containing query parameters
	
	// Create a button with a URL containing query parameters
	confirmURL := "https://mailing.example.com/notification-center?action=confirm&email=test@example.com&email_hmac=abc123&lid=newsletter&lname=Newsletter&mid=msg123&wid=workspace123"
	
	buttonBlock := &MJButtonBlock{
		BaseBlock: BaseBlock{
			ID:   "confirm-button",
			Type: MJMLComponentMjButton,
			Attributes: map[string]interface{}{
				"href":             "{{ confirm_subscription_url }}",
				"background-color": "#007bff",
				"color":            "#ffffff",
			},
		},
		Content: stringPtr("Confirm Subscription"),
	}

	// Create complete MJML structure
	column := &MJColumnBlock{
		BaseBlock: BaseBlock{
			ID:       "column-1",
			Type:     MJMLComponentMjColumn,
			Children: []interface{}{buttonBlock},
		},
	}

	section := &MJSectionBlock{
		BaseBlock: BaseBlock{
			ID:       "section-1",
			Type:     MJMLComponentMjSection,
			Children: []interface{}{column},
		},
	}

	body := &MJBodyBlock{
		BaseBlock: BaseBlock{
			ID:       "body-1",
			Type:     MJMLComponentMjBody,
			Children: []interface{}{section},
		},
	}

	mjml := &MJMLBlock{
		BaseBlock: BaseBlock{
			ID:       "mjml-1",
			Type:     MJMLComponentMjml,
			Children: []interface{}{body},
		},
	}

	// Compile template with template data
	req := CompileTemplateRequest{
		WorkspaceID:      "test-workspace",
		MessageID:        "test-message",
		VisualEditorTree: mjml,
		TemplateData: MapOfAny{
			"confirm_subscription_url": confirmURL,
		},
		TrackingSettings: TrackingSettings{
			EnableTracking: false, // Disable tracking to test just the entity decoding
		},
	}

	resp, err := CompileTemplate(req)
	if err != nil {
		t.Fatalf("CompileTemplate failed: %v", err)
	}

	if !resp.Success {
		t.Fatalf("Expected successful compilation, got error: %v", resp.Error)
	}

	if resp.HTML == nil {
		t.Fatal("Expected HTML in response")
	}

	// Verify that the HTML contains the decoded URL (with & not &amp;)
	if !strings.Contains(*resp.HTML, "action=confirm&email=test@example.com") {
		t.Errorf("Expected HTML to contain decoded query parameters with '&', but got:\n%s", *resp.HTML)
	}

	// Verify that &amp; is NOT in the href attribute
	if strings.Contains(*resp.HTML, "href=\"https://mailing.example.com/notification-center?action=confirm&amp;email") {
		t.Errorf("HTML still contains &amp; in href attribute, entity decoding failed:\n%s", *resp.HTML)
	}

	t.Logf("Generated HTML (excerpt):\n%s", *resp.HTML)
}

func TestCompileTemplateButtonVsTextURL(t *testing.T) {
	// Verify that both button href and text content handle URLs correctly
	confirmURL := "https://example.com/confirm?action=confirm&email=test@example.com&token=abc"

	// Button with URL in href attribute
	buttonBlock := &MJButtonBlock{
		BaseBlock: BaseBlock{
			ID:   "button-1",
			Type: MJMLComponentMjButton,
			Attributes: map[string]interface{}{
				"href": "{{ confirm_url }}",
			},
		},
		Content: stringPtr("Confirm via Button"),
	}

	// Text block with URL in content
	textContent := `<a href="{{ confirm_url }}">Confirm via Text Link</a>`
	textBlock := &MJTextBlock{
		BaseBlock: BaseBlock{
			ID:   "text-1",
			Type: MJMLComponentMjText,
		},
		Content: &textContent,
	}

	// Create complete structure
	column := &MJColumnBlock{
		BaseBlock: BaseBlock{
			ID:       "column-1",
			Type:     MJMLComponentMjColumn,
			Children: []interface{}{buttonBlock, textBlock},
		},
	}

	section := &MJSectionBlock{
		BaseBlock: BaseBlock{
			ID:       "section-1",
			Type:     MJMLComponentMjSection,
			Children: []interface{}{column},
		},
	}

	body := &MJBodyBlock{
		BaseBlock: BaseBlock{
			ID:       "body-1",
			Type:     MJMLComponentMjBody,
			Children: []interface{}{section},
		},
	}

	mjml := &MJMLBlock{
		BaseBlock: BaseBlock{
			ID:       "mjml-1",
			Type:     MJMLComponentMjml,
			Children: []interface{}{body},
		},
	}

	req := CompileTemplateRequest{
		WorkspaceID:      "test-workspace",
		MessageID:        "test-message",
		VisualEditorTree: mjml,
		TemplateData: MapOfAny{
			"confirm_url": confirmURL,
		},
		TrackingSettings: TrackingSettings{
			EnableTracking: false,
		},
	}

	resp, err := CompileTemplate(req)
	if err != nil {
		t.Fatalf("CompileTemplate failed: %v", err)
	}

	if !resp.Success {
		t.Fatalf("Expected successful compilation")
	}

	// Both should have properly decoded URLs with & not &amp;
	expectedURLPart := "action=confirm&email=test@example.com"
	occurrences := strings.Count(*resp.HTML, expectedURLPart)
	
	if occurrences < 2 {
		t.Errorf("Expected at least 2 occurrences of properly decoded URL (button + text), got %d\nHTML:\n%s", 
			occurrences, *resp.HTML)
	}
}
