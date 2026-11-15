package notifuse_mjml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hostileFirstName is what a public form can store as a contact's first name.
const hostileFirstName = `<a href="https://phishing.example">Your account is locked</a>"'`

func contactData(firstName string) MapOfAny {
	return MapOfAny{"contact": map[string]interface{}{"first_name": firstName}}
}

func codeSource(body string) *string {
	s := `<mjml><mj-body><mj-section><mj-column>` + body + `</mj-column></mj-section></mj-body></mjml>`
	return &s
}

// Liquid outputs in the HTML of an email are HTML-escaped: a contact field filled in
// a public form cannot add markup or links to an email the brand sends.
func TestLiquidOutputIsEscapedInEmailHTML(t *testing.T) {
	for name, req := range map[string]CompileTemplateRequest{
		"visual editor": {VisualEditorTree: editorTree(editorText(`<p>Hello {{ contact.first_name }}</p>`))},
		"code":          {MjmlSource: codeSource(`<mj-text>Hello {{ contact.first_name }}</mj-text>`)},
	} {
		t.Run(name, func(t *testing.T) {
			req.WorkspaceID, req.MessageID, req.TemplateData = "ws", "m", contactData(hostileFirstName)
			resp, err := CompileTemplate(req)
			require.NoError(t, err)
			require.True(t, resp.Success, "error: %v", resp.Error)
			assert.NotContains(t, *resp.HTML, `<a href="https://phishing.example"`)
			assert.Contains(t, *resp.HTML, `Hello &lt;a href=&quot;https://phishing.example&quot;&gt;Your account is locked&lt;/a&gt;&quot;&#39;`)
		})
	}
}

// raw writes HTML on purpose, such as a feed that holds an HTML block; escape is not
// escaped a second time; a literal the editor encoded is the text the author typed.
func TestLiquidOutputEscapeExceptions(t *testing.T) {
	data := MapOfAny{
		"contact":     map[string]interface{}{"first_name": "Ann & Bob"},
		"global_feed": map[string]interface{}{"html": `<p class="feed">From the feed</p>`},
	}
	cases := []struct{ name, text, want string }{
		{"raw", `{{ global_feed.html | raw }}`, `<p class="feed">From the feed</p>`},
		{"escape is not escaped twice", `{{ contact.first_name | escape }}`, `Ann &amp; Bob`},
		{"a literal the editor encoded", `{{ 'Terms &amp; conditions' }}`, `Terms &amp; conditions`},
		{"a capture keeps the template's markup", `{% capture b %}<b>{{ contact.first_name }}</b>{% endcapture %}{{ b }}`, `<b>Ann &amp; Bob</b>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := CompileTemplate(CompileTemplateRequest{
				WorkspaceID: "ws", MessageID: "m", TemplateData: data,
				VisualEditorTree: editorTree(editorText(`<p>` + tc.text + `</p>`)),
			})
			require.NoError(t, err)
			require.True(t, resp.Success, "error: %v", resp.Error)
			assert.Contains(t, *resp.HTML, `<p>`+tc.want+`</p>`)
		})
	}
}

// In code mode, a quote in a value written into an attribute is escaped instead of
// closing the attribute, which made the compile fail for that contact.
func TestLiquidOutputInACodeModeAttribute(t *testing.T) {
	resp, err := CompileTemplate(CompileTemplateRequest{
		WorkspaceID: "ws", MessageID: "m", TemplateData: contactData(`Ann "the boss"`), Channel: "web",
		MjmlSource: codeSource(`<mj-image src="https://img.example/a.png" alt="{{ contact.first_name }}" />`),
	})
	require.NoError(t, err)
	require.True(t, resp.Success, "error: %v", resp.Error)
	alt, ok := attrFromRenderedHTML(t, *resp.HTML, "alt")
	require.True(t, ok)
	assert.Equal(t, `Ann "the boss"`, alt)
}

// The subject and the text version are not HTML: they are not escaped.
func TestLiquidOutputIsNotEscapedInText(t *testing.T) {
	subject := "Hello {{ contact.first_name }}"
	resp, err := CompileTemplate(CompileTemplateRequest{
		WorkspaceID: "ws", MessageID: "m", TemplateData: contactData("Ann & Bob <team>"),
		Subject:          &subject,
		IncludeText:      true,
		VisualEditorTree: editorTree(editorText(`<p>Hi {{ contact.first_name }}</p>`)),
	})
	require.NoError(t, err)
	require.True(t, resp.Success, "error: %v", resp.Error)
	require.NotNil(t, resp.Subject)
	assert.Equal(t, "Hello Ann & Bob <team>", *resp.Subject)
	require.NotNil(t, resp.Text)
	assert.True(t, strings.Contains(*resp.Text, "Hi Ann & Bob <team>"), "text version: %q", *resp.Text)
}

// decodeLiquidLiteralEntities decodes the entities inside the string literals of a
// markup, but not one that stands for the literal's own quote, which would end it.
func TestDecodeLiquidLiteralEntities(t *testing.T) {
	for in, want := range map[string]string{
		`{{ 'Terms &amp; conditions' }}`: `{{ 'Terms & conditions' }}`,
		`{{ 'say &quot;hi&quot;' }}`:     `{{ 'say "hi"' }}`,
		`{{ "say &quot;hi&quot;" }}`:     `{{ "say &quot;hi&quot;" }}`,
		`{{ 'it&#39;s' }}`:               `{{ 'it&#39;s' }}`,
		`{{ x | default: "a &lt; b" }}`:  `{{ x | default: "a < b" }}`,
		`{% if a &lt; 5 %}`:              `{% if a &lt; 5 %}`,
		`{{ 'no entity' }}`:              `{{ 'no entity' }}`,
		`{{ 'unclosed &amp;`:             `{{ 'unclosed &amp;`,
	} {
		assert.Equal(t, want, decodeLiquidLiteralEntities(in), in)
	}
}
