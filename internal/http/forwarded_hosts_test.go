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
	// The proxies nearest the server append last, so a client sending many entries
	// cannot push theirs out.
	req := httptest.NewRequest(http.MethodGet, "/r/x", nil)
	req.Host = "notifuse:8080"
	var first, second []string
	for i := 0; i < 12; i++ {
		first = append(first, "c"+string(rune('a'+i))+".example")
	}
	for i := 0; i < 8; i++ {
		second = append(second, "p"+string(rune('a'+i))+".example")
	}
	req.Header.Add("X-Forwarded-Host", strings.Join(first, ", ")+", ,")
	req.Header.Add("X-Forwarded-Host", strings.Join(second[:5], ","))
	req.Header.Add("Forwarded", "host="+strings.Join(second[5:], ", host="))

	got := clickOwnHosts(req, "api.notifuse.test", "links.brand.com")
	want := append(append([]string{"notifuse:8080"}, second...), "api.notifuse.test", "links.brand.com")
	assert.Equal(t, want, got)
	assert.Len(t, got, 1+maxForwardedHosts+2)
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
