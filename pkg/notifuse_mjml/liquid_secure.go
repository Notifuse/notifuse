package notifuse_mjml

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	"github.com/Notifuse/liquidgo/v5/liquid"
	"github.com/Notifuse/liquidgo/v5/liquid/tags"

	"github.com/Notifuse/notifuse/pkg/logger"
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
	// mu guards parsed: an engine is shared by every goroutine that renders.
	mu sync.Mutex
	// parsed caches the templates renderCached parses, keyed by their source. A
	// parsed template is never written by a render (liquid.Template.Render),
	// so one entry serves concurrent renders.
	parsed map[string]*liquid.Template
}

// maxParsedTemplates bounds an engine's parse cache; past it the cache starts over.
// One shared engine serves every workspace, hence room for many conditions.
const maxParsedTemplates = 1024

// secureLiquidConfig is the config of a liquidgo environment with the standard tags
// and the resource limits (output length, work, assigns), shared by every engine
// constructor.
func secureLiquidConfig(renderLength int) liquid.EnvironmentConfig {
	score, assign := DefaultMaxRenderScore, DefaultMaxAssignScore
	return liquid.EnvironmentConfig{
		Tags: tags.Standard(),
		Limits: liquid.ResourceLimitsConfig{
			RenderLengthLimit: &renderLength,
			RenderScoreLimit:  &score,
			AssignScoreLimit:  &assign,
		},
	}
}

// newLiquidEnv builds the liquidgo environment of config, which this package writes:
// an error is a bug of the config, not of a template.
func newLiquidEnv(config liquid.EnvironmentConfig) *liquid.Environment {
	env, err := liquid.NewEnvironment(config)
	if err != nil {
		panic(fmt.Sprintf("liquid environment: %v", err))
	}
	return env
}

// newSecureLiquidEnv builds the environment of secureLiquidConfig.
func newSecureLiquidEnv(renderLength int) *liquid.Environment {
	return newLiquidEnv(secureLiquidConfig(renderLength))
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
	config := secureLiquidConfig(DefaultMaxRenderLength)
	config.OutputEscape = true
	engine := NewSecureLiquidEngine()
	engine.env = newLiquidEnv(config)
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
// display condition, which it parses once per engine. Each render starts from its own
// data: liquid.Template.Render keeps nothing of a render in the template.
func (s *SecureLiquidEngine) renderCached(content string, data map[string]interface{}) (string, error) {
	return s.render(content, data, true)
}

func (s *SecureLiquidEngine) render(content string, data map[string]interface{}, cached bool) (string, error) {
	return s.renderIn(nil, content, data, cached)
}

// renderIn renders content within budget, the work left to its compilation, or with the
// engine's own limits alone when budget is nil. The render takes the lower of its own timeout
// and the time left, and of its own render score limit and the score left, and its score is
// taken off the budget whatever its outcome.
func (s *SecureLiquidEngine) renderIn(budget *renderBudget, content string, data map[string]interface{}, cached bool) (string, error) {
	// Validate template size
	if len(content) > s.maxSize {
		return "", fmt.Errorf("template size (%d bytes) exceeds maximum allowed size (%d bytes)", len(content), s.maxSize)
	}

	timeout := s.timeout
	opts := []liquid.RenderOption{liquid.RaiseErrors()}
	// The budget, rather than the render's own limit, sets its timeout or its score limit.
	budgetTime, budgetScore := false, 0
	if budget != nil {
		left, score, ok := budget.remaining()
		if !ok {
			return "", errBudgetExhausted
		}
		if left < timeout {
			timeout, budgetTime = left, true
		}
		if score < DefaultMaxRenderScore {
			budgetScore = score
			opts = append(opts, liquid.WithLimits(liquid.ResourceLimitsConfig{RenderScoreLimit: &budgetScore}))
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// The outcome of the render goroutine: its output, or its error, and its score.
	type outcome struct {
		output string
		score  int
		err    error // a render error, as Render returned it
		failed error // a parse error or a recovered panic, already described
	}
	done := make(chan outcome, 1)

	// Run rendering in goroutine with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- outcome{failed: fmt.Errorf("panic during liquid rendering: %v", r)}
			}
		}()

		var tmpl *liquid.Template
		if cached {
			tmpl = s.cachedTemplate(content)
		}
		if tmpl == nil {
			var err error
			tmpl, err = liquid.ParseTemplate(content, &liquid.TemplateOptions{
				Environment: s.env,
			})
			if err != nil {
				done <- outcome{failed: fmt.Errorf("liquid parsing failed: %w", err)}
				return
			}
			if cached {
				s.cacheTemplate(content, tmpl)
			}
		}

		// The context stops the render at its next node once the timeout is reached,
		// so a runaway template does not keep its goroutine running.
		// A Liquid error the render records (a filter given the wrong arguments, a
		// partial that cannot be read...) would write "Liquid error: ..." in the email:
		// it fails the render instead, and its message tells the author what to fix.
		res, err := tmpl.Render(ctx, data, opts...)
		done <- outcome{output: res.Output, score: res.RenderScore, err: err}
	}()

	// Wait for result, error, or timeout
	select {
	case o := <-done:
		if budget != nil {
			budget.spend(o.score)
		}
		switch {
		case o.failed != nil:
			return "", o.failed
		case o.err != nil:
			if budget != nil && budgetStopped(o.err, budgetTime, budgetScore, o.score) {
				return "", errBudgetExhausted
			}
			return "", renderError(o.err, content, s.timeout)
		}
		return o.output, nil
	case <-ctx.Done():
		if budget != nil {
			budget.spend(budget.score) // the score of a render cut by its timeout is not known
		}
		if budgetTime {
			return "", errBudgetExhausted
		}
		return "", fmt.Errorf("liquid rendering timeout after %v (possible infinite loop or excessive computation)", s.timeout)
	}
}

// budgetStopped reports whether the compilation's budget, rather than the render's own limits,
// ended a render that failed with err after spending score: its time, when the budget set the
// timeout, or its render score, when the budget set a lower score limit and the render reached it.
func budgetStopped(err error, budgetTime bool, budgetScore, score int) bool {
	var canceled *liquid.CanceledError
	if budgetTime && errors.As(err, &canceled) {
		return true
	}
	var limit *liquid.MemoryError
	return budgetScore > 0 && errors.As(err, &limit) && score >= budgetScore
}

// renderError is the error of a render of content that could not complete. A Go
// panic during the render (a bug, never the template's fault) is logged with its
// stack once per template, and reported to the caller without it.
func renderError(err error, content string, timeout time.Duration) error {
	var limit *liquid.MemoryError
	var canceled *liquid.CanceledError
	var panicked *liquid.PanicError
	switch {
	case errors.As(err, &limit):
		// it is an error, never the text of an email
		return fmt.Errorf("liquid rendering exceeded its resource limits (output size, work or assigns)")
	case errors.As(err, &canceled):
		return fmt.Errorf("liquid rendering timeout after %v (possible infinite loop or excessive computation)", timeout)
	case errors.As(err, &panicked):
		logPanicOnce(content, panicked)
		return fmt.Errorf("liquid rendering failed: internal error")
	}
	return fmt.Errorf("liquid rendering failed: %w", err)
}

// panicLogger logs the Go panics of renders; loggedPanics holds the templates whose
// panic it already logged, keyed by a hash of their source.
var (
	panicLogger  = logger.NewLogger()
	loggedPanics sync.Map
)

// logPanicOnce logs a render's Go panic with its stack, the first time content panics.
func logPanicOnce(content string, panicked *liquid.PanicError) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(content))
	if _, logged := loggedPanics.LoadOrStore(h.Sum64(), true); logged {
		return
	}
	panicLogger.WithFields(map[string]interface{}{
		"panic":          fmt.Sprint(panicked.Value),
		"stack":          string(panicked.Stack),
		"template_sha64": fmt.Sprintf("%016x", h.Sum64()),
	}).Error("Go panic while rendering a Liquid template")
}

// cachedTemplate returns the template renderCached parsed for content, if any.
func (s *SecureLiquidEngine) cachedTemplate(content string) *liquid.Template {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.parsed[content]
}

// cacheTemplate keeps tmpl for content; past maxParsedTemplates the cache starts over.
func (s *SecureLiquidEngine) cacheTemplate(content string, tmpl *liquid.Template) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parsed == nil || len(s.parsed) >= maxParsedTemplates {
		s.parsed = make(map[string]*liquid.Template)
	}
	s.parsed[content] = tmpl
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
	config := secureLiquidConfig(DefaultMaxRenderLength)
	for _, name := range []string{"include", "render"} {
		config.Tags[name] = tags.TagConstructor(func(tagName, _ string, _ liquid.ParseContextInterface) (liquid.Node, error) {
			return nil, fmt.Errorf("the %s tag is not supported", tagName)
		})
	}
	return newLiquidEnv(config)
}
