package notifuse_mjml

import (
	"strings"
	"testing"
	"time"
)

func TestSecureLiquidEngine_TimeoutEnforcement(t *testing.T) {
	t.Run("infinite loop causes timeout", func(t *testing.T) {
		engine := NewSecureLiquidEngine()

		// Template with massive nested loops that should timeout
		template := `
		{% assign limit = 1000000 %}
		{% for i in (1..limit) %}
			{% for j in (1..limit) %}
				<div>{{ i }} - {{ j }}</div>
			{% endfor %}
		{% endfor %}
		`
		data := map[string]interface{}{}

		start := time.Now()
		_, err := engine.RenderWithTimeout(template, data)
		elapsed := time.Since(start)

		// Should timeout
		if err == nil {
			t.Fatal("Expected timeout error, got nil")
		}

		// Should contain timeout message
		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("Expected timeout error, got: %v", err)
		}

		// Should timeout within reasonable time (5 seconds + 1 second tolerance)
		if elapsed > 6*time.Second {
			t.Errorf("Timeout took too long: %v", elapsed)
		}
	})

	t.Run("fast template completes before timeout", func(t *testing.T) {
		engine := NewSecureLiquidEngine()

		template := `<div>{{ message }}</div>`
		data := map[string]interface{}{
			"message": "Hello World",
		}

		result, err := engine.RenderWithTimeout(template, data)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "<div>Hello World</div>" {
			t.Errorf("Unexpected result: %s", result)
		}
	})
}

func TestSecureLiquidEngine_TemplateSizeLimit(t *testing.T) {
	t.Run("rejects templates exceeding size limit", func(t *testing.T) {
		engine := NewSecureLiquidEngine()

		// Create a template larger than 100KB
		largeTemplate := strings.Repeat("<div>{{ item }}</div>\n", 10000) // ~200KB
		data := map[string]interface{}{
			"item": "test",
		}

		_, err := engine.RenderWithTimeout(largeTemplate, data)
		if err == nil {
			t.Fatal("Expected size limit error, got nil")
		}

		if !strings.Contains(err.Error(), "exceeds maximum allowed size") {
			t.Errorf("Expected size limit error, got: %v", err)
		}
	})

	t.Run("accepts templates within size limit", func(t *testing.T) {
		engine := NewSecureLiquidEngine()

		// Template under 100KB (about 2KB)
		template := strings.Repeat("<div>{{ item }}</div>\n", 100)
		data := map[string]interface{}{
			"item": "test",
		}

		result, err := engine.RenderWithTimeout(template, data)
		if err != nil {
			t.Fatalf("Expected no error for small template, got: %v", err)
		}

		if !strings.Contains(result, "<div>test</div>") {
			t.Error("Template did not render correctly")
		}
	})

	t.Run("custom size limit works", func(t *testing.T) {
		// Create engine with 1KB limit
		engine := NewSecureLiquidEngineWithOptions(5*time.Second, 1024)

		// Template larger than 1KB but smaller than 100KB
		template := strings.Repeat("<div>test</div>", 100) // ~1.5KB
		data := map[string]interface{}{}

		_, err := engine.RenderWithTimeout(template, data)
		if err == nil {
			t.Fatal("Expected size limit error with custom limit")
		}
	})
}

func TestSecureLiquidEngine_NormalTemplatesWork(t *testing.T) {
	engine := NewSecureLiquidEngine()

	testCases := []struct {
		name     string
		template string
		data     map[string]interface{}
		expected string
	}{
		{
			name:     "simple variable",
			template: "<p>{{ name }}</p>",
			data:     map[string]interface{}{"name": "John"},
			expected: "<p>John</p>",
		},
		{
			name:     "simple loop",
			template: `<ul>{% for item in items %}<li>{{ item }}</li>{% endfor %}</ul>`,
			data:     map[string]interface{}{"items": []string{"one", "two", "three"}},
			expected: "<ul><li>one</li><li>two</li><li>three</li></ul>",
		},
		{
			name:     "conditional",
			template: `{% if show %}<p>Visible</p>{% endif %}`,
			data:     map[string]interface{}{"show": true},
			expected: "<p>Visible</p>",
		},
		{
			name:     "filters",
			template: `{{ text | upcase }}`,
			data:     map[string]interface{}{"text": "hello"},
			expected: "HELLO",
		},
		{
			name:     "nested data",
			template: `<h1>{{ user.name }}</h1><p>{{ user.email }}</p>`,
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name":  "Jane",
					"email": "jane@example.com",
				},
			},
			expected: "<h1>Jane</h1><p>jane@example.com</p>",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := engine.Render(tc.template, tc.data)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestSecureLiquidEngine_PanicRecovery(t *testing.T) {
	t.Run("recovers from panics gracefully", func(t *testing.T) {
		engine := NewSecureLiquidEngine()

		// This might cause issues depending on liquid implementation
		// but we should recover from any panic
		template := `{{ nil | upcase }}`
		data := map[string]interface{}{}

		result, err := engine.RenderWithTimeout(template, data)

		// Either returns error or empty result, but should not panic
		if err != nil && !strings.Contains(err.Error(), "panic") && !strings.Contains(err.Error(), "rendering failed") {
			// It's ok if it's a rendering error
		}

		// The important thing is we didn't panic - test passes if we get here
		_ = result
	})
}

func TestSecureLiquidEngine_DeepNesting(t *testing.T) {
	t.Run("handles deep nesting", func(t *testing.T) {
		engine := NewSecureLiquidEngine()

		// Test deep nesting (5 levels)
		template := `
		{% if level1 %}
			{% if level2 %}
				{% if level3 %}
					{% if level4 %}
						{% if level5 %}
							<div>Deep content</div>
						{% endif %}
					{% endif %}
				{% endif %}
			{% endif %}
		{% endif %}
		`
		data := map[string]interface{}{
			"level1": true,
			"level2": true,
			"level3": true,
			"level4": true,
			"level5": true,
		}

		result, err := engine.RenderWithTimeout(template, data)
		if err != nil {
			t.Fatalf("Expected no error for deep nesting, got: %v", err)
		}

		if !strings.Contains(result, "Deep content") {
			t.Error("Deep nesting did not render correctly")
		}
	})
}

func TestSecureLiquidEngine_MemoryExhaustion(t *testing.T) {
	t.Run("large iteration with timeout", func(t *testing.T) {
		engine := NewSecureLiquidEngine()

		// Template that tries to create large strings
		template := `
		{% assign huge = "" %}
		{% for i in (1..10000) %}
			{% assign huge = huge | append: "XXXXXXXXXX" %}
		{% endfor %}
		{{ huge }}
		`
		data := map[string]interface{}{}

		// This should either timeout or complete
		_, err := engine.RenderWithTimeout(template, data)

		// If it times out, that's good (protection working)
		// If it completes, that's also ok (Go Liquid handled it)
		// The important thing is we don't crash
		if err != nil && !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "rendering failed") {
			t.Errorf("Unexpected error type: %v", err)
		}
	})
}

func TestSecureLiquidEngine_ErrorMessages(t *testing.T) {
	t.Run("clear error for invalid syntax", func(t *testing.T) {
		engine := NewSecureLiquidEngine()

		// Invalid liquid syntax
		template := `{% for item in items %}<li>{{ item }}</li>` // Missing endfor
		data := map[string]interface{}{"items": []string{"test"}}

		_, err := engine.RenderWithTimeout(template, data)
		if err == nil {
			t.Fatal("Expected error for invalid syntax")
		}

		if !strings.Contains(err.Error(), "rendering failed") {
			t.Errorf("Expected rendering error, got: %v", err)
		}
	})

	t.Run("clear error for timeout", func(t *testing.T) {
		// Create engine with very short timeout
		engine := NewSecureLiquidEngineWithOptions(10*time.Millisecond, 100*1024)

		// Template with loop that will take longer than 10ms
		template := `{% for i in (1..100000) %}<div>{{ i }}</div>{% endfor %}`
		data := map[string]interface{}{}

		_, err := engine.RenderWithTimeout(template, data)
		if err == nil {
			t.Fatal("Expected timeout error")
		}

		if !strings.Contains(err.Error(), "timeout") || !strings.Contains(err.Error(), "infinite loop") {
			t.Errorf("Expected clear timeout message, got: %v", err)
		}
	})
}

func TestSecureLiquidEngine_EdgeCases(t *testing.T) {
	engine := NewSecureLiquidEngine()

	t.Run("empty template", func(t *testing.T) {
		result, err := engine.Render("", map[string]interface{}{})
		if err != nil {
			t.Fatalf("Expected no error for empty template, got: %v", err)
		}
		if result != "" {
			t.Errorf("Expected empty result, got: %q", result)
		}
	})

	t.Run("nil data", func(t *testing.T) {
		result, err := engine.Render("<p>Static content</p>", nil)
		if err != nil {
			t.Fatalf("Expected no error for nil data, got: %v", err)
		}
		if result != "<p>Static content</p>" {
			t.Errorf("Expected static content, got: %q", result)
		}
	})

	t.Run("empty data map", func(t *testing.T) {
		result, err := engine.Render("<p>{{ missing }}</p>", map[string]interface{}{})
		if err != nil {
			t.Fatalf("Expected no error for empty data, got: %v", err)
		}
		// Liquid renders undefined variables as empty string
		if result != "<p></p>" {
			t.Errorf("Expected empty variable, got: %q", result)
		}
	})

	t.Run("template without liquid markup", func(t *testing.T) {
		result, err := engine.Render("<p>No liquid here</p>", map[string]interface{}{})
		if err != nil {
			t.Fatalf("Expected no error for static template, got: %v", err)
		}
		if result != "<p>No liquid here</p>" {
			t.Errorf("Unexpected result: %q", result)
		}
	})
}
