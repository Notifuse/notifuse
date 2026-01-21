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
				"https://track.example.com/redirect/visit?mid=test-message&wid=test-workspace&ts=",
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
			name: "Skip mailto and tel links with tracking disabled",
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
			name: "Skip mailto and tel links with tracking ENABLED (issue #163)",
			htmlInput: `<a href="mailto:test@example.com">Email</a>
<a href="tel:+1234567890">Call</a>
<a href="sms:+1234567890">Text</a>
<a href="https://example.com">Normal Link</a>`,
			trackingSettings: TrackingSettings{
				EnableTracking: true,
				Endpoint:       "https://track.example.com",
				UTMSource:      "email",
				WorkspaceID:    "test-workspace",
				MessageID:      "test-message",
			},
			expectedContains: []string{
				`href="mailto:test@example.com"`,  // mailto should be unchanged
				`href="tel:+1234567890"`,          // tel should be unchanged
				`href="sms:+1234567890"`,          // sms should be unchanged
				"track.example.com/visit",         // normal links should be tracked
			},
			expectedNotContains: []string{
				"url=mailto",  // mailto should NOT be in a tracking redirect URL param
				"url=tel",     // tel should NOT be in a tracking redirect URL param
				"url=sms",     // sms should NOT be in a tracking redirect URL param
			},
			shouldError: false,
		},
		{
			name: "Skip anchor links with tracking enabled",
			htmlInput: `<a href="#section1">Jump to section</a>
<a href="https://example.com">Normal Link</a>`,
			trackingSettings: TrackingSettings{
				EnableTracking: true,
				Endpoint:       "https://track.example.com",
				UTMSource:      "email",
				WorkspaceID:    "test-workspace",
				MessageID:      "test-message",
			},
			expectedContains: []string{
				`href="#section1"`,        // anchor should be unchanged
				"track.example.com/visit", // normal links should be tracked
			},
			shouldError: false,
		},
		{
			name: "Skip javascript links with tracking enabled",
			htmlInput: `<a href="javascript:void(0)">No-op Link</a>
<a href="https://example.com">Normal Link</a>`,
			trackingSettings: TrackingSettings{
				EnableTracking: true,
				Endpoint:       "https://track.example.com",
				UTMSource:      "email",
				WorkspaceID:    "test-workspace",
				MessageID:      "test-message",
			},
			expectedContains: []string{
				`href="javascript:void(0)"`, // javascript should be unchanged
				"track.example.com/visit",   // normal links should be tracked
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
				"https://track.example.com/redirect/visit?mid=test-message&wid=test-workspace&ts=",
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
				"https://track.example.com/visit?mid=test-message&wid=test-workspace&ts=",
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
	textBase := NewBaseBlock("text-1", MJMLComponentMjText)
	textBase.Content = stringPtr("Check out our latest offers!")
	textBlock := &MJTextBlock{BaseBlock: textBase}

	buttonBase := NewBaseBlock("button-1", MJMLComponentMjButton)
	buttonBase.Attributes["href"] = "https://shop.example.com/offers"
	buttonBase.Content = stringPtr("Shop Now")
	buttonBlock := &MJButtonBlock{BaseBlock: buttonBase}

	columnBlock := &MJColumnBlock{BaseBlock: NewBaseBlock("column-1", MJMLComponentMjColumn)}
	columnBlock.Children = []EmailBlock{textBlock, buttonBlock}

	sectionBlock := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
	sectionBlock.Children = []EmailBlock{columnBlock}

	bodyBlock := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
	bodyBlock.Children = []EmailBlock{sectionBlock}

	// Create MJML structure
	mjml := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
	mjml.Children = []EmailBlock{bodyBlock}

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
	// Check for the pattern with ts parameter (which is dynamic)
	hasPixelPattern := strings.Contains(result, `opens?mid=test-message&wid=test-workspace&ts=`) &&
		strings.Contains(result, `alt="" width="1" height="1">`)
	if !hasPixelPattern {
		t.Errorf("Expected tracking pixel pattern to be present in the HTML. Result: %s", result)
	}

	// Check that the pixel is placed before the closing body tag
	bodyCloseIndex := strings.Index(result, "</body>")
	pixelMarker := `opens?mid=test-message&wid=test-workspace&ts=`
	pixelIndex := strings.Index(result, pixelMarker)

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
	// Check for the pattern with ts parameter (which is dynamic)
	hasPixelPattern := strings.Contains(result, `opens?mid=test-message&wid=test-workspace&ts=`) &&
		strings.Contains(result, `alt="" width="1" height="1">`)
	if !hasPixelPattern {
		t.Error("Expected tracking pixel pattern to be present in the HTML")
	}

	// Check that the pixel is at the end (check for the closing tag pattern)
	if !strings.HasSuffix(strings.TrimSpace(result), `alt="" width="1" height="1">`) {
		t.Error("Expected tracking pixel to be at the end when no body tag is present")
	}
}

func TestIsNonTrackableURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		// Non-trackable URLs
		{name: "empty string", url: "", expected: true},
		{name: "mailto link", url: "mailto:test@example.com", expected: true},
		{name: "mailto with subject", url: "mailto:test@example.com?subject=Hello", expected: true},
		{name: "MAILTO uppercase", url: "MAILTO:test@example.com", expected: true},
		{name: "tel link", url: "tel:+1234567890", expected: true},
		{name: "TEL uppercase", url: "TEL:+1234567890", expected: true},
		{name: "sms link", url: "sms:+1234567890", expected: true},
		{name: "sms with body", url: "sms:+1234567890?body=Hello", expected: true},
		{name: "javascript void", url: "javascript:void(0)", expected: true},
		{name: "javascript alert", url: "javascript:alert('test')", expected: true},
		{name: "data URL", url: "data:image/png;base64,abc123", expected: true},
		{name: "blob URL", url: "blob:https://example.com/uuid", expected: true},
		{name: "file URL", url: "file:///path/to/file.txt", expected: true},
		{name: "anchor link", url: "#section1", expected: true},
		{name: "anchor with path", url: "#top", expected: true},
		{name: "liquid double brace", url: "https://example.com/{{ user.id }}", expected: true},
		{name: "liquid tag", url: "{% if cond %}https://example.com{% endif %}", expected: true},

		// Trackable URLs
		{name: "http URL", url: "http://example.com", expected: false},
		{name: "https URL", url: "https://example.com", expected: false},
		{name: "https with path", url: "https://example.com/path/to/page", expected: false},
		{name: "https with query", url: "https://example.com?foo=bar", expected: false},
		{name: "relative URL", url: "/path/to/page", expected: false},
		{name: "URL with utm params", url: "https://example.com?utm_source=email", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNonTrackableURL(tt.url)
			if result != tt.expected {
				t.Errorf("isNonTrackableURL(%q) = %v, expected %v", tt.url, result, tt.expected)
			}
		})
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

	buttonBase := NewBaseBlock("confirm-button", MJMLComponentMjButton)
	buttonBase.Attributes["href"] = "{{ confirm_subscription_url }}"
	buttonBase.Attributes["background-color"] = "#007bff"
	buttonBase.Attributes["color"] = "#ffffff"
	buttonBase.Content = stringPtr("Confirm Subscription")
	buttonBlock := &MJButtonBlock{BaseBlock: buttonBase}

	// Create complete MJML structure
	column := &MJColumnBlock{BaseBlock: NewBaseBlock("column-1", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{buttonBlock}

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
	section.Children = []EmailBlock{column}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}

	mjml := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
	mjml.Children = []EmailBlock{body}

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

func TestPreprocessMjmlForXML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "convert br tag to self-closing",
			input:    "<mj-text><p>Line 1<br>Line 2</p></mj-text>",
			expected: "<mj-text><p>Line 1<br/>Line 2</p></mj-text>",
		},
		{
			name:     "convert br tag with space to self-closing",
			input:    "<mj-text><p>Line 1<br >Line 2</p></mj-text>",
			expected: "<mj-text><p>Line 1<br/>Line 2</p></mj-text>",
		},
		{
			name:     "preserve already self-closing br",
			input:    "<mj-text><p>Line 1<br/>Line 2</p></mj-text>",
			expected: "<mj-text><p>Line 1<br/>Line 2</p></mj-text>",
		},
		{
			name:     "preserve already self-closing br with space",
			input:    "<mj-text><p>Line 1<br />Line 2</p></mj-text>",
			expected: "<mj-text><p>Line 1<br />Line 2</p></mj-text>",
		},
		{
			name:     "convert hr tag to self-closing",
			input:    "<mj-raw><hr></mj-raw>",
			expected: "<mj-raw><hr/></mj-raw>",
		},
		{
			name:     "convert multiple void tags",
			input:    "<mj-text><p>A<br>B<br>C<hr>D</p></mj-text>",
			expected: "<mj-text><p>A<br/>B<br/>C<hr/>D</p></mj-text>",
		},
		{
			name:     "convert img tag with attributes to self-closing",
			input:    `<mj-raw><img src="test.jpg" alt="test"></mj-raw>`,
			expected: `<mj-raw><img src="test.jpg" alt="test"/></mj-raw>`,
		},
		{
			name:     "convert nbsp entity to numeric",
			input:    "<mj-text>Hello&nbsp;World</mj-text>",
			expected: "<mj-text>Hello&#160;World</mj-text>",
		},
		{
			name:     "convert multiple nbsp entities",
			input:    "<mj-text>A&nbsp;&nbsp;&nbsp;B</mj-text>",
			expected: "<mj-text>A&#160;&#160;&#160;B</mj-text>",
		},
		{
			name:     "convert copy entity to numeric",
			input:    "<mj-text>&copy; 2024</mj-text>",
			expected: "<mj-text>&#169; 2024</mj-text>",
		},
		{
			name:     "preserve xml predefined entities",
			input:    "<mj-text>&amp; &lt; &gt; &quot; &apos;</mj-text>",
			expected: "<mj-text>&amp; &lt; &gt; &quot; &apos;</mj-text>",
		},
		{
			name:     "preserve numeric entities",
			input:    "<mj-text>&#160;&#169;</mj-text>",
			expected: "<mj-text>&#160;&#169;</mj-text>",
		},
		{
			name:     "combined void tags and entities",
			input:    "<mj-text><p>Hello<br>World&nbsp;&copy;</p></mj-text>",
			expected: "<mj-text><p>Hello<br/>World&#160;&#169;</p></mj-text>",
		},
		{
			name:     "preserve liquid double braces",
			input:    "<mj-text>Hello {{ user.name }}&nbsp;welcome!</mj-text>",
			expected: "<mj-text>Hello {{ user.name }}&#160;welcome!</mj-text>",
		},
		{
			name:     "preserve liquid tags",
			input:    "<mj-text>{% if show %}<br>Show this{% endif %}</mj-text>",
			expected: "<mj-text>{% if show %}<br/>Show this{% endif %}</mj-text>",
		},
		{
			name:     "combined liquid, void tags and entities",
			input:    "<mj-text><p>Dear {{ user.name }},<br>Your order #{{ order.id }}&nbsp;shipped!</p></mj-text>",
			expected: "<mj-text><p>Dear {{ user.name }},<br/>Your order #{{ order.id }}&#160;shipped!</p></mj-text>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := preprocessMjmlForXML(tt.input)
			if result != tt.expected {
				t.Errorf("preprocessMjmlForXML() =\n%s\nwant:\n%s", result, tt.expected)
			}
		})
	}
}

func TestCompileTemplateWithHtmlVoidTags(t *testing.T) {
	// Test that templates with <br> tags compile successfully
	textContent := "<p>Line 1<br>Line 2<br>Line 3</p>"
	textBase := NewBaseBlock("text-1", MJMLComponentMjText)
	textBase.Content = &textContent
	textBlock := &MJTextBlock{BaseBlock: textBase}

	column := &MJColumnBlock{BaseBlock: NewBaseBlock("column-1", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{textBlock}

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
	section.Children = []EmailBlock{column}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}

	mjml := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
	mjml.Children = []EmailBlock{body}

	req := CompileTemplateRequest{
		WorkspaceID:      "test-workspace",
		MessageID:        "test-message",
		VisualEditorTree: mjml,
		TrackingSettings: TrackingSettings{
			EnableTracking: false,
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

	// Verify the HTML contains the content
	if !strings.Contains(*resp.HTML, "Line 1") || !strings.Contains(*resp.HTML, "Line 2") {
		t.Errorf("Expected HTML to contain the text content:\n%s", *resp.HTML)
	}
}

func TestCompileTemplateWithHtmlEntitiesAndLiquid(t *testing.T) {
	// Test that templates with HTML entities AND Liquid markup work correctly
	textContent := "<p>Hello {{ user.name }},<br>Welcome&nbsp;to&nbsp;our&nbsp;service!<br>&copy; 2024</p>"
	textBase := NewBaseBlock("text-1", MJMLComponentMjText)
	textBase.Content = &textContent
	textBlock := &MJTextBlock{BaseBlock: textBase}

	column := &MJColumnBlock{BaseBlock: NewBaseBlock("column-1", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{textBlock}

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
	section.Children = []EmailBlock{column}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}

	mjml := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
	mjml.Children = []EmailBlock{body}

	req := CompileTemplateRequest{
		WorkspaceID:      "test-workspace",
		MessageID:        "test-message",
		VisualEditorTree: mjml,
		TemplateData: MapOfAny{
			"user": map[string]interface{}{
				"name": "John",
			},
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
		t.Fatalf("Expected successful compilation, got error: %v", resp.Error)
	}

	if resp.HTML == nil {
		t.Fatal("Expected HTML in response")
	}

	// Verify the Liquid variable was replaced
	if !strings.Contains(*resp.HTML, "Hello John") {
		t.Errorf("Expected HTML to contain 'Hello John' (Liquid processed), got:\n%s", *resp.HTML)
	}

	// Verify the content is present
	if !strings.Contains(*resp.HTML, "Welcome") {
		t.Errorf("Expected HTML to contain 'Welcome', got:\n%s", *resp.HTML)
	}
}

func TestCompileTemplateWithHtmlEntities(t *testing.T) {
	// Test that templates with &nbsp; and other HTML entities compile successfully
	textContent := "<p>Hello&nbsp;World &copy; 2024</p>"
	textBase := NewBaseBlock("text-1", MJMLComponentMjText)
	textBase.Content = &textContent
	textBlock := &MJTextBlock{BaseBlock: textBase}

	column := &MJColumnBlock{BaseBlock: NewBaseBlock("column-1", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{textBlock}

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
	section.Children = []EmailBlock{column}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}

	mjml := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
	mjml.Children = []EmailBlock{body}

	req := CompileTemplateRequest{
		WorkspaceID:      "test-workspace",
		MessageID:        "test-message",
		VisualEditorTree: mjml,
		TrackingSettings: TrackingSettings{
			EnableTracking: false,
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

	// Verify the HTML contains the content
	if !strings.Contains(*resp.HTML, "Hello") || !strings.Contains(*resp.HTML, "World") {
		t.Errorf("Expected HTML to contain the text content:\n%s", *resp.HTML)
	}
}

func TestCompileTemplateButtonVsTextURL(t *testing.T) {
	// Verify that both button href and text content handle URLs correctly
	confirmURL := "https://example.com/confirm?action=confirm&email=test@example.com&token=abc"

	// Button with URL in href attribute
	buttonBase := NewBaseBlock("button-1", MJMLComponentMjButton)
	buttonBase.Attributes["href"] = "{{ confirm_url }}"
	buttonBase.Content = stringPtr("Confirm via Button")
	buttonBlock := &MJButtonBlock{BaseBlock: buttonBase}

	// Text block with URL in content
	textContent := `<a href="{{ confirm_url }}">Confirm via Text Link</a>`
	textBase := NewBaseBlock("text-1", MJMLComponentMjText)
	textBase.Content = &textContent
	textBlock := &MJTextBlock{BaseBlock: textBase}

	// Create complete structure
	column := &MJColumnBlock{BaseBlock: NewBaseBlock("column-1", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{buttonBlock, textBlock}

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
	section.Children = []EmailBlock{column}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}

	mjml := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
	mjml.Children = []EmailBlock{body}

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

// TestCompileTemplateWithImageLiquidOnlySrc tests that mj-image with only Liquid syntax
// in the src attribute compiles successfully without test data (GitHub issue #226)
func TestCompileTemplateWithImageLiquidOnlySrc(t *testing.T) {
	// Create an mj-image with only Liquid syntax in src attribute
	// This is a common use case for transactional emails where the image URL
	// is dynamically populated from the application
	imageBase := NewBaseBlock("image-1", MJMLComponentMjImage)
	imageBase.Attributes["src"] = "{{ postImage }}"
	imageBase.Attributes["alt"] = "Post Image"
	imageBlock := &MJImageBlock{BaseBlock: imageBase}

	// Create complete MJML structure
	column := &MJColumnBlock{BaseBlock: NewBaseBlock("column-1", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{imageBlock}

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
	section.Children = []EmailBlock{column}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}

	mjml := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
	mjml.Children = []EmailBlock{body}

	// Test 1: Compile WITHOUT test data (simulates export without preview data)
	// This is the bug scenario - it should still compile successfully
	t.Run("without test data", func(t *testing.T) {
		req := CompileTemplateRequest{
			WorkspaceID:      "test-workspace",
			MessageID:        "test-message",
			VisualEditorTree: mjml,
			// No TemplateData - this simulates export without test data
			TrackingSettings: TrackingSettings{
				EnableTracking: false,
			},
		}

		resp, err := CompileTemplate(req)
		if err != nil {
			t.Fatalf("CompileTemplate should not return error: %v", err)
		}

		// Log MJML output for debugging even on failure
		if resp.MJML != nil {
			t.Logf("Generated MJML:\n%s", *resp.MJML)
		}

		if !resp.Success {
			t.Fatalf("Expected successful compilation, got error: %v", resp.Error)
		}

		if resp.MJML == nil {
			t.Fatal("Expected MJML in response")
		}

		if resp.HTML == nil {
			t.Fatal("Expected HTML in response")
		}

		// The Liquid syntax should be preserved in the output
		if !strings.Contains(*resp.MJML, "{{ postImage }}") {
			t.Errorf("Expected MJML to contain preserved Liquid syntax '{{ postImage }}', got:\n%s", *resp.MJML)
		}

		t.Logf("Generated MJML:\n%s", *resp.MJML)
		t.Logf("Generated HTML:\n%s", *resp.HTML)
	})

	// Test 2: Compile WITH test data (simulates preview with actual data)
	// This should work and replace the Liquid variable
	t.Run("with test data", func(t *testing.T) {
		req := CompileTemplateRequest{
			WorkspaceID:      "test-workspace",
			MessageID:        "test-message",
			VisualEditorTree: mjml,
			TemplateData: MapOfAny{
				"postImage": "https://example.com/images/post.jpg",
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
			t.Fatalf("Expected successful compilation, got error: %v", resp.Error)
		}

		if resp.HTML == nil {
			t.Fatal("Expected HTML in response")
		}

		// The Liquid variable should be replaced with the actual URL
		if !strings.Contains(*resp.HTML, "https://example.com/images/post.jpg") {
			t.Errorf("Expected HTML to contain the resolved image URL, got:\n%s", *resp.HTML)
		}

		t.Logf("Generated HTML:\n%s", *resp.HTML)
	})
}

// TestCompileTemplateWithPreserveLiquid tests that the preserve_liquid flag
// correctly skips Liquid template processing and preserves raw syntax (GitHub issue #225)
func TestCompileTemplateWithPreserveLiquid(t *testing.T) {
	// Create a text block with Liquid syntax in a link
	textContent := `<a href="https://example.com?external_id={{contact.external_id}}&amp;unsubscribe=true">Unsubscribe</a>`
	textBlock := &MJTextBlock{
		BaseBlock: NewBaseBlock("text-1", MJMLComponentMjText),
	}
	textBlock.Content = &textContent

	// Create complete MJML structure
	column := &MJColumnBlock{BaseBlock: NewBaseBlock("column-1", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{textBlock}

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
	section.Children = []EmailBlock{column}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}

	mjml := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
	mjml.Children = []EmailBlock{body}

	// Test 1: Compile with empty template data - Liquid syntax should be preserved
	// This is the expected behavior: when no template data is provided, Liquid syntax is preserved
	// to prevent broken URLs like src="" which would fail MJML compilation (issue #225, #226)
	t.Run("with empty template data", func(t *testing.T) {
		req := CompileTemplateRequest{
			WorkspaceID:      "test-workspace",
			MessageID:        "test-message",
			VisualEditorTree: mjml,
			TemplateData:     MapOfAny{}, // Empty test data - Liquid syntax should be preserved
			PreserveLiquid:   false,
			TrackingSettings: TrackingSettings{
				EnableTracking: false,
			},
		}

		resp, err := CompileTemplate(req)
		if err != nil {
			t.Fatalf("CompileTemplate should not return error: %v", err)
		}

		if !resp.Success {
			t.Fatalf("Expected successful compilation, got error: %v", resp.Error)
		}

		// With empty template data, Liquid syntax should be preserved (not rendered to empty)
		if resp.MJML == nil || !strings.Contains(*resp.MJML, "{{contact.external_id}}") {
			t.Errorf("With empty template data, Liquid syntax should be preserved in MJML, got:\n%s", *resp.MJML)
		}

		t.Logf("MJML with empty template data:\n%s", *resp.MJML)
	})

	// Test 2: Compile WITH preserve_liquid - Liquid syntax should be preserved
	t.Run("with preserve_liquid", func(t *testing.T) {
		req := CompileTemplateRequest{
			WorkspaceID:      "test-workspace",
			MessageID:        "test-message",
			VisualEditorTree: mjml,
			TemplateData:     MapOfAny{}, // This should be ignored when PreserveLiquid is true
			PreserveLiquid:   true,       // Preserve Liquid syntax
			TrackingSettings: TrackingSettings{
				EnableTracking: false,
			},
		}

		resp, err := CompileTemplate(req)
		if err != nil {
			t.Fatalf("CompileTemplate should not return error: %v", err)
		}

		if !resp.Success {
			t.Fatalf("Expected successful compilation, got error: %v", resp.Error)
		}

		// With preserve_liquid, the Liquid syntax should be preserved in MJML output
		if resp.MJML == nil {
			t.Fatal("Expected MJML in response")
		}

		if !strings.Contains(*resp.MJML, "{{contact.external_id}}") {
			t.Errorf("With preserve_liquid, Liquid syntax '{{contact.external_id}}' should be preserved in MJML, got:\n%s", *resp.MJML)
		}

		t.Logf("MJML with preserve_liquid:\n%s", *resp.MJML)
	})

	// Test 3: Compile with test data but preserve_liquid=true should still preserve Liquid
	t.Run("with test data and preserve_liquid", func(t *testing.T) {
		req := CompileTemplateRequest{
			WorkspaceID:      "test-workspace",
			MessageID:        "test-message",
			VisualEditorTree: mjml,
			TemplateData: MapOfAny{
				"contact": map[string]interface{}{
					"external_id": "12345",
				},
			},
			PreserveLiquid: true, // Should ignore TemplateData
			TrackingSettings: TrackingSettings{
				EnableTracking: false,
			},
		}

		resp, err := CompileTemplate(req)
		if err != nil {
			t.Fatalf("CompileTemplate should not return error: %v", err)
		}

		if !resp.Success {
			t.Fatalf("Expected successful compilation, got error: %v", resp.Error)
		}

		// With preserve_liquid, the Liquid syntax should be preserved even with test data
		if resp.MJML == nil {
			t.Fatal("Expected MJML in response")
		}

		if !strings.Contains(*resp.MJML, "{{contact.external_id}}") {
			t.Errorf("With preserve_liquid, Liquid syntax should be preserved even with test data, got:\n%s", *resp.MJML)
		}

		// Should NOT contain the rendered value "12345"
		if strings.Contains(*resp.MJML, "12345") {
			t.Errorf("With preserve_liquid, test data should not be rendered, but found '12345' in MJML:\n%s", *resp.MJML)
		}

		t.Logf("MJML with test data and preserve_liquid:\n%s", *resp.MJML)
	})
}

// TestCompileTemplateWithImageLiquidOnlySrcPartialData tests the bug scenario from issue #226
// where an mj-image has only Liquid syntax in src (e.g., "{{ postImage }}") and template data
// exists but doesn't include the referenced variable. This would cause the Liquid engine
// to render the variable as an empty string, resulting in src="" which breaks MJML compilation.
func TestCompileTemplateWithImageLiquidOnlySrcPartialData(t *testing.T) {
	// Create an mj-image with only Liquid syntax in src attribute
	imageBase := NewBaseBlock("image-1", MJMLComponentMjImage)
	imageBase.Attributes["src"] = "{{ postImage }}"
	imageBase.Attributes["alt"] = "Post Image"
	imageBlock := &MJImageBlock{BaseBlock: imageBase}

	// Create complete MJML structure
	column := &MJColumnBlock{BaseBlock: NewBaseBlock("column-1", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{imageBlock}

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
	section.Children = []EmailBlock{column}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}

	mjml := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
	mjml.Children = []EmailBlock{body}

	// BUG SCENARIO: Template data exists but doesn't include "postImage"
	// Without the fix, this would render {{ postImage }} to empty string,
	// resulting in src="" which breaks the MJML compiler or produces invalid HTML.
	t.Run("with partial template data missing image variable", func(t *testing.T) {
		req := CompileTemplateRequest{
			WorkspaceID:      "test-workspace",
			MessageID:        "test-message",
			VisualEditorTree: mjml,
			TemplateData: MapOfAny{
				"other_var": "some_value",
				"contact": map[string]interface{}{
					"email": "test@example.com",
				},
			}, // Has data, but NOT the "postImage" variable
			TrackingSettings: TrackingSettings{
				EnableTracking: false,
			},
		}

		resp, err := CompileTemplate(req)
		if err != nil {
			t.Fatalf("CompileTemplate should not return error: %v", err)
		}

		// Log MJML output for debugging
		if resp.MJML != nil {
			t.Logf("Generated MJML:\n%s", *resp.MJML)
		}

		if !resp.Success {
			t.Fatalf("Expected successful compilation, got error: %v", resp.Error)
		}

		if resp.MJML == nil {
			t.Fatal("Expected MJML in response")
		}

		if resp.HTML == nil {
			t.Fatal("Expected HTML in response")
		}

		// CRITICAL: A helpful debug message should be shown when the variable is not in template data.
		// Without the fix, it would render to src="" which is broken.
		if !strings.Contains(*resp.MJML, "[undefined: postImage]") {
			t.Errorf("Expected MJML to contain debug message '[undefined: postImage]' when variable is not in template data.\n"+
				"This likely means the Liquid engine rendered it to empty string.\n"+
				"Got MJML:\n%s", *resp.MJML)
		}

		// The HTML should also contain the debug message
		if !strings.Contains(*resp.HTML, "[undefined: postImage]") {
			t.Errorf("Expected HTML to contain debug message '[undefined: postImage]' when variable is not in template data.\n"+
				"This likely means the Liquid engine rendered it to empty string.\n"+
				"Got HTML:\n%s", *resp.HTML)
		}

		t.Logf("Generated HTML:\n%s", *resp.HTML)
	})

	// Verify that when the variable IS present, it gets properly replaced
	t.Run("with complete template data including image variable", func(t *testing.T) {
		req := CompileTemplateRequest{
			WorkspaceID:      "test-workspace",
			MessageID:        "test-message",
			VisualEditorTree: mjml,
			TemplateData: MapOfAny{
				"postImage": "https://example.com/images/my-post.jpg",
				"other_var": "some_value",
			}, // Has the "postImage" variable
			TrackingSettings: TrackingSettings{
				EnableTracking: false,
			},
		}

		resp, err := CompileTemplate(req)
		if err != nil {
			t.Fatalf("CompileTemplate failed: %v", err)
		}

		if !resp.Success {
			t.Fatalf("Expected successful compilation, got error: %v", resp.Error)
		}

		// When the variable IS present, it should be replaced
		if !strings.Contains(*resp.HTML, "https://example.com/images/my-post.jpg") {
			t.Errorf("Expected HTML to contain the resolved image URL when variable is present, got:\n%s", *resp.HTML)
		}

		// Should NOT contain the raw Liquid syntax
		if strings.Contains(*resp.HTML, "{{ postImage }}") {
			t.Errorf("Expected HTML to NOT contain raw Liquid syntax when variable is present, got:\n%s", *resp.HTML)
		}

		t.Logf("Generated HTML:\n%s", *resp.HTML)
	})
}
