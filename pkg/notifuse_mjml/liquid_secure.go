package notifuse_mjml

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/liquidgo/v5/liquid"
	"github.com/Notifuse/liquidgo/v5/liquid/tags"
)

// Security limits for Liquid template rendering
const (
	DefaultRenderTimeout   = 5 * time.Second
	DefaultMaxTemplateSize = 100 * 1024       // 100KB
	DefaultMaxMemory       = 10 * 1024 * 1024 // 10MB (informational, not enforced by Go Liquid)
	// DefaultMaxRenderLength caps the cumulative OUTPUT of a single Liquid render at
	// 10MB (a real email body is well under 1MB). It bounds a render that expands to an
	// enormous size — an output-producing runaway loop or a hostile feed.
	DefaultMaxRenderLength = 10_000_000 // 10MB
	// DefaultMaxRenderScore caps the work of a render: liquidgo counts each node rendered
	// and each loop iteration, so it bounds a loop that writes nothing. 10 million is
	// about 0.2 s of work, thousands of times a real email.
	DefaultMaxRenderScore = 10_000_000
	// DefaultMaxAssignScore caps what assign and capture build during a render (about
	// their size in bytes), which the output cap does not see.
	DefaultMaxAssignScore = 10_000_000
)

// SecureLiquidEngine wraps the liquidgo engine with security protections
type SecureLiquidEngine struct {
	timeout time.Duration
	maxSize int
	env     *liquid.Environment
	// parsed caches the templates renderCached parses, keyed by their source. It is
	// only touched by the render goroutine of the engine's current user (an engine is
	// never shared, and one that fails is dropped), so it needs no lock.
	parsed map[string]*liquid.Template
}

// maxParsedTemplates bounds an engine's parse cache; past it the cache starts over.
const maxParsedTemplates = 256

// newSecureLiquidEnv builds a liquidgo environment with the standard tags and the
// resource limits (output length, work, assigns), shared by every engine constructor.
func newSecureLiquidEnv(renderLength int) *liquid.Environment {
	env := liquid.NewEnvironment()
	tags.RegisterStandardTags(env)
	env.SetDefaultResourceLimits(map[string]interface{}{
		"render_length_limit": renderLength,
		"render_score_limit":  DefaultMaxRenderScore,
		"assign_score_limit":  DefaultMaxAssignScore,
	})
	return env
}

// newSecureLiquidEngineWithRenderLength builds an engine with a custom render-length
// cap. Used by tests to exercise the cap with a small, fast-to-reach limit; production
// always uses DefaultMaxRenderLength via the exported constructors.
func newSecureLiquidEngineWithRenderLength(renderLength int) *SecureLiquidEngine {
	return &SecureLiquidEngine{timeout: DefaultRenderTimeout, maxSize: DefaultMaxTemplateSize, env: newSecureLiquidEnv(renderLength)}
}

// newSecureHTMLLiquidEngine builds an engine for the HTML of an email: every
// {{ }} output is HTML-escaped (liquidgo's output escape), so a value of the data,
// such as a contact field filled in a public form, cannot add markup or links to
// the email. {{ value | raw }} writes HTML on purpose (a feed that holds HTML),
// and an output escaped by escape or escape_once is not escaped again.
func newSecureHTMLLiquidEngine() *SecureLiquidEngine {
	engine := NewSecureLiquidEngine()
	engine.env.SetOutputEscape(true)
	return engine
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
	return s.render(content, data, false)
}

// renderCached is RenderWithTimeout for a template rendered over and over, such as a
// display condition, which it parses once per engine. Only for a template whose every
// variable is assigned before it is read: liquidgo keeps a template's assigns from one
// render to the next.
func (s *SecureLiquidEngine) renderCached(content string, data map[string]interface{}) (string, error) {
	return s.render(content, data, true)
}

func (s *SecureLiquidEngine) render(content string, data map[string]interface{}, cached bool) (string, error) {
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

		var tmpl *liquid.Template
		if cached {
			tmpl = s.parsed[content]
		}
		if tmpl == nil {
			var err error
			tmpl, err = liquid.ParseTemplate(content, &liquid.TemplateOptions{
				Environment: s.env,
			})
			if err != nil {
				errorChan <- fmt.Errorf("liquid parsing failed: %w", err)
				return
			}
			if cached {
				if s.parsed == nil || len(s.parsed) >= maxParsedTemplates {
					s.parsed = make(map[string]*liquid.Template)
				}
				s.parsed[content] = tmpl
			}
		}

		// The context stops the render at its next node once the timeout is reached,
		// so a runaway template does not keep its goroutine running.
		rendered := tmpl.Render(data, &liquid.RenderOptions{Context: ctx})
		// A render over its limits ends with "Liquid error: Memory limits exceeded" in
		// its output: it is an error, never the text of an email.
		if limits := tmpl.ResourceLimits(); limits != nil && limits.Reached() {
			errorChan <- fmt.Errorf("liquid rendering exceeded its resource limits (output size, work or assigns)")
			return
		}
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
// environment, where include and render refuse to parse. Neither is supported: with no
// file system configured, a partial they name renders as an error in the output. Refused
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
