package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/pkg/branding"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func brandingRootHandler(t *testing.T, consoleDir string, effective func() branding.Resolved) *RootHandler {
	t.Helper()
	installed := true
	h := NewRootHandler(consoleDir, "notification_center_test", logger.NewLogger(), "https://api.example.com",
		"45.0", "root@example.com", &installed, false, "", 0, "off", nil, nil, nil, nil, "", false, false, false)
	if effective != nil {
		h.SetBranding(effective)
	}
	return h
}

func fixedBranding(b branding.Branding) func() branding.Resolved {
	return func() branding.Resolved { return branding.Resolve(b) }
}

func serve(h *RootHandler, method, path string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.Handle(rr, httptest.NewRequest(method, path, nil))
	return rr
}

// instanceBrandingFromConfigJS extracts the JSON of the window.INSTANCE_BRANDING statement.
func instanceBrandingFromConfigJS(t *testing.T, body string) branding.Resolved {
	t.Helper()
	const prefix = "window.INSTANCE_BRANDING = "
	start := strings.Index(body, prefix)
	require.NotEqual(t, -1, start, "config.js carries no INSTANCE_BRANDING")
	rest := body[start+len(prefix):]
	end := strings.Index(rest, ";")
	require.NotEqual(t, -1, end)
	var got branding.Resolved
	require.NoError(t, json.Unmarshal([]byte(rest[:end]), &got))
	return got
}

func TestRootHandler_ConfigJS_InstanceBranding(t *testing.T) {
	t.Run("serves the default identity when nothing is wired", func(t *testing.T) {
		rr := serve(brandingRootHandler(t, "console_test", nil), http.MethodGet, "/config.js")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, branding.Resolve(branding.Branding{}), instanceBrandingFromConfigJS(t, rr.Body.String()))
		assert.Contains(t, rr.Body.String(), `window.API_ENDPOINT = "https://api.example.com";`, "existing variables are kept")
	})

	t.Run("serves the effective identity, asked on each request", func(t *testing.T) {
		current := branding.Branding{Name: "Acme"}
		h := brandingRootHandler(t, "console_test", func() branding.Resolved { return branding.Resolve(current) })
		assert.Equal(t, "Acme", instanceBrandingFromConfigJS(t, serve(h, http.MethodGet, "/config.js").Body.String()).Name)

		current = branding.Branding{}
		assert.Equal(t, branding.DefaultName, instanceBrandingFromConfigJS(t, serve(h, http.MethodGet, "/config.js").Body.String()).Name)
	})

	t.Run("carries the background URL, quotes and parentheses escaped as JSON", func(t *testing.T) {
		bg := `https://cdn.example.com/bg.jpg?a="b"&c=(d)`
		h := brandingRootHandler(t, "console_test", fixedBranding(branding.Branding{BackgroundURL: bg}))
		got := instanceBrandingFromConfigJS(t, serve(h, http.MethodGet, "/config.js").Body.String())
		assert.Equal(t, bg, got.BackgroundURL)
		assert.False(t, got.Custom, "a background alone keeps the Notifuse identity")
	})

	t.Run("escapes brand text so it cannot close the script", func(t *testing.T) {
		h := brandingRootHandler(t, "console_test", fixedBranding(branding.Branding{Name: `</script><script>alert(1)</script> & "x"`}))
		body := serve(h, http.MethodGet, "/config.js").Body.String()
		assert.NotContains(t, body, "</script>")
		assert.Equal(t, `</script><script>alert(1)</script> & "x"`, instanceBrandingFromConfigJS(t, body).Name)
	})

	t.Run("sends a charset and nosniff, and is never cached", func(t *testing.T) {
		rr := serve(brandingRootHandler(t, "console_test", nil), http.MethodGet, "/config.js")
		assert.Equal(t, "application/javascript; charset=utf-8", rr.Header().Get("Content-Type"))
		assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
		assert.Contains(t, rr.Header().Get("Cache-Control"), "no-store")
	})

	t.Run("answers HEAD without a body and refuses other methods", func(t *testing.T) {
		h := brandingRootHandler(t, "console_test", nil)
		head := serve(h, http.MethodHead, "/config.js")
		assert.Equal(t, http.StatusOK, head.Code)
		assert.Empty(t, head.Body.String())

		post := serve(h, http.MethodPost, "/config.js")
		assert.Equal(t, http.StatusMethodNotAllowed, post.Code)
		assert.Equal(t, "GET, HEAD", post.Header().Get("Allow"))
	})
}

func decodeManifest(t *testing.T, rr *httptest.ResponseRecorder) webManifest {
	t.Helper()
	var m webManifest
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &m))
	return m
}

func TestRootHandler_InstanceManifest(t *testing.T) {
	t.Run("default mode lists the local Notifuse icons with their size and type", func(t *testing.T) {
		rr := serve(brandingRootHandler(t, "console_test", nil), http.MethodGet, instanceManifestPath)
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/manifest+json; charset=utf-8", rr.Header().Get("Content-Type"))
		assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"))

		m := decodeManifest(t, rr)
		assert.Equal(t, "Notifuse", m.Name)
		assert.Equal(t, "/console/", m.StartURL)
		assert.Equal(t, "/console/", m.Scope)
		require.Len(t, m.Icons, 2)
		for _, icon := range m.Icons {
			assert.True(t, strings.HasPrefix(icon.Src, "/console/"), "local assets live under /console/: %s", icon.Src)
			assert.Equal(t, "image/png", icon.Type)
			assert.NotEmpty(t, icon.Sizes)
		}
	})

	t.Run("a configured icon is listed without a size or type, then the neutral icon", func(t *testing.T) {
		h := brandingRootHandler(t, "console_test", fixedBranding(branding.Branding{Name: "Acme", IconURL: "https://cdn.example.com/i.png"}))
		m := decodeManifest(t, serve(h, http.MethodGet, instanceManifestPath))
		assert.Equal(t, "Acme", m.Name)
		assert.Equal(t, "Acme", m.ShortName)
		assert.Equal(t, []manifestIcon{
			{Src: "https://cdn.example.com/i.png"},
			{Src: branding.NeutralIconPath, Sizes: "192x192", Type: "image/png"},
		}, m.Icons)
	})

	t.Run("a brand without an icon uses the neutral icon", func(t *testing.T) {
		h := brandingRootHandler(t, "console_test", fixedBranding(branding.Branding{Name: "Acme"}))
		m := decodeManifest(t, serve(h, http.MethodGet, instanceManifestPath))
		assert.Equal(t, []manifestIcon{{Src: branding.NeutralIconPath, Sizes: "192x192", Type: "image/png"}}, m.Icons)
	})

	t.Run("HEAD has no body, other methods are refused", func(t *testing.T) {
		h := brandingRootHandler(t, "console_test", nil)
		head := serve(h, http.MethodHead, instanceManifestPath)
		assert.Equal(t, http.StatusOK, head.Code)
		assert.Empty(t, head.Body.String())

		post := serve(h, http.MethodPost, instanceManifestPath)
		assert.Equal(t, http.StatusMethodNotAllowed, post.Code)
		assert.Equal(t, "GET, HEAD", post.Header().Get("Allow"))
	})

	t.Run("the old manifest path redirects to the generated one", func(t *testing.T) {
		h := brandingRootHandler(t, "console_test", nil)
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			rr := serve(h, method, legacyManifestPath)
			assert.Equal(t, http.StatusTemporaryRedirect, rr.Code, method)
			assert.Equal(t, instanceManifestPath, rr.Header().Get("Location"))
			assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
		}
		assert.Equal(t, http.StatusMethodNotAllowed, serve(h, http.MethodPost, legacyManifestPath).Code)
	})
}

func TestRootHandler_BrowserConfig(t *testing.T) {
	t.Run("default mode serves the Notifuse tile under /console/", func(t *testing.T) {
		rr := serve(brandingRootHandler(t, "console_test", nil), http.MethodGet, browserConfigPath)
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/xml; charset=utf-8", rr.Header().Get("Content-Type"))
		assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
		assert.Contains(t, rr.Body.String(), `<square150x150logo src="/console/mstile-150x150.png">`)
		assert.True(t, strings.HasPrefix(rr.Body.String(), "<?xml"))
	})

	t.Run("a custom brand turns the legacy tiles off", func(t *testing.T) {
		h := brandingRootHandler(t, "console_test", fixedBranding(branding.Branding{Name: "Acme"}))
		rr := serve(h, http.MethodGet, browserConfigPath)
		assert.Equal(t, http.StatusNoContent, rr.Code)
		assert.Empty(t, rr.Body.String())
		assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
	})

	t.Run("HEAD has no body, other methods are refused", func(t *testing.T) {
		h := brandingRootHandler(t, "console_test", nil)
		head := serve(h, http.MethodHead, browserConfigPath)
		assert.Equal(t, http.StatusOK, head.Code)
		assert.Empty(t, head.Body.String())
		assert.Equal(t, http.StatusMethodNotAllowed, serve(h, http.MethodPut, browserConfigPath).Code)
	})
}

func TestRootHandler_ConsoleIndexIsRevalidated(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>Console</title>"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o600))
	h := brandingRootHandler(t, dir, nil)

	for _, path := range []string{"/console/", "/console/workspace/acme/contacts"} {
		rr := serve(h, http.MethodGet, path)
		assert.Equal(t, http.StatusOK, rr.Code, path)
		assert.Equal(t, "no-cache", rr.Header().Get("Cache-Control"), "%s serves index.html", path)
	}

	asset := serve(h, http.MethodGet, "/console/assets/app.js")
	assert.Equal(t, http.StatusOK, asset.Code)
	assert.Empty(t, asset.Header().Get("Cache-Control"), "hashed assets keep their default caching")
}
