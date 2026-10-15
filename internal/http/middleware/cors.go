package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CORSMiddleware handles CORS headers for all requests
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get allowed origin from environment variable with default value "*"
		allowOrigin, varies := resolveAllowOrigin(os.Getenv("CORS_ALLOW_ORIGIN"), r.Header.Get("Origin"))
		if varies {
			w.Header().Add("Vary", "Origin")
		}
		if allowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		}

		// Allow specific HTTP methods
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		// Allow specific headers
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization")

		// Allow credentials
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// resolveAllowOrigin picks the Access-Control-Allow-Origin value for one request.
//
// Browsers accept only "*" or the single exact origin of the page making the request, so a
// comma-separated CORS_ALLOW_ORIGIN cannot be sent as written: the browser compares the whole
// string to its origin and refuses every response. Each workspace custom domain serves the
// console too, and that console calls API_ENDPOINT from its own origin, so an operator
// restricting CORS has to list several origins. A list answers the request's Origin when it is
// listed and nothing otherwise; varies reports that the answer depends on Origin, so a shared
// cache does not hand one origin's answer to another.
//
// Unset or blank allows every origin, as does a "*" entry. A single origin is sent whatever the
// request's Origin, as it always was. A value naming no origin at all, such as ",", allows none:
// it used to reach browsers as written, which refused it, and must not turn into "*".
func resolveAllowOrigin(configured, requestOrigin string) (allowOrigin string, varies bool) {
	if strings.TrimSpace(configured) == "" {
		return "*", false
	}

	var origins []string
	for origin := range strings.SplitSeq(configured, ",") {
		// A browser serializes an origin as a lowercase scheme://host[:port] with no trailing
		// slash; normalizing the configured side the same way lets an exact comparison match.
		origin = strings.ToLower(strings.TrimRight(strings.TrimSpace(origin), "/"))
		if origin == "*" {
			return "*", false
		}
		if origin != "" {
			origins = append(origins, origin)
		}
	}

	switch len(origins) {
	case 0:
		return "", false
	case 1:
		return origins[0], false
	}

	// Exact comparison, and the configured value is what goes back: the request's Origin is
	// never reflected, so no look-alike of a listed origin is ever answered with itself.
	for _, origin := range origins {
		if origin == requestOrigin {
			return origin, true
		}
	}
	return "", true
}
