package notifuse_mjml

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/liquidgo/liquid"
	"github.com/Notifuse/liquidgo/liquid/tags"
)

// Security limits for Liquid template rendering
const (
	DefaultRenderTimeout   = 5 * time.Second
	DefaultMaxTemplateSize = 100 * 1024       // 100KB
	DefaultMaxMemory       = 10 * 1024 * 1024 // 10MB (informational, not enforced by Go Liquid)
	// DefaultMaxRenderLength caps the cumulative OUTPUT of a single Liquid render at
	// 10MB (a real email body is well under 1MB). It bounds a render that expands to an
	// enormous size — an output-producing runaway loop or a hostile feed — since the
	// timeout below cannot interrupt the render goroutine once it is running. It does
	// NOT bound a loop that emits little or no output (an empty-bodied or {% capture %}
	// loop): liquidgo tracks only output length here, so the 5s timeout remains the only
	// backstop for those.
	DefaultMaxRenderLength = 10_000_000 // 10MB
)

// SecureLiquidEngine wraps the liquidgo engine with security protections
type SecureLiquidEngine struct {
	timeout time.Duration
	maxSize int
	env     *liquid.Environment
}

// newSecureLiquidEnv builds a liquidgo environment with the standard tags and a
// render-length cap, shared by every engine constructor.
func newSecureLiquidEnv(renderLength int) *liquid.Environment {
	env := liquid.NewEnvironment()
	tags.RegisterStandardTags(env)
	env.SetDefaultResourceLimits(map[string]interface{}{"render_length_limit": renderLength})
	return env
}

// newSecureLiquidEngineWithRenderLength builds an engine with a custom render-length
// cap. Used by tests to exercise the cap with a small, fast-to-reach limit; production
// always uses DefaultMaxRenderLength via the exported constructors.
func newSecureLiquidEngineWithRenderLength(renderLength int) *SecureLiquidEngine {
	return &SecureLiquidEngine{timeout: DefaultRenderTimeout, maxSize: DefaultMaxTemplateSize, env: newSecureLiquidEnv(renderLength)}
}

// NewSecureLiquidEngine creates a new secure liquidgo engine with default settings
func NewSecureLiquidEngine() *SecureLiquidEngine {
	env := newSecureLiquidEnv(DefaultMaxRenderLength)

	return &SecureLiquidEngine{
		timeout: DefaultRenderTimeout,
		maxSize: DefaultMaxTemplateSize,
		env:     env,
	}
}

// NewSecureLiquidEngineWithOptions creates a new secure liquidgo engine with custom settings
func NewSecureLiquidEngineWithOptions(timeout time.Duration, maxSize int) *SecureLiquidEngine {
	env := newSecureLiquidEnv(DefaultMaxRenderLength)

	return &SecureLiquidEngine{
		timeout: timeout,
		maxSize: maxSize,
		env:     env,
	}
}

// RenderWithTimeout renders a Liquid template with timeout and size protection
func (s *SecureLiquidEngine) RenderWithTimeout(content string, data map[string]interface{}) (string, error) {
	// Validate template size
	if len(content) > s.maxSize {
		return "", fmt.Errorf("template size (%d bytes) exceeds maximum allowed size (%d bytes)", len(content), s.maxSize)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	// Channel to receive result or error
	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	// Run rendering in goroutine with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errorChan <- fmt.Errorf("panic during liquid rendering: %v", r)
			}
		}()

		// Parse the template
		tmpl, err := liquid.ParseTemplate(content, &liquid.TemplateOptions{
			Environment: s.env,
		})
		if err != nil {
			errorChan <- fmt.Errorf("liquid parsing failed: %w", err)
			return
		}

		// Render the template (second parameter is 'assigns', pass nil)
		rendered := tmpl.Render(data, nil)
		resultChan <- rendered
	}()

	// Wait for result, error, or timeout
	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errorChan:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("liquid rendering timeout after %v (possible infinite loop or excessive computation)", s.timeout)
	}
}

// Render is a convenience method that calls RenderWithTimeout
func (s *SecureLiquidEngine) Render(content string, data map[string]interface{}) (string, error) {
	return s.RenderWithTimeout(content, data)
}

// ValidateLiquidSyntax reports whether content would parse when rendered: it applies the
// cleaning the renderer applies (see cleanLiquidTemplate) and parses with the same
// environment, where include and render refuse to parse. Those two parse at render but
// can only write an error into the output, since no file system is configured; refused
// by the parser itself, they are refused wherever it would run them (any spelling, a
// {% liquid %} line) and nowhere else (raw and comment bodies).
func ValidateLiquidSyntax(content string) (err error) {
	if !strings.Contains(content, "{{") && !strings.Contains(content, "{%") {
		return nil
	}
	content = cleanLiquidTemplate(content)

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("liquid parsing failed: %v", r)
		}
	}()
	if _, parseErr := liquid.ParseTemplate(content, &liquid.TemplateOptions{
		Environment: newValidationLiquidEnv(),
	}); parseErr != nil {
		return parseErr
	}
	return nil
}

// newValidationLiquidEnv is the render environment where the tags that read a partial
// from a file system fail to parse.
func newValidationLiquidEnv() *liquid.Environment {
	env := newSecureLiquidEnv(DefaultMaxRenderLength)
	for _, name := range []string{"include", "render"} {
		env.RegisterTag(name, tags.TagConstructor(func(tagName, _ string, _ liquid.ParseContextInterface) (interface{}, error) {
			return nil, fmt.Errorf("the %s tag is not supported", tagName)
		}))
	}
	return env
}
