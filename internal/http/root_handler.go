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
	webPublicationHandler WebPublicationHandler
	workspaceRepo         domain.WorkspaceRepository
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
	webPublicationHandler WebPublicationHandler,
	workspaceRepo domain.WorkspaceRepository,
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
		webPublicationHandler: webPublicationHandler,
		workspaceRepo:         workspaceRepo,
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

	// 5. ROOT PATH LOGIC: Detect workspace by host header
	host := strings.Split(r.Host, ":")[0] // Strip port
	workspace := h.detectWorkspaceByHost(r.Context(), host)

	if workspace == nil {
		// No workspace found for this domain → redirect to console
		http.Redirect(w, r, "/console", http.StatusTemporaryRedirect)
		return
	}

	// 6. Workspace found - check if web publications feature is enabled
	if !workspace.Settings.WebPublicationsEnabled {
		// Web publications not enabled → redirect to console
		http.Redirect(w, r, "/console", http.StatusTemporaryRedirect)
		return
	}

	// 7. Web publications enabled - serve web content
	h.webPublicationHandler.Handle(w, r, workspace)
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
