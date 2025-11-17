package liquid

import (
	"fmt"

	"github.com/osteele/liquid"
)

// memoryTemplateStore is an in-memory template store for Liquid partials
type memoryTemplateStore struct {
	templates map[string]string
}

// ReadTemplate implements the render.TemplateStore interface
func (ts *memoryTemplateStore) ReadTemplate(templateName string) ([]byte, error) {
	content, ok := ts.templates[templateName]
	if !ok {
		return nil, fmt.Errorf("partial not found: %s", templateName)
	}
	return []byte(content), nil
}

// RenderBlogTemplate renders a Liquid template with the provided data
// This is used for rendering blog theme templates (home, post, category, etc.)
//
// The partials parameter is optional - pass nil if no partials are needed.
// Partials can be rendered in templates using: {% render 'partial_name' %}
func RenderBlogTemplate(template string, data map[string]interface{}, partials map[string]string) (string, error) {
	if template == "" {
		return "", fmt.Errorf("template content is empty")
	}

	// Create Liquid engine with partial support
	engine := liquid.NewEngine()

	// Register partials if provided
	if partials != nil && len(partials) > 0 {
		store := &memoryTemplateStore{
			templates: make(map[string]string),
		}

		// Add each partial to the template store
		for name, content := range partials {
			if content != "" {
				store.templates[name] = content
			}
		}

		// Register the template store with the engine
		engine.RegisterTemplateStore(store)
	}

	// Render the template with the provided data
	rendered, err := engine.ParseAndRenderString(template, data)
	if err != nil {
		return "", fmt.Errorf("liquid rendering failed: %w", err)
	}

	return rendered, nil
}
