package liquid

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/liquidgo/liquid"
	"github.com/Notifuse/liquidgo/liquid/tags"
)

// Security limits for blog template rendering (matching V8/LiquidJS limits)
const (
	BlogRenderTimeout   = 5 * time.Second
	BlogMaxTemplateSize = 100 * 1024 // 100KB
)

// mapFileSystem is a simple in-memory FileSystem for partials
type mapFileSystem struct {
	templates map[string]string
}

func (m *mapFileSystem) ReadTemplateFile(path string) (string, error) {
	// Try exact match first
	if content, ok := m.templates[path]; ok {
		return content, nil
	}

	// Try without .liquid extension (e.g., "header" for "header.liquid")
	pathWithoutExt := path
	if len(path) > 7 && path[len(path)-7:] == ".liquid" {
		pathWithoutExt = path[:len(path)-7]
		if content, ok := m.templates[pathWithoutExt]; ok {
			return content, nil
		}
	}

	// Try with .liquid extension (e.g., "header.liquid" for "header")
	pathWithExt := path + ".liquid"
	if content, ok := m.templates[pathWithExt]; ok {
		return content, nil
	}

	return "", fmt.Errorf("template not found: %s (tried: %s, %s, %s)", path, path, pathWithoutExt, pathWithExt)
}

// BlogTemplateRenderer renders blog templates using liquidgo (with render tag support)
type BlogTemplateRenderer struct {
	env *liquid.Environment
}

// NewBlogTemplateRenderer creates a new liquidgo renderer for blog templates
func NewBlogTemplateRenderer() *BlogTemplateRenderer {
	env := liquid.NewEnvironment()

	// Register standard tags (CRITICAL - required for if, for, assign, etc.)
	tags.RegisterStandardTags(env)

	// Register any custom filters if needed in the future
	// env.RegisterFilter(&MyCustomFilters{})

	return &BlogTemplateRenderer{
		env: env,
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

	// Validate partial sizes
	for name, content := range partials {
		if len(content) > BlogMaxTemplateSize {
			return "", fmt.Errorf("partial '%s' size (%d bytes) exceeds maximum allowed size (%d bytes)", name, len(content), BlogMaxTemplateSize)
		}
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
		// Parse the template with the environment
		tmpl, err := liquid.ParseTemplate(template, &liquid.TemplateOptions{
			Environment: r.env,
		})
		if err != nil {
			resultChan <- result{output: "", err: fmt.Errorf("failed to parse template: %w", err)}
			return
		}

		// Register the file system for partials if provided
		// Must be done after parsing but before rendering
		if len(partials) > 0 {
			fs := &mapFileSystem{templates: partials}
			tmpl.Registers()["file_system"] = fs
		}

		// Render the template (first parameter is 'assigns' - the template data)
		output := tmpl.Render(data, nil)
		resultChan <- result{output: output, err: nil}
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

// RenderBlogTemplateGo renders a Liquid template with the provided data using liquidgo
// This is the drop-in replacement for RenderBlogTemplate (V8 version)
//
// The partials parameter is optional - pass nil if no partials are needed.
// Partials can be rendered in templates using: {% render 'partial_name' %}
// or with parameters: {% render 'partial_name', param: value %}
func RenderBlogTemplateGo(template string, data map[string]interface{}, partials map[string]string) (string, error) {
	renderer := NewBlogTemplateRenderer()
	return renderer.Render(template, data, partials)
}
