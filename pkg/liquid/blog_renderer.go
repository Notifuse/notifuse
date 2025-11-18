package liquid

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RenderBlogTemplate renders a Liquid template with the provided data using V8 + liquidjs
// This is used for rendering blog theme templates (home, post, category, etc.)
//
// The partials parameter is optional - pass nil if no partials are needed.
// Partials can be rendered in templates using: {% render 'partial_name' %}
// or with parameters: {% render 'partial_name', param: value %}
func RenderBlogTemplate(template string, data map[string]interface{}, partials map[string]string) (string, error) {
	if template == "" {
		return "", fmt.Errorf("template content is empty")
	}

	// Get V8 context from pool
	pool := GetPool()
	ctx := pool.Acquire()
	if ctx == nil {
		return "", fmt.Errorf("failed to acquire V8 context from pool")
	}
	defer pool.Release(ctx)

	// Convert data to JSON for passing to JavaScript
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal template data: %w", err)
	}

	// Escape template and partials for JavaScript string literals
	escapedTemplate := escapeJSString(template)

	// Build JavaScript code to initialize Liquid engine
	var jsCode strings.Builder
	jsCode.WriteString("(function() {\n")
	jsCode.WriteString("  const Liquid = liquidjs.Liquid;\n")

	// Create custom file system for partials
	if partials != nil && len(partials) > 0 {
		jsCode.WriteString("  const templates = {};\n")
		for name, content := range partials {
			escapedName := escapeJSString(name)
			escapedContent := escapeJSString(content)
			jsCode.WriteString(fmt.Sprintf("  templates[%q] = %q;\n", escapedName, escapedContent))
		}

		jsCode.WriteString("  const engine = new Liquid({\n")
		jsCode.WriteString("    fs: {\n")
		jsCode.WriteString("      readFileSync: function(file) { return templates[file] || ''; },\n")
		jsCode.WriteString("      existsSync: function(file) { return templates.hasOwnProperty(file); },\n")
		jsCode.WriteString("      resolve: function(root, file) { return file; }\n")
		jsCode.WriteString("    },\n")
		jsCode.WriteString("    parseLimit: 102400,\n")   // 100KB template size limit
		jsCode.WriteString("    renderLimit: 5000,\n")    // 5 second render timeout
		jsCode.WriteString("    memoryLimit: 10485760\n") // 10MB memory limit
		jsCode.WriteString("  });\n")
	} else {
		jsCode.WriteString("  const engine = new Liquid({\n")
		jsCode.WriteString("    parseLimit: 102400,\n")   // 100KB template size limit
		jsCode.WriteString("    renderLimit: 5000,\n")    // 5 second render timeout
		jsCode.WriteString("    memoryLimit: 10485760\n") // 10MB memory limit
		jsCode.WriteString("  });\n")
	}

	// Parse and render template
	jsCode.WriteString(fmt.Sprintf("  const template = %q;\n", escapedTemplate))
	jsCode.WriteString(fmt.Sprintf("  const data = %s;\n", string(dataJSON)))
	jsCode.WriteString("  return engine.parseAndRenderSync(template, data);\n")
	jsCode.WriteString("})();\n")

	// Execute JavaScript and get result
	result, err := ctx.RunScript(jsCode.String(), "render.js")
	if err != nil {
		return "", fmt.Errorf("liquidjs rendering failed: %w", err)
	}

	return result.String(), nil
}

// escapeJSString escapes a string for use in JavaScript string literals
func escapeJSString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
