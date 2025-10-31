package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/PuerkitoBio/goquery"
)

type NotificationCenterHandler struct {
	service     domain.NotificationCenterService
	listService domain.ListService
	logger      logger.Logger
}

func NewNotificationCenterHandler(service domain.NotificationCenterService, listService domain.ListService, logger logger.Logger) *NotificationCenterHandler {
	return &NotificationCenterHandler{
		service:     service,
		listService: listService,
		logger:      logger,
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
	// favicon detection endpoint
	mux.HandleFunc("/api/detect-favicon", h.HandleDetectFavicon)
}

func (h *NotificationCenterHandler) handlePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	fromAPI := false

	if err := h.listService.SubscribeToLists(r.Context(), &req, fromAPI); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to subscribe to lists")
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
	defer resp.Body.Close()

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
		json.NewEncoder(w).Encode(response)
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
			defer resp.Body.Close()

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
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href, nil
	}
	resolvedURL := baseURL.ResolveReference(&url.URL{Path: href})
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
