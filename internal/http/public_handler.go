package http

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/Notifuse/notifuse/pkg/safehttpclient"
	"github.com/PuerkitoBio/goquery"
)

type NotificationCenterHandler struct {
	service     domain.NotificationCenterService
	listService domain.ListService
	logger      logger.Logger
	rateLimiter *ratelimiter.RateLimiter
	httpClient  *http.Client // SSRF-safe client for outbound requests
	// iconDetectBudget bounds one detect-favicon call, every fetch included
	iconDetectBudget time.Duration
}

func NewNotificationCenterHandler(service domain.NotificationCenterService, listService domain.ListService, logger logger.Logger, rateLimiter *ratelimiter.RateLimiter) *NotificationCenterHandler {
	return &NotificationCenterHandler{
		service:          service,
		listService:      listService,
		logger:           logger,
		rateLimiter:      rateLimiter,
		httpClient:       safehttpclient.New(),
		iconDetectBudget: defaultIconDetectBudget,
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
	// first-party notification center unsubscribe (widget + console), JSON body
	mux.HandleFunc("/unsubscribe", h.handleUnsubscribe)
	// RFC 8058 one-click unsubscribe target baked into the List-Unsubscribe email header
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

// maxUnsubscribeBodyBytes caps the request body read by the unsubscribe handlers - the
// SPA payload is a few hundred bytes; anything larger is malformed or hostile.
const maxUnsubscribeBodyBytes = 4096

// handleUnsubscribe is the first-party notification center unsubscribe endpoint (the
// widget's "Unsubscribe" action and per-list toggle, and the console). It is the JSON
// sibling of /subscribe: the SPA POSTs {wid, email, email_hmac, lids, mid} and the
// email_hmac authorizes the request, verified by ListService.UnsubscribeFromLists.
//
// Mail-provider RFC 8058 one-click POSTs go to /unsubscribe-oneclick instead - that is
// the URL baked into the List-Unsubscribe email header.
func (h *NotificationCenterHandler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.UnsubscribeFromListsRequest
	if err := req.FromJSONBody(io.LimitReader(r.Body, maxUnsubscribeBodyBytes)); err != nil {
		h.logger.WithField("error", err.Error()).Error("Invalid notification center unsubscribe request")
		WriteJSONError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := h.listService.UnsubscribeFromLists(r.Context(), &req, false); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to unsubscribe from lists")
		WriteJSONError(w, "Failed to unsubscribe from lists", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *NotificationCenterHandler) handleUnsubscribeOneClick(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		h.redirectOneClickToConfirmation(w, r)
		return
	}

	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// This endpoint is the RFC 8058 one-click target baked into the List-Unsubscribe
	// header of sent emails, so it must keep serving mail-provider POSTs forever. It
	// branches on Content-Type. Either way authorization is owned by
	// ListService.UnsubscribeFromLists, which verifies email_hmac against the workspace
	// secret key.
	//
	//   1. Mail providers (Gmail/Yahoo/Apple) issue the RFC 8058 one-click POST: params
	//      in the URL query (RFC 8058 section 2.1) plus a "List-Unsubscribe=One-Click"
	//      token in the body (section 3.1). This is the primary, permanent contract.
	//   2. A JSON body (no query string) is accepted as a backward-compat shim: the
	//      notification center now posts to the dedicated /unsubscribe endpoint
	//      (handleUnsubscribe), but already-cached widget bundles may still hit this
	//      path. New code should use /unsubscribe.
	//
	// A GET or HEAD never unsubscribes - see redirectOneClickToConfirmation.
	var req domain.UnsubscribeFromListsRequest
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		// Backward-compat shim for the SPA (see above). A link-prefetcher/scanner cannot
		// reach here: it does not run the widget's JS and so never builds this JSON body,
		// so the RFC 8058 token gate is unnecessary for this shape - and email_hmac still
		// gates the actual unsubscribe in the service.
		if err := req.FromJSONBody(io.LimitReader(r.Body, maxUnsubscribeBodyBytes)); err != nil {
			h.logger.WithField("error", err.Error()).Error("Invalid notification center unsubscribe request")
			WriteJSONError(w, "Invalid request", http.StatusBadRequest)
			return
		}
	} else {
		// RFC 8058 one-click path. Per section 2.1 the parameters are carried in the URL
		// query string (not the body); read them from the query here.
		if err := req.FromOneClickURLParams(r.URL.Query()); err != nil {
			h.logger.WithField("error", err.Error()).Error("Invalid one-click unsubscribe request")
			WriteJSONError(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// RFC 8058 (section 3.1): the POST body carries the "List-Unsubscribe=One-Click"
		// pair (see carriesOneClickToken for its encodings). Requiring that token is the
		// defense-in-depth against link-prefetchers and security scanners that fire a
		// bare POST: a conformant one-click request always carries it. We do NOT apply
		// User-Agent bot detection here - an RFC 8058 POST is always machine-generated
		// by the mail provider, so a UA blocklist only drops legitimate unsubscribes
		// (the contact stays subscribed while the provider sees success). A missing
		// token is a real 400, never a silent success. Authorization is the HMAC below.
		bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, maxUnsubscribeBodyBytes))
		if !carriesOneClickToken(r.Header.Get("Content-Type"), bodyBytes) {
			h.logger.WithField("user_agent", r.Header.Get("User-Agent")).
				Warn("One-click unsubscribe POST missing RFC 8058 List-Unsubscribe=One-Click token")
			WriteJSONError(w, "Invalid request", http.StatusBadRequest)
			return
		}
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

// carriesOneClickToken reports whether a one-click POST body carries the RFC 8058
// "List-Unsubscribe=One-Click" pair. Receivers send it url-encoded (some as text/plain)
// or, as RFC 8058 section 3.2 prefers, as multipart/form-data, where the pair is split
// into a part named "List-Unsubscribe" whose content is "One-Click" and the literal
// "List-Unsubscribe=One-Click" never appears.
//
// The multipart body is matched as text rather than parsed: the RFC's own multipart
// example (section 8.3) declares a boundary its delimiter lines do not use, so a strict
// parser would reject a receiver that copied it. Text matching is enough here - this
// check only has to tell a one-click body from a scanner's bare POST, and email_hmac is
// what authorizes the unsubscribe.
func carriesOneClickToken(contentType string, body []byte) bool {
	text := string(body)
	if strings.Contains(text, "List-Unsubscribe=One-Click") {
		return true
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "multipart/form-data" &&
		strings.Contains(text, "List-Unsubscribe") &&
		strings.Contains(text, "One-Click")
}

// oneClickConfirmationAction is the notification center action a GET on the one-click
// URL lands on: the page names the list and unsubscribes only when the visitor clicks.
const oneClickConfirmationAction = "confirm_unsubscribe"

// redirectOneClickToConfirmation answers a GET or HEAD on the List-Unsubscribe URL. Mail
// clients open that URL in a browser instead of POSTing whenever they do not offer
// one-click - RFC 8058 section 4 tells them not to unless a DKIM signature covers both
// List-Unsubscribe headers - and people open it themselves. Anti-spam software also
// fetches header URLs on its own (RFC 8058 section 1), so a GET must not unsubscribe
// anyone: it redirects to the notification center, which asks the visitor to confirm.
// The redirect is host-relative, keeping the visitor on the domain the email linked to.
func (h *NotificationCenterHandler) redirectOneClickToConfirmation(w http.ResponseWriter, r *http.Request) {
	var req domain.UnsubscribeFromListsRequest
	if err := req.FromOneClickURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	params := url.Values{}
	params.Set("action", oneClickConfirmationAction)
	params.Set("wid", req.WorkspaceID)
	params.Set("email", req.Email)
	params.Set("email_hmac", req.EmailHMAC)
	// The emitted URL carries exactly one list (BuildTemplateData), and the confirmation
	// page asks about one list.
	params.Set("lid", req.ListIDs[0])
	if req.MessageID != "" {
		params.Set("mid", req.MessageID)
	}

	http.Redirect(w, r, "/notification-center?"+params.Encode(), http.StatusSeeOther)
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

	// Block non-http/https schemes
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		http.Error(w, "Only http and https URLs are supported", http.StatusBadRequest)
		return
	}

	// One budget covers the page, the manifest and every icon fetched to check it
	ctx, cancel := context.WithTimeout(r.Context(), h.iconDetectBudget)
	defer cancel()

	// Fetch the webpage using SSRF-safe client
	resp, err := fetchWithContext(ctx, h.httpClient, req.URL, "")
	if err != nil {
		http.Error(w, "Error fetching URL", http.StatusInternalServerError)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse HTML with a body size limit to prevent resource exhaustion
	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		http.Error(w, "Error parsing HTML", http.StatusInternalServerError)
		return
	}

	// Links resolve against the page reached after redirects, or its <base href>
	pageURL := finalResponseURL(resp, baseURL)
	docBase := documentBaseURL(doc, pageURL)

	// Prepare response with both icon and cover URLs
	response := FaviconResponse{}

	// Check for cover image
	if coverURL := findOpenGraphImage(doc, docBase); coverURL != "" {
		response.CoverURL = coverURL
	} else if coverURL := findTwitterCardImage(doc, docBase); coverURL != "" {
		response.CoverURL = coverURL
	} else if coverURL := findLargeImage(doc, docBase); coverURL != "" {
		response.CoverURL = coverURL
	}

	response.IconURL = h.detectIcon(ctx, doc, docBase, pageURL, baseURL)

	// Return the combined results
	if response.IconURL != "" || response.CoverURL != "" {
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	http.Error(w, "No favicon or cover image found", http.StatusNotFound)
}

// Favicon detection helper functions

const (
	// defaultIconDetectBudget bounds one detect-favicon call, every fetch included
	defaultIconDetectBudget = 20 * time.Second
	// maxIconChecks caps the icon candidates one call fetches. With the page and
	// the single manifest it reads, a call makes at most maxIconChecks+2 requests.
	maxIconChecks = 6
)

// iconCandidate is an icon declared by the page or its manifest, not yet fetched.
type iconCandidate struct {
	href     string   // as written in the page or manifest
	base     *url.URL // what href resolves against
	scalable bool     // SVG, or sizes="any"
	size     int      // largest declared side, 0 when undeclared
	demoted  bool     // a dark-scheme variant, or a manifest icon not meant for general use
}

// detectIcon returns the first declared icon that actually loads as an image.
// Groups are tried in a fixed order (apple-touch-icon, the web app manifest,
// rel=icon, then /favicon.ico) and best-first within a group. Nothing is
// returned unchecked: an empty or "#" href resolves to the page itself and a
// declared icon can be missing, and both used to become the workspace logo.
func (h *NotificationCenterHandler) detectIcon(ctx context.Context, doc *goquery.Document, docBase, pageURL, typedURL *url.URL) string {
	touch, icons, manifestHref := collectLinkIcons(doc, docBase)
	rankIconCandidates(touch)
	rankIconCandidates(icons)

	// The page is never its own icon, whichever URL it was reached by
	seen := map[string]bool{sameDocumentKey(typedURL): true, sameDocumentKey(pageURL): true}
	checks := 0
	firstImage := func(candidates []iconCandidate) string {
		for _, c := range candidates {
			if checks >= maxIconChecks || ctx.Err() != nil {
				return ""
			}
			iconURL, ok := usableIconURL(c.href, c.base, seen)
			if !ok {
				continue
			}
			checks++
			if verifyIconURL(ctx, h.httpClient, iconURL) {
				return iconURL
			}
		}
		return ""
	}

	if iconURL := firstImage(touch); iconURL != "" {
		return iconURL
	}
	if manifestURL, ok := usableIconURL(manifestHref, docBase, seen); ok {
		if iconURL := firstImage(fetchManifestIcons(ctx, h.httpClient, manifestURL)); iconURL != "" {
			return iconURL
		}
	}
	if iconURL := firstImage(icons); iconURL != "" {
		return iconURL
	}
	// Browsers look for /favicon.ico on the page's own origin, whatever <base> says
	origin := &url.URL{Scheme: pageURL.Scheme, Host: pageURL.Host}
	return firstImage([]iconCandidate{{href: "/favicon.ico", base: origin}})
}

// collectLinkIcons gathers a page's icon links in document order: touch icons,
// rel=icon links, and the href of the first manifest link, the only one a
// browser reads. rel is matched by token and case-insensitively, so
// "Shortcut Icon" counts and "mask-icon" (a monochrome Safari pin) does not.
func collectLinkIcons(doc *goquery.Document, base *url.URL) (touch, icons []iconCandidate, manifestHref string) {
	manifestFound := false
	doc.Find("link[rel][href]").Each(func(_ int, s *goquery.Selection) {
		rel, _ := s.Attr("rel")
		href, _ := s.Attr("href")
		for token := range strings.FieldsSeq(strings.ToLower(rel)) {
			switch token {
			case "apple-touch-icon", "apple-touch-icon-precomposed":
				touch = append(touch, linkIconCandidate(s, href, base))
				return
			case "icon":
				c := linkIconCandidate(s, href, base)
				media, _ := s.Attr("media")
				c.demoted = prefersDarkScheme(media)
				icons = append(icons, c)
				return
			case "manifest":
				if !manifestFound {
					manifestFound = true
					manifestHref = href
				}
				return
			}
		}
	})
	return touch, icons, manifestHref
}

func linkIconCandidate(s *goquery.Selection, href string, base *url.URL) iconCandidate {
	sizes, _ := s.Attr("sizes")
	declaredType, _ := s.Attr("type")
	size, scalable := parseIconSizes(sizes)
	return iconCandidate{href: href, base: base, size: size, scalable: scalable || isSVGIcon(href, declaredType)}
}

// parseIconSizes reads a sizes value ("16x16 32x32", "any") into its largest
// side and whether the icon is scalable. Unreadable tokens are ignored.
func parseIconSizes(sizes string) (largest int, scalable bool) {
	for token := range strings.FieldsSeq(strings.ToLower(sizes)) {
		if token == "any" {
			scalable = true
			continue
		}
		width, height, ok := strings.Cut(token, "x")
		if !ok {
			continue
		}
		w, errW := strconv.Atoi(width)
		h, errH := strconv.Atoi(height)
		if errW != nil || errH != nil || w <= 0 || h <= 0 {
			continue
		}
		largest = max(largest, w, h)
	}
	return largest, scalable
}

func isSVGIcon(href, declaredType string) bool {
	if mediaType, _, _ := mime.ParseMediaType(declaredType); mediaType == "image/svg+xml" {
		return true
	}
	u, err := url.Parse(cleanIconHref(href))
	return err == nil && strings.HasSuffix(strings.ToLower(u.Path), ".svg")
}

func prefersDarkScheme(media string) bool {
	return strings.Contains(strings.ReplaceAll(strings.ToLower(media), " ", ""), "prefers-color-scheme:dark")
}

// rankIconCandidates sorts best-first: general-purpose before demoted, scalable
// before raster, larger before smaller, then document order.
func rankIconCandidates(candidates []iconCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.demoted != b.demoted {
			return !a.demoted
		}
		if a.scalable != b.scalable {
			return a.scalable
		}
		return a.size > b.size
	})
}

// fetchManifestIcons reads the icons of a web app manifest. Their src values
// are relative to the manifest, not to the page that links it.
func fetchManifestIcons(ctx context.Context, client *http.Client, manifestURL string) []iconCandidate {
	resp, err := fetchWithContext(ctx, client, manifestURL, "")
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil
	}

	var manifest struct {
		Icons []struct {
			Src     string `json:"src"`
			Sizes   string `json:"sizes"`
			Type    string `json:"type"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1*1024*1024)).Decode(&manifest); err != nil {
		return nil
	}

	requested, err := url.Parse(manifestURL)
	if err != nil {
		return nil
	}
	base := finalResponseURL(resp, requested)
	candidates := make([]iconCandidate, 0, len(manifest.Icons))
	for _, icon := range manifest.Icons {
		size, scalable := parseIconSizes(icon.Sizes)
		candidates = append(candidates, iconCandidate{
			href:     icon.Src,
			base:     base,
			size:     size,
			scalable: scalable || isSVGIcon(icon.Src, icon.Type),
			// maskable and monochrome icons are cropped or flattened by design
			demoted: !purposeIncludesAny(icon.Purpose),
		})
	}
	rankIconCandidates(candidates)
	return candidates
}

// purposeIncludesAny reports whether a manifest icon is meant for general use;
// an icon without a purpose is.
func purposeIncludesAny(purpose string) bool {
	tokens := strings.Fields(strings.ToLower(purpose))
	return len(tokens) == 0 || slices.Contains(tokens, "any")
}

// usableIconURL resolves an icon href and reports whether it is worth fetching:
// an http(s) URL that is neither the page itself nor one already seen, which
// it then marks seen. An empty or fragment-only href resolves to the page.
func usableIconURL(href string, base *url.URL, seen map[string]bool) (string, bool) {
	href = cleanIconHref(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return "", false
	}
	resolved, err := resolveURL(base, href)
	if err != nil {
		return "", false
	}
	u, err := url.Parse(resolved)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	key := sameDocumentKey(u)
	if seen[key] {
		return "", false
	}
	seen[key] = true
	return resolved, true
}

// cleanIconHref strips what browsers' URL parser ignores: ASCII tab and newline
// anywhere, and C0 controls and spaces at either end. Go's url.Parse rejects the
// former and percent-encodes the latter.
func cleanIconHref(href string) string {
	href = strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(href)
	return strings.TrimFunc(href, func(r rune) bool { return r <= ' ' })
}

// sameDocumentKey identifies the resource a URL points at, ignoring the
// fragment and reading an empty path as "/".
func sameDocumentKey(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	key := strings.ToLower(u.Scheme+"://"+u.Host) + path
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return key
}

// verifyIconURL fetches an icon and reports whether a browser would show it: a
// 2xx answer whose declared type is an image or, when the server allows
// sniffing, whose first bytes are a raster image. Neither browsers nor
// http.DetectContentType sniff SVG, so an SVG must be served as image/svg+xml.
func verifyIconURL(ctx context.Context, client *http.Client, iconURL string) bool {
	resp, err := fetchWithContext(ctx, client, iconURL, "image/*,*/*;q=0.8")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false
	}
	if mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); strings.HasPrefix(mediaType, "image/") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("X-Content-Type-Options")), "nosniff") {
		return false
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(resp.Body, head)
	return strings.HasPrefix(http.DetectContentType(head[:n]), "image/")
}

func fetchWithContext(ctx context.Context, client *http.Client, rawURL, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return client.Do(req)
}

// finalResponseURL is the URL a response was served from, after redirects.
// Responses built by hand carry no request, hence the fallback.
func finalResponseURL(resp *http.Response, fallback *url.URL) *url.URL {
	if resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL
	}
	return fallback
}

// documentBaseURL is what the page's relative links resolve against: its first
// <base href>, itself resolved against the page URL, or the page URL.
func documentBaseURL(doc *goquery.Document, pageURL *url.URL) *url.URL {
	href, ok := doc.Find("base[href]").First().Attr("href")
	if !ok {
		return pageURL
	}
	ref, err := url.Parse(cleanIconHref(href))
	if err != nil {
		return pageURL
	}
	base := pageURL.ResolveReference(ref)
	if base.Scheme != "http" && base.Scheme != "https" {
		return pageURL
	}
	return base
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

// getClientIP extracts the client IP from the request, checking X-Forwarded-For first.
//
// The header is taken as presented, so the value is client-supplied unless a trusted
// proxy in front of the app overwrites it. Anything keyed on this IP — the rate
// limiters above all are — is therefore only as strong as the deployment's proxy
// configuration, and must never be the only thing standing between an attacker and a
// guessable secret. Where a limit here is the sole control, the secret itself needs
// enough entropy and a short enough life to survive without it.
func getClientIP(r *http.Request) string {
	return middleware.ClientIP(r)
}
