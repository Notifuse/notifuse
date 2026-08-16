package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAcceptsGzip(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{"empty header is not consent", "", false},
		{"bare token", "gzip", true},
		{"real Chrome", "gzip, deflate, br, zstd", true},
		{"token is case-insensitive", "GZIP", true},
		{"q=0 is an explicit refusal", "gzip;q=0", false},
		{"q=0.0 is a refusal", "gzip;q=0.0", false},
		{"q=0.000 is a refusal", "gzip;q=0.000", false},
		{"low but non-zero q is consent", "gzip;q=0.5", true},
		{"spaces around parameters", "gzip ; q=1.0", true},
		{"uppercase q parameter", "gzip;Q=0", false},
		{"other codings only", "deflate, br", false},
		{"br must not substring-match", "br", false},
		{"x-gzip is not gzip", "x-gzip", false},
		{"wildcard accepts", "*", true},
		{"wildcard refused", "*;q=0", false},
		{"explicit refusal beats a later wildcard", "gzip;q=0, *", false},
		{"explicit refusal beats an earlier wildcard", "*, gzip;q=0", false},
		{"wildcard refusal with other codings", "deflate, *;q=0", false},
		{"malformed q is ignored", "gzip;q=bogus", true},
		{"parameter that is not q", "gzip;level=9", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, acceptsGzip(tt.header))
		})
	}
}

func TestETagMatches(t *testing.T) {
	tests := []struct {
		name        string
		ifNoneMatch string
		etag        string
		want        bool
	}{
		{"empty If-None-Match", "", `"abc"`, false},
		{"empty etag", `"abc"`, "", false},
		{"exact match", `"abc"`, `"abc"`, true},
		{"different value", `"abc"`, `"abd"`, false},
		{"wildcard matches", "*", `"abc"`, true},
		{"weak candidate matches strong etag", `W/"abc"`, `"abc"`, true},
		{"strong candidate matches weak etag", `"abc"`, `W/"abc"`, true},
		{"list containing the etag", `"x", "abc"`, `"abc"`, true},
		{"list without the etag", `"x","y"`, `"abc"`, false},
		{"codings never cross", `"abc-gzip"`, `"abc"`, false},
		{"quotes are significant", "abc", `"abc"`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, etagMatches(tt.ifNoneMatch, tt.etag))
		})
	}
}
