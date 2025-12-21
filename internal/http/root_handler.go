package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type RootHandler struct {
	consoleDir            string
	notificationCenterDir string
	logger                logger.Logger
	apiEndpoint           string
	version               string
	rootEmail             string
	isInstalledPtr        *bool // Pointer to installation status that updates dynamically
	smtpRelayEnabled      bool
	smtpRelayDomain       string
	smtpRelayPort         int
	smtpRelayTLSEnabled   bool
	workspaceRepo         domain.WorkspaceRepository
	blogService           domain.BlogService
	cache                 cache.Cache
}

// NewRootHandler creates a root handler that serves both console and notification center static files
func NewRootHandler(
	consoleDir string,
	notificationCenterDir string,
	logger logger.Logger,
	apiEndpoint string,
	version string,
	rootEmail string,
	isInstalledPtr *bool,
	smtpRelayEnabled bool,
	smtpRelayDomain string,
	smtpRelayPort int,
	smtpRelayTLSEnabled bool,
	workspaceRepo domain.WorkspaceRepository,
	blogService domain.BlogService,
	cache cache.Cache,
) *RootHandler {
	return &RootHandler{
		consoleDir:            consoleDir,
		notificationCenterDir: notificationCenterDir,
		logger:                logger,
		apiEndpoint:           apiEndpoint,
		version:               version,
		rootEmail:             rootEmail,
		isInstalledPtr:        isInstalledPtr,
		smtpRelayEnabled:      smtpRelayEnabled,
		smtpRelayDomain:       smtpRelayDomain,
		smtpRelayPort:         smtpRelayPort,
		smtpRelayTLSEnabled:   smtpRelayTLSEnabled,
		workspaceRepo:         workspaceRepo,
		blogService:           blogService,
		cache:                 cache,
	}
}

func (h *RootHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// 1. Handle /config.js
	if r.URL.Path == "/config.js" {
		h.serveConfigJS(w, r)
		return
	}

	// 2. Handle /console/* - serve console SPA
	if strings.HasPrefix(r.URL.Path, "/console") {
		h.serveConsole(w, r)
		return
	}

	// 3. Handle /notification-center/*
	if strings.HasPrefix(r.URL.Path, "/notification-center") || strings.Contains(r.Header.Get("Referer"), "/notification-center") {
		h.serveNotificationCenter(w, r)
		return
	}

	// 4. Handle /api/*
	if strings.HasPrefix(r.URL.Path, "/api") {
		// Default API root response
		if r.URL.Path == "/api" || r.URL.Path == "/api/" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "api running",
			})
		}
		// Other API routes handled by other handlers
		return
	}

	// 5. Check if this is a custom domain for a workspace with blog enabled
	host := r.Host
	// Strip port if present
	if colonIdx := strings.Index(host, ":"); colonIdx != -1 {
		host = host[:colonIdx]
	}

	workspace := h.detectWorkspaceByHost(r.Context(), host)
	if workspace != nil && workspace.Settings.BlogEnabled && h.blogService != nil {
		h.serveBlog(w, r, workspace)
		return
	}

	// 6. ROOT PATH LOGIC: Default behavior is to redirect to console
	http.Redirect(w, r, "/console", http.StatusTemporaryRedirect)
}

// serveConfigJS generates and serves the config.js file with environment variables
func (h *RootHandler) serveConfigJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	// Prevent 304 responses by removing ETag and setting a dynamic Last-Modified
	w.Header().Del("ETag")
	w.Header().Del("Last-Modified")

	isInstalledStr := "false"
	if h.isInstalledPtr != nil && *h.isInstalledPtr {
		isInstalledStr = "true"
	}

	// Serialize timezones to JSON
	timezonesJSON, err := json.Marshal(domain.Timezones)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to marshal timezones")
		timezonesJSON = []byte("[]")
	}

	smtpRelayEnabledStr := "false"
	if h.smtpRelayEnabled {
		smtpRelayEnabledStr = "true"
	}

	smtpRelayTLSEnabledStr := "false"
	if h.smtpRelayTLSEnabled {
		smtpRelayTLSEnabledStr = "true"
	}

	configJS := fmt.Sprintf(
		"window.API_ENDPOINT = %q;\nwindow.VERSION = %q;\nwindow.ROOT_EMAIL = %q;\nwindow.IS_INSTALLED = %s;\nwindow.TIMEZONES = %s;\nwindow.SMTP_RELAY_ENABLED = %s;\nwindow.SMTP_RELAY_DOMAIN = %q;\nwindow.SMTP_RELAY_PORT = %d;\nwindow.SMTP_RELAY_TLS_ENABLED = %s;",
		h.apiEndpoint,
		h.version,
		h.rootEmail,
		isInstalledStr,
		string(timezonesJSON),
		smtpRelayEnabledStr,
		h.smtpRelayDomain,
		h.smtpRelayPort,
		smtpRelayTLSEnabledStr,
	)
	_, _ = w.Write([]byte(configJS))
}

// serveConsole handles serving static files, with a fallback for SPA routing
func (h *RootHandler) serveConsole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Strip /console prefix before serving files
	originalPath := r.URL.Path
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/console")
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}

	// Create file server for console files
	fs := http.FileServer(http.Dir(h.consoleDir))

	path := h.consoleDir + r.URL.Path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// If the requested file doesn't exist, serve index.html for SPA routing
		r.URL.Path = "/"
	}

	h.logger.WithField("original_path", originalPath).WithField("served_path", r.URL.Path).Debug("Serving console")

	fs.ServeHTTP(w, r)
}

// serveNotificationCenter handles serving notification center static files, with a fallback for SPA routing
func (h *RootHandler) serveNotificationCenter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Strip the prefix to match the file structure
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/notification-center")
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}

	// Create file server for notification center files
	fs := http.FileServer(http.Dir(h.notificationCenterDir))

	path := h.notificationCenterDir + r.URL.Path
	log.Println("path", path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// If the requested file doesn't exist, serve index.html for SPA routing
		r.URL.Path = "/"
	}

	fs.ServeHTTP(w, r)
}

// serveBlog handles blog content requests
func (h *RootHandler) serveBlog(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace) {
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace.ID)

	// Handle special paths
	switch r.URL.Path {
	case "/robots.txt":
		h.serveBlogRobots(w, r)
		return
	case "/sitemap.xml":
		h.serveBlogSitemap(w, r, workspace)
		return
	case "/":
		h.serveBlogHome(w, r, workspace)
		return
	}

	// Try to parse URL parts
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	// Handle /{category-slug} - category page
	if len(parts) == 1 && parts[0] != "" {
		categorySlug := parts[0]
		// Try to get the category (public access, no authentication required)
		category, err := h.blogService.GetPublicCategoryBySlug(ctx, categorySlug)
		if err == nil && category != nil {
			h.serveBlogCategory(w, r, workspace, categorySlug)
			return
		}
	}

	// Handle /{category-slug}/{post-slug} - post page
	if len(parts) == 2 {
		categorySlug := parts[0]
		postSlug := parts[1]

		// Try to get the post (public access, no authentication required)
		post, err := h.blogService.GetPublicPostByCategoryAndSlug(ctx, categorySlug, postSlug)
		if err == nil && post != nil {
			h.serveBlogPost(w, r, workspace, post)
			return
		}
	}

	// Not found
	h.serveBlog404(w, r)
}

// serveBlogHome serves the blog home page with a list of posts
func (h *RootHandler) serveBlogHome(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace) {
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace.ID)

	// Extract page parameter from query string
	pageStr := r.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		var err error
		page, err = strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			// Invalid page parameter, redirect to page 1
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	}

	// Redirect ?page=1 to base URL to avoid duplicate content
	if page == 1 && pageStr != "" {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
		return
	}

	// Extract preview_theme_version
	var themeVersion *int
	if versionStr := r.URL.Query().Get("preview_theme_version"); versionStr != "" {
		if v, err := strconv.Atoi(versionStr); err == nil {
			themeVersion = &v
		}
	}

	// Try cache first (include page in cache key)
	// Skip cache if previewing
	cacheKey := fmt.Sprintf("%s:/?page=%d", r.Host, page)
	if h.cache != nil && themeVersion == nil {
		if cached, found := h.cache.Get(cacheKey); found {
			if html, ok := cached.(string); ok {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("X-Cache", "HIT")
				_, _ = w.Write([]byte(html))
				return
			}
		}
	}

	// Cache miss - render the page
	html, err := h.blogService.RenderHomePage(ctx, workspace.ID, page, themeVersion)
	if err != nil {
		// Map error codes to HTTP status codes (includes 404 for invalid pages)
		if blogErr, ok := err.(*domain.BlogRenderError); ok {
			h.handleBlogRenderError(w, blogErr)
			return
		}
		// Fallback for unexpected errors
		h.logger.WithField("error", err.Error()).Error("Failed to render blog home page")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Store in cache (skip if previewing)
	if h.cache != nil && themeVersion == nil {
		h.cache.Set(cacheKey, html, domain.BlogCacheTTL)
	}

	// Serve the rendered HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if themeVersion != nil {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("X-Cache", "BYPASS")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	_, _ = w.Write([]byte(html))
}

// serveBlogCategory serves a blog category page with posts in that category
func (h *RootHandler) serveBlogCategory(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace, categorySlug string) {
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace.ID)

	// Extract page parameter from query string
	pageStr := r.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		var err error
		page, err = strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			// Invalid page parameter, redirect to page 1
			http.Redirect(w, r, "/"+categorySlug, http.StatusFound)
			return
		}
	}

	// Redirect ?page=1 to base URL to avoid duplicate content
	if page == 1 && pageStr != "" {
		http.Redirect(w, r, "/"+categorySlug, http.StatusMovedPermanently)
		return
	}

	// Extract preview_theme_version
	var themeVersion *int
	if versionStr := r.URL.Query().Get("preview_theme_version"); versionStr != "" {
		if v, err := strconv.Atoi(versionStr); err == nil {
			themeVersion = &v
		}
	}

	// Try cache first (include page in cache key)
	// Skip cache if previewing
	cacheKey := fmt.Sprintf("%s:/%s?page=%d", r.Host, categorySlug, page)
	if h.cache != nil && themeVersion == nil {
		if cached, found := h.cache.Get(cacheKey); found {
			if html, ok := cached.(string); ok {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("X-Cache", "HIT")
				_, _ = w.Write([]byte(html))
				return
			}
		}
	}

	// Cache miss - render the page
	html, err := h.blogService.RenderCategoryPage(ctx, workspace.ID, categorySlug, page, themeVersion)
	if err != nil {
		// Map error codes to HTTP status codes
		if blogErr, ok := err.(*domain.BlogRenderError); ok {
			h.handleBlogRenderError(w, blogErr)
			return
		}
		// Fallback for unexpected errors
		h.logger.WithField("error", err.Error()).Error("Failed to render blog category page")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Store in cache (skip if previewing)
	if h.cache != nil && themeVersion == nil {
		h.cache.Set(cacheKey, html, domain.BlogCacheTTL)
	}

	// Serve the rendered HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if themeVersion != nil {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("X-Cache", "BYPASS")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	_, _ = w.Write([]byte(html))
}

// serveBlogPost serves a single blog post
func (h *RootHandler) serveBlogPost(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace, post *domain.BlogPost) {
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace.ID)

	// Get category from post to build proper URL
	// Extract category slug and post slug from URL
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 2 {
		h.serveBlog404(w, r)
		return
	}
	categorySlug := parts[0]
	postSlug := parts[1]

	// Extract preview_theme_version
	var themeVersion *int
	if versionStr := r.URL.Query().Get("preview_theme_version"); versionStr != "" {
		if v, err := strconv.Atoi(versionStr); err == nil {
			themeVersion = &v
		}
	}

	// Try cache first
	// Skip cache if previewing
	cacheKey := fmt.Sprintf("%s:/%s/%s", r.Host, categorySlug, postSlug)
	if h.cache != nil && themeVersion == nil {
		if cached, found := h.cache.Get(cacheKey); found {
			if html, ok := cached.(string); ok {
				h.logger.WithFields(map[string]interface{}{
					"cache_key":     cacheKey,
					"cache_size":    h.cache.Size(),
					"category_slug": categorySlug,
					"post_slug":     postSlug,
				}).Info("Blog post cache HIT")

				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("X-Cache", "HIT")
				_, _ = w.Write([]byte(html))
				return
			}
		}
	}

	// Cache miss - render the page
	html, err := h.blogService.RenderPostPage(ctx, workspace.ID, categorySlug, postSlug, themeVersion)
	if err != nil {
		// Map error codes to HTTP status codes
		if blogErr, ok := err.(*domain.BlogRenderError); ok {
			h.handleBlogRenderError(w, blogErr)
			return
		}
		// Fallback for unexpected errors
		h.logger.WithField("error", err.Error()).Error("Failed to render blog post page")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Store in cache (skip if previewing)
	if h.cache != nil && themeVersion == nil {
		h.cache.Set(cacheKey, html, domain.BlogCacheTTL)
	}

	// Serve the rendered HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if themeVersion != nil {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("X-Cache", "BYPASS")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	_, _ = w.Write([]byte(html))
}

// serveBlogRobots serves robots.txt for the blog
func (h *RootHandler) serveBlogRobots(w http.ResponseWriter, r *http.Request) {
	robotsTxt := `User-agent: *
Allow: /

Sitemap: /sitemap.xml
`
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(robotsTxt))
}

// serveBlogSitemap serves sitemap.xml for the blog
func (h *RootHandler) serveBlogSitemap(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace) {
	ctx := context.WithValue(r.Context(), domain.WorkspaceIDKey, workspace.ID)

	// Get all published posts
	params := &domain.ListBlogPostsRequest{
		Status: domain.BlogPostStatusPublished,
		Limit:  1000,
		Offset: 0,
	}

	response, err := h.blogService.ListPublicPosts(ctx, params)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list posts for sitemap")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Build sitemap XML
	var sitemap strings.Builder
	sitemap.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sitemap.WriteString("\n")
	sitemap.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	sitemap.WriteString("\n")

	// Add homepage
	sitemap.WriteString("  <url>\n")
	sitemap.WriteString(fmt.Sprintf("    <loc>https://%s/</loc>\n", r.Host))
	sitemap.WriteString("    <changefreq>daily</changefreq>\n")
	sitemap.WriteString("    <priority>1.0</priority>\n")
	sitemap.WriteString("  </url>\n")

	// Add posts
	for _, post := range response.Posts {
		if post.CategoryID != "" {
			// Get category to build the URL
			category, err := h.blogService.GetCategory(ctx, post.CategoryID)
			if err == nil && category != nil {
				sitemap.WriteString("  <url>\n")
				sitemap.WriteString(fmt.Sprintf("    <loc>https://%s/%s/%s</loc>\n", r.Host, category.Slug, post.Slug))
				if post.PublishedAt != nil {
					sitemap.WriteString(fmt.Sprintf("    <lastmod>%s</lastmod>\n", post.PublishedAt.Format("2006-01-02")))
				}
				sitemap.WriteString("    <changefreq>monthly</changefreq>\n")
				sitemap.WriteString("    <priority>0.8</priority>\n")
				sitemap.WriteString("  </url>\n")
			}
		}
	}

	sitemap.WriteString("</urlset>")

	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(sitemap.String()))
}

// serveBlog404 serves a 404 page for blog
func (h *RootHandler) serveBlog404(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>404 Not Found</title>
</head>
<body>
<h1>404 Not Found</h1>
<p>The page you're looking for doesn't exist.</p>
<p><a href="/">Go back home</a></p>
</body>
</html>`))
}

// detectWorkspaceByHost finds a workspace by matching the custom endpoint URL hostname
func (h *RootHandler) detectWorkspaceByHost(ctx context.Context, host string) *domain.Workspace {
	// Return nil if workspace repo not configured (e.g., in tests)
	if h.workspaceRepo == nil {
		return nil
	}

	// Use optimized repository method to find workspace by custom domain
	workspace, err := h.workspaceRepo.GetWorkspaceByCustomDomain(ctx, host)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get workspace by custom domain")
		return nil
	}

	if workspace == nil {
		h.logger.WithField("host", host).Debug("No workspace found for host")
		return nil
	}

	h.logger.
		WithField("workspace_id", workspace.ID).
		WithField("workspace_name", workspace.Name).
		WithField("host", host).
		Debug("Workspace detected by host")
	return workspace
}

// handleBlogRenderError maps blog render error codes to HTTP status codes
func (h *RootHandler) handleBlogRenderError(w http.ResponseWriter, blogErr *domain.BlogRenderError) {
	// Log the error
	h.logger.WithFields(map[string]interface{}{
		"error_code": blogErr.Code,
		"message":    blogErr.Message,
		"details":    blogErr.Details,
	}).Error("Blog render error")

	// Map error codes to HTTP status codes
	switch blogErr.Code {
	case domain.ErrCodeThemeNotFound, domain.ErrCodeThemeNotPublished:
		// Service unavailable - blog not properly configured
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Blog Unavailable</title>
</head>
<body>
<h1>Blog Temporarily Unavailable</h1>
<p>The blog is currently being set up. Please check back later.</p>
</body>
</html>`))

	case domain.ErrCodePostNotFound, domain.ErrCodeCategoryNotFound:
		// Not found
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>404 Not Found</title>
</head>
<body>
<h1>404 Not Found</h1>
<p>The page you're looking for doesn't exist.</p>
<p><a href="/">Go back home</a></p>
</body>
</html>`))

	case domain.ErrCodeRenderFailed, domain.ErrCodeInvalidLiquidSyntax:
		// Internal server error
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Error</title>
</head>
<body>
<h1>Something Went Wrong</h1>
<p>We're sorry, but something went wrong. Please try again later.</p>
</body>
</html>`))

	default:
		// Unknown error - treat as internal server error
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *RootHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/config.js", h.serveConfigJS)
	// catch all route
	mux.HandleFunc("/", h.Handle)
}
