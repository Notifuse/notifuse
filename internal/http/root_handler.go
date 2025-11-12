package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
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
			json.NewEncoder(w).Encode(map[string]string{
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
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

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
	w.Write([]byte(configJS))
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
	ctx := context.WithValue(r.Context(), "workspace_id", workspace.ID)

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

	// Try to parse as /{category-slug}/{post-slug}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 2 {
		categorySlug := parts[0]
		postSlug := parts[1]

		// Try to get the post
		post, err := h.blogService.GetPostByCategoryAndSlug(ctx, categorySlug, postSlug)
		if err == nil && post != nil && post.IsPublished() {
			h.serveBlogPost(w, r, workspace, post)
			return
		}
	}

	// Not found
	h.serveBlog404(w, r)
}

// serveBlogHome serves the blog home page with a list of posts
func (h *RootHandler) serveBlogHome(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace) {
	ctx := context.WithValue(r.Context(), "workspace_id", workspace.ID)

	// List published posts
	params := &domain.ListBlogPostsRequest{
		Status: domain.BlogPostStatusPublished,
		Limit:  50,
		Offset: 0,
	}

	response, err := h.blogService.ListPublicPosts(ctx, params)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list blog posts")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Simple HTML response (you can enhance this with templates later)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s Blog</title>
</head>
<body>
<h1>%s Blog</h1>
<p>Coming soon: Blog posts will be displayed here.</p>
<p>Total posts: %d</p>
</body>
</html>`, workspace.Name, workspace.Name, response.TotalCount)))
}

// serveBlogPost serves a single blog post
func (h *RootHandler) serveBlogPost(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace, post *domain.BlogPost) {
	// Simple HTML response (you can enhance this with templates later)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	seoTitle := post.Settings.Title
	if post.Settings.SEO != nil && post.Settings.SEO.MetaTitle != "" {
		seoTitle = post.Settings.SEO.MetaTitle
	}

	seoDescription := post.Settings.Excerpt
	if post.Settings.SEO != nil && post.Settings.SEO.MetaDescription != "" {
		seoDescription = post.Settings.SEO.MetaDescription
	}

	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
<meta name="description" content="%s">
</head>
<body>
<h1>%s</h1>
<p>Reading time: %d minutes</p>
<p>Coming soon: Full blog post content will be displayed here.</p>
</body>
</html>`, seoTitle, seoDescription, post.Settings.Title, post.Settings.ReadingTimeMinutes)))
}

// serveBlogRobots serves robots.txt for the blog
func (h *RootHandler) serveBlogRobots(w http.ResponseWriter, r *http.Request) {
	robotsTxt := `User-agent: *
Allow: /

Sitemap: /sitemap.xml
`
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(robotsTxt))
}

// serveBlogSitemap serves sitemap.xml for the blog
func (h *RootHandler) serveBlogSitemap(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace) {
	ctx := context.WithValue(r.Context(), "workspace_id", workspace.ID)

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
	w.Write([]byte(sitemap.String()))
}

// serveBlog404 serves a 404 page for blog
func (h *RootHandler) serveBlog404(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!DOCTYPE html>
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

	// List all workspaces and find one that matches the host
	// Note: This could be optimized with a dedicated repository method in the future
	workspaces, err := h.workspaceRepo.List(ctx)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to list workspaces for host detection")
		return nil
	}

	for _, workspace := range workspaces {
		if workspace.Settings.CustomEndpointURL == nil {
			continue
		}

		customURL := *workspace.Settings.CustomEndpointURL
		parsedURL, err := url.Parse(customURL)
		if err != nil {
			continue
		}

		// Compare hostnames (case-insensitive)
		if strings.EqualFold(parsedURL.Hostname(), host) {
			h.logger.
				WithField("workspace_id", workspace.ID).
				WithField("workspace_name", workspace.Name).
				WithField("host", host).
				Debug("Workspace detected by host")
			return workspace
		}
	}

	h.logger.WithField("host", host).Debug("No workspace found for host")
	return nil
}

func (h *RootHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/config.js", h.serveConfigJS)
	// catch all route
	mux.HandleFunc("/", h.Handle)
}
