package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirecrawlService_Scrape(t *testing.T) {
	log := logger.NewLogger()

	t.Run("successful scrape", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/scrape", r.URL.Path)
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req FirecrawlScrapeRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "https://example.com", req.URL)
			assert.Equal(t, []string{"markdown"}, req.Formats)

			resp := FirecrawlScrapeResponse{
				Success: true,
				Data: struct {
					Markdown string `json:"markdown,omitempty"`
					HTML     string `json:"html,omitempty"`
					Title    string `json:"title,omitempty"`
				}{
					Markdown: "# Hello World\n\nThis is the content.",
					Title:    "Example Page",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		result, err := svc.Scrape(context.Background(), settings, "https://example.com", nil)
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "Example Page", result.Data.Title)
		assert.Contains(t, result.Data.Markdown, "Hello World")
	})

	t.Run("scrape failure response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := FirecrawlScrapeResponse{
				Success: false,
				Error:   "URL not accessible",
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		_, err := svc.Scrape(context.Background(), settings, "https://example.com", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "URL not accessible")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		_, err := svc.Scrape(context.Background(), settings, "https://example.com", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse response")
	})

	t.Run("HTTP 401 unauthorized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "Invalid API key"}`))
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "invalid-key",
			BaseURL: server.URL,
		}

		_, err := svc.Scrape(context.Background(), settings, "https://example.com", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 401")
		assert.Contains(t, err.Error(), "Invalid API key")
	})

	t.Run("HTTP 429 rate limited", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "Rate limit exceeded"}`))
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		_, err := svc.Scrape(context.Background(), settings, "https://example.com", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 429")
		assert.Contains(t, err.Error(), "Rate limit exceeded")
	})

	t.Run("HTTP 500 server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Internal server error"}`))
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		_, err := svc.Scrape(context.Background(), settings, "https://example.com", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 500")
		assert.Contains(t, err.Error(), "Internal server error")
	})

	t.Run("scrape with HTML format option", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req FirecrawlScrapeRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, []string{"html"}, req.Formats)
			assert.NotNil(t, req.OnlyMainContent)
			assert.True(t, *req.OnlyMainContent)

			resp := FirecrawlScrapeResponse{
				Success: true,
				Data: struct {
					Markdown string `json:"markdown,omitempty"`
					HTML     string `json:"html,omitempty"`
					Title    string `json:"title,omitempty"`
				}{
					HTML:  "<html><body><h1>Hello</h1></body></html>",
					Title: "HTML Page",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		opts := &ScrapeOptions{
			Formats:         []string{"html"},
			OnlyMainContent: true,
		}
		result, err := svc.Scrape(context.Background(), settings, "https://example.com", opts)
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "<html><body><h1>Hello</h1></body></html>", result.Data.HTML)
	})

	t.Run("scrape with only_main_content false", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req FirecrawlScrapeRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.NotNil(t, req.OnlyMainContent)
			assert.False(t, *req.OnlyMainContent)

			resp := FirecrawlScrapeResponse{
				Success: true,
				Data: struct {
					Markdown string `json:"markdown,omitempty"`
					HTML     string `json:"html,omitempty"`
					Title    string `json:"title,omitempty"`
				}{
					Markdown: "# Full page with nav and footer",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		opts := &ScrapeOptions{
			OnlyMainContent: false,
		}
		result, err := svc.Scrape(context.Background(), settings, "https://example.com", opts)
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.Data.Markdown, "Full page")
	})
}

func TestFirecrawlService_Search(t *testing.T) {
	log := logger.NewLogger()

	t.Run("successful search", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/search", r.URL.Path)
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

			var req FirecrawlSearchRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "golang testing", req.Query)
			assert.Equal(t, 3, req.Limit)

			resp := FirecrawlSearchResponse{
				Success: true,
				Data: []FirecrawlSearchResult{
					{URL: "https://golang.org/pkg/testing/", Title: "testing - Go", Description: "Package testing provides support for automated testing"},
					{URL: "https://go.dev/doc/", Title: "Go Documentation", Description: "The Go programming language documentation"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		result, err := svc.Search(context.Background(), settings, "golang testing", &SearchOptions{Limit: 3})
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Len(t, result.Data, 2)
		assert.Equal(t, "testing - Go", result.Data[0].Title)
	})

	t.Run("search with default limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req FirecrawlSearchRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, 5, req.Limit)

			resp := FirecrawlSearchResponse{Success: true, Data: []FirecrawlSearchResult{}}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		_, err := svc.Search(context.Background(), settings, "query", nil)
		require.NoError(t, err)
	})

	t.Run("search failure response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := FirecrawlSearchResponse{
				Success: false,
				Error:   "API rate limit exceeded",
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		_, err := svc.Search(context.Background(), settings, "query", &SearchOptions{Limit: 5})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "API rate limit exceeded")
	})

	t.Run("HTTP 401 unauthorized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "Invalid API key"}`))
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "invalid-key",
			BaseURL: server.URL,
		}

		_, err := svc.Search(context.Background(), settings, "query", &SearchOptions{Limit: 5})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 401")
		assert.Contains(t, err.Error(), "Invalid API key")
	})

	t.Run("HTTP 429 rate limited", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "Rate limit exceeded"}`))
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		_, err := svc.Search(context.Background(), settings, "query", &SearchOptions{Limit: 5})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 429")
		assert.Contains(t, err.Error(), "Rate limit exceeded")
	})

	t.Run("HTTP 500 server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Internal server error"}`))
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		_, err := svc.Search(context.Background(), settings, "query", &SearchOptions{Limit: 5})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 500")
		assert.Contains(t, err.Error(), "Internal server error")
	})

	t.Run("search with language and country options", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req FirecrawlSearchRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "de", req.Lang)
			assert.Equal(t, "DE", req.Country)
			assert.Equal(t, "qdr:w", req.TBS)

			resp := FirecrawlSearchResponse{Success: true, Data: []FirecrawlSearchResult{}}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		svc := NewFirecrawlService(log)
		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		opts := &SearchOptions{
			Limit:   5,
			Lang:    "de",
			Country: "DE",
			TBS:     "qdr:w",
		}
		_, err := svc.Search(context.Background(), settings, "query", opts)
		require.NoError(t, err)
	})
}
