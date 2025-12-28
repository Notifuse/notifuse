package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const (
	// ToolScrapeURL is the name of the scrape URL tool
	ToolScrapeURL = "scrape_url"
	// ToolSearchWeb is the name of the search web tool
	ToolSearchWeb = "search_web"
	// MaxContentSize is the maximum size of content to return (50K chars)
	MaxContentSize = 50000
)

// ServerSideToolRegistry manages server-side tool definitions and execution
type ServerSideToolRegistry struct {
	firecrawlService *FirecrawlService
	logger           logger.Logger
}

// NewServerSideToolRegistry creates a new server-side tool registry
func NewServerSideToolRegistry(firecrawlService *FirecrawlService, log logger.Logger) *ServerSideToolRegistry {
	return &ServerSideToolRegistry{
		firecrawlService: firecrawlService,
		logger:           log,
	}
}

// GetAvailableTools returns tools to inject when Firecrawl is configured
func (r *ServerSideToolRegistry) GetAvailableTools() []domain.LLMTool {
	scrapeSchema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to scrape",
			},
			"only_main_content": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, only extracts the main content excluding headers, footers, nav, etc. Default is true.",
			},
			"format": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"markdown", "html"},
				"description": "Output format: 'markdown' (default) or 'html'",
			},
			"include_tags": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "HTML tags or CSS selectors to include (e.g., ['article', 'main', '.content'])",
			},
			"exclude_tags": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "HTML tags or CSS selectors to exclude (e.g., ['nav', 'footer', '#ads'])",
			},
			"wait_for": map[string]interface{}{
				"type":        "integer",
				"description": "Milliseconds to wait before scraping, useful for JavaScript-rendered pages",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Request timeout in milliseconds (default 30000)",
			},
			"mobile": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, renders the page as a mobile device",
			},
		},
		"required": []string{"url"},
	})

	searchSchema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results (default 5, max 10)",
			},
			"lang": map[string]interface{}{
				"type":        "string",
				"description": "Language code for results (e.g., 'en', 'de', 'fr', 'es', 'ja')",
			},
			"country": map[string]interface{}{
				"type":        "string",
				"description": "Country code for results (e.g., 'US', 'DE', 'FR', 'GB', 'JP')",
			},
			"location": map[string]interface{}{
				"type":        "string",
				"description": "Location for geo-targeted results (e.g., 'San Francisco,California,United States')",
			},
			"tbs": map[string]interface{}{
				"type":        "string",
				"description": "Time-based search filter: 'qdr:h' (past hour), 'qdr:d' (past day), 'qdr:w' (past week), 'qdr:m' (past month), 'qdr:y' (past year)",
			},
		},
		"required": []string{"query"},
	})

	return []domain.LLMTool{
		{
			Name:        ToolScrapeURL,
			Description: "Scrapes a URL and returns its content. By default extracts only main content as markdown. Can optionally return full page or HTML format.",
			InputSchema: scrapeSchema,
		},
		{
			Name:        ToolSearchWeb,
			Description: "Searches the web and returns a list of relevant URLs with titles and descriptions. Use this to find information online.",
			InputSchema: searchSchema,
		},
	}
}

// IsServerSideTool checks if a tool should be executed server-side
func (r *ServerSideToolRegistry) IsServerSideTool(toolName string) bool {
	return toolName == ToolScrapeURL || toolName == ToolSearchWeb
}

// ExecuteTool executes a server-side tool and returns the result as string
func (r *ServerSideToolRegistry) ExecuteTool(ctx context.Context, settings *domain.FirecrawlSettings, toolName string, input map[string]interface{}) (string, error) {
	switch toolName {
	case ToolScrapeURL:
		return r.executeScrape(ctx, settings, input)
	case ToolSearchWeb:
		return r.executeSearch(ctx, settings, input)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

func (r *ServerSideToolRegistry) executeScrape(ctx context.Context, settings *domain.FirecrawlSettings, input map[string]interface{}) (string, error) {
	url, ok := input["url"].(string)
	if !ok || url == "" {
		return "", fmt.Errorf("url is required")
	}

	// Parse options
	opts := &ScrapeOptions{
		OnlyMainContent: true,                 // Default to main content only
		Formats:         []string{"markdown"}, // Default to markdown
	}

	if onlyMain, ok := input["only_main_content"].(bool); ok {
		opts.OnlyMainContent = onlyMain
	}

	if format, ok := input["format"].(string); ok && (format == "markdown" || format == "html") {
		opts.Formats = []string{format}
	}

	// Parse include_tags as array of strings
	if includeTags, ok := input["include_tags"].([]interface{}); ok {
		for _, tag := range includeTags {
			if tagStr, ok := tag.(string); ok {
				opts.IncludeTags = append(opts.IncludeTags, tagStr)
			}
		}
	}

	// Parse exclude_tags as array of strings
	if excludeTags, ok := input["exclude_tags"].([]interface{}); ok {
		for _, tag := range excludeTags {
			if tagStr, ok := tag.(string); ok {
				opts.ExcludeTags = append(opts.ExcludeTags, tagStr)
			}
		}
	}

	// Parse wait_for
	if waitFor, ok := input["wait_for"].(float64); ok && waitFor > 0 {
		opts.WaitFor = int(waitFor)
	}

	// Parse timeout
	if timeout, ok := input["timeout"].(float64); ok && timeout > 0 {
		opts.Timeout = int(timeout)
	}

	// Parse mobile
	if mobile, ok := input["mobile"].(bool); ok {
		opts.Mobile = mobile
	}

	r.logger.WithFields(map[string]interface{}{
		"url":               url,
		"only_main_content": opts.OnlyMainContent,
		"format":            opts.Formats[0],
	}).Debug("Executing scrape_url tool")

	result, err := r.firecrawlService.Scrape(ctx, settings, url, opts)
	if err != nil {
		r.logger.WithField("error", err.Error()).Error("Scrape failed")
		return fmt.Sprintf("Error scraping URL: %s", err.Error()), nil
	}

	// Use the appropriate content based on format
	var content string
	if opts.Formats[0] == "html" && result.Data.HTML != "" {
		content = result.Data.HTML
	} else {
		content = result.Data.Markdown
	}

	if len(content) > MaxContentSize {
		content = content[:MaxContentSize] + "\n\n[Content truncated...]"
	}

	if result.Data.Title != "" {
		return fmt.Sprintf("# %s\n\n%s", result.Data.Title, content), nil
	}
	return content, nil
}

func (r *ServerSideToolRegistry) executeSearch(ctx context.Context, settings *domain.FirecrawlSettings, input map[string]interface{}) (string, error) {
	query, ok := input["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("query is required")
	}

	// Parse options
	opts := &SearchOptions{
		Limit: 5, // Default limit
	}

	if l, ok := input["limit"].(float64); ok && l > 0 {
		opts.Limit = int(l)
	}

	if lang, ok := input["lang"].(string); ok && lang != "" {
		opts.Lang = lang
	}

	if country, ok := input["country"].(string); ok && country != "" {
		opts.Country = country
	}

	if location, ok := input["location"].(string); ok && location != "" {
		opts.Location = location
	}

	if tbs, ok := input["tbs"].(string); ok && tbs != "" {
		opts.TBS = tbs
	}

	r.logger.WithFields(map[string]interface{}{
		"query":    query,
		"limit":    opts.Limit,
		"lang":     opts.Lang,
		"country":  opts.Country,
		"location": opts.Location,
		"tbs":      opts.TBS,
	}).Debug("Executing search_web tool")

	result, err := r.firecrawlService.Search(ctx, settings, query, opts)
	if err != nil {
		r.logger.WithField("error", err.Error()).Error("Search failed")
		return fmt.Sprintf("Error searching: %s", err.Error()), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for \"%s\":\n\n", query))
	for i, res := range result.Data {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   URL: %s\n   %s\n\n", i+1, res.Title, res.URL, res.Description))
	}
	return sb.String(), nil
}
