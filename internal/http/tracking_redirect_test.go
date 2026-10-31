package http

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	classifyReqHost  = "Track.Brand.com:8443"
	classifyAPIHost  = "api.notifuse.test"
	classifyEndpoint = "links.brand.com"
)

func TestClassifyTrackedDestination(t *testing.T) {
	type want struct {
		decision trackedDecision
		target   string // for redirects
		link     bool   // for interstitials: offers Continue
	}
	cases := map[string]want{
		// Redirect: the installation's own hosts.
		"https://track.brand.com/u":          {decision: decisionRedirect, target: "https://track.brand.com/u"},
		"https://TRACK.BRAND.COM:9999/u":     {decision: decisionRedirect, target: "https://TRACK.BRAND.COM:9999/u"},
		"https://api.notifuse.test/nc?x=1":   {decision: decisionRedirect, target: "https://api.notifuse.test/nc?x=1"},
		"http://links.brand.com/u":           {decision: decisionRedirect, target: "http://links.brand.com/u"},
		" https://track.brand.com/u ":        {decision: decisionRedirect, target: "https://track.brand.com/u"},
		"https://track.brand.com/u\n":        {decision: decisionRedirect, target: "https://track.brand.com/u"},
		"https://track.brand.com/a\tb":       {decision: decisionRedirect, target: "https://track.brand.com/ab"},
		"//track.brand.com/u":                {decision: decisionRedirect, target: "https://track.brand.com/u"},
		"https://track.brand.com/50%off":     {decision: decisionRedirect, target: "https://track.brand.com/50%25off"},
		"https://track.brand.com/u?q=%zz&x=": {decision: decisionRedirect, target: "https://track.brand.com/u?q=%zz&x="}, // net/url keeps the query verbatim

		// Not one of the installation's hosts: interstitial with Continue.
		"https://evil.example/x":         {decision: decisionInterstitial, link: true},
		"https://brand.com/":             {decision: decisionInterstitial, link: true}, // the website gets no exemption
		"https://track.brand.com./u":     {decision: decisionInterstitial, link: true}, // trailing dot
		"https://trac\u212a.brand.com/u": {decision: decisionInterstitial, link: true}, // Kelvin sign: not the request host, shown as Punycode
		"//shop.com/promo":               {decision: decisionInterstitial, link: true},
		"https://shop.com/50%off":        {decision: decisionInterstitial, link: true},
		"https://my_host.example.com/x":  {decision: decisionInterstitial, link: true},
		"http://[::1]:8080/x":            {decision: decisionInterstitial, link: true},
		"https://evil.track.brand.com/u": {decision: decisionInterstitial, link: true},
		"https://track.brand.com.evil/u": {decision: decisionInterstitial, link: true},
		"https://xn--brnd-4qa.com/":      {decision: decisionInterstitial, link: true},
		"https://br\u0430nd.com/":        {decision: decisionInterstitial, link: true}, // Cyrillic a, shown as Punycode

		// Interstitial without a link.
		"https://track.brand.com@evil.com/":    {decision: decisionInterstitial},
		"https://track.brand.com%2f@evil.com/": {decision: decisionInterstitial},
		`https:\\evil.com`:                     {decision: decisionInterstitial},
		`https:/\evil.com`:                     {decision: decisionInterstitial},
		`https://evil.com\@track.brand.com`:    {decision: decisionInterstitial},
		"myapp://open":                         {decision: decisionInterstitial},
		"ms-msdt:/id x":                        {decision: decisionInterstitial},
		"search-ms:query=x":                    {decision: decisionInterstitial},
		"mailto:a@b.c":                         {decision: decisionInterstitial},

		// Rejected.
		"":                   {decision: decisionReject},
		"   ":                {decision: decisionReject},
		"?x=1":               {decision: decisionReject},
		"#y":                 {decision: decisionReject},
		"/pricing":           {decision: decisionReject},
		`/\evil.com`:         {decision: decisionReject},
		"pricing":            {decision: decisionReject},
		"javascript:x":       {decision: decisionReject},
		"JavaScript:x":       {decision: decisionReject},
		" javascript:x":      {decision: decisionReject},
		"java\tscript:x":     {decision: decisionReject},
		"java\nscript:%zz":   {decision: decisionReject},
		"data:text/html,x":   {decision: decisionReject},
		"vbscript:x":         {decision: decisionReject},
		"file:///etc/passwd": {decision: decisionReject},
		"blob:https://x/y":   {decision: decisionReject},
	}

	for raw, w := range cases {
		t.Run(raw, func(t *testing.T) {
			decision, target, parsed, _ := classifyTrackedDestination(raw, classifyReqHost, classifyAPIHost, classifyEndpoint)
			require.Equal(t, w.decision, decision, "decision for %q", raw)
			switch w.decision {
			case decisionRedirect:
				assert.Equal(t, w.target, target)
			case decisionInterstitial:
				rec := httptest.NewRecorder()
				_, _, scheme := normalizeDestination(raw)
				writeRedirectInterstitial(rec, parsed, scheme)
				assert.Equal(t, w.link, strings.Contains(rec.Body.String(), "<a "), "Continue link for %q", raw)
			}
		})
	}
}

func TestClassifyTrackedDestination_NoEndpointHostWhenUnknown(t *testing.T) {
	decision, _, _, _ := classifyTrackedDestination("https://links.brand.com/u", classifyReqHost, classifyAPIHost, "")
	assert.Equal(t, decisionInterstitial, decision, "the custom endpoint only counts for the token's own workspace")
	decision, _, _, _ = classifyTrackedDestination("https://anything/u", "", "", "")
	assert.Equal(t, decisionInterstitial, decision, "empty own hosts never match")
}

func TestAsciiHostEqual(t *testing.T) {
	assert.True(t, asciiHostEqual("Brand.COM", "brand.com"))
	assert.False(t, asciiHostEqual("\u212aey.com", "key.com"), "the Kelvin sign is not k")
	assert.False(t, asciiHostEqual("", ""))
	assert.False(t, asciiHostEqual("brand.com.", "brand.com"))
}

func TestDisplayHost(t *testing.T) {
	for _, h := range []string{"brand.com", "my_host.example.com", "r3---sn-abc.googlevideo.com", "::1"} {
		got, ok := displayHost(h)
		assert.True(t, ok, h)
		assert.Equal(t, h, got)
	}

	got, ok := displayHost("br\u0430nd.com")
	assert.True(t, ok)
	assert.True(t, strings.HasPrefix(got, "xn--"), got)

	_, ok = displayHost("")
	assert.False(t, ok)
	_, ok = displayHost("xn--zz\u00e9.com")
	assert.False(t, ok, "a host Punycode cannot convert is not displayable")
}

func TestWriteRedirectInterstitial(t *testing.T) {
	assertHeaders := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		assert.Equal(t, 200, rec.Code)
		assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
		assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		assert.Equal(t, "noindex", rec.Header().Get("X-Robots-Tag"))
		assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
		assert.Equal(t, "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'", rec.Header().Get("Content-Security-Policy"))
		body := rec.Body.String()
		assert.NotContains(t, body, "http-equiv")
		assert.NotContains(t, body, "<script")
	}

	t.Run("web destination: hostname and Continue", func(t *testing.T) {
		u, err := url.Parse("https://shop.example.com/path?a=1&b=<x>")
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		writeRedirectInterstitial(rec, u, "https")
		assertHeaders(t, rec)
		body := rec.Body.String()
		assert.Contains(t, body, "shop.example.com")
		assert.Contains(t, body, `href="https://shop.example.com/path?a=1&amp;b=%3cx%3e"`)
		assert.Contains(t, body, `rel="noopener noreferrer"`)
		assert.Equal(t, 1, strings.Count(body, "<a "))
	})

	t.Run("homograph host shown as Punycode", func(t *testing.T) {
		u, err := url.Parse("https://br\u0430nd.com/")
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		writeRedirectInterstitial(rec, u, "https")
		body := rec.Body.String()
		assert.Contains(t, body, "xn--")
		assert.NotContains(t, body, "br\u0430nd")
	})

	const marker = "CALL-SUPPORT-NOW"
	noLink := map[string]string{
		"userinfo":         "https://brand.com@evil.com/" + marker,
		"app scheme":       "myapp://" + marker,
		"scheme no host":   `https:\\evil.com\` + marker,
		"protocol handler": "ms-msdt:/id " + marker,
		"unparsable":       `https://evil.com\@brand.com/` + marker,
	}
	for name, raw := range noLink {
		t.Run("no link: "+name, func(t *testing.T) {
			_, parsed, scheme := normalizeDestination(raw)
			rec := httptest.NewRecorder()
			writeRedirectInterstitial(rec, parsed, scheme)
			assertHeaders(t, rec)
			body := rec.Body.String()
			assert.NotContains(t, body, "<a ")
			assert.NotContains(t, body, marker, "the URL is never echoed")
			assert.NotContains(t, body, "evil")
			assert.NotContains(t, body, "brand.com")
			if scheme != "" {
				assert.Contains(t, body, scheme+":")
			}
		})
	}

	t.Run("nil URL, no scheme", func(t *testing.T) {
		rec := httptest.NewRecorder()
		writeRedirectInterstitial(rec, nil, "")
		assertHeaders(t, rec)
		assert.NotContains(t, rec.Body.String(), "<a ")
	})
}

func TestRepairPercentEscapes(t *testing.T) {
	assert.Equal(t, "50%25off", repairPercentEscapes("50%off"))
	assert.Equal(t, "%41%25", repairPercentEscapes("%41%"))
	assert.Equal(t, "%252", repairPercentEscapes("%2"))
}
