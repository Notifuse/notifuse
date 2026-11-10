package notifuse_mjml

import (
	"flag"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

var updatePlainTextGolden = flag.Bool("update-plaintext-golden", false, "rewrite testdata/plain-text-golden.txt")

const plainTextMJML = `<mjml><mj-head><mj-preview>Hidden preheader words</mj-preview></mj-head><mj-body><mj-section><mj-column>` +
	`<mj-text>Hello {{ name }}</mj-text>` +
	`<mj-button href="https://shop.example.com/offers">Shop now</mj-button>` +
	`</mj-column></mj-section></mj-body></mjml>`

func compilePlainText(t *testing.T, req CompileTemplateRequest) *CompileTemplateResponse {
	t.Helper()
	if req.WorkspaceID == "" {
		req.WorkspaceID = "ws"
	}
	if req.MessageID == "" {
		req.MessageID = "msg"
	}
	resp, err := CompileTemplate(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	return resp
}

func plainTextSource() *string {
	s := plainTextMJML
	return &s
}

var textURL = regexp.MustCompile(`https?://[^\s()]+`)

// urlSet returns the distinct values, sorted.
func urlSet(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// htmlHrefs returns the http(s) hrefs of every anchor in the document.
func htmlHrefs(t *testing.T, src string) []string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(src))
	require.NoError(t, err)
	var hrefs []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" && textURL.MatchString(a.Val) && strings.HasPrefix(strings.ToLower(a.Val), "http") {
					hrefs = append(hrefs, a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return hrefs
}

func TestCompileTemplate_PlainText(t *testing.T) {
	t.Run("not requested: no text, as on every send", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{MjmlSource: plainTextSource()})
		require.True(t, resp.Success)
		assert.Nil(t, resp.Text)
	})

	t.Run("requested with no channel: text of the rendered email", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{
			MjmlSource:   plainTextSource(),
			TemplateData: MapOfAny{"name": "Ada"},
			IncludeText:  true,
		})
		require.True(t, resp.Success)
		require.NotNil(t, resp.Text)
		assert.Contains(t, *resp.Text, "Hello Ada")
		assert.Contains(t, *resp.Text, "Shop now (https://shop.example.com/offers)")
		assert.NotContains(t, *resp.Text, "Hidden preheader words")
	})

	t.Run("web channel has no text", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{MjmlSource: plainTextSource(), Channel: "web", IncludeText: true})
		require.True(t, resp.Success)
		assert.Nil(t, resp.Text)
	})

	t.Run("compile error has no text", func(t *testing.T) {
		broken := "<mjml><mj-body><mj-section><mj-column><mj-text>{{ unclosed </mj-text></mj-column></mj-section></mj-body></mjml>"
		resp := compilePlainText(t, CompileTemplateRequest{MjmlSource: &broken, TemplateData: MapOfAny{"a": 1}, IncludeText: true})
		require.False(t, resp.Success)
		assert.Nil(t, resp.Text)
	})

	t.Run("visual tree: text and button label with its url", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{VisualEditorTree: CreateSimpleEmail(), IncludeText: true})
		require.True(t, resp.Success)
		require.NotNil(t, resp.Text)
		assert.Regexp(t, `\(https://example\.com[^)]*\)`, *resp.Text)
	})

	t.Run("tracking on: every url in the text is an href of the html", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{
			MjmlSource:  plainTextSource(),
			IncludeText: true,
			TrackingSettings: TrackingSettings{
				EnableTracking: true,
				Endpoint:       "https://track.example.com",
				UTMSource:      "email",
				WorkspaceID:    "ws",
				MessageID:      "msg",
			},
		})
		require.True(t, resp.Success)
		require.NotNil(t, resp.Text)
		require.NotNil(t, resp.HTML)
		assert.Contains(t, *resp.Text, "https://track.example.com")
		assert.NotContains(t, *resp.Text, "(https://shop.example.com/offers)")
		assert.Equal(t, urlSet(htmlHrefs(t, *resp.HTML)), urlSet(textURL.FindAllString(*resp.Text, -1)))
	})

	t.Run("tracking off: direct links", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{MjmlSource: plainTextSource(), IncludeText: true})
		require.NotNil(t, resp.Text)
		assert.Equal(t, urlSet(htmlHrefs(t, *resp.HTML)), urlSet(textURL.FindAllString(*resp.Text, -1)))
		assert.Equal(t, []string{"https://shop.example.com/offers"}, urlSet(textURL.FindAllString(*resp.Text, -1)))
	})
}

func TestCompileTemplate_PlainTextGolden(t *testing.T) {
	src, err := os.ReadFile("testdata/plain-text-golden.json")
	require.NoError(t, err)
	tree, err := UnmarshalEmailBlock(src)
	require.NoError(t, err)

	resp := compilePlainText(t, CompileTemplateRequest{
		VisualEditorTree: tree,
		TemplateData: MapOfAny{
			"contact":         MapOfAny{"first_name": "Camille"},
			"unsubscribe_url": "https://www.example.com/unsubscribe/abc",
		},
		IncludeText: true,
	})
	require.True(t, resp.Success, "%+v", resp.Error)
	require.NotNil(t, resp.Text)

	if *updatePlainTextGolden {
		require.NoError(t, os.WriteFile("testdata/plain-text-golden.txt", []byte(*resp.Text), 0o644))
	}
	want, err := os.ReadFile("testdata/plain-text-golden.txt")
	require.NoError(t, err)
	assert.Equal(t, string(want), *resp.Text)
	assert.Equal(t, urlSet(htmlHrefs(t, *resp.HTML)), urlSet(textURL.FindAllString(*resp.Text, -1)))
}

// With click tracking every link gets its own token, so the icon and the label of a
// social element no longer share a URL: the icon must still not be written.
func TestCompileTemplate_PlainTextGoldenTracked(t *testing.T) {
	src, err := os.ReadFile("testdata/plain-text-golden.json")
	require.NoError(t, err)
	tree, err := UnmarshalEmailBlock(src)
	require.NoError(t, err)

	resp := compilePlainText(t, CompileTemplateRequest{
		VisualEditorTree: tree,
		TemplateData:     MapOfAny{"unsubscribe_url": "https://www.example.com/unsubscribe/abc"},
		IncludeText:      true,
		TrackingSettings: TrackingSettings{
			EnableTracking: true, Endpoint: "https://t.example.com", WorkspaceID: "ws", MessageID: "msg",
		},
	})
	require.True(t, resp.Success, "%+v", resp.Error)
	require.NotNil(t, resp.Text)

	for _, line := range strings.Split(*resp.Text, "\n") {
		assert.False(t, textURL.MatchString(line) && textURL.FindString(line) == line, "bare URL line %q", line)
	}
	assert.Contains(t, *resp.Text, "Facebook (https://t.example.com/r/")
	assert.Contains(t, *resp.Text, "Instagram (https://t.example.com/r/")
	hrefs := map[string]bool{}
	for _, h := range htmlHrefs(t, *resp.HTML) {
		hrefs[h] = true
	}
	for _, u := range textURL.FindAllString(*resp.Text, -1) {
		assert.True(t, hrefs[u], "text URL %q is an href of the HTML", u)
	}
}

func BenchmarkCompileTemplate(b *testing.B) {
	src, err := os.ReadFile("testdata/plain-text-golden.json")
	if err != nil {
		b.Fatal(err)
	}
	tree, err := UnmarshalEmailBlock(src)
	if err != nil {
		b.Fatal(err)
	}
	for _, includeText := range []bool{false, true} {
		name := "html only"
		if includeText {
			name = "with text"
		}
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := CompileTemplate(CompileTemplateRequest{
					WorkspaceID: "ws", MessageID: "msg", VisualEditorTree: tree, IncludeText: includeText,
				}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
