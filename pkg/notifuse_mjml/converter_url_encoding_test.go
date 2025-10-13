package notifuse_mjml

import (
	"strings"
	"testing"
)

func TestEscapeUnescapedAmpersands(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Basic URL with unescaped ampersands",
			input:    "https://example.com?a=1&b=2&c=3",
			expected: "https://example.com?a=1&amp;b=2&amp;c=3",
		},
		{
			name:     "URL with already escaped ampersands",
			input:    "https://example.com?a=1&amp;b=2&amp;c=3",
			expected: "https://example.com?a=1&amp;b=2&amp;c=3",
		},
		{
			name:     "Mixed escaped and unescaped ampersands",
			input:    "https://example.com?safe=1&amp;unsafe=2&bad=3",
			expected: "https://example.com?safe=1&amp;unsafe=2&amp;bad=3",
		},
		{
			name:     "Other XML entities should be preserved",
			input:    "Test &lt;tag&gt; &quot;quote&quot; &apos;apos&apos;",
			expected: "Test &lt;tag&gt; &quot;quote&quot; &apos;apos&apos;",
		},
		{
			name:     "Numeric entities should be preserved",
			input:    "Copyright &#169; &#xA9;",
			expected: "Copyright &#169; &#xA9;",
		},
		{
			name:     "Plain ampersand in text",
			input:    "Tom & Jerry",
			expected: "Tom &amp; Jerry",
		},
		{
			name:     "Multiple unescaped ampersands in text",
			input:    "A & B & C",
			expected: "A &amp; B &amp; C",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "No ampersands",
			input:    "https://example.com/page",
			expected: "https://example.com/page",
		},
		{
			name:     "Complex URL with multiple parameters",
			input:    "https://example.com/page?utm_source=email&utm_medium=newsletter&utm_campaign=test&id=123",
			expected: "https://example.com/page?utm_source=email&amp;utm_medium=newsletter&amp;utm_campaign=test&amp;id=123",
		},
		{
			name:     "URL with fragment and query params",
			input:    "https://example.com/page?a=1&b=2#section&anchor",
			expected: "https://example.com/page?a=1&amp;b=2#section&amp;anchor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeUnescapedAmpersands(tt.input)
			if result != tt.expected {
				t.Errorf("escapeUnescapedAmpersands() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestEscapeAttributeValue(t *testing.T) {
	tests := []struct {
		name          string
		value         string
		attributeName string
		expected      string
	}{
		{
			name:          "URL with unescaped ampersands",
			value:         "https://example.com?a=1&b=2",
			attributeName: "href",
			expected:      "https://example.com?a=1&amp;b=2",
		},
		{
			name:          "URL with already escaped ampersands",
			value:         "https://example.com?a=1&amp;b=2",
			attributeName: "href",
			expected:      "https://example.com?a=1&amp;b=2",
		},
		{
			name:          "Text with quotes",
			value:         `He said "Hello"`,
			attributeName: "title",
			expected:      "He said &quot;Hello&quot;",
		},
		{
			name:          "Text with single quotes",
			value:         "It's working",
			attributeName: "title",
			expected:      "It&#39;s working",
		},
		{
			name:          "Text with angle brackets",
			value:         "A < B > C",
			attributeName: "title",
			expected:      "A &lt; B &gt; C",
		},
		{
			name:          "Complex value with multiple special characters",
			value:         `"Tom & Jerry" <says> 'Hi'`,
			attributeName: "title",
			expected:      "&quot;Tom &amp; Jerry&quot; &lt;says&gt; &#39;Hi&#39;",
		},
		{
			name:          "URL with mixed entities and special chars",
			value:         `https://example.com?x=1&y=2&name="test"&ref=<page>`,
			attributeName: "href",
			expected:      "https://example.com?x=1&amp;y=2&amp;name=&quot;test&quot;&amp;ref=&lt;page&gt;",
		},
		{
			name:          "Already escaped entities should not be double-escaped",
			value:         "https://example.com?a=1&amp;b=2&lt;test&gt;",
			attributeName: "href",
			expected:      "https://example.com?a=1&amp;b=2&lt;test&gt;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeAttributeValue(tt.value, tt.attributeName)
			if result != tt.expected {
				t.Errorf("escapeAttributeValue() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMJMLButtonURLEncoding(t *testing.T) {
	tests := []struct {
		name        string
		href        string
		expectInMJML string
	}{
		{
			name:         "Button with unescaped ampersands",
			href:         "https://example.com?foo=bar&baz=qux",
			expectInMJML: `href="https://example.com?foo=bar&amp;baz=qux"`,
		},
		{
			name:         "Button with already escaped ampersands",
			href:         "https://example.com?foo=bar&amp;baz=qux",
			expectInMJML: `href="https://example.com?foo=bar&amp;baz=qux"`,
		},
		{
			name:         "Button with mixed escaped and unescaped",
			href:         "https://example.com?a=1&amp;b=2&c=3",
			expectInMJML: `href="https://example.com?a=1&amp;b=2&amp;c=3"`,
		},
		{
			name:         "Button with UTM parameters",
			href:         "https://shop.example.com?utm_source=email&utm_medium=newsletter&utm_campaign=spring",
			expectInMJML: `href="https://shop.example.com?utm_source=email&amp;utm_medium=newsletter&amp;utm_campaign=spring"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "Click Me"
			buttonBlock := &MJButtonBlock{
				BaseBlock: BaseBlock{
					ID:   "button-1",
					Type: MJMLComponentMjButton,
					Attributes: map[string]interface{}{
						"href": tt.href,
					},
				},
				Content: &content,
			}

			columnBlock := &MJColumnBlock{
				BaseBlock: BaseBlock{
					ID:       "column-1",
					Type:     MJMLComponentMjColumn,
					Children: []interface{}{buttonBlock},
				},
			}

			sectionBlock := &MJSectionBlock{
				BaseBlock: BaseBlock{
					ID:       "section-1",
					Type:     MJMLComponentMjSection,
					Children: []interface{}{columnBlock},
				},
			}

			bodyBlock := &MJBodyBlock{
				BaseBlock: BaseBlock{
					ID:       "body-1",
					Type:     MJMLComponentMjBody,
					Children: []interface{}{sectionBlock},
				},
			}

			mjmlBlock := &MJMLBlock{
				BaseBlock: BaseBlock{
					ID:       "mjml-1",
					Type:     MJMLComponentMjml,
					Children: []interface{}{bodyBlock},
				},
			}

			mjmlString := ConvertJSONToMJML(mjmlBlock)

			if !strings.Contains(mjmlString, tt.expectInMJML) {
				t.Errorf("Expected MJML to contain %q, but it didn't.\nGenerated MJML:\n%s", tt.expectInMJML, mjmlString)
			}

			// Verify no double-escaping (should not contain &amp;amp;)
			if strings.Contains(mjmlString, "&amp;amp;") {
				t.Errorf("MJML contains double-escaped ampersands (&amp;amp;), which is incorrect.\nGenerated MJML:\n%s", mjmlString)
			}
		})
	}
}
