package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeDoc(t *testing.T, doc string) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(doc), &out))
	return out
}

// tiptapFixtures are one document per node and mark type of the console's
// jsonToHtml (console/src/components/blog/utils.ts), with the HTML and the
// plain text that jsonToHtml and extractTextContent return for it, as a node
// script running that file printed them.
var tiptapFixtures = []struct {
	name, doc, html, text string
}{
	{
		name: "doc",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"One"}]},{"type":"paragraph","content":[{"type":"text","text":"Two"}]}]}`,
		html: `<p>One</p><p>Two</p>`,
		text: `One Two`,
	},
	{
		name: "paragraph",
		doc:  `{"type":"doc","content":[{"type":"paragraph","attrs":{"textAlign":"center","backgroundColor":"#fef9c3","color":"rgb(68, 64, 60)"},"content":[{"type":"text","text":"Centered"}]}]}`,
		html: `<p style="text-align: center; background-color: #fef9c3; color: rgb(68, 64, 60)">Centered</p>`,
		text: `Centered`,
	},
	{
		name: "paragraph_left_and_null_attrs",
		doc:  `{"type":"doc","content":[{"type":"paragraph","attrs":{"textAlign":"left","backgroundColor":null,"color":null},"content":[{"type":"text","text":"Left"}]}]}`,
		html: `<p>Left</p>`,
		text: `Left`,
	},
	{
		name: "paragraph_empty",
		doc:  `{"type":"doc","content":[{"type":"paragraph"}]}`,
		html: `<p><br></p>`,
		text: ``,
	},
	{
		name: "heading",
		doc:  `{"type":"doc","content":[{"type":"heading","attrs":{"level":3,"textAlign":"right","id":"toc-1"},"content":[{"type":"text","text":"Section"}]}]}`,
		html: `<h3 style="text-align: right">Section</h3>`,
		text: `Section`,
	},
	{
		name: "heading_default_level",
		doc:  `{"type":"doc","content":[{"type":"heading","content":[{"type":"text","text":"Untitled level"}]}]}`,
		html: `<h2>Untitled level</h2>`,
		text: `Untitled level`,
	},
	{
		name: "blockquote",
		doc:  `{"type":"doc","content":[{"type":"blockquote","attrs":{"backgroundColor":"#e0f2fe"},"content":[{"type":"paragraph","content":[{"type":"text","text":"Quoted"}]}]}]}`,
		html: `<blockquote style="background-color: #e0f2fe"><p>Quoted</p></blockquote>`,
		text: `Quoted`,
	},
	{
		name: "bulletList",
		doc:  `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"First"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Second"}]}]}]}]}`,
		html: `<ul><li><p>First</p></li><li><p>Second</p></li></ul>`,
		text: `First Second`,
	},
	{
		name: "orderedList",
		doc:  `{"type":"doc","content":[{"type":"orderedList","attrs":{"start":3},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Third"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Nested"}]}]}]}]}]}]}`,
		html: `<ol start="3"><li><p>Third</p><ul><li><p>Nested</p></li></ul></li></ol>`,
		text: `Third Nested`,
	},
	{
		name: "orderedList_string_start",
		doc:  `{"type":"doc","content":[{"type":"orderedList","attrs":{"start":"5"},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Fifth"}]}]}]}]}`,
		html: `<ol start="5"><li><p>Fifth</p></li></ol>`,
		text: `Fifth`,
	},
	{
		name: "codeBlock",
		doc:  `{"type":"doc","content":[{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"if a < b && c > d {"},{"type":"text","text":"\treturn \"x\"\n}"}]}]}`,
		html: "<pre><code class=\"language-go\">if a &lt; b &amp;&amp; c &gt; d {\n\treturn &quot;x&quot;\n}</code></pre>",
		text: "if a < b && c > d { \treturn \"x\"\n}",
	},
	{
		name: "codeBlock_default_language",
		doc:  `{"type":"doc","content":[{"type":"codeBlock","content":[{"type":"text","text":"plain 'text'"}]}]}`,
		html: `<pre><code class="language-plaintext">plain &#039;text&#039;</code></pre>`,
		text: `plain 'text'`,
	},
	{
		name: "horizontalRule",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Above"}]},{"type":"horizontalRule"},{"type":"paragraph","content":[{"type":"text","text":"Below"}]}]}`,
		html: `<p>Above</p><hr><p>Below</p>`,
		text: `Above  Below`,
	},
	{
		name: "hardBreak",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Line one"},{"type":"hardBreak"},{"type":"text","text":"Line two"}]}]}`,
		html: `<p>Line one<br>Line two</p>`,
		text: `Line one  Line two`,
	},
	{
		name: "image",
		doc:  `{"type":"doc","content":[{"type":"image","attrs":{"src":"https://images.example.com/a.jpg?w=600&h=400","alt":"A \"quoted\" alt","title":"Cover","width":600,"height":"400","align":"center","caption":"Photo by Ada","showCaption":true}}]}`,
		html: `<img src="https://images.example.com/a.jpg?w=600&amp;h=400" alt="A &quot;quoted&quot; alt" title="Cover" data-width="600" data-height="400" data-align="center" data-caption="Photo by Ada" data-show-caption="true" />`,
		text: ``,
	},
	{
		name: "image_minimal",
		doc:  `{"type":"doc","content":[{"type":"image","attrs":{"src":"http://images.example.com/b.png","alt":null,"title":null,"width":null,"height":null,"align":null,"caption":null,"showCaption":false}}]}`,
		html: `<img src="http://images.example.com/b.png" alt="" data-show-caption="false" />`,
		text: ``,
	},
	{
		name: "youtube",
		doc:  `{"type":"doc","content":[{"type":"youtube","attrs":{"src":"dQw4w9WgXcQ","width":800,"height":450,"align":"center","cc":true,"loop":"1","controls":false,"modestbranding":1,"start":"42","showCaption":"true","caption":"Talk & demo"}}]}`,
		html: `<div data-youtube-video data-align="center" data-width="800" data-show-caption="true" data-caption="Talk &amp; demo" data-cc="true" data-loop="true" data-controls="false" data-modestbranding="true" data-start="42"><iframe src="https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ?cc_load_policy=1&amp;loop=1&amp;playlist=dQw4w9WgXcQ&amp;controls=0&amp;modestbranding=1&amp;start=42" width="800" height="450" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe></div>`,
		text: ``,
	},
	{
		name: "youtube_defaults",
		doc:  `{"type":"doc","content":[{"type":"youtube","attrs":{"src":"dQw4w9WgXcQ"}}]}`,
		html: `<div data-youtube-video data-align="left" data-width="640"><iframe src="https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ" width="640" height="360" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe></div>`,
		text: ``,
	},
	{
		name: "text",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Tom & Jerry <say> \"hi\" it's"}]}]}`,
		html: `<p>Tom &amp; Jerry &lt;say&gt; &quot;hi&quot; it&#039;s</p>`,
		text: `Tom & Jerry <say> "hi" it's`,
	},
	{
		name: "bold",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Strong","marks":[{"type":"bold"}]}]}]}`,
		html: `<p><strong>Strong</strong></p>`,
		text: `Strong`,
	},
	{
		name: "italic",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Slanted","marks":[{"type":"italic"}]}]}]}`,
		html: `<p><em>Slanted</em></p>`,
		text: `Slanted`,
	},
	{
		name: "underline",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Under","marks":[{"type":"underline"}]}]}]}`,
		html: `<p><u>Under</u></p>`,
		text: `Under`,
	},
	{
		name: "strike",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Gone","marks":[{"type":"strike"}]}]}]}`,
		html: `<p><s>Gone</s></p>`,
		text: `Gone`,
	},
	{
		name: "code",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a < b","marks":[{"type":"code"}]}]}]}`,
		html: `<p><code>a &lt; b</code></p>`,
		text: `a < b`,
	},
	{
		name: "link",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Docs","marks":[{"type":"link","attrs":{"href":"https://docs.example.com/a?b=1&c=2","target":"_blank","rel":"noopener noreferrer nofollow","class":null}}]}]}]}`,
		html: `<p><a href="https://docs.example.com/a?b=1&amp;c=2" target="_blank">Docs</a></p>`,
		text: `Docs`,
	},
	{
		name: "link_mailto_no_target",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Write","marks":[{"type":"link","attrs":{"href":"mailto:help@example.com","target":null}}]}]}]}`,
		html: `<p><a href="mailto:help@example.com">Write</a></p>`,
		text: `Write`,
	},
	{
		name: "textStyle",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Red","marks":[{"type":"textStyle","attrs":{"color":"#ff0000"}}]}]}]}`,
		html: `<p><span style="color: #ff0000">Red</span></p>`,
		text: `Red`,
	},
	{
		name: "textStyle_without_color",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Plain","marks":[{"type":"textStyle","attrs":{"color":null}}]}]}]}`,
		html: `<p>Plain</p>`,
		text: `Plain`,
	},
	{
		name: "highlight",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Marked","marks":[{"type":"highlight","attrs":{"color":"var(--tt-color-highlight-green)"}}]}]}]}`,
		html: `<p><mark style="background-color: var(--tt-color-highlight-green)">Marked</mark></p>`,
		text: `Marked`,
	},
	{
		name: "highlight_default_color",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Yellow","marks":[{"type":"highlight"}]}]}]}`,
		html: `<p><mark style="background-color: #ffff00">Yellow</mark></p>`,
		text: `Yellow`,
	},
	{
		name: "subscript",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"H"},{"type":"text","text":"2","marks":[{"type":"subscript"}]},{"type":"text","text":"O"}]}]}`,
		html: `<p>H<sub>2</sub>O</p>`,
		text: `H 2 O`,
	},
	{
		name: "superscript",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x"},{"type":"text","text":"2","marks":[{"type":"superscript"}]}]}]}`,
		html: `<p>x<sup>2</sup></p>`,
		text: `x 2`,
	},
	{
		name: "marks_in_order",
		doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"All","marks":[{"type":"bold"},{"type":"italic"},{"type":"link","attrs":{"href":"https://example.com"}}]}]}]}`,
		html: `<p><a href="https://example.com"><em><strong>All</strong></em></a></p>`,
		text: `All`,
	},
	{
		name: "empty_doc",
		doc:  `{"type":"doc","content":[]}`,
		html: ``,
		text: ``,
	},
}

func TestTiptap_RendersAsTheConsole(t *testing.T) {
	for _, fixture := range tiptapFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			doc := decodeDoc(t, fixture.doc)
			require.Nil(t, checkTiptapDocument(doc))
			assert.Equal(t, fixture.html, renderTiptapHTML(doc))
			assert.Equal(t, fixture.text, tiptapPlainText(doc))
		})
	}
}

// The fixtures cover every node and mark type the console renders.
func TestTiptap_FixturesCoverEveryType(t *testing.T) {
	nodes, marks := map[string]bool{}, map[string]bool{}
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		nodes[node["type"].(string)] = true
		for _, mark := range asSlice(node["marks"]) {
			marks[mark.(map[string]any)["type"].(string)] = true
		}
		for _, child := range asSlice(node["content"]) {
			walk(child.(map[string]any))
		}
	}
	for _, fixture := range tiptapFixtures {
		walk(decodeDoc(t, fixture.doc))
	}
	assert.Equal(t, []string{
		"blockquote", "bulletList", "codeBlock", "doc", "hardBreak", "heading", "horizontalRule", "image",
		"listItem", "orderedList", "paragraph", "text", "youtube",
	}, sortedSet(nodes))
	assert.Equal(t, []string{
		"bold", "code", "highlight", "italic", "link", "strike", "subscript", "superscript", "textStyle", "underline",
	}, sortedSet(marks))
}

func asSlice(v any) []any {
	out, _ := v.([]any)
	return out
}

func sortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// paragraphWith wraps one inline node in a document.
func paragraphWith(inline string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[` + inline + `]}]}`
}

// Text and every attribute value come out escaped, whatever they hold.
func TestTiptap_EscapesTextAndAttributes(t *testing.T) {
	for name, tc := range map[string]struct{ doc, html string }{
		"text": {
			doc:  paragraphWith(`{"type":"text","text":"<script>alert('x')</script>"}`),
			html: `<p>&lt;script&gt;alert(&#039;x&#039;)&lt;/script&gt;</p>`,
		},
		"an image's alt, title and caption": {
			doc:  `{"type":"doc","content":[{"type":"image","attrs":{"src":"https://img.example.com/a.png","alt":"\"><script>x</script>","title":"a\"b","caption":"<b>c</b>"}}]}`,
			html: `<img src="https://img.example.com/a.png" alt="&quot;&gt;&lt;script&gt;x&lt;/script&gt;" title="a&quot;b" data-caption="&lt;b&gt;c&lt;/b&gt;" />`,
		},
		"a link's href and target": {
			doc:  paragraphWith(`{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"https://example.com/?q=\"><x","target":"_blank\" onclick=\"x"}}]}`),
			html: `<p><a href="https://example.com/?q=&quot;&gt;&lt;x" target="_blank&quot; onclick=&quot;x">x</a></p>`,
		},
		"a code block's language": {
			doc:  `{"type":"doc","content":[{"type":"codeBlock","attrs":{"language":"go\"><img src=x>"},"content":[{"type":"text","text":"x"}]}]}`,
			html: `<pre><code class="language-go&quot;&gt;&lt;img src=x&gt;">x</code></pre>`,
		},
		"a video's caption and alignment": {
			doc: `{"type":"doc","content":[{"type":"youtube","attrs":{"src":"dQw4w9WgXcQ","caption":"\"<x>","align":"a\"b"}}]}`,
			html: `<div data-youtube-video data-align="a&quot;b" data-width="640" data-caption="&quot;&lt;x&gt;"><iframe src="https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ" width="640" height="360" ` +
				`frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe></div>`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			doc := decodeDoc(t, tc.doc)
			require.Nil(t, checkTiptapDocument(doc))
			assert.Equal(t, tc.html, renderTiptapHTML(doc))
		})
	}
}

// The console writes some attributes without escaping them: a value that
// could leave its attribute or its CSS declaration is refused before any
// rendering.
func TestTiptap_RefusesAttributesTheConsoleWritesUnescaped(t *testing.T) {
	for name, doc := range map[string]string{
		"a paragraph color":     `{"type":"doc","content":[{"type":"paragraph","attrs":{"color":"red\" onmouseover=\"x"}}]}`,
		"a CSS declaration":     `{"type":"doc","content":[{"type":"paragraph","attrs":{"backgroundColor":"red; background-image: url(https://x)"}}]}`,
		"a text alignment":      `{"type":"doc","content":[{"type":"heading","attrs":{"level":2,"textAlign":"center; color: red"}}]}`,
		"a heading level":       `{"type":"doc","content":[{"type":"heading","attrs":{"level":"2 onclick=x"}}]}`,
		"a list's first number": `{"type":"doc","content":[{"type":"orderedList","attrs":{"start":"1\" onclick=\"x"},"content":[]}]}`,
		"an image width":        `{"type":"doc","content":[{"type":"image","attrs":{"src":"https://img.example.com/a.png","width":"600\" onerror=\"x"}}]}`,
		"an image showCaption":  `{"type":"doc","content":[{"type":"image","attrs":{"src":"https://img.example.com/a.png","showCaption":"x\" onerror=\"y"}}]}`,
		"a video ID":            `{"type":"doc","content":[{"type":"youtube","attrs":{"src":"x\"><script>"}}]}`,
		"a video height":        `{"type":"doc","content":[{"type":"youtube","attrs":{"src":"dQw4w9WgXcQ","height":"1\"><x"}}]}`,
		"a highlight color":     paragraphWith(`{"type":"text","text":"x","marks":[{"type":"highlight","attrs":{"color":"\"><x"}}]}`),
		"a text color":          paragraphWith(`{"type":"text","text":"x","marks":[{"type":"textStyle","attrs":{"color":"red;position:fixed"}}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			toolErr := checkTiptapDocument(decodeDoc(t, doc))
			require.NotNil(t, toolErr)
			assert.Equal(t, CodeValidation, toolErr.Code)
			assert.True(t, strings.HasPrefix(toolErr.Message, "body.content[0]"), toolErr.Message)
		})
	}
}

func TestTiptap_LinkAndImageURLs(t *testing.T) {
	link := func(href string) string {
		data, _ := json.Marshal(href)
		return paragraphWith(fmt.Sprintf(`{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":%s}}]}`, data))
	}
	image := func(src string) string {
		data, _ := json.Marshal(src)
		return fmt.Sprintf(`{"type":"doc","content":[{"type":"image","attrs":{"src":%s}}]}`, data)
	}
	for _, href := range []string{"https://example.com", "HTTP://example.com/a", "mailto:help@example.com"} {
		assert.Nil(t, checkTiptapDocument(decodeDoc(t, link(href))), href)
	}
	for _, src := range []string{"https://img.example.com/a.png", "http://img.example.com/a.png"} {
		assert.Nil(t, checkTiptapDocument(decodeDoc(t, image(src))), src)
	}
	for _, href := range []string{
		"javascript:alert(1)", "JavaScript:alert(1)", " javascript:alert(1)", "java\tscript:alert(1)", "https://x.example.com/\nx",
		"data:text/html,<script>alert(1)</script>", "vbscript:x", "/relative", "//evil.example.com", "#top", "https://", "",
	} {
		toolErr := checkTiptapDocument(decodeDoc(t, link(href)))
		require.NotNil(t, toolErr, href)
		assert.Equal(t, "body.content[0].content[0].marks[0].attrs.href: a link needs an http, https or mailto URL", toolErr.Message, href)
	}
	for _, src := range []string{"data:image/png;base64,iVBORw0KGgo=", "javascript:alert(1)", "mailto:a@example.com", "/a.png"} {
		toolErr := checkTiptapDocument(decodeDoc(t, image(src)))
		require.NotNil(t, toolErr, src)
		assert.Equal(t, "body.content[0].attrs.src: an image needs an http or https URL", toolErr.Message, src)
	}
	toolErr := checkTiptapDocument(decodeDoc(t, paragraphWith(`{"type":"text","text":"x","marks":[{"type":"link"}]}`)))
	require.NotNil(t, toolErr, "a link without href")
}

// Shapes the editor cannot load, or that the console would drop, are refused
// with the path of the node.
func TestTiptap_RefusesWhatTheEditorCannotLoad(t *testing.T) {
	deep := `{"type":"text","text":"x"}`
	deep = `{"type":"paragraph","content":[` + deep + `]}`
	for range maxTiptapDepth {
		deep = `{"type":"blockquote","content":[` + deep + `]}`
	}
	for name, tc := range map[string]struct{ doc, message string }{
		"a root that is not a doc":  {`{"type":"paragraph","content":[]}`, `body: the root must be {"type":"doc","content":[...]}`},
		"a doc without content":     {`{"type":"doc"}`, "body.content: must be an array of block nodes"},
		"an unknown node":           {`{"type":"doc","content":[{"type":"table","content":[]}]}`, `body.content[0]: a doc node cannot hold a "table" node; it holds paragraph, heading, blockquote, bulletList, orderedList, codeBlock, horizontalRule, image, youtube`},
		"text straight in the doc":  {`{"type":"doc","content":[{"type":"text","text":"x"}]}`, `body.content[0]: a doc node cannot hold a "text" node; it holds paragraph, heading, blockquote, bulletList, orderedList, codeBlock, horizontalRule, image, youtube`},
		"a list item outside lists": {`{"type":"doc","content":[{"type":"listItem","content":[]}]}`, `body.content[0]: a doc node cannot hold a "listItem" node; it holds paragraph, heading, blockquote, bulletList, orderedList, codeBlock, horizontalRule, image, youtube`},
		"a paragraph in a list":     {`{"type":"doc","content":[{"type":"bulletList","content":[{"type":"paragraph"}]}]}`, `body.content[0].content[0]: a bulletList node cannot hold a "paragraph" node; it holds listItem`},
		"an empty text":             {paragraphWith(`{"type":"text","text":""}`), "body.content[0].content[0].text: a text node needs a non-empty text"},
		"children instead of content": {`{"type":"doc","content":[{"type":"paragraph","children":[]}]}`,
			`body.content[0]: unknown key "children"; a node has type, attrs, content, and marks and text for a text node`},
		"marks on a paragraph":     {`{"type":"doc","content":[{"type":"paragraph","marks":[]}]}`, "body.content[0]: only a text node has marks"},
		"an unknown mark":          {paragraphWith(`{"type":"text","text":"x","marks":[{"type":"emoji"}]}`), `body.content[0].content[0].marks[0]: unknown mark "emoji"; marks are bold, italic, underline, strike, code, link, textStyle, highlight, subscript, superscript`},
		"content in a leaf":        {`{"type":"doc","content":[{"type":"horizontalRule","content":[]}]}`, "body.content[0]: a horizontalRule node has no content"},
		"marks in a code block":    {`{"type":"doc","content":[{"type":"codeBlock","content":[{"type":"text","text":"x","marks":[{"type":"bold"}]}]}]}`, "body.content[0].content[0]: text in a code block has no marks"},
		"a heading level of seven": {`{"type":"doc","content":[{"type":"heading","attrs":{"level":7}}]}`, "body.content[0].attrs.level: must be an integer from 1 to 6"},
		"a full YouTube URL":       {`{"type":"doc","content":[{"type":"youtube","attrs":{"src":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"}}]}`, `body.content[0].attrs.src: must be a YouTube video ID of 11 characters, such as "dQw4w9WgXcQ", not a URL`},
		"nodes nested too deep":    {`{"type":"doc","content":[` + deep + `]}`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			toolErr := checkTiptapDocument(decodeDoc(t, tc.doc))
			require.NotNil(t, toolErr)
			assert.Equal(t, CodeValidation, toolErr.Code)
			if tc.message != "" {
				assert.Equal(t, tc.message, toolErr.Message)
			} else {
				assert.Contains(t, toolErr.Message, "nodes nest more than")
			}
		})
	}
}
