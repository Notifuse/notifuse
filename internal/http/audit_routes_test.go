package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// registeredAPIRoutes walks this package's non-test sources for every
// "/api/..." route literal handed to a mux, the way the licence ledger test
// walks the tree for consumers of the entitlement provider.
func registeredAPIRoutes(t *testing.T) []string {
	t.Helper()
	pattern := regexp.MustCompile(`\.Handle(?:Func)?\(\s*"/api/([A-Za-z0-9_.-]+)"`)

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	seen := make(map[string]struct{})
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(".", name))
		require.NoError(t, err)
		for _, match := range pattern.FindAllStringSubmatch(string(contents), -1) {
			seen[match[1]] = struct{}{}
		}
	}

	routes := make([]string, 0, len(seen))
	for route := range seen {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	return routes
}

// Every /api/ route is either audited, a logged read, explicitly excluded, or
// recorded by its own service.
// A route that is none of the three cannot be registered: the audit catalogue
// is the compensating control for a middleware that would otherwise silently
// ignore whatever nobody classified.
func TestEveryAPIRouteIsClassifiedForAudit(t *testing.T) {
	routes := registeredAPIRoutes(t)
	require.NotEmpty(t, routes)

	for _, route := range routes {
		_, audited := domain.AuditedActions[route]
		_, read := domain.AuditLoggedReads[route]
		_, excluded := domain.AuditExcludedRoutes[route]
		_, selfRecorded := domain.AuditSelfRecordedRoutes[route]

		classified := 0
		for _, ok := range []bool{audited, read, excluded, selfRecorded} {
			if ok {
				classified++
			}
		}
		assert.Equal(t, 1, classified,
			"/api/%s must be named in exactly one of domain.AuditedActions, AuditLoggedReads, AuditExcludedRoutes or AuditSelfRecordedRoutes "+
				"(audited=%v read=%v excluded=%v self-recorded=%v)", route, audited, read, excluded, selfRecorded)
	}
}

// And the reverse: a classified route that nobody registers is a typo in the
// catalogue, which would let the real route go unrecorded.
func TestEveryClassifiedAuditRouteIsRegistered(t *testing.T) {
	registered := make(map[string]struct{})
	for _, route := range registeredAPIRoutes(t) {
		registered[route] = struct{}{}
	}

	check := func(route string) {
		_, ok := registered[route]
		assert.True(t, ok, "%s is classified in the audit catalogue but no handler registers /api/%s", route, route)
	}
	for route := range domain.AuditedActions {
		check(route)
	}
	for route := range domain.AuditLoggedReads {
		check(route)
	}
	for route := range domain.AuditExcludedRoutes {
		check(route)
	}
	// The self-recorded routes are /api/ routes, but for the one the OAuth
	// token family serves outside it.
	outsideAPI := map[string]string{"oauth.revoke": "/oauth/revoke"}
	mux := http.NewServeMux()
	NewOAuthHandler(OAuthHandlerConfig{}).RegisterRoutes(mux)
	for route := range domain.AuditSelfRecordedRoutes {
		path, outside := outsideAPI[route]
		if !outside {
			check(route)
			continue
		}
		_, pattern := mux.Handler(httptest.NewRequest(http.MethodPost, path, nil))
		assert.Equal(t, path, pattern, "%s is self-recorded but no handler registers %s", route, path)
	}
}
