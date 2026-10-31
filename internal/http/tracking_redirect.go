package http

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/idna"

	"github.com/Notifuse/notifuse/pkg/trackingtoken"
)

// trackedDecision is what the click routes do with a destination they cannot verify.
type trackedDecision int

const (
	// decisionReject answers 400 and records nothing.
	decisionReject trackedDecision = iota
	// decisionRedirect answers 303 to the target.
	decisionRedirect
	// decisionInterstitial answers a confirmation page and never redirects by itself.
	decisionInterstitial
)

// blockedSchemes are the destinations no tracked link may lead to, signed or not.
var blockedSchemes = map[string]bool{
	"javascript": true,
	"data":       true,
	"vbscript":   true,
	"file":       true,
	"blob":       true,
}

// normalizeDestination prepares a destination that could not be verified: WHATWG
// preprocessing (TrackLinks mints an href as written, and before signing net/http trimmed
// it on the way out, so " https://brand.com/sale " must keep working), invalid percent
// escapes repaired, and a protocol-relative URL read as https. It returns the
// preprocessed string, the parsed URL (nil when it still does not parse) and the
// lowercase scheme, read leniently when the URL does not parse.
func normalizeDestination(raw string) (string, *url.URL, string) {
	s := trackingtoken.Preprocess(raw)
	if s == "" {
		return "", nil, ""
	}
	u, err := url.Parse(s)
	if err != nil {
		u, err = url.Parse(repairPercentEscapes(s))
		if err != nil {
			u = nil
		}
	}
	if u != nil && u.Scheme == "" && u.Host != "" {
		u.Scheme = "https"
	}
	scheme := trackingtoken.SchemeOf(s)
	if u != nil {
		scheme = strings.ToLower(u.Scheme)
	}
	return s, u, scheme
}

// repairPercentEscapes rewrites every '%' not followed by two hex digits as "%25", so a
// URL such as https://shop.com/50%off parses.
func repairPercentEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && (i+2 >= len(s) || !isHex(s[i+1]) || !isHex(s[i+2])) {
			b.WriteString("%25")
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// classifyTrackedDestination decides what a click route does with a destination it could
// not verify:
//  1. reject when it is empty, relative (no scheme and no host), or uses a blocked scheme.
//     Relative destinations are never resolved: in an email they never pointed anywhere,
//     and resolving "?x" against the request would loop back to the same link;
//  2. redirect to u.String() when it is http(s) with a host, no userinfo, and a hostname
//     that is one of the installation's own: the request's, the API endpoint's, or the
//     workspace's custom endpoint. This keeps wrapped unsubscribe and notification
//     center links working. u.String() is what was checked, never the raw string;
//  3. otherwise, the interstitial.
//
// There is deliberately no exemption for the workspace's website or web analytics
// domains: wildcards over hosting suffixes, shared platforms and scheme-less website
// values would each turn it into an open redirect.
func classifyTrackedDestination(raw, requestHost, apiHost, endpointHost string) (trackedDecision, string, *url.URL, string) {
	s, u, scheme := normalizeDestination(raw)
	if s == "" || blockedSchemes[scheme] {
		return decisionReject, "", nil, scheme
	}
	if u != nil && u.Scheme == "" && u.Host == "" {
		return decisionReject, "", nil, scheme
	}
	if isPlainWebURL(u) {
		host := u.Hostname()
		for _, own := range []string{hostnameOf(requestHost), apiHost, endpointHost} {
			if asciiHostEqual(host, own) {
				return decisionRedirect, u.String(), u, scheme
			}
		}
	}
	return decisionInterstitial, "", u, scheme
}

// isPlainWebURL reports whether u is an http(s) URL with a host and no userinfo: the
// only destinations a visitor is ever offered to continue to.
func isPlainWebURL(u *url.URL) bool {
	return u != nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.Hostname() != "" && u.User == nil
}

// hostnameOf strips the port from a host[:port] value such as a request's Host.
func hostnameOf(hostport string) string {
	return (&url.URL{Host: hostport}).Hostname()
}

// asciiHostEqual compares two hostnames, ports already removed, folding ASCII letters
// only: strings.EqualFold would also fold, for instance, the Kelvin sign onto 'k'.
func asciiHostEqual(a, b string) bool {
	if a == "" || len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if lowerASCII(a[i]) != lowerASCII(b[i]) {
			return false
		}
	}
	return true
}

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// displayHost returns the hostname to show on the interstitial: an all-ASCII hostname as
// it is (idna.Lookup would reject valid hosts such as my_host.example.com), a non-ASCII
// one in Punycode so a homograph shows as xn--…, and false when that conversion fails.
func displayHost(hostname string) (string, bool) {
	if hostname == "" {
		return "", false
	}
	ascii := true
	for i := 0; i < len(hostname); i++ {
		if hostname[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return hostname, true
	}
	converted, err := idna.Punycode.ToASCII(hostname)
	if err != nil || converted == "" {
		return "", false
	}
	return converted, true
}

var redirectInterstitialTemplate = template.Must(template.New("interstitial").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>Leaving this link</title>
<style>
body{margin:0;padding:0 16px;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;background:#f5f5f5;color:#1f1f1f}
main{box-sizing:border-box;max-width:480px;margin:15vh auto 0;padding:32px 24px;background:#fff;border-radius:8px;box-shadow:0 1px 3px rgba(0,0,0,.08)}
h1{font-size:20px;margin:0 0 16px}
p{line-height:1.5;margin:0 0 16px}
.host{font-size:18px;font-weight:600;word-break:break-all}
a.continue{display:inline-block;padding:10px 20px;background:#1f1f1f;color:#fff;border-radius:6px;text-decoration:none}
</style>
</head>
<body>
<main>
{{if .Host}}<h1>You are leaving this link</h1>
<p>This link leads to:</p>
<p class="host">{{.Host}}</p>
<p>Continue only if you trust this site.</p>
<p><a class="continue" href="{{.Href}}" rel="noopener noreferrer">Continue</a></p>
{{else}}<h1>This link could not be checked</h1>
<p>The destination of this link could not be verified, so it is not opened.</p>
{{if .Scheme}}<p>It leads to an address of type <strong>{{.Scheme}}:</strong></p>
{{end}}{{end}}</main>
</body>
</html>
`))

// writeRedirectInterstitial answers the page shown instead of a redirect to a destination
// that could not be verified. A plain web destination gets its hostname and a Continue
// link; anything else (userinfo, an app scheme, no host, unparsable) gets no link and at
// most its scheme, never the URL itself, which an attacker could fill with a fake message.
func writeRedirectInterstitial(w http.ResponseWriter, u *url.URL, scheme string) {
	data := struct {
		Host   string
		Href   template.URL
		Scheme string
	}{}
	if isPlainWebURL(u) {
		if host, ok := displayHost(u.Hostname()); ok {
			data.Host = host
			// html/template escapes the attribute; template.URL only stops it from
			// replacing a value it does not recognise as safe, which isPlainWebURL ensured.
			data.Href = template.URL(u.String())
		}
	}
	if data.Host == "" && isDisplayableScheme(scheme) {
		data.Scheme = scheme
	}

	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Robots-Tag", "noindex")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_ = redirectInterstitialTemplate.Execute(w, data)
}

// isDisplayableScheme reports whether scheme is short and made of scheme characters
// only, so showing it cannot carry a message.
func isDisplayableScheme(scheme string) bool {
	if scheme == "" || len(scheme) > 32 {
		return false
	}
	return trackingtoken.SchemeOf(scheme+":") == scheme
}
