package http

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The SDK routes are exercised here over a real socket because
// httptest.ResponseRecorder derives ContentLength by parsing whatever header
// the handler happened to set — it never shows what net/http actually puts on
// the wire. HEAD length and the stdlib's 304 header stripping are only
// observable through a real server.
//
// The client must disable transparent compression: the default transport adds
// Accept-Encoding: gzip on its own, then deletes Content-Encoding, deletes
// Content-Length and hands back an inflated body, so every assertion below
// would fail for reasons that have nothing to do with the handler.
func TestWebAnalyticsHandlerSDKOverTheWire(t *testing.T) {
	sdk := compressibleSDK()
	handler, _, mux := newWebAnalyticsHandlerForTest(t, sdk)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}

	do := func(t *testing.T, method string, headers map[string]string) (*http.Response, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+"/na.js", nil)
		require.NoError(t, err)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp, body
	}

	t.Run("gzip body arrives length-delimited", func(t *testing.T) {
		resp, body := do(t, http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))
		assert.Equal(t, int64(len(body)), resp.ContentLength, "must not fall back to chunked")
		assert.Less(t, len(body), len(sdk))
		assert.True(t, bytes.Equal(sdk, gunzip(t, body)))
	})

	t.Run("identity body arrives length-delimited", func(t *testing.T) {
		resp, body := do(t, http.MethodGet, nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, resp.Header.Get("Content-Encoding"))
		assert.Equal(t, int64(len(sdk)), resp.ContentLength)
		assert.True(t, bytes.Equal(sdk, body))
	})

	t.Run("HEAD reports the size a GET would return", func(t *testing.T) {
		get, gzipped := do(t, http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
		require.Equal(t, http.StatusOK, get.StatusCode)

		resp, body := do(t, http.MethodHead, map[string]string{"Accept-Encoding": "gzip"})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, body)
		assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))
		assert.Equal(t, int64(len(gzipped)), resp.ContentLength,
			"a HEAD that writes nothing gets no Content-Length from net/http, so the handler must set it")
	})

	t.Run("304 carries validators and no representation headers", func(t *testing.T) {
		resp, body := do(t, http.MethodGet, map[string]string{
			"Accept-Encoding": "gzip",
			"If-None-Match":   `"` + handler.SDKHash() + `-gzip"`,
		})

		assert.Equal(t, http.StatusNotModified, resp.StatusCode)
		assert.Empty(t, body)
		assert.Equal(t, `"`+handler.SDKHash()+`-gzip"`, resp.Header.Get("ETag"))
		assert.Contains(t, resp.Header.Values("Vary"), "Accept-Encoding")
		assert.Equal(t, "public, max-age=3600", resp.Header.Get("Cache-Control"))
		// net/http strips Content-Type and Content-Length from a 304 but leaves
		// Content-Encoding alone, which is why the handler sets it only after
		// the conditional check.
		assert.Empty(t, resp.Header.Get("Content-Encoding"))
		assert.Empty(t, resp.Header.Get("Content-Length"))
	})
}
