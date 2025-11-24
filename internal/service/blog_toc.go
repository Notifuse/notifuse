package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/PuerkitoBio/goquery"
)

// ExtractTableOfContents extracts headings from HTML and generates a table of contents
// It also ensures all headings have ID attributes for anchor linking
func ExtractTableOfContents(html string) ([]domain.TOCItem, string, error) {
	if html == "" {
		return []domain.TOCItem{}, html, nil
	}

	// Parse HTML using goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, html, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var tocItems []domain.TOCItem
	headingIndex := 0

	// Extract headings h2 through h6 and ensure they have IDs
	doc.Find("h2, h3, h4, h5, h6").Each(func(i int, s *goquery.Selection) {
		// Get the tag name to determine level
		tagName := goquery.NodeName(s)
		level := getHeadingLevel(tagName)
		if level == 0 {
			return // Skip if not a valid heading level
		}

		// Get text content
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return // Skip empty headings
		}

		// Check if heading already has an ID attribute
		var id string
		if existingID, exists := s.Attr("id"); exists && existingID != "" {
			id = existingID
		} else {
			// Generate anchor ID from text
			id = generateAnchorID(text, headingIndex)
			// Set the ID attribute on the heading
			s.SetAttr("id", id)
		}

		tocItems = append(tocItems, domain.TOCItem{
			ID:    id,
			Level: level,
			Text:  text,
		})

		headingIndex++
	})

	// Get the modified HTML
	modifiedHTML, err := doc.Html()
	if err != nil {
		return tocItems, html, fmt.Errorf("failed to get modified HTML: %w", err)
	}

	// Remove the html/body wrapper that goquery adds
	modifiedHTML = strings.TrimPrefix(modifiedHTML, "<html><head></head><body>")
	modifiedHTML = strings.TrimSuffix(modifiedHTML, "</body></html>")

	return tocItems, modifiedHTML, nil
}

// getHeadingLevel converts a heading tag name to its numeric level
func getHeadingLevel(tagName string) int {
	switch tagName {
	case "h2":
		return 2
	case "h3":
		return 3
	case "h4":
		return 4
	case "h5":
		return 5
	case "h6":
		return 6
	default:
		return 0
	}
}

// generateAnchorID creates a URL-friendly anchor ID from heading text
func generateAnchorID(text string, index int) string {
	// Convert to lowercase
	id := strings.ToLower(text)

	// Replace spaces and underscores with hyphens
	id = regexp.MustCompile(`[\s_]+`).ReplaceAllString(id, "-")

	// Remove any characters that aren't lowercase letters, numbers, or hyphens
	id = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(id, "")

	// Replace multiple consecutive hyphens with a single hyphen
	id = regexp.MustCompile(`-+`).ReplaceAllString(id, "-")

	// Remove leading and trailing hyphens
	id = strings.Trim(id, "-")

	// If empty after processing, use a fallback
	if id == "" {
		id = fmt.Sprintf("heading-%d", index)
	}

	return id
}
