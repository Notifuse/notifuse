package liquid

import (
	"context"
	"fmt"
	"time"

	"github.com/osteele/liquid"
)

// Security limits for blog template rendering (matching V8/LiquidJS limits)
const (
	BlogRenderTimeout   = 5 * time.Second
	BlogMaxTemplateSize = 100 * 1024 // 100KB
)

// mapTemplateStore is a simple in-memory TemplateStore for partials
type mapTemplateStore struct {
	templates map[string]string
}

func (m *mapTemplateStore) ReadTemplate(templatename string) ([]byte, error) {
	content, ok := m.templates[templatename]
	if !ok {
		return nil, fmt.Errorf("template not found: %s", templatename)
	}
	return []byte(content), nil
}

// BlogTemplateRenderer renders blog templates using Go Liquid (with render tag support)
type BlogTemplateRenderer struct {
	engine *liquid.Engine
}

// NewBlogTemplateRenderer creates a new Go Liquid renderer for blog templates
func NewBlogTemplateRenderer() *BlogTemplateRenderer {
	engine := liquid.NewEngine()

	// Register any custom filters if needed in the future
	// engine.RegisterFilter("custom_filter", customFilterFunc)

	return &BlogTemplateRenderer{
		engine: engine,
	}
}

// Render renders a blog template with the provided data and partials
func (r *BlogTemplateRenderer) Render(
	template string,
	data map[string]interface{},
	partials map[string]string,
) (string, error) {
	if template == "" {
		return "", fmt.Errorf("template content is empty")
	}

	// Validate template size (security limit)
	if len(template) > BlogMaxTemplateSize {
		return "", fmt.Errorf("template size (%d bytes) exceeds maximum allowed size (%d bytes)", len(template), BlogMaxTemplateSize)
	}

	// Validate partial sizes and register template store
	for name, content := range partials {
		if len(content) > BlogMaxTemplateSize {
			return "", fmt.Errorf("partial '%s' size (%d bytes) exceeds maximum allowed size (%d bytes)", name, len(content), BlogMaxTemplateSize)
		}
	}

	// Create a new engine instance for this render to avoid concurrent modification issues
	// This is necessary because we register a custom TemplateStore per render
	engine := liquid.NewEngine()

	// Register the partials via a custom TemplateStore
	if len(partials) > 0 {
		store := &mapTemplateStore{templates: partials}
		engine.RegisterTemplateStore(store)
	}

	// Create context with timeout for security
	ctx, cancel := context.WithTimeout(context.Background(), BlogRenderTimeout)
	defer cancel()

	// Channel to capture result or error
	type result struct {
		output string
		err    error
	}
	resultChan := make(chan result, 1)

	// Render in a goroutine to enforce timeout
	go func() {
		output, err := engine.ParseAndRenderString(template, data)
		resultChan <- result{output: output, err: err}
	}()

	// Wait for result or timeout
	select {
	case res := <-resultChan:
		if res.err != nil {
			return "", fmt.Errorf("liquid rendering failed: %w", res.err)
		}
		return res.output, nil
	case <-ctx.Done():
		return "", fmt.Errorf("template rendering timeout after %v", BlogRenderTimeout)
	}
}

// RenderBlogTemplateGo renders a Liquid template with the provided data using Go Liquid
// This is the drop-in replacement for RenderBlogTemplate (V8 version)
//
// The partials parameter is optional - pass nil if no partials are needed.
// Partials can be rendered in templates using: {% render 'partial_name' %}
// or with parameters: {% render 'partial_name', param: value %}
func RenderBlogTemplateGo(template string, data map[string]interface{}, partials map[string]string) (string, error) {
	renderer := NewBlogTemplateRenderer()
	return renderer.Render(template, data, partials)
}
