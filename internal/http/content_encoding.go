package http

import (
	"strconv"
	"strings"
)

// Content negotiation helpers shared by the handlers that serve fixed bytes.
// Both are pure string functions with no index arithmetic: nothing in the
// global middleware chain recovers from a panic, so a bug here would take the
// route down rather than degrade it.

// acceptsGzip reports whether the client will accept a gzip-encoded response.
//
// It walks the comma-separated coding list and honours q-values, so
// "gzip;q=0" — an explicit refusal — is not read as consent, and a two-letter
// token like "br" can never substring-match its way into a yes.
//
// An empty header returns false. RFC 9110 section 12.5.3 says an absent
// Accept-Encoding means any coding is acceptable, but identity is always safe
// and a client that sent no header is a client that never asked for gzip.
// Every browser sends the header, so nothing real loses compression by this.
func acceptsGzip(header string) bool {
	if header == "" {
		return false
	}
	wildcard, wildcardSeen := false, false
	for _, element := range strings.Split(header, ",") {
		coding, acceptable := parseCoding(element)
		switch coding {
		case "gzip":
			return acceptable
		case "*":
			wildcard, wildcardSeen = acceptable, true
		}
	}
	if wildcardSeen {
		return wildcard
	}
	return false
}

// parseCoding splits one Accept-Encoding element into its lowercased coding
// token and whether its q-value permits use. A malformed q-value is ignored
// rather than treated as a refusal.
func parseCoding(element string) (coding string, acceptable bool) {
	params := strings.Split(element, ";")
	coding = strings.ToLower(strings.TrimSpace(params[0]))
	for _, param := range params[1:] {
		param = strings.ToLower(strings.TrimSpace(param))
		if !strings.HasPrefix(param, "q=") {
			continue
		}
		q, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(param, "q=")), 64)
		if err != nil {
			continue
		}
		return coding, q > 0
	}
	return coding, true
}

// etagMatches reports whether an If-None-Match header selects etag, using the
// weak comparison RFC 9110 section 13.1.2 mandates for that header: the "W/"
// prefix is ignored on both sides, "*" matches any current representation, and
// the value may be a list.
func etagMatches(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" || etag == "" {
		return false
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		if strings.TrimPrefix(candidate, "W/") == want {
			return true
		}
	}
	return false
}
