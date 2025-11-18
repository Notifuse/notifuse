package notifuse_mjml

import (
	"context"
	"fmt"
	"time"

	"github.com/osteele/liquid"
)

// Security limits for Liquid template rendering
const (
	DefaultRenderTimeout   = 5 * time.Second
	DefaultMaxTemplateSize = 100 * 1024       // 100KB
	DefaultMaxMemory       = 10 * 1024 * 1024 // 10MB (informational, not enforced by Go Liquid)
)

// SecureLiquidEngine wraps the Liquid engine with security protections
type SecureLiquidEngine struct {
	timeout time.Duration
	maxSize int
	engine  *liquid.Engine
}

// NewSecureLiquidEngine creates a new secure Liquid engine with default settings
func NewSecureLiquidEngine() *SecureLiquidEngine {
	return &SecureLiquidEngine{
		timeout: DefaultRenderTimeout,
		maxSize: DefaultMaxTemplateSize,
		engine:  liquid.NewEngine(),
	}
}

// NewSecureLiquidEngineWithOptions creates a new secure Liquid engine with custom settings
func NewSecureLiquidEngineWithOptions(timeout time.Duration, maxSize int) *SecureLiquidEngine {
	return &SecureLiquidEngine{
		timeout: timeout,
		maxSize: maxSize,
		engine:  liquid.NewEngine(),
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

		// Render the template
		rendered, err := s.engine.ParseAndRenderString(content, data)
		if err != nil {
			errorChan <- fmt.Errorf("liquid rendering failed: %w", err)
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
