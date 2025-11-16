package liquid

import (
	"fmt"

	"github.com/osteele/liquid"
)

// RenderBlogTemplate renders a Liquid template with the provided data
// This is used for rendering blog theme templates (home, post, category, etc.)
func RenderBlogTemplate(template string, data map[string]interface{}) (string, error) {
	if template == "" {
		return "", fmt.Errorf("template content is empty")
	}

	// Create Liquid engine
	engine := liquid.NewEngine()

	// Render the template with the provided data
	rendered, err := engine.ParseAndRenderString(template, data)
	if err != nil {
		return "", fmt.Errorf("liquid rendering failed: %w", err)
	}

	return rendered, nil
}
