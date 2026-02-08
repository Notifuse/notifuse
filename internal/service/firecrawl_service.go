package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// FirecrawlScrapeRequest is the request for scraping a URL
type FirecrawlScrapeRequest struct {
	URL                 string   `json:"url"`
	Formats             []string `json:"formats"`                       // ["markdown", "html", "links", "screenshot"]
	OnlyMainContent     *bool    `json:"onlyMainContent,omitempty"`     // Extract only main content
	IncludeTags         []string `json:"includeTags,omitempty"`         // HTML tags to include
	ExcludeTags         []string `json:"excludeTags,omitempty"`         // HTML tags to exclude
	WaitFor             *int     `json:"waitFor,omitempty"`             // Milliseconds to wait before scraping
	Timeout             *int     `json:"timeout,omitempty"`             // Request timeout in milliseconds
	Mobile              *bool    `json:"mobile,omitempty"`              // Render as mobile device
	SkipTLSVerification *bool    `json:"skipTlsVerification,omitempty"` // Skip SSL certificate validation
}

// FirecrawlScrapeResponse is the response from scraping
type FirecrawlScrapeResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Markdown string `json:"markdown,omitempty"`
		HTML     string `json:"html,omitempty"`
		Title    string `json:"title,omitempty"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

// FirecrawlSearchRequest is the request for web search
type FirecrawlSearchRequest struct {
	Query         string                  `json:"query"`
	Limit         int                     `json:"limit,omitempty"`         // default 5
	Lang          string                  `json:"lang,omitempty"`          // Language code (e.g., "en", "de")
	Country       string                  `json:"country,omitempty"`       // Country code (e.g., "US", "DE")
	Location      string                  `json:"location,omitempty"`      // Location string (e.g., "San Francisco,California,United States")
	TBS           string                  `json:"tbs,omitempty"`           // Time-based search filter (qdr:d, qdr:w, qdr:m)
	ScrapeOptions *FirecrawlScrapeOptions `json:"scrapeOptions,omitempty"` // Options for scraping search results
}

// FirecrawlScrapeOptions contains scrape options for search results
type FirecrawlScrapeOptions struct {
	Formats         []string `json:"formats,omitempty"`         // Output formats
	OnlyMainContent *bool    `json:"onlyMainContent,omitempty"` // Extract only main content
}

// FirecrawlSearchResult is a single search result
type FirecrawlSearchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// FirecrawlSearchResponse is the response from search
type FirecrawlSearchResponse struct {
	Success bool                    `json:"success"`
	Data    []FirecrawlSearchResult `json:"data"`
	Error   string                  `json:"error,omitempty"`
}

// FirecrawlService handles interactions with the Firecrawl API
type FirecrawlService struct {
	logger     logger.Logger
	httpClient *http.Client
}

// NewFirecrawlService creates a new Firecrawl service
func NewFirecrawlService(log logger.Logger) *FirecrawlService {
	return &FirecrawlService{
		logger: log,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ScrapeOptions contains options for scraping
type ScrapeOptions struct {
	OnlyMainContent     bool     // Extract only main content (default true)
	Formats             []string // Output formats: "markdown", "html" (default ["markdown"])
	IncludeTags         []string // HTML tags to include (e.g., ["article", "main"])
	ExcludeTags         []string // HTML tags to exclude (e.g., ["nav", "footer"])
	WaitFor             int      // Milliseconds to wait before scraping (default 0)
	Timeout             int      // Request timeout in milliseconds (default 30000)
	Mobile              bool     // Render as mobile device (default false)
	SkipTLSVerification bool     // Skip SSL certificate validation (default false)
}

// SearchOptions contains options for web search
type SearchOptions struct {
	Limit                 int      // Maximum number of results (default 5)
	Lang                  string   // Language code (e.g., "en", "de")
	Country               string   // Country code (e.g., "US", "DE")
	Location              string   // Location string (e.g., "San Francisco,California,United States")
	TBS                   string   // Time-based search filter (qdr:d, qdr:w, qdr:m, qdr:y)
	ScrapeFormats         []string // Output formats for scraped results
	ScrapeOnlyMainContent bool     // Extract only main content from results
}

// Scrape fetches a URL and returns content based on options
func (s *FirecrawlService) Scrape(ctx context.Context, settings *domain.FirecrawlSettings, url string, opts *ScrapeOptions) (*FirecrawlScrapeResponse, error) {
	// Default options
	formats := []string{"markdown"}
	onlyMainContent := true

	if opts != nil {
		if len(opts.Formats) > 0 {
			formats = opts.Formats
		}
		onlyMainContent = opts.OnlyMainContent
	}

	reqBody := FirecrawlScrapeRequest{
		URL:             url,
		Formats:         formats,
		OnlyMainContent: &onlyMainContent,
	}

	// Apply additional options if provided
	if opts != nil {
		if len(opts.IncludeTags) > 0 {
			reqBody.IncludeTags = opts.IncludeTags
		}
		if len(opts.ExcludeTags) > 0 {
			reqBody.ExcludeTags = opts.ExcludeTags
		}
		if opts.WaitFor > 0 {
			reqBody.WaitFor = &opts.WaitFor
		}
		if opts.Timeout > 0 {
			reqBody.Timeout = &opts.Timeout
		}
		if opts.Mobile {
			reqBody.Mobile = &opts.Mobile
		}
		if opts.SkipTLSVerification {
			reqBody.SkipTLSVerification = &opts.SkipTLSVerification
		}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", settings.GetBaseURL()+"/v1/scrape", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scrape request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("scrape request returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result FirecrawlScrapeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return nil, fmt.Errorf("scrape failed: %s", errMsg)
	}

	return &result, nil
}

// Search performs a web search and returns results
func (s *FirecrawlService) Search(ctx context.Context, settings *domain.FirecrawlSettings, query string, opts *SearchOptions) (*FirecrawlSearchResponse, error) {
	limit := 5
	if opts != nil && opts.Limit > 0 {
		limit = opts.Limit
	}

	reqBody := FirecrawlSearchRequest{
		Query: query,
		Limit: limit,
	}

	// Apply additional options if provided
	if opts != nil {
		if opts.Lang != "" {
			reqBody.Lang = opts.Lang
		}
		if opts.Country != "" {
			reqBody.Country = opts.Country
		}
		if opts.Location != "" {
			reqBody.Location = opts.Location
		}
		if opts.TBS != "" {
			reqBody.TBS = opts.TBS
		}
		// Add scrape options if formats or onlyMainContent specified
		if len(opts.ScrapeFormats) > 0 || opts.ScrapeOnlyMainContent {
			onlyMain := opts.ScrapeOnlyMainContent
			reqBody.ScrapeOptions = &FirecrawlScrapeOptions{
				Formats:         opts.ScrapeFormats,
				OnlyMainContent: &onlyMain,
			}
		}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", settings.GetBaseURL()+"/v1/search", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search request returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result FirecrawlSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return nil, fmt.Errorf("search failed: %s", errMsg)
	}

	return &result, nil
}
