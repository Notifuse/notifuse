package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTableOfContents(t *testing.T) {
	t.Run("empty HTML returns empty TOC", func(t *testing.T) {
		tocItems, html, err := ExtractTableOfContents("")
		require.NoError(t, err)
		assert.Empty(t, tocItems)
		assert.Empty(t, html)
	})

	t.Run("HTML with no headings returns empty TOC", func(t *testing.T) {
		html := "<p>This is a paragraph with no headings.</p>"
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		assert.Empty(t, tocItems)
		assert.Equal(t, html, modifiedHTML)
	})

	t.Run("extracts single h2 heading", func(t *testing.T) {
		html := "<h2>Introduction</h2><p>Some content</p>"
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 1)
		assert.Equal(t, "introduction", tocItems[0].ID)
		assert.Equal(t, 2, tocItems[0].Level)
		assert.Equal(t, "Introduction", tocItems[0].Text)
		assert.Contains(t, modifiedHTML, `id="introduction"`)
	})

	t.Run("extracts multiple headings of different levels", func(t *testing.T) {
		html := `
			<h2>First Section</h2>
			<p>Content</p>
			<h3>Subsection</h3>
			<p>More content</p>
			<h4>Sub-subsection</h4>
			<p>Even more content</p>
		`
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 3)

		assert.Equal(t, "first-section", tocItems[0].ID)
		assert.Equal(t, 2, tocItems[0].Level)
		assert.Equal(t, "First Section", tocItems[0].Text)

		assert.Equal(t, "subsection", tocItems[1].ID)
		assert.Equal(t, 3, tocItems[1].Level)
		assert.Equal(t, "Subsection", tocItems[1].Text)

		assert.Equal(t, "sub-subsection", tocItems[2].ID)
		assert.Equal(t, 4, tocItems[2].Level)
		assert.Equal(t, "Sub-subsection", tocItems[2].Text)

		assert.Contains(t, modifiedHTML, `id="first-section"`)
		assert.Contains(t, modifiedHTML, `id="subsection"`)
		assert.Contains(t, modifiedHTML, `id="sub-subsection"`)
	})

	t.Run("uses existing ID attributes", func(t *testing.T) {
		html := `<h2 id="custom-id">Custom Heading</h2>`
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 1)
		assert.Equal(t, "custom-id", tocItems[0].ID)
		assert.Equal(t, "Custom Heading", tocItems[0].Text)
		assert.Contains(t, modifiedHTML, `id="custom-id"`)
	})

	t.Run("generates IDs for headings without IDs", func(t *testing.T) {
		html := `<h2>Heading Without ID</h2>`
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 1)
		assert.Equal(t, "heading-without-id", tocItems[0].ID)
		assert.Contains(t, modifiedHTML, `id="heading-without-id"`)
	})

	t.Run("handles headings with special characters", func(t *testing.T) {
		html := `<h2>Hello, World! (2024)</h2>`
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 1)
		assert.Equal(t, "hello-world-2024", tocItems[0].ID)
		assert.Equal(t, "Hello, World! (2024)", tocItems[0].Text)
		assert.Contains(t, modifiedHTML, `id="hello-world-2024"`)
	})

	t.Run("handles headings with numbers", func(t *testing.T) {
		html := `<h2>Chapter 1: The Beginning</h2>`
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 1)
		assert.Equal(t, "chapter-1-the-beginning", tocItems[0].ID)
		assert.Contains(t, modifiedHTML, `id="chapter-1-the-beginning"`)
	})

	t.Run("handles headings with multiple spaces", func(t *testing.T) {
		html := `<h2>Multiple    Spaces   Here</h2>`
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 1)
		assert.Equal(t, "multiple-spaces-here", tocItems[0].ID)
		assert.Contains(t, modifiedHTML, `id="multiple-spaces-here"`)
	})

	t.Run("skips empty headings", func(t *testing.T) {
		html := `<h2></h2><h3>Valid Heading</h3><h4>   </h4>`
		tocItems, _, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 1)
		assert.Equal(t, "valid-heading", tocItems[0].ID)
		assert.Equal(t, 3, tocItems[0].Level)
	})

	t.Run("handles all heading levels h2 through h6", func(t *testing.T) {
		html := `
			<h2>Level 2</h2>
			<h3>Level 3</h3>
			<h4>Level 4</h4>
			<h5>Level 5</h5>
			<h6>Level 6</h6>
		`
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 5)

		for i, expectedLevel := range []int{2, 3, 4, 5, 6} {
			assert.Equal(t, expectedLevel, tocItems[i].Level)
			assert.Contains(t, modifiedHTML, tocItems[i].ID)
		}
	})

	t.Run("ignores h1 headings", func(t *testing.T) {
		html := `<h1>Main Title</h1><h2>Section</h2>`
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 1)
		assert.Equal(t, "section", tocItems[0].ID)
		assert.NotContains(t, modifiedHTML, `id="main-title"`)
	})

	t.Run("handles nested HTML in headings", func(t *testing.T) {
		html := `<h2>Heading with <strong>bold</strong> and <em>italic</em> text</h2>`
		tocItems, _, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 1)
		// Text should include the formatted content
		assert.Contains(t, tocItems[0].Text, "Heading with")
		assert.Contains(t, tocItems[0].Text, "bold")
		assert.Contains(t, tocItems[0].Text, "italic")
		assert.Contains(t, tocItems[0].Text, "text")
	})

	t.Run("handles duplicate heading text with different IDs", func(t *testing.T) {
		html := `
			<h2>Introduction</h2>
			<p>Content</p>
			<h2>Introduction</h2>
			<p>More content</p>
		`
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 2)
		// Both should have "introduction" as base
		assert.Equal(t, "introduction", tocItems[0].ID)
		assert.Equal(t, "introduction", tocItems[1].ID)
		assert.Contains(t, modifiedHTML, tocItems[0].ID)
		assert.Contains(t, modifiedHTML, tocItems[1].ID)
	})

	t.Run("handles headings with only special characters", func(t *testing.T) {
		html := `<h2>!!!</h2>`
		tocItems, _, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 1)
		// Should fallback to heading-0 format
		assert.True(t, strings.HasPrefix(tocItems[0].ID, "heading-"))
		assert.Equal(t, "!!!", tocItems[0].Text)
	})

	t.Run("preserves HTML structure", func(t *testing.T) {
		html := `
			<div>
				<h2>Section One</h2>
				<p>Paragraph one</p>
				<h3>Subsection</h3>
				<p>Paragraph two</p>
			</div>
		`
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 2)
		// Verify HTML structure is preserved
		assert.Contains(t, modifiedHTML, "<div>")
		assert.Contains(t, modifiedHTML, "<p>Paragraph one</p>")
		assert.Contains(t, modifiedHTML, "<p>Paragraph two</p>")
		assert.Contains(t, modifiedHTML, `id="section-one"`)
		assert.Contains(t, modifiedHTML, `id="subsection"`)
	})

	t.Run("handles complex real-world HTML", func(t *testing.T) {
		html := `
			<article>
				<h2>Getting Started</h2>
				<p>Welcome to our guide.</p>
				<h3>Installation</h3>
				<p>Follow these steps:</p>
				<h4>Step 1: Download</h4>
				<p>Download the package.</p>
				<h4>Step 2: Install</h4>
				<p>Run the installer.</p>
				<h3>Configuration</h3>
				<p>Configure your settings.</p>
				<h2>Advanced Topics</h2>
				<p>Learn more advanced concepts.</p>
			</article>
		`
		tocItems, modifiedHTML, err := ExtractTableOfContents(html)
		require.NoError(t, err)
		require.Len(t, tocItems, 6)

		// Verify structure
		assert.Equal(t, "getting-started", tocItems[0].ID)
		assert.Equal(t, 2, tocItems[0].Level)
		assert.Equal(t, "installation", tocItems[1].ID)
		assert.Equal(t, 3, tocItems[1].Level)
		assert.Equal(t, "step-1-download", tocItems[2].ID)
		assert.Equal(t, 4, tocItems[2].Level)
		assert.Equal(t, "step-2-install", tocItems[3].ID)
		assert.Equal(t, 4, tocItems[3].Level)
		assert.Equal(t, "configuration", tocItems[4].ID)
		assert.Equal(t, 3, tocItems[4].Level)
		assert.Equal(t, "advanced-topics", tocItems[5].ID)
		assert.Equal(t, 2, tocItems[5].Level)

		// Verify all IDs are in the modified HTML
		for _, item := range tocItems {
			assert.Contains(t, modifiedHTML, `id="`+item.ID+`"`)
		}
	})
}

func TestGetHeadingLevel(t *testing.T) {
	tests := []struct {
		name     string
		tagName  string
		expected int
	}{
		{"h2", "h2", 2},
		{"h3", "h3", 3},
		{"h4", "h4", 4},
		{"h5", "h5", 5},
		{"h6", "h6", 6},
		{"h1", "h1", 0},
		{"p", "p", 0},
		{"div", "div", 0},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getHeadingLevel(tt.tagName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateAnchorID(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		index    int
		expected string
	}{
		{"simple text", "Hello World", 0, "hello-world"},
		{"with numbers", "Chapter 1", 0, "chapter-1"},
		{"with special chars", "Hello, World!", 0, "hello-world"},
		{"multiple spaces", "Multiple   Spaces", 0, "multiple-spaces"},
		{"with underscores", "Hello_World", 0, "hello-world"},
		{"mixed case", "HelloWorld", 0, "helloworld"},
		{"only special chars", "!!!", 0, "heading-0"},
		{"empty string", "", 0, "heading-0"},
		{"trailing spaces", "  Hello  ", 0, "hello"},
		{"leading spaces", "  World", 0, "world"},
		{"hyphens", "Hello-World", 0, "hello-world"},
		{"multiple hyphens", "Hello---World", 0, "hello-world"},
		{"unicode characters", "Café & Restaurant", 0, "caf-restaurant"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateAnchorID(tt.text, tt.index)
			assert.Equal(t, tt.expected, result)
		})
	}
}
