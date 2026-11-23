package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/pkg/safehttpclient"
)

// localFetcher reaches httptest servers: the production fetcher refuses
// loopback addresses, which TestHTTPOAuthClientFetcher_RefusesPrivateAddresses
// pins.
func localFetcher() *HTTPOAuthClientFetcher {
	return newHTTPOAuthClientFetcher(&http.Transport{})
}

func TestHTTPOAuthClientFetcher(t *testing.T) {
	var redirectTargetHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/client.json", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "max-age=600")
		_, _ = w.Write([]byte(`{"client_id":"x"}`))
	})
	mux.HandleFunc("/vendor.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.example+json")
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/target.json", http.StatusFound)
	})
	mux.HandleFunc("/target.json", func(w http.ResponseWriter, r *http.Request) {
		redirectTargetHit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/no-type", func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Content-Type"] = nil
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/at-cap", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat(" ", 65536)))
	})
	mux.HandleFunc("/over-cap", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat(" ", 65537)))
	})
	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	fetcher := localFetcher()

	document, err := fetcher.Fetch(context.Background(), server.URL+"/client.json")
	require.NoError(t, err)
	assert.Equal(t, `{"client_id":"x"}`, string(document.Body))
	assert.Equal(t, "max-age=600", document.Header.Get("Cache-Control"))

	_, err = fetcher.Fetch(context.Background(), server.URL+"/vendor.json")
	assert.NoError(t, err, "a +json type is JSON")

	_, err = fetcher.Fetch(context.Background(), server.URL+"/moved")
	assert.ErrorContains(t, err, "answered 302", "a redirect is a failure")
	assert.False(t, redirectTargetHit, "no redirect is followed")

	_, err = fetcher.Fetch(context.Background(), server.URL+"/error")
	assert.ErrorContains(t, err, "answered 404")
	_, err = fetcher.Fetch(context.Background(), server.URL+"/html")
	assert.ErrorContains(t, err, "not served as JSON")
	_, err = fetcher.Fetch(context.Background(), server.URL+"/no-type")
	assert.ErrorContains(t, err, "not served as JSON")

	document, err = fetcher.Fetch(context.Background(), server.URL+"/at-cap")
	require.NoError(t, err, "64 KB is allowed")
	assert.Len(t, document.Body, 65536)
	_, err = fetcher.Fetch(context.Background(), server.URL+"/over-cap")
	assert.ErrorContains(t, err, "larger than 65536 bytes")
}

func TestHTTPOAuthClientFetcher_Timeout(t *testing.T) {
	assert.Equal(t, 5*time.Second, NewHTTPOAuthClientFetcher().client.Timeout)

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)
	fetcher := localFetcher()
	fetcher.client.Timeout = 50 * time.Millisecond
	_, err := fetcher.Fetch(context.Background(), server.URL+"/slow")
	assert.ErrorContains(t, err, "could not be fetched")
}

// The production fetcher dials through safehttpclient: a loopback or private
// address is refused before any request is sent.
func TestHTTPOAuthClientFetcher_RefusesPrivateAddresses(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	_, err := NewHTTPOAuthClientFetcher().Fetch(context.Background(), server.URL+"/client.json")
	require.Error(t, err)
	assert.True(t, errors.Is(err, safehttpclient.ErrPrivateIP), "got %v", err)
	assert.False(t, hit)
}
