package liquid

import (
	"bytes"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
)

// SanitizeFeedHTML prepares post body HTML for emission inside an RSS
// `<content:encoded>` block or a JSON Feed `content_html` field.
//
// Steps:
//  1. Rewrite relative href/src/srcset to absolute URLs against baseURL.
//  2. Run through a bluemonday policy that strips <script>, on* handlers,
//     and javascript:/vbscript: URI schemes. data: URIs survive only when
//     the MIME type is image/*.
//  3. Strip C0 control characters that XML 1.0 forbids (keeping TAB/LF/CR).
//
// A blank baseURL is allowed — in that case relative URLs pass through
// untouched. The policy is constructed once per call because bluemonday
// policies are not thread-hostile but mutation during use is unsafe.
func SanitizeFeedHTML(html, baseURL string) (string, error) {
	var base *url.URL
	if baseURL != "" {
		u, err := url.Parse(baseURL)
		if err == nil && u.IsAbs() {
			base = u
		}
	}

	// Run the HTML pass unconditionally: even without a base URL we still need
	// to scrub srcset entries whose URLs use javascript:/vbscript:/data: —
	// bluemonday does not scheme-check srcset values by default.
	rewritten, err := rewriteRelativeURLs(html, base)
	if err != nil {
		return "", err
	}
	html = rewritten

	html = feedSanitizer().Sanitize(html)
	html = stripControlChars(html)
	return html, nil
}

// safeURLSchemes lists the URL schemes permitted in feed content. Matches
// the bluemonday policy in feedSanitizer() so that srcset (which bluemonday
// does not validate) and other pre-sanitizer rewrites stay consistent.
var safeURLSchemes = map[string]struct{}{
	"http":   {},
	"https":  {},
	"mailto": {},
	"tel":    {},
}

// isSafeURL reports whether a URL string resolves to a scheme in the
// safeURLSchemes set. Relative URLs (no scheme) are considered safe.
func isSafeURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme == "" {
		return true
	}
	_, ok := safeURLSchemes[strings.ToLower(u.Scheme)]
	return ok
}

func rewriteRelativeURLs(html string, base *url.URL) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	if base != nil {
		absolutize := func(attr string) func(int, *goquery.Selection) {
			return func(_ int, s *goquery.Selection) {
				v, ok := s.Attr(attr)
				if !ok || v == "" {
					return
				}
				if abs, ok := absoluteOrNil(v, base); ok {
					s.SetAttr(attr, abs)
				}
			}
		}

		doc.Find("[href]").Each(absolutize("href"))
		doc.Find("[src]").Each(absolutize("src"))
	}

	// srcset pass always runs — even with no base URL, we must drop entries
	// whose URL scheme is not on safeURLSchemes. bluemonday's srcset
	// whitelist does not scheme-check values.
	doc.Find("[srcset]").Each(func(_ int, s *goquery.Selection) {
		v, _ := s.Attr("srcset")
		if v == "" {
			return
		}
		cleaned := cleanSrcset(v, base)
		if cleaned == "" {
			s.RemoveAttr("srcset")
			return
		}
		s.SetAttr("srcset", cleaned)
	})

	// goquery wraps fragments in <html><head></head><body>...</body></html>.
	// For feed content we want just the body contents back.
	body := doc.Find("body")
	if body.Length() > 0 {
		out, err := body.Html()
		if err != nil {
			return "", err
		}
		return out, nil
	}
	return doc.Html()
}

func absoluteOrNil(raw string, base *url.URL) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	// Leave anchors, javascript:, and mailto: alone — sanitizer will handle
	// the dangerous ones.
	if strings.HasPrefix(raw, "#") {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.IsAbs() {
		return "", false
	}
	return base.ResolveReference(u).String(), true
}

// cleanSrcset parses a `srcset` value into comma-separated candidates,
// drops entries whose URL uses an unsafe scheme, and optionally rewrites
// relative URLs against base. Returns "" if no candidates survive.
func cleanSrcset(srcset string, base *url.URL) string {
	parts := strings.Split(srcset, ",")
	kept := parts[:0]
	for _, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		raw := fields[0]
		if !isSafeURL(raw) {
			continue
		}
		if base != nil {
			if abs, ok := absoluteOrNil(raw, base); ok {
				fields[0] = abs
			}
		}
		kept = append(kept, strings.Join(fields, " "))
	}
	return strings.Join(kept, ", ")
}

func feedSanitizer() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowImages()
	p.AllowStandardURLs()
	// AllowImages() whitelists src/alt/width/height but not srcset.
	p.AllowAttrs("srcset").OnElements("img", "source")
	p.AllowAttrs("loading").Matching(bluemonday.SpaceSeparatedTokens).OnElements("img")
	// Restrict URL schemes to safe ones. `data:` is dropped entirely — most
	// feed readers don't render base64 images anyway, and allowing `data:`
	// opens a wide surface (e.g., `data:text/html,...`).
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	return p
}

// stripControlChars removes XML 1.0-illegal control characters (U+0000–U+001F
// except TAB/LF/CR) so aggregator parsers don't choke.
func stripControlChars(s string) string {
	if !strings.ContainsFunc(s, isIllegalXMLControl) {
		return s
	}
	var b bytes.Buffer
	b.Grow(len(s))
	for _, r := range s {
		if isIllegalXMLControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isIllegalXMLControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return r < 0x20
}
