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
		{"include inside a comment renders nothing", "{% comment %}{% include 'x' %}{% endcomment %}"},
		{"include as a word of the text", "Please include your order number."},
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
		{"include in a liquid tag", "{% liquid include 'footer' %}"},
		{"include in a nested liquid tag", "{% liquid liquid include 'footer' %}"},
		{"include with no space before its name", "{% include'footer' %}"},
		{"render with no space before its name", "{% render'footer' %}"},
	}
	for _, tt := range rejected {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			assert.Error(t, ValidateLiquidSyntax(tt.content))
		})
	}

	// The refusal names the tag, and the render environment keeps it.
	for _, tag := range []string{"include", "render"} {
		t.Run("names the "+tag+" tag", func(t *testing.T) {
			assert.EqualError(t, ValidateLiquidSyntax("{% "+tag+" 'footer' %}"), "the "+tag+" tag is not supported")
			assert.NotNil(t, newSecureLiquidEnv(DefaultMaxRenderLength).TagForName(tag))
		})
	}
}
