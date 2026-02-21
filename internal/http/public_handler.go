package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/botdetection"
	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/PuerkitoBio/goquery"
)

type NotificationCenterHandler struct {
	service     domain.NotificationCenterService
	listService domain.ListService
	logger      logger.Logger
	rateLimiter *ratelimiter.RateLimiter
}

func NewNotificationCenterHandler(service domain.NotificationCenterService, listService domain.ListService, logger logger.Logger, rateLimiter *ratelimiter.RateLimiter) *NotificationCenterHandler {
	return &NotificationCenterHandler{
		service:     service,
		listService: listService,
		logger:      logger,
		rateLimiter: rateLimiter,
	}
}

// FaviconRequest represents the request to detect a favicon
type FaviconRequest struct {
	URL string `json:"url"`
}

// FaviconResponse represents the response with detected favicon and cover URLs
type FaviconResponse struct {
	IconURL  string `json:"iconUrl,omitempty"`
	CoverURL string `json:"coverUrl,omitempty"`
	Message  string `json:"message,omitempty"`
}

func (h *NotificationCenterHandler) RegisterRoutes(mux *http.ServeMux) {
	// Register public routes
	mux.HandleFunc("/preferences", h.handlePreferences)
	mux.HandleFunc("/subscribe", h.handleSubscribe)
	// one-click unsubscribe for GMAIL header link
	mux.HandleFunc("/unsubscribe-oneclick", h.handleUnsubscribeOneClick)
	// public health endpoint with connection stats
	mux.HandleFunc("/health", h.handleHealth)
	// lightweight health check for container orchestration
	mux.HandleFunc("/healthz", h.handleHealthz)
	// favicon detection endpoint
	mux.HandleFunc("/api/detect-favicon", h.HandleDetectFavicon)
}

func (h *NotificationCenterHandler) handlePreferences(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGetPreferences(w, r)
	case http.MethodPost:
		h.handleUpdatePreferences(w, r)
	default:
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *NotificationCenterHandler) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	var req domain.NotificationCenterRequest
	if err := req.FromURLValues(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Note: Confirmation action is now handled by the frontend via AJAX
	// The frontend will detect action=confirm and make a POST request to /subscribe

	// Get notification center data for the contact
	response, err := h.service.GetContactPreferences(r.Context(), req.WorkspaceID, req.Email, req.EmailHMAC)
	if err != nil {
		if strings.Contains(err.Error(), "invalid email verification") {
			WriteJSONError(w, "Unauthorized: invalid verification", http.StatusUnauthorized)
			return
		}
		if strings.Contains(err.Error(), "contact not found") {
			WriteJSONError(w, "Contact not found", http.StatusNotFound)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to get contact preferences")
		WriteJSONError(w, "Failed to get contact preferences", http.StatusInternalServerError)
		return
	}

	// Write the response
	writeJSON(w, http.StatusOK, response)
}

func (h *NotificationCenterHandler) handleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	var req domain.UpdateContactPreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Rate limit by email
	if h.rateLimiter != nil && !h.rateLimiter.Allow("preferences:email", req.Email) {
		retryAfter := h.rateLimiter.GetRemainingWindow("preferences:email", req.Email)
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
		WriteJSONError(w, "Too many requests. Please try again later.", http.StatusTooManyRequests)
		return
	}

	// Rate limit by IP
	clientIP := getClientIP(r)
	if h.rateLimiter != nil && !h.rateLimiter.Allow("preferences:ip", clientIP) {
		retryAfter := h.rateLimiter.GetRemainingWindow("preferences:ip", clientIP)
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
		WriteJSONError(w, "Too many requests. Please try again later.", http.StatusTooManyRequests)
		return
	}

	if err := h.service.UpdateContactPreferences(r.Context(), &req); err != nil {
		if strings.Contains(err.Error(), "invalid email verification") {
			WriteJSONError(w, "Unauthorized: invalid verification", http.StatusUnauthorized)
			return
		}
		h.logger.WithField("error", err.Error()).Error("Failed to update contact preferences")
		WriteJSONError(w, "Failed to update contact preferences", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *NotificationCenterHandler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SubscribeToListsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to validate request")
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check rate limit by email
	if h.rateLimiter != nil && !h.rateLimiter.Allow("subscribe:email", req.Contact.Email) {
		retryAfter := h.rateLimiter.GetRemainingWindow("subscribe:email", req.Contact.Email)
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
		h.logger.WithField("email", req.Contact.Email).Warn("Subscribe: Rate limit exceeded")
		WriteJSONError(w, "Too many subscription attempts. Please try again in a few minutes.", http.StatusTooManyRequests)
		return
	}

	// Check rate limit by IP (prevents email-spam attacks)
	clientIP := getClientIP(r)
	if h.rateLimiter != nil && !h.rateLimiter.Allow("subscribe:ip", clientIP) {
		retryAfter := h.rateLimiter.GetRemainingWindow("subscribe:ip", clientIP)
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
		h.logger.WithField("ip", clientIP).Warn("Subscribe: IP rate limit exceeded")
		WriteJSONError(w, "Too many subscription attempts. Please try again in a few minutes.", http.StatusTooManyRequests)
		return
	}

	fromAPI := false

	if err := h.listService.SubscribeToLists(r.Context(), &req, fromAPI); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to subscribe to lists")

		// Return specific error for non-public lists (matches OpenAPI spec)
		if strings.Contains(err.Error(), "list is not public") {
			WriteJSONError(w, "list is not public", http.StatusBadRequest)
			return
		}

		WriteJSONError(w, "Failed to subscribe to lists", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *NotificationCenterHandler) handleUnsubscribeOneClick(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// this is one-click unsubscribe from GMAIL header link

	var req domain.UnsubscribeFromListsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Bot detection: check user agent before processing unsubscribe
	userAgent := r.Header.Get("User-Agent")
	if botdetection.IsBotUserAgent(userAgent) {
		// Return success without actually unsubscribing to avoid revealing bot detection
		h.logger.WithField("user_agent", userAgent).Debug("Bot detected by user agent - not processing unsubscribe")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
		})
		return
	}

	fromBearerToken := false

	if err := h.listService.UnsubscribeFromLists(r.Context(), &req, fromBearerToken); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to unsubscribe from lists")
		WriteJSONError(w, "Failed to unsubscribe from lists", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *NotificationCenterHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get connection manager
	connManager, err := pkgDatabase.GetConnectionManager()
	if err != nil {
		h.logger.Error("Failed to get connection manager")
		WriteJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get stats
	stats := connManager.GetStats()

	// Create response without workspace-specific details
	response := map[string]interface{}{
		"max_connections":            stats.MaxConnections,
		"max_connections_per_db":     stats.MaxConnectionsPerDB,
		"system_connections":         stats.SystemConnections,
		"total_open_connections":     stats.TotalOpenConnections,
		"total_in_use_connections":   stats.TotalInUseConnections,
		"total_idle_connections":     stats.TotalIdleConnections,
		"active_workspace_databases": stats.ActiveWorkspaceDatabases,
	}

	// Return as JSON
	writeJSON(w, http.StatusOK, response)
}

func (h *NotificationCenterHandler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get connection manager
	connManager, err := pkgDatabase.GetConnectionManager()
	if err != nil {
		h.logger.Error("Failed to get connection manager")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
		})
		return
	}

	// Get system database connection
	systemDB := connManager.GetSystemConnection()
	if systemDB == nil {
		h.logger.Error("System database connection is nil")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
		})
		return
	}

	// Ping the database with a 2-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := systemDB.PingContext(ctx); err != nil {
		h.logger.WithField("error", err.Error()).Error("Database ping failed")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
		})
		return
	}

	// Database is healthy
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (h *NotificationCenterHandler) HandleDetectFavicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req FaviconRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	// Validate URL
	baseURL, err := url.Parse(req.URL)
	if err != nil {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// Fetch the webpage
	resp, err := http.Get(req.URL)
	if err != nil {
		http.Error(w, "Error fetching URL", http.StatusInternalServerError)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		http.Error(w, "Error parsing HTML", http.StatusInternalServerError)
		return
	}

	// Prepare response with both icon and cover URLs
	response := FaviconResponse{}

	// Check for cover image
	if coverURL := findOpenGraphImage(doc, baseURL); coverURL != "" {
		response.CoverURL = coverURL
	} else if coverURL := findTwitterCardImage(doc, baseURL); coverURL != "" {
		response.CoverURL = coverURL
	} else if coverURL := findLargeImage(doc, baseURL); coverURL != "" {
		response.CoverURL = coverURL
	}

	// Check for apple-touch-icon
	if iconURL := findAppleTouchIcon(doc, baseURL); iconURL != "" {
		response.IconURL = iconURL
	} else if iconURL := findManifestIcon(doc, baseURL); iconURL != "" { // Check for manifest.json
		response.IconURL = iconURL
	} else if iconURL := findTraditionalFavicon(doc, baseURL); iconURL != "" { // Check for traditional favicon
		response.IconURL = iconURL
	} else if iconURL := tryDefaultFavicon(baseURL); iconURL != "" { // Try default favicon location
		response.IconURL = iconURL
	}

	// Return the combined results
	if response.IconURL != "" || response.CoverURL != "" {
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	http.Error(w, "No favicon or cover image found", http.StatusNotFound)
}

// Favicon detection helper functions

func findAppleTouchIcon(doc *goquery.Document, baseURL *url.URL) string {
	var iconURL string
	doc.Find("link[rel='apple-touch-icon']").Each(func(_ int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			if resolvedURL, err := resolveURL(baseURL, href); err == nil {
				iconURL = resolvedURL
				return
			}
		}
	})
	return iconURL
}

func findManifestIcon(doc *goquery.Document, baseURL *url.URL) string {
	var iconURL string
	doc.Find("link[rel='manifest']").Each(func(_ int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			manifestURL, err := resolveURL(baseURL, href)
			if err != nil {
				return
			}

			resp, err := http.Get(manifestURL)
			if err != nil {
				return
			}
			defer func() { _ = resp.Body.Close() }()

			var manifest struct {
				Icons []struct {
					Src   string `json:"src"`
					Sizes string `json:"sizes"`
				} `json:"icons"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
				return
			}

			if len(manifest.Icons) > 0 {
				// Find the largest icon
				largestIcon := manifest.Icons[0]
				for _, icon := range manifest.Icons[1:] {
					if icon.Sizes > largestIcon.Sizes {
						largestIcon = icon
					}
				}

				if resolvedURL, err := resolveURL(baseURL, largestIcon.Src); err == nil {
					iconURL = resolvedURL
				}
			}
		}
	})
	return iconURL
}

func findTraditionalFavicon(doc *goquery.Document, baseURL *url.URL) string {
	var iconURL string
	doc.Find("link[rel='icon'], link[rel='shortcut icon']").Each(func(_ int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			if resolvedURL, err := resolveURL(baseURL, href); err == nil {
				iconURL = resolvedURL
				return
			}
		}
	})
	return iconURL
}

func tryDefaultFavicon(baseURL *url.URL) string {
	faviconURL := baseURL.ResolveReference(&url.URL{Path: "/favicon.ico"}).String()
	resp, err := http.Head(faviconURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		return faviconURL
	}
	return ""
}

func resolveURL(baseURL *url.URL, href string) (string, error) {
	// Handle absolute URLs
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href, nil
	}

	// Handle protocol-relative URLs (//domain.com/path)
	if strings.HasPrefix(href, "//") {
		return baseURL.Scheme + ":" + href, nil
	}

	// Parse the href to properly handle paths with query strings
	refURL, err := url.Parse(href)
	if err != nil {
		return "", err
	}

	// Resolve the reference URL against the base URL
	resolvedURL := baseURL.ResolveReference(refURL)
	return resolvedURL.String(), nil
}

func findOpenGraphImage(doc *goquery.Document, baseURL *url.URL) string {
	var ogImage string
	doc.Find("meta[property='og:image']").Each(func(_ int, s *goquery.Selection) {
		if content, exists := s.Attr("content"); exists && content != "" {
			if resolvedURL, err := resolveURL(baseURL, content); err == nil {
				ogImage = resolvedURL
				return
			}
		}
	})
	return ogImage
}

func findTwitterCardImage(doc *goquery.Document, baseURL *url.URL) string {
	var twitterImage string
	doc.Find("meta[name='twitter:image']").Each(func(_ int, s *goquery.Selection) {
		if content, exists := s.Attr("content"); exists && content != "" {
			if resolvedURL, err := resolveURL(baseURL, content); err == nil {
				twitterImage = resolvedURL
				return
			}
		}
	})
	return twitterImage
}

func findLargeImage(doc *goquery.Document, baseURL *url.URL) string {
	var largeImage string
	var maxWidth, maxHeight int

	doc.Find("img[src]").Each(func(_ int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists || src == "" {
			return
		}

		// Check for width and height attributes
		width := 0
		height := 0
		if w, exists := s.Attr("width"); exists {
			if wInt, err := parseInt(w); err == nil {
				width = wInt
			}
		}
		if h, exists := s.Attr("height"); exists {
			if hInt, err := parseInt(h); err == nil {
				height = hInt
			}
		}

		// If this image is larger than previous ones, remember it
		if width*height > maxWidth*maxHeight {
			maxWidth = width
			maxHeight = height
			if resolvedURL, err := resolveURL(baseURL, src); err == nil {
				largeImage = resolvedURL
			}
		}
	})

	return largeImage
}

func parseInt(val string) (int, error) {
	var result int
	_, err := fmt.Sscanf(val, "%d", &result)
	return result, err
}

// getClientIP extracts the client IP from the request, checking X-Forwarded-For first
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (if behind proxy)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take first IP in the list
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}
	return ip
}
