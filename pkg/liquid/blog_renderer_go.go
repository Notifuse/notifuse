package liquid

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/Notifuse/liquidgo/v5/liquid"
	"github.com/Notifuse/liquidgo/v5/liquid/tags"

	"github.com/Notifuse/notifuse/pkg/logger"
)

// Security limits for blog template rendering (matching V8/LiquidJS limits)
const (
	BlogRenderTimeout   = 5 * time.Second
	BlogMaxTemplateSize = 100 * 1024 // 100KB
	// The resource limits liquidgo checks while it renders, partials included (they
	// share the limits of the page that renders them): the output (a page is well
	// under 1MB), the work, which counts each node rendered and each loop iteration
	// (10 million is about 0.2s), and what assign and capture build.
	BlogMaxRenderLength = 10_000_000 // 10MB
	BlogMaxRenderScore  = 10_000_000
	BlogMaxAssignScore  = 10_000_000
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
	length, score, assign := BlogMaxRenderLength, BlogMaxRenderScore, BlogMaxAssignScore
	env, err := liquid.NewEnvironment(liquid.EnvironmentConfig{
		// the standard tags (if, for, assign...), required by every theme
		Tags: tags.Standard(),
		// lax: render errors inline, don't fail
		ErrorMode: "lax",
		Limits: liquid.ResourceLimitsConfig{
			RenderLengthLimit: &length,
			RenderScoreLimit:  &score,
			AssignScoreLimit:  &assign,
		},
		// Custom filters, if ever needed, are declared here:
		// Filters: []liquid.FilterDef{liquid.Filter("name", fn)},
	})
	if err != nil {
		// the config above is fixed: an error is a bug of this code
		panic(fmt.Sprintf("liquid environment: %v", err))
	}

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
		// Add panic recovery to capture actual errors before liquidgo converts them
		defer func() {
			if r := recover(); r != nil {
				resultChan <- result{output: "", err: fmt.Errorf("panic during rendering: %v", r)}
			}
		}()

		// Parse the template with the environment
		tmpl, err := liquid.ParseTemplate(template, &liquid.TemplateOptions{
			Environment: r.env,
		})
		if err != nil {
			resultChan <- result{output: "", err: fmt.Errorf("failed to parse template: %w", err)}
			return
		}

		// The partials of this render, if provided
		var opts []liquid.RenderOption
		if len(partials) > 0 {
			opts = append(opts, liquid.WithFileSystem(&mapFileSystem{templates: partials}))
		}

		// The context stops the render at its next node once the timeout is reached, so a
		// runaway theme does not keep its goroutine running.
		res, err := tmpl.Render(ctx, data, opts...)
		if err != nil {
			var limit *liquid.MemoryError
			var canceled *liquid.CanceledError
			var panicked *liquid.PanicError
			switch {
			case errors.As(err, &limit):
				// a render over its limits is an error, never a page
				err = fmt.Errorf("template rendering exceeded its resource limits (output size, work or assigns)")
			case errors.As(err, &canceled):
				err = fmt.Errorf("template rendering timeout after %v", BlogRenderTimeout)
			case errors.As(err, &panicked):
				// a bug, never the theme's fault: the stack goes to the server log
				logBlogPanicOnce(template, panicked)
				err = fmt.Errorf("internal error")
			}
			resultChan <- result{output: "", err: err}
			return
		}

		// In lax mode (default), errors are rendered inline in the output, not as errors.
		// Only return an error if rendering completely failed (empty output with errors).
		// If there's output, it means rendering succeeded (errors are shown inline).
		if len(res.Errors) > 0 && len(res.Output) == 0 {
			// Complete rendering failure
			resultChan <- result{output: res.Output, err: fmt.Errorf("template rendering error: %w", res.Errors[0])}
			return
		}

		resultChan <- result{output: res.Output, err: nil}
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

// blogPanicLogger logs the Go panics of blog renders; loggedBlogPanics holds the
// templates whose panic it already logged, keyed by a hash of their source.
var (
	blogPanicLogger  = logger.NewLogger()
	loggedBlogPanics sync.Map
)

// logBlogPanicOnce logs a blog render's Go panic with its stack, the first time the
// template panics.
func logBlogPanicOnce(template string, panicked *liquid.PanicError) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(template))
	if _, logged := loggedBlogPanics.LoadOrStore(h.Sum64(), true); logged {
		return
	}
	blogPanicLogger.WithFields(map[string]interface{}{
		"panic":          fmt.Sprint(panicked.Value),
		"stack":          string(panicked.Stack),
		"template_sha64": fmt.Sprintf("%016x", h.Sum64()),
	}).Error("Go panic while rendering a blog template")
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
