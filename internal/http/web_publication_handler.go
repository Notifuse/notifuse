package http

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

//go:embed templates/*.html
var templateFS embed.FS

// WebPublicationHandler handles web publication requests
type WebPublicationHandler struct {
	webPublicationService *service.WebPublicationService
	logger                logger.Logger
	templates             *template.Template
}

// NewWebPublicationHandler creates a new web publication handler
func NewWebPublicationHandler(
	webPublicationService *service.WebPublicationService,
	logger logger.Logger,
) *WebPublicationHandler {
	// Load embedded templates
	templates := template.Must(template.ParseFS(templateFS, "templates/*.html"))

	return &WebPublicationHandler{
		webPublicationService: webPublicationService,
		logger:                logger,
		templates:             templates,
	}
}

// Handle routes web publication requests
func (h *WebPublicationHandler) Handle(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace) {
	switch {
	case r.URL.Path == "/":
		h.handleList(w, r, workspace)
	case r.URL.Path == "/robots.txt":
		h.handleRobots(w, r, workspace)
	case r.URL.Path == "/sitemap.xml":
		h.handleSitemap(w, r, workspace)
	case isPublicationSlug(r.URL.Path):
		h.handlePost(w, r, workspace)
	default:
		h.handle404(w, r)
	}
}

// handleList displays a paginated list of published posts
func (h *WebPublicationHandler) handleList(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace) {
	// Parse page parameter
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// Get published posts
	posts, err := h.webPublicationService.GetPublishedPosts(r.Context(), workspace.ID, page, 20)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get published posts")
		h.handleError(w, r, "Failed to load posts", http.StatusInternalServerError)
		return
	}

	// Prepare template data
	data := map[string]interface{}{
		"WorkspaceName": workspace.Name,
		"Posts":         posts.Posts,
		"Page":          posts.Page,
		"TotalPages":    posts.TotalPages,
		"HasPrevious":   posts.HasPrevious,
		"HasNext":       posts.HasNext,
		"PrevPage":      posts.Page - 1,
		"NextPage":      posts.Page + 1,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "list.html", data); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to execute list template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handlePost displays a single post
func (h *WebPublicationHandler) handlePost(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace) {
	// Extract slug from path
	slug := strings.TrimPrefix(r.URL.Path, "/")

	// Get post
	post, err := h.webPublicationService.GetPostBySlug(r.Context(), workspace.ID, slug)
	if err != nil {
		h.logger.WithFields(map[string]interface{}{
			"workspace_id": workspace.ID,
			"slug":         slug,
			"error":        err.Error(),
		}).Debug("Post not found")
		h.handle404(w, r)
		return
	}

	// Build current URL for Open Graph
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	currentURL := fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.Path)

	// Prepare template data
	data := map[string]interface{}{
		"Post":       post,
		"CurrentURL": currentURL,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "post.html", data); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to execute post template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// handleRobots serves robots.txt
func (h *WebPublicationHandler) handleRobots(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace) {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	sitemapURL := fmt.Sprintf("%s://%s/sitemap.xml", scheme, r.Host)

	robotsTxt := fmt.Sprintf(`User-agent: *
Allow: /

Sitemap: %s
`, sitemapURL)

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(robotsTxt))
}

// handleSitemap serves sitemap.xml
func (h *WebPublicationHandler) handleSitemap(w http.ResponseWriter, r *http.Request, workspace *domain.Workspace) {
	// Get all published posts (up to 1000)
	posts, err := h.webPublicationService.GetPublishedPosts(r.Context(), workspace.ID, 1, 1000)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get posts for sitemap")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}

	// Build sitemap XML
	var sitemap strings.Builder
	sitemap.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sitemap.WriteString("\n")
	sitemap.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	sitemap.WriteString("\n")

	// Add homepage
	sitemap.WriteString("  <url>\n")
	sitemap.WriteString(fmt.Sprintf("    <loc>%s://%s/</loc>\n", scheme, r.Host))
	sitemap.WriteString("    <changefreq>daily</changefreq>\n")
	sitemap.WriteString("    <priority>1.0</priority>\n")
	sitemap.WriteString("  </url>\n")

	// Add posts
	for _, post := range posts.Posts {
		sitemap.WriteString("  <url>\n")
		sitemap.WriteString(fmt.Sprintf("    <loc>%s://%s/%s</loc>\n", scheme, r.Host, post.Slug))
		sitemap.WriteString(fmt.Sprintf("    <lastmod>%s</lastmod>\n", post.PublishedAt.Format("2006-01-02")))
		sitemap.WriteString("    <changefreq>monthly</changefreq>\n")
		sitemap.WriteString("    <priority>0.8</priority>\n")
		sitemap.WriteString("  </url>\n")
	}

	sitemap.WriteString("</urlset>")

	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(sitemap.String()))
}

// handle404 serves 404 error page
func (h *WebPublicationHandler) handle404(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "error_404.html", nil); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to execute 404 template")
		http.Error(w, "404 Not Found", http.StatusNotFound)
	}
}

// handleError serves generic error page
func (h *WebPublicationHandler) handleError(w http.ResponseWriter, r *http.Request, message string, statusCode int) {
	w.WriteHeader(statusCode)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	errorHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Error</title>
</head>
<body style="font-family: sans-serif; padding: 2rem; text-align: center;">
    <h1>Oops!</h1>
    <p>%s</p>
    <p><a href="/">Go back</a></p>
</body>
</html>`, message)

	w.Write([]byte(errorHTML))
}

// isPublicationSlug checks if the path matches a publication slug pattern
func isPublicationSlug(path string) bool {
	path = strings.TrimPrefix(path, "/")
	// Match pattern: {slug}-{6-char-nanoid}
	matched, _ := regexp.MatchString(`^[a-z0-9-]+-[a-z0-9]{6}$`, path)
	return matched
}
