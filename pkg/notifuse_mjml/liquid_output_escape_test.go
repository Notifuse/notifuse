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
		`{{ 'Terms &amp; conditions' }}`:              `{{ 'Terms & conditions' }}`,
		`{{ 'say &quot;hi&quot;' }}`:                  `{{ 'say "hi"' }}`,
		`{{ "say &quot;hi&quot;" }}`:                  `{{ 'say "hi"' }}`,
		`{{ 'it&#39;s' }}`:                            `{{ "it's" }}`,
		`{{ 'it&#39;s &quot;x&quot;' }}`:              `{{ 'it&#39;s "x"' }}`,
		`{{ x | default: "a &lt; b" }}`:               `{{ x | default: "a < b" }}`,
		`{% if a &lt; 5 %}`:                           `{% if a &lt; 5 %}`,
		`{{ 'no entity' }}`:                           `{{ 'no entity' }}`,
		`{{ 'unclosed &amp;`:                          `{{ 'unclosed &amp;`,
		`{{ x | default: 'a &#125;&#125; b' }}`:       `{{ x | default: 'a &#125;&#125; b' }}`,
		`{{ x | default: 'a &#37;&#125; b' }}`:        `{{ x | default: 'a &#37;&#125; b' }}`,
		`{{ 'https://x.example/?a=1&copy=2&amp;b' }}`: `{{ 'https://x.example/?a=1&copy=2&b' }}`,
		`{{ 'Tom&#39s; ok' }}`:                        `{{ 'Tom&#39s; ok' }}`,
	} {
		assert.Equal(t, want, decodeLiquidLiteralEntities(in), in)
	}
}

const phishingLink = `<a href=https://phishing.example>Your account is locked</a>`

// The link decoding after the render does not turn escaped data back into markup:
// escaped text that ends with src=, href= or action= before a quote of the template
// is not an attribute, and a value in an Outlook conditional comment cannot end it.
func TestEscapedDataStaysEscapedThroughTheLinkDecoding(t *testing.T) {
	cases := map[string]CompileTemplateRequest{
		"text after an attribute, visual": {
			TemplateData:     MapOfAny{"contact": map[string]interface{}{"first_name": "x href=", "last_name": phishingLink}},
			VisualEditorTree: editorTree(editorText(`<p><img src="https://ok.example/a.png" alt="{{ contact.first_name }}">Hello {{ contact.last_name }}</p>`), editorText(`<p>Footer "quoted"</p>`)),
		},
		"text after a quote of the template, code": {
			TemplateData: MapOfAny{"contact": map[string]interface{}{"company": "Acme src=", "first_name": phishingLink}},
			MjmlSource:   codeSource(`<mj-text><p>Welcome to "{{ contact.company }}", {{ contact.first_name }}!</p></mj-text><mj-text>Footer "x"</mj-text>`),
		},
		"a background-url in an Outlook comment, code": {
			TemplateData: contactData(`--><a href=https://phishing.example>Your account is locked</a><!--`),
			MjmlSource:   &[]string{`<mjml><mj-body><mj-section background-url="{{ contact.first_name }}"><mj-column><mj-text>Hi</mj-text></mj-column></mj-section></mj-body></mjml>`}[0],
		},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			req.WorkspaceID, req.MessageID = "ws", "m"
			resp, err := CompileTemplate(req)
			require.NoError(t, err)
			require.True(t, resp.Success, "error: %v", resp.Error)
			assert.NotContains(t, *resp.HTML, `<a href=https://phishing.example>`)
		})
	}
}

// Every way Liquid reaches the HTML escapes its outputs: cycle, an mj-liquid block
// and a visual attribute that the whole-document pass renders, and code mode.
func TestLiquidOutputEscapeReachesEveryPath(t *testing.T) {
	escaped := `&lt;a href=https://phishing.example&gt;Your account is locked&lt;/a&gt;`
	cases := map[string]CompileTemplateRequest{
		"cycle":          {VisualEditorTree: editorTree(editorText(`<p>{% cycle contact.first_name, 'x' %}</p>`))},
		"mj-liquid":      {VisualEditorTree: editorTree(editorLiquid(`<mj-text>{{ contact.first_name }}</mj-text>`))},
		"code mj-raw":    {MjmlSource: codeSource(`<mj-raw><div>{{ contact.first_name }}</div></mj-raw>`)},
		"code mj-button": {MjmlSource: codeSource(`<mj-button href="https://shop.example">{{ contact.first_name }}</mj-button>`)},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			req.WorkspaceID, req.MessageID, req.TemplateData = "ws", "m", contactData(phishingLink)
			resp, err := CompileTemplate(req)
			require.NoError(t, err)
			require.True(t, resp.Success, "error: %v", resp.Error)
			assert.NotContains(t, *resp.HTML, `<a href=https://phishing.example>`)
			assert.Contains(t, *resp.HTML, escaped)
		})
	}
}

// A visual attribute that is not a URL is rendered by the whole-document pass: a
// quote in the data stays inside the attribute, and the browser reads the data.
func TestLiquidOutputInAVisualAttribute(t *testing.T) {
	resp, err := CompileTemplate(CompileTemplateRequest{
		WorkspaceID: "ws", MessageID: "m", TemplateData: contactData(`Ann "the boss" <x>`),
		VisualEditorTree: editorTree(editorImage("title", `{{ contact.first_name }}`)),
	})
	require.NoError(t, err)
	require.True(t, resp.Success, "error: %v", resp.Error)
	title, ok := attrFromRenderedHTML(t, *resp.HTML, "title")
	require.True(t, ok)
	assert.Equal(t, `Ann "the boss" <x>`, title)
}

// HTML on purpose in code mode, and a literal that holds its own quote as an entity.
func TestLiquidOutputEscapeExceptionsInCodeMode(t *testing.T) {
	data := MapOfAny{
		"contact":     map[string]interface{}{"first_name": "Ann & Bob", "bio": "line 1\nline <2>"},
		"global_feed": map[string]interface{}{"html": `<p class="feed">From the feed</p>`},
	}
	cases := []struct{ name, text, want string }{
		{"raw", `{{ global_feed.html | raw }}`, `<p class="feed">From the feed</p>`},
		{"escape", `{{ contact.first_name | escape }}`, `Ann &amp; Bob`},
		{"escape then newline_to_br", `{{ contact.bio | escape | newline_to_br }}`, "line 1<br />\nline &lt;2&gt;"},
		{"a capture through strip", `{% capture b %} <b>{{ contact.first_name }}</b> {% endcapture %}{{ b | strip }}`, `<b>Ann &amp; Bob</b>`},
		{"a literal with its own quote", `{{ contact.nick | default: 'it&#39;s you' }}`, `it&#39;s you`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := CompileTemplate(CompileTemplateRequest{
				WorkspaceID: "ws", MessageID: "m", TemplateData: data,
				MjmlSource: codeSource(`<mj-text><p>` + tc.text + `</p></mj-text>`),
			})
			require.NoError(t, err)
			require.True(t, resp.Success, "error: %v", resp.Error)
			assert.Contains(t, *resp.HTML, `<p>`+tc.want+`</p>`)
		})
	}
}

// Data written into a URL or alt attribute of the visual editor is text: one that
// reads as an entity is shown as written, not decoded.
func TestLiquidOutputInAVisualAltIsText(t *testing.T) {
	resp, err := CompileTemplate(CompileTemplateRequest{
		WorkspaceID: "ws", MessageID: "m", TemplateData: contactData(`5 &lt; 6 & "seven"`),
		VisualEditorTree: editorTree(editorImage("alt", `{{ contact.first_name }}`)),
	})
	require.NoError(t, err)
	require.True(t, resp.Success, "error: %v", resp.Error)
	alt, ok := attrFromRenderedHTML(t, *resp.HTML, "alt")
	require.True(t, ok)
	assert.Equal(t, `5 &lt; 6 & "seven"`, alt)
}

// Escaped data placed between components stays text: safe-mjml v1.0.1 no longer
// turns &lt; and &gt; there back into markup, so it cannot become a component.
func TestEscapedDataBetweenComponentsIsNotMJML(t *testing.T) {
	button := `<mj-button href="https://phishing.example">Click</mj-button>`
	cases := map[string]CompileTemplateRequest{
		"code, in a column": {MjmlSource: codeSource(`<mj-text>a</mj-text>{{ contact.first_name }}`)},
		"code, in the body": {MjmlSource: &[]string{`<mjml><mj-body>{{ contact.first_name }}<mj-section><mj-column><mj-text>a</mj-text></mj-column></mj-section></mj-body></mjml>`}[0]},
		"mj-liquid block":   {VisualEditorTree: editorTree(editorLiquid(`<mj-text>a</mj-text>{{ contact.first_name }}`))},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			req.WorkspaceID, req.MessageID, req.TemplateData = "ws", "m", contactData(button)
			resp, err := CompileTemplate(req)
			require.NoError(t, err)
			require.True(t, resp.Success, "error: %v", resp.Error)
			assert.NotContains(t, *resp.HTML, "phishing.example")
		})
	}
}
