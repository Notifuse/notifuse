package notifuse_mjml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateLiquidSyntax(t *testing.T) {
	accepted := []struct{ name, content string }{
		{"empty", ""},
		{"plain text", "Hello,\nyour order has shipped."},
		{"json-like braces", `{"a":{"b":1}}`},
		{"output with filters", "Hi {{ contact.first_name | default: 'there' | upcase }}"},
		{"if and for blocks", "{% if vip %}VIP{% endif %}{% for i in items %}- {{ i.name }}\n{% endfor %}"},
		{"raw block", "{% raw %}{{ not rendered }}{% endraw %}"},
		{"include inside raw is text", "{% raw %}{% include 'x' %}{% endraw %}"},
		// Only parses once cleaned the way the renderer cleans it: the zero-width space
		// inside "if" makes an unknown tag otherwise.
		{"markup cleaned like at render", "{% i​f x %}a{% endif %}"},
	}
	for _, tt := range accepted {
		t.Run("accepts "+tt.name, func(t *testing.T) {
			assert.NoError(t, ValidateLiquidSyntax(tt.content))
		})
	}

	rejected := []struct{ name, content string }{
		{"unclosed if", "{% if x %}Hello"},
		{"unknown tag", "{% nosuchtag %}"},
		{"unterminated output", "Hi {{ name"},
		{"include", "{% include 'footer' %}"},
		{"render", "{%- render 'footer' -%}"},
		{"include in a liquid tag", "{% liquid\n  assign a = 1\n  include 'footer'\n%}"},
	}
	for _, tt := range rejected {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			assert.Error(t, ValidateLiquidSyntax(tt.content))
		})
	}
}
