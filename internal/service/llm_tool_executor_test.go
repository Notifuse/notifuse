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

func TestServerSideToolRegistry_GetAvailableTools(t *testing.T) {
	log := logger.NewLogger()
	firecrawlSvc := NewFirecrawlService(log)
	registry := NewServerSideToolRegistry(firecrawlSvc, log)

	tools := registry.GetAvailableTools()

	assert.Len(t, tools, 2)

	// Find scrape_url tool
	var scrapeURLTool *domain.LLMTool
	var searchWebTool *domain.LLMTool
	for i := range tools {
		if tools[i].Name == ToolScrapeURL {
			scrapeURLTool = &tools[i]
		}
		if tools[i].Name == ToolSearchWeb {
			searchWebTool = &tools[i]
		}
	}

	require.NotNil(t, scrapeURLTool)
	assert.Equal(t, "scrape_url", scrapeURLTool.Name)
	assert.Contains(t, scrapeURLTool.Description, "Scrapes a URL")
	var scrapeSchema map[string]interface{}
	err := json.Unmarshal(scrapeURLTool.InputSchema, &scrapeSchema)
	require.NoError(t, err)
	assert.Equal(t, "object", scrapeSchema["type"])

	require.NotNil(t, searchWebTool)
	assert.Equal(t, "search_web", searchWebTool.Name)
	assert.Contains(t, searchWebTool.Description, "Searches the web")
	var searchSchema map[string]interface{}
	err = json.Unmarshal(searchWebTool.InputSchema, &searchSchema)
	require.NoError(t, err)
	assert.Equal(t, "object", searchSchema["type"])
}

func TestServerSideToolRegistry_IsServerSideTool(t *testing.T) {
	log := logger.NewLogger()
	firecrawlSvc := NewFirecrawlService(log)
	registry := NewServerSideToolRegistry(firecrawlSvc, log)

	tests := []struct {
		toolName string
		expected bool
	}{
		{ToolScrapeURL, true},
		{ToolSearchWeb, true},
		{"unknown_tool", false},
		{"generate_content", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.toolName, func(t *testing.T) {
			result := registry.IsServerSideTool(tc.toolName)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestServerSideToolRegistry_ExecuteTool_ScrapeURL(t *testing.T) {
	log := logger.NewLogger()

	t.Run("successful scrape", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := FirecrawlScrapeResponse{
				Success: true,
				Data: struct {
					Markdown string `json:"markdown,omitempty"`
					HTML     string `json:"html,omitempty"`
					Title    string `json:"title,omitempty"`
				}{
					Markdown: "# Test Content\n\nSome text here.",
					Title:    "Test Page",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		firecrawlSvc := NewFirecrawlService(log)
		registry := NewServerSideToolRegistry(firecrawlSvc, log)

		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		result, err := registry.ExecuteTool(context.Background(), settings, ToolScrapeURL, map[string]interface{}{
			"url": "https://example.com",
		})

		require.NoError(t, err)
		assert.Contains(t, result, "# Test Page")
		assert.Contains(t, result, "Test Content")
	})

	t.Run("missing url parameter", func(t *testing.T) {
		firecrawlSvc := NewFirecrawlService(log)
		registry := NewServerSideToolRegistry(firecrawlSvc, log)

		settings := &domain.FirecrawlSettings{APIKey: "test"}

		_, err := registry.ExecuteTool(context.Background(), settings, ToolScrapeURL, map[string]interface{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "url is required")
	})

	t.Run("empty url parameter", func(t *testing.T) {
		firecrawlSvc := NewFirecrawlService(log)
		registry := NewServerSideToolRegistry(firecrawlSvc, log)

		settings := &domain.FirecrawlSettings{APIKey: "test"}

		_, err := registry.ExecuteTool(context.Background(), settings, ToolScrapeURL, map[string]interface{}{
			"url": "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "url is required")
	})

	t.Run("content truncation", func(t *testing.T) {
		// Create large content that exceeds MaxContentSize
		largeContent := make([]byte, MaxContentSize+1000)
		for i := range largeContent {
			largeContent[i] = 'a'
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := FirecrawlScrapeResponse{
				Success: true,
				Data: struct {
					Markdown string `json:"markdown,omitempty"`
					HTML     string `json:"html,omitempty"`
					Title    string `json:"title,omitempty"`
				}{
					Markdown: string(largeContent),
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		firecrawlSvc := NewFirecrawlService(log)
		registry := NewServerSideToolRegistry(firecrawlSvc, log)

		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		result, err := registry.ExecuteTool(context.Background(), settings, ToolScrapeURL, map[string]interface{}{
			"url": "https://example.com",
		})

		require.NoError(t, err)
		assert.Contains(t, result, "[Content truncated...]")
		assert.True(t, len(result) < len(largeContent))
	})

	t.Run("scrape error returns message not error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := FirecrawlScrapeResponse{
				Success: false,
				Error:   "Page not found",
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		firecrawlSvc := NewFirecrawlService(log)
		registry := NewServerSideToolRegistry(firecrawlSvc, log)

		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		result, err := registry.ExecuteTool(context.Background(), settings, ToolScrapeURL, map[string]interface{}{
			"url": "https://example.com",
		})

		require.NoError(t, err)
		assert.Contains(t, result, "Error scraping URL")
		assert.Contains(t, result, "Page not found")
	})

	t.Run("scrape with HTML format", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req FirecrawlScrapeRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, []string{"html"}, req.Formats)

			resp := FirecrawlScrapeResponse{
				Success: true,
				Data: struct {
					Markdown string `json:"markdown,omitempty"`
					HTML     string `json:"html,omitempty"`
					Title    string `json:"title,omitempty"`
				}{
					HTML:  "<h1>Test</h1><p>Content</p>",
					Title: "HTML Test",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		firecrawlSvc := NewFirecrawlService(log)
		registry := NewServerSideToolRegistry(firecrawlSvc, log)

		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		result, err := registry.ExecuteTool(context.Background(), settings, ToolScrapeURL, map[string]interface{}{
			"url":    "https://example.com",
			"format": "html",
		})

		require.NoError(t, err)
		assert.Contains(t, result, "<h1>Test</h1>")
		assert.Contains(t, result, "# HTML Test") // Title is prepended
	})

	t.Run("scrape with only_main_content false", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req FirecrawlScrapeRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.NotNil(t, req.OnlyMainContent)
			assert.False(t, *req.OnlyMainContent)

			resp := FirecrawlScrapeResponse{
				Success: true,
				Data: struct {
					Markdown string `json:"markdown,omitempty"`
					HTML     string `json:"html,omitempty"`
					Title    string `json:"title,omitempty"`
				}{
					Markdown: "# Full page content",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		firecrawlSvc := NewFirecrawlService(log)
		registry := NewServerSideToolRegistry(firecrawlSvc, log)

		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		result, err := registry.ExecuteTool(context.Background(), settings, ToolScrapeURL, map[string]interface{}{
			"url":               "https://example.com",
			"only_main_content": false,
		})

		require.NoError(t, err)
		assert.Contains(t, result, "Full page content")
	})
}

func TestServerSideToolRegistry_ExecuteTool_SearchWeb(t *testing.T) {
	log := logger.NewLogger()

	t.Run("successful search", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := FirecrawlSearchResponse{
				Success: true,
				Data: []FirecrawlSearchResult{
					{URL: "https://example.com/1", Title: "Result 1", Description: "First result"},
					{URL: "https://example.com/2", Title: "Result 2", Description: "Second result"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		firecrawlSvc := NewFirecrawlService(log)
		registry := NewServerSideToolRegistry(firecrawlSvc, log)

		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		result, err := registry.ExecuteTool(context.Background(), settings, ToolSearchWeb, map[string]interface{}{
			"query": "test query",
			"limit": float64(5), // JSON numbers are float64
		})

		require.NoError(t, err)
		assert.Contains(t, result, "Search results for \"test query\"")
		assert.Contains(t, result, "Result 1")
		assert.Contains(t, result, "Result 2")
		assert.Contains(t, result, "https://example.com/1")
	})

	t.Run("missing query parameter", func(t *testing.T) {
		firecrawlSvc := NewFirecrawlService(log)
		registry := NewServerSideToolRegistry(firecrawlSvc, log)

		settings := &domain.FirecrawlSettings{APIKey: "test"}

		_, err := registry.ExecuteTool(context.Background(), settings, ToolSearchWeb, map[string]interface{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "query is required")
	})

	t.Run("search error returns message not error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := FirecrawlSearchResponse{
				Success: false,
				Error:   "Rate limit exceeded",
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		firecrawlSvc := NewFirecrawlService(log)
		registry := NewServerSideToolRegistry(firecrawlSvc, log)

		settings := &domain.FirecrawlSettings{
			APIKey:  "test-api-key",
			BaseURL: server.URL,
		}

		result, err := registry.ExecuteTool(context.Background(), settings, ToolSearchWeb, map[string]interface{}{
			"query": "test",
		})

		require.NoError(t, err)
		assert.Contains(t, result, "Error searching")
		assert.Contains(t, result, "Rate limit exceeded")
	})
}

func TestServerSideToolRegistry_ExecuteTool_UnknownTool(t *testing.T) {
	log := logger.NewLogger()
	firecrawlSvc := NewFirecrawlService(log)
	registry := NewServerSideToolRegistry(firecrawlSvc, log)

	settings := &domain.FirecrawlSettings{APIKey: "test"}

	_, err := registry.ExecuteTool(context.Background(), settings, "unknown_tool", map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
}
