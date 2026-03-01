// White-box tests: uses package `service` (not `service_test`) to access unexported helpers.
package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeXMLElementContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no special chars", "Hello World", "Hello World"},
		{"ampersand", "A & B", "A &amp; B"},
		{"less than", "A < B", "A &lt; B"},
		{"greater than", "A > B", "A &gt; B"},
		{"all special chars", "A & B <C>", "A &amp; B &lt;C&gt;"},
		{"empty string", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := escapeXMLElementContent(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestOverrideMjmlTag(t *testing.T) {
	tests := []struct {
		name     string
		mjml     string
		tagName  string
		content  string
		assertFn func(t *testing.T, result string)
	}{
		{
			name:    "replace existing mj-title",
			mjml:    "<mjml><mj-head><mj-title>Old Title</mj-title></mj-head><mj-body/></mjml>",
			tagName: "mj-title",
			content: "New Title",
			assertFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "<mj-title>New Title</mj-title>")
				assert.NotContains(t, result, "Old Title")
			},
		},
		{
			name:    "replace existing mj-preview",
			mjml:    "<mjml><mj-head><mj-preview>Old Preview</mj-preview></mj-head><mj-body/></mjml>",
			tagName: "mj-preview",
			content: "New Preview",
			assertFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "<mj-preview>New Preview</mj-preview>")
				assert.NotContains(t, result, "Old Preview")
			},
		},
		{
			name:    "inject when mj-head exists but tag missing",
			mjml:    "<mjml><mj-head><mj-style>.test{}</mj-style></mj-head><mj-body/></mjml>",
			tagName: "mj-title",
			content: "Injected Title",
			assertFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "<mj-title>Injected Title</mj-title>")
				// The tag should be inside mj-head
				headStart := strings.Index(result, "<mj-head>")
				headEnd := strings.Index(result, "</mj-head>")
				titleStart := strings.Index(result, "<mj-title>")
				assert.True(t, titleStart > headStart && titleStart < headEnd,
					"mj-title should be inside mj-head")
			},
		},
		{
			name:    "inject when no mj-head exists",
			mjml:    "<mjml><mj-body><mj-section/></mj-body></mjml>",
			tagName: "mj-title",
			content: "Title Without Head",
			assertFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "<mj-title>Title Without Head</mj-title>")
				assert.Contains(t, result, "<mj-head>")
				assert.Contains(t, result, "</mj-head>")
			},
		},
		{
			name:    "XML escaping in content",
			mjml:    "<mjml><mj-head><mj-title>Old</mj-title></mj-head></mjml>",
			tagName: "mj-title",
			content: "A & B <C>",
			assertFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "A &amp; B &lt;C&gt;")
			},
		},
		{
			name:    "case insensitive replacement",
			mjml:    "<mjml><mj-head><MJ-TITLE>Old</MJ-TITLE></mj-head></mjml>",
			tagName: "mj-title",
			content: "New Title",
			assertFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "New Title")
				assert.NotContains(t, result, "Old")
			},
		},
		{
			name:    "no mjml tag at all returns unchanged",
			mjml:    "<html><body>Hello</body></html>",
			tagName: "mj-title",
			content: "Title",
			assertFn: func(t *testing.T, result string) {
				assert.Equal(t, "<html><body>Hello</body></html>", result)
			},
		},
		{
			name:    "empty content replaces with empty",
			mjml:    "<mjml><mj-head><mj-title>Old</mj-title></mj-head></mjml>",
			tagName: "mj-title",
			content: "",
			assertFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "<mj-title></mj-title>")
			},
		},
		{
			name:    "content with dollar signs is preserved literally",
			mjml:    "<mjml><mj-head><mj-title>Old</mj-title></mj-head></mjml>",
			tagName: "mj-title",
			content: "Price: $100 or ${variable}",
			assertFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "<mj-title>Price: $100 or ${variable}</mj-title>")
			},
		},
		{
			name:    "tag with whitespace inside",
			mjml:    "<mjml><mj-head><mj-title >\n  Old Title\n</mj-title ></mj-head></mjml>",
			tagName: "mj-title",
			content: "New Title",
			assertFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "New Title")
				assert.NotContains(t, result, "Old Title")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := overrideMjmlTag(tc.mjml, tc.tagName, tc.content)
			tc.assertFn(t, result)
		})
	}
}
