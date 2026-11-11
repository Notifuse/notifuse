package plaintext

import (
	"html"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromHTML(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"empty input", "", ""},
		{"whitespace only", "   \n\t  ", ""},
		{"image only without alt is empty, not a newline", `<img src="pixel.gif" width="1" height="1">`, ""},
		{
			"head, title, style and script are skipped",
			`<html><head><title>T</title><style>p{color:red}</style></head><body><p>Hi</p><script>track()</script></body></html>`,
			"Hi\n",
		},
		{
			"hidden preheader and its padding characters are skipped",
			`<div style="display:none;font-size:1px;max-height:0">Preview text&zwnj;&nbsp;&#847;&zwnj;</div><p>Body</p>`,
			"Body\n",
		},
		{"visibility hidden is skipped", `<span style="visibility: hidden">x</span><p>Body</p>`, "Body\n"},
		{"hidden attribute is skipped", `<div hidden>x</div><p>Body</p>`, "Body\n"},
		{
			"mso-hide only hides in Outlook, so the link is kept",
			`<a href="https://e.com/buy" style="mso-hide:all;color:red">Buy</a>`,
			"Buy (https://e.com/buy)\n",
		},
		{
			"non-content elements are skipped",
			`<p>A</p><noscript>N</noscript><iframe>I</iframe><svg><text>S</text></svg><template>T</template><math>M</math><object>O</object><input value="v"><select><option>Opt</option></select><textarea>Ta</textarea><p>B</p>`,
			"A\n\nB\n",
		},
		{"paragraphs are separated by one blank line", `<p>One</p><p>Two</p>`, "One\n\nTwo\n"},
		{"empty blocks never add a second blank line", `<p>One</p><p></p><div></div><p> </p><p>Two</p>`, "One\n\nTwo\n"},
		{"br is a line break", `<p>Line 1<br>Line 2<br/>Line 3</p>`, "Line 1\nLine 2\nLine 3\n"},
		{"heading is followed by a blank line", `<h1>Title</h1><p>Text</p>`, "Title\n\nText\n"},
		{"inline whitespace collapses", "<p>Hello   <strong>big</strong>\n world</p>", "Hello big world\n"},
		{"link gives label and url", `<p>Read <a href="https://e.com/a">the post</a> now</p>`, "Read the post (https://e.com/a) now\n"},
		{"link whose label is its url gives the url once", `<a href="https://e.com/a">https://e.com/a</a>`, "https://e.com/a\n"},
		{"link whose label is another url gives the href", `<a href="https://t.e.com/r/tok">https://e.com/a</a>`, "https://t.e.com/r/tok\n"},
		{
			"adjacent anchors never glue a url to the next label",
			`<div><a href="https://e.com/n1">Nav1</a><a href="https://e.com/n2">Nav2</a><a href="https://e.com/n3">Nav3</a></div>`,
			"Nav1 (https://e.com/n1) Nav2 (https://e.com/n2) Nav3 (https://e.com/n3)\n",
		},
		{
			"adjacent bare urls are separated",
			`<a href="https://e.com/1"><img src="x.png"></a><a href="https://e.com/2"><img src="y.png"></a>`,
			"https://e.com/1 https://e.com/2\n",
		},
		{
			"punctuation right after a link stays attached",
			`<p>Read <a href="https://e.com/a">the post</a>. Then <a href="https://e.com/b">https://e.com/b</a>, ok</p>`,
			"Read the post (https://e.com/a). Then https://e.com/b, ok\n",
		},
		{"mailto with the address as label", `<a href="mailto:help@e.com">help@e.com</a>`, "help@e.com\n"},
		{"mailto drops its query", `<a href="mailto:help@e.com?subject=Hi">Write to us</a>`, "Write to us (help@e.com)\n"},
		{"tel", `<a href="tel:+33123456789">Call</a>`, "Call (+33123456789)\n"},
		{"other schemes keep the label only", `<a href="sms:+331">Text us</a>`, "Text us\n"},
		{
			"fragment, missing and javascript hrefs keep the label only",
			`<p><a href="#">One</a> <a>Two</a> <a href="javascript:void(0)">Three</a></p>`,
			"One Two Three\n",
		},
		{"linked image uses its alt as label", `<a href="https://e.com"><img src="logo.png" alt="Acme"></a>`, "Acme (https://e.com)\n"},
		{"standalone images give their alt or nothing", `<p><img src="a.png" alt="A chart"></p><p><img src="pixel.gif" alt=""></p>`, "A chart\n"},
		{"unordered list", `<ul><li>One</li><li>Two</li></ul>`, "- One\n- Two\n"},
		{"ordered list", `<ol><li>One</li><li>Two</li></ol>`, "1. One\n2. Two\n"},
		{"nested list is indented", `<ul><li>One<ul><li>Sub</li></ul></li><li>Two</li></ul>`, "- One\n  - Sub\n- Two\n"},
		{"list items written as paragraphs, as the editor stores them", `<ul><li><p>One</p></li><li><p>Two</p></li></ul>`, "- One\n- Two\n"},
		{"ordered list honours start", `<ol start="3"><li><p>Three</p></li><li><p>Four</p></li></ol>`, "3. Three\n4. Four\n"},
		{
			"nested list of paragraphs keeps its indent",
			`<ul><li><p>One</p></li><li><p>Two</p><ul><li><p>Sub</p></li></ul></li></ul>`,
			"- One\n- Two\n  - Sub\n",
		},
		{"second paragraph of an item is indented under it", `<ol><li><p>First</p><p>More</p></li></ol>`, "1. First\n   More\n"},
		{"item holding a div", `<ul><li><div>One</div></li></ul>`, "- One\n"},
		{"empty item writes nothing", `<ul><li></li><li>Two</li></ul>`, "- Two\n"},
		{"two br leave a blank line", `<p>Line 1<br><br>Line 2</p>`, "Line 1\n\nLine 2\n"},
		{
			"punctuation glued to a following url is separated",
			`<p><a href="https://e.com/a">https://e.com/a</a>,<a href="https://e.com/b">https://e.com/b</a></p>`,
			"https://e.com/a, https://e.com/b\n",
		},
		{"a query-like word after a bare url is separated", `<p><a href="https://e.com/a">https://e.com/a</a>?x=1</p>`, "https://e.com/a ?x=1\n"},
		{
			// With click tracking the icon and the label of a social element get different
			// URLs: both are kept, since nothing in the HTML says they lead to the same place.
			"an icon next to a labelled link with another url keeps both",
			`<table><tr><td><table><tr><td><a href="https://t.e.com/r/1"><img src="fb.png" alt=""></a></td></tr></table></td><td><a href="https://t.e.com/r/2">Facebook</a></td></tr></table>`,
			"https://t.e.com/r/1\n\nFacebook (https://t.e.com/r/2)\n",
		},
		{
			"a logo next to a view-in-browser link keeps both",
			`<table><tr><td><a href="https://acme.example/"><img src="logo.png"></a></td><td><a href="https://acme.example/view/123">View in browser</a></td></tr></table>`,
			"https://acme.example/\nView in browser (https://acme.example/view/123)\n",
		},
		{
			// Autolinkers read a typographic closing quote as part of the URL, so it is
			// kept apart; an ASCII quote is left out of the link and stays attached.
			"a typographic closing quote is kept apart from the url before it",
			"<p>\u201cSee the <a href=\"https://e.com/faq\">FAQ</a>.\u201d</p>",
			"\u201cSee the FAQ (https://e.com/faq) .\u201d\n",
		},
		{"a bare url before \u201d is kept apart", "<p>\u201c<a href=\"https://e.com/a\">https://e.com/a</a>\u201d</p>", "\u201chttps://e.com/a \u201d\n"},
		{"a bare url before \u2019 is kept apart", "<p>\u2018<a href=\"https://e.com/a\">https://e.com/a</a>\u2019</p>", "\u2018https://e.com/a \u2019\n"},
		{"a bare url before \u00bb is kept apart", "<p>\u00ab<a href=\"https://e.com/a\">https://e.com/a</a>\u00bb</p>", "\u00abhttps://e.com/a \u00bb\n"},
		{"a bare url before an ascii quote keeps it attached", `<p>"<a href="https://e.com/a">https://e.com/a</a>"</p>`, "\"https://e.com/a\"\n"},
		{
			"icon links side by side are all kept",
			`<table><tr><td><a href="https://e.com/fb"><img src="fb.png"></a></td><td><a href="https://e.com/x"><img src="x.png"></a></td></tr></table>`,
			"https://e.com/fb\nhttps://e.com/x\n",
		},
		{"blockquote lines are prefixed", `<blockquote><p>Quoted line</p><p>Second</p></blockquote>`, "> Quoted line\n\n> Second\n"},
		{"pre keeps its whitespace", "<pre>a  b\n  c</pre>", "a  b\n  c\n"},
		{"table cells are on their own lines", `<table><tr><td>Left</td><td>Right</td></tr></table>`, "Left\nRight\n"},
		{"entities are decoded and nbsp is a space", `<p>Fish &amp; chips&nbsp;for&#160;two</p>`, "Fish & chips for two\n"},
		{"zero-width and soft hyphen characters are removed", "<p>A​‌B­</p>", "AB\n"},
		{"outlook conditional comment is skipped", `<!--[if mso]><p>Outlook only</p><![endif]--><p>All</p>`, "All\n"},
		{
			"an icon link followed by the same link as text is written once",
			`<table><tr><td><a href="https://e.com/fb"><img src="fb.png" alt=""></a></td></tr><tr><td><a href="https://e.com/fb">Facebook</a></td></tr></table>`,
			"Facebook (https://e.com/fb)\n",
		},
		{"a bare url line followed by another link is kept", `<p><a href="https://e.com/a">https://e.com/a</a></p><p><a href="https://e.com/b">B</a></p>`, "https://e.com/a\n\nB (https://e.com/b)\n"},
		{"hr is a separator line", `<p>A</p><hr><p>B</p>`, "A\n\n" + separator + "\n\nB\n"},
		// An href still holding Liquid only reaches the converter from a compile that kept
		// Liquid (the seed of a custom text): it is written as a destination, so the seed
		// keeps links such as the unsubscribe link.
		{"a liquid href with a label is written as a destination", `<p><a href="{{ unsubscribe_url }}">Unsubscribe</a></p>`, "Unsubscribe ({{ unsubscribe_url }})\n"},
		{"a liquid href without spaces is written as a destination", `<a href="{{url}}">Open</a>`, "Open ({{url}})\n"},
		{"a liquid href with a path is written as a destination", `<a href="{{ workspace.base_url }}/x">Site</a>`, "Site ({{ workspace.base_url }}/x)\n"},
		{"a liquid tag href is written as a destination", `<a href="{% if a %}https://e.com/a{% endif %}">A</a>`, "A ({% if a %}https://e.com/a{% endif %})\n"},
		{"a liquid href equal to its label is written once", `<a href="{{ unsubscribe_url }}">{{ unsubscribe_url }}</a>`, "{{ unsubscribe_url }}\n"},
		{"a linked image with a liquid href keeps its alt and destination", `<a href="{{ shop_url }}"><img src="logo.png" alt="Shop"></a>`, "Shop ({{ shop_url }})\n"},
		{"liquid inside a web url is unchanged", `<a href="https://e.com/?id={{ contact.id }}">Profile</a>`, "Profile (https://e.com/?id={{ contact.id }})\n"},
		{"liquid tags in text are kept", `<p>{% if vip %}Hello VIP{% endif %}</p>`, "{% if vip %}Hello VIP{% endif %}\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FromHTML(tt.html))
		})
	}
}

func TestFromHTML_InvalidUTF8IsReplaced(t *testing.T) {
	got := FromHTML("<p>caf\xe9</p>")
	assert.True(t, utf8.ValidString(got))
	assert.Equal(t, "caf�\n", got)
}

func TestFromHTML_Fixture(t *testing.T) {
	src, err := os.ReadFile("testdata/newsletter-links.html")
	require.NoError(t, err)

	got := FromHTML(string(src))

	assert.NotEmpty(t, got)
	assert.NotContains(t, got, "<td")
	assert.NotContains(t, got, "\n\n\n")
	assert.Contains(t, got, "https://example.com/")
}

func FuzzFromHTML(f *testing.F) {
	for _, seed := range []string{
		"",
		"<p>Hello</p>",
		`<a href="https://e.com">x</a><a href="https://e.com/2">y</a>`,
		"<ul><li>a<ol><li>b</li></ol></li></ul>",
		"<blockquote><pre> x\n y</pre></blockquote>",
		"\xff\xfe<br>",
	} {
		f.Add(seed)
	}

	const marker = "zqxjscriptmarker"
	f.Fuzz(func(t *testing.T, s string) {
		out := FromHTML(s)
		if !utf8.ValidString(out) {
			t.Fatalf("invalid UTF-8 output for %q", s)
		}
		if strings.Contains(out, "\n\n\n") {
			t.Fatalf("more than one blank line in a row for %q", s)
		}
		if out != "" && !strings.HasSuffix(out, "\n") {
			t.Fatalf("non-empty output without final newline for %q", s)
		}
		if strings.TrimSpace(out) == "" && out != "" {
			t.Fatalf("whitespace-only output for %q", s)
		}

		// Arbitrary text followed by a script: the script never reaches the text.
		if strings.Contains(s, marker) {
			return
		}
		withScript := FromHTML("<p>" + html.EscapeString(s) + "</p><script>" + marker + "</script>")
		if strings.Contains(withScript, marker) {
			t.Fatalf("script content leaked for %q", s)
		}
	})
}

func BenchmarkFromHTML(b *testing.B) {
	src, err := os.ReadFile("testdata/newsletter-links.html")
	if err != nil {
		b.Fatal(err)
	}
	s := string(src)
	b.SetBytes(int64(len(s)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FromHTML(s)
	}
}
