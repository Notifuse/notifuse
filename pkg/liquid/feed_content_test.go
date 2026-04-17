package liquid

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeFeedHTML_AbsoluteURLRewriting(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		baseURL  string
		contains []string
		absent   []string
	}{
		{
			name:     "relative href becomes absolute",
			html:     `<a href="/posts/hello">Hi</a>`,
			baseURL:  "https://blog.example.com",
			contains: []string{`href="https://blog.example.com/posts/hello"`},
		},
		{
			name:     "relative src becomes absolute",
			html:     `<img src="/img/cat.png" alt="cat">`,
			baseURL:  "https://blog.example.com",
			contains: []string{`src="https://blog.example.com/img/cat.png"`},
		},
		{
			name:     "srcset rewritten entry-by-entry",
			html:     `<img srcset="/a.png 1x, /b.png 2x">`,
			baseURL:  "https://blog.example.com",
			contains: []string{"https://blog.example.com/a.png 1x", "https://blog.example.com/b.png 2x"},
		},
		{
			name:     "already-absolute href passes through",
			html:     `<a href="https://other.example/x">x</a>`,
			baseURL:  "https://blog.example.com",
			contains: []string{`href="https://other.example/x"`},
		},
		{
			name:    "anchor-only href passes through unchanged",
			html:    `<a href="#top">top</a>`,
			baseURL: "https://blog.example.com",
			// bluemonday may drop `id`-less anchor refs under UGC policy; we only
			// care that no accidental base URL prefix was added.
			absent: []string{"https://blog.example.com#top"},
		},
		{
			name:     "no base URL leaves relative as-is",
			html:     `<a href="/x">x</a>`,
			baseURL:  "",
			contains: []string{`href="/x"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := SanitizeFeedHTML(tt.html, tt.baseURL)
			require.NoError(t, err)
			for _, want := range tt.contains {
				assert.Contains(t, out, want, "missing %q in %q", want, out)
			}
			for _, bad := range tt.absent {
				assert.NotContains(t, out, bad, "unexpected %q in %q", bad, out)
			}
		})
	}
}

func TestSanitizeFeedHTML_XSS(t *testing.T) {
	tests := []struct {
		name       string
		html       string
		mustStrip  []string
		mustKeep   []string
	}{
		{
			name:      "script tag stripped",
			html:      `<p>hi</p><script>alert(1)</script>`,
			mustStrip: []string{"<script", "alert(1)"},
			mustKeep:  []string{"<p>hi</p>"},
		},
		{
			name:      "onerror attribute stripped",
			html:      `<img src="https://x/y.png" onerror="alert(1)">`,
			mustStrip: []string{"onerror", "alert(1)"},
			mustKeep:  []string{"https://x/y.png"},
		},
		{
			name:      "javascript: href stripped",
			html:      `<a href="javascript:alert(1)">bad</a>`,
			mustStrip: []string{"javascript:"},
		},
		{
			name:      "vbscript: href stripped",
			html:      `<a href="vbscript:msgbox(1)">bad</a>`,
			mustStrip: []string{"vbscript:"},
		},
		{
			name:      "data:text/html href stripped",
			html:      `<a href="data:text/html,<script>alert(1)</script>">bad</a>`,
			mustStrip: []string{"data:text/html"},
		},
		{
			name:      "data:image/png also stripped by the strict policy",
			html:      `<img src="data:image/png;base64,iVBORw0KGgo=">`,
			mustStrip: []string{"data:image/png"},
		},
		{
			name:     "benign markup preserved",
			html:     `<p><strong>bold</strong> <em>italic</em> <a href="https://x/">link</a></p>`,
			mustKeep: []string{"<strong>bold</strong>", "<em>italic</em>", `href="https://x/"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := SanitizeFeedHTML(tt.html, "")
			require.NoError(t, err)
			for _, bad := range tt.mustStrip {
				assert.NotContains(t, out, bad, "expected %q to be stripped from %q", bad, out)
			}
			for _, keep := range tt.mustKeep {
				assert.Contains(t, out, keep, "expected %q preserved in %q", keep, out)
			}
		})
	}
}

func TestSanitizeFeedHTML_SrcsetXSS(t *testing.T) {
	// Regression: bluemonday's srcset whitelist does not scheme-check URLs,
	// so a raw AllowAttrs("srcset") call lets `javascript:alert(1) 1x`
	// through. The goquery pre-pass must drop those entries unconditionally.
	tests := []struct {
		name      string
		html      string
		baseURL   string
		mustStrip []string
		mustKeep  []string
	}{
		{
			name:      "javascript: entry in srcset dropped",
			html:      `<img srcset="javascript:alert(1) 1x, https://ok.example/x.png 2x">`,
			mustStrip: []string{"javascript:", "alert(1)"},
			mustKeep:  []string{"https://ok.example/x.png 2x"},
		},
		{
			name:      "vbscript: entry dropped",
			html:      `<img srcset="vbscript:msgbox(1) 1x, /img/a.png 2x">`,
			baseURL:   "https://blog.example.com",
			mustStrip: []string{"vbscript:"},
			mustKeep:  []string{"https://blog.example.com/img/a.png 2x"},
		},
		{
			name:      "data: entry dropped",
			html:      `<img srcset="data:image/png;base64,AAA 1x, https://ok.example/x.png 2x">`,
			mustStrip: []string{"data:image/png"},
			mustKeep:  []string{"https://ok.example/x.png 2x"},
		},
		{
			name:      "all entries unsafe -> srcset attribute removed",
			html:      `<img src="https://ok/y.png" srcset="javascript:alert(1) 1x, vbscript:msgbox() 2x">`,
			mustStrip: []string{"srcset=", "javascript:", "vbscript:"},
			mustKeep:  []string{`src="https://ok/y.png"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := SanitizeFeedHTML(tt.html, tt.baseURL)
			require.NoError(t, err)
			for _, bad := range tt.mustStrip {
				assert.NotContains(t, out, bad, "expected %q stripped from %q", bad, out)
			}
			for _, keep := range tt.mustKeep {
				assert.Contains(t, out, keep, "expected %q preserved in %q", keep, out)
			}
		})
	}
}

func TestSanitizeFeedHTML_ControlChars(t *testing.T) {
	// Build input with NUL (U+0000), BEL (U+0007), and VT (U+000B) — all
	// illegal in XML 1.0. Tabs and newlines must survive.
	in := "<p>ok\x00bad\x07more\x0b\tkeep\nnext</p>"
	out, err := SanitizeFeedHTML(in, "")
	require.NoError(t, err)

	assert.NotContains(t, out, "\x00")
	assert.NotContains(t, out, "\x07")
	assert.NotContains(t, out, "\x0b")
	assert.Contains(t, out, "\t")
	assert.Contains(t, out, "\n")
	// Expected text survives with control chars removed.
	assert.Contains(t, strings.ReplaceAll(out, "\n", ""), "okbadmore\tkeepnext")
}

func TestSanitizeFeedHTML_InvalidBaseURL(t *testing.T) {
	// Non-absolute baseURL is treated as missing — relative links stay put.
	out, err := SanitizeFeedHTML(`<a href="/x">x</a>`, "not-a-url")
	require.NoError(t, err)
	assert.Contains(t, out, `href="/x"`)
}
