package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForwardedHosts(t *testing.T) {
	cases := map[string]struct {
		xfh, fwd []string
		want     []string
	}{
		"none": {},
		"X-Forwarded-Host entries, trimmed, empty ones skipped": {
			xfh:  []string{" a.example , ,b.example", ""},
			want: []string{"a.example", "b.example"},
		},
		"Forwarded host parameters, quoted or not": {
			fwd:  []string{`for=1.2.3.4;host=a.example, host="b.example:8443";proto=https`},
			want: []string{"a.example", "b.example:8443"},
		},
		"the parameter name is case-insensitive": {
			fwd:  []string{"for=1.2.3.4;Host=a.example", "HOST = b.example"},
			want: []string{"a.example", "b.example"},
		},
		"a quoted value keeps its comma and semicolon, escapes read": {
			fwd:  []string{`host="a.example,x;y", host="b\"c"`},
			want: []string{"a.example,x;y", `b"c`},
		},
		"empty values and elements without a host are skipped": {
			fwd:  []string{`host=, host="", for=1.2.3.4, ;;, host=c.example`},
			want: []string{"c.example"},
		},
		"X-Forwarded-Host entries come first": {
			xfh:  []string{"x.example"},
			fwd:  []string{"host=f.example"},
			want: []string{"x.example", "f.example"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := http.Header{}
			for _, v := range tc.xfh {
				h.Add("X-Forwarded-Host", v)
			}
			for _, v := range tc.fwd {
				h.Add("Forwarded", v)
			}
			assert.Equal(t, tc.want, forwardedHosts(h))
		})
	}
}

func TestClickOwnHosts_KeepsTheLastForwardedHosts(t *testing.T) {
	names := func(prefix string, n int) []string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, prefix+string(rune('a'+i))+".example")
		}
		return out
	}

	t.Run("in one header, a client sending many entries cannot push the proxies' out", func(t *testing.T) {
		// The proxies nearest the server append last.
		req := httptest.NewRequest(http.MethodGet, "/r/x", nil)
		req.Host = "notifuse:8080"
		client, proxies := names("c", 12), names("p", 8)
		req.Header.Add("X-Forwarded-Host", strings.Join(client, ", ")+", ,")
		req.Header.Add("X-Forwarded-Host", strings.Join(proxies, ","))

		got := clickOwnHosts(req, "api.notifuse.test", "links.brand.com")
		want := append(append([]string{"notifuse:8080"}, proxies...), "api.notifuse.test", "links.brand.com")
		assert.Equal(t, want, got)
	})

	t.Run("Forwarded values cannot push the proxy's X-Forwarded-Host out", func(t *testing.T) {
		// A proxy sets X-Forwarded-Host and passes the client's Forwarded through.
		req := httptest.NewRequest(http.MethodGet, "/r/x", nil)
		req.Host = "notifuse:8080"
		req.Header.Set("X-Forwarded-Host", "old.brand.example")
		client := names("c", 12)
		req.Header.Set("Forwarded", "host="+strings.Join(client, ", host="))

		got := clickOwnHosts(req, "api.notifuse.test", "")
		want := append(append([]string{"notifuse:8080", "old.brand.example"}, client[4:]...), "api.notifuse.test")
		assert.Equal(t, want, got)
	})

	t.Run("X-Forwarded-Host values cannot push the proxy's Forwarded out", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/r/x", nil)
		req.Host = "notifuse:8080"
		client := names("c", 12)
		req.Header.Set("X-Forwarded-Host", strings.Join(client, ","))
		req.Header.Set("Forwarded", "for=1.2.3.4;host=old.brand.example")

		got := clickOwnHosts(req, "api.notifuse.test", "")
		want := append(append([]string{"notifuse:8080"}, client[4:]...), "old.brand.example", "api.notifuse.test")
		assert.Equal(t, want, got)
	})
}

func TestWebVersionRequestHost(t *testing.T) {
	cases := map[string]struct {
		xfh, fwd string
		want     string
	}{
		"no header":                            {want: "notifuse:8080"},
		"the first X-Forwarded-Host entry":     {xfh: "a.example, b.example", fwd: "host=f.example", want: "a.example"},
		"only Forwarded":                       {fwd: `for=1.2.3.4;host="f.example:8443", host=g.example`, want: "f.example:8443"},
		"an empty first X-Forwarded-Host":      {xfh: " , b.example", fwd: "host=f.example", want: "f.example"},
		"an empty first X-Forwarded-Host only": {xfh: " , b.example", want: "notifuse:8080"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = "notifuse:8080"
			if tc.xfh != "" {
				req.Header.Set("X-Forwarded-Host", tc.xfh)
			}
			if tc.fwd != "" {
				req.Header.Set("Forwarded", tc.fwd)
			}
			assert.Equal(t, tc.want, webVersionRequestHost(req))
		})
	}
}
