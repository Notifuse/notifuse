package http

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

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
}

// NewRootHandler creates a root handler that serves both console and notification center static files
func NewRootHandler(consoleDir string, notificationCenterDir string, logger logger.Logger, apiEndpoint string, version string, rootEmail string, isInstalledPtr *bool) *RootHandler {
	return &RootHandler{
		consoleDir:            consoleDir,
		notificationCenterDir: notificationCenterDir,
		logger:                logger,
		apiEndpoint:           apiEndpoint,
		version:               version,
		rootEmail:             rootEmail,
		isInstalledPtr:        isInstalledPtr,
	}
}

func (h *RootHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// Handle config.js request
	if r.URL.Path == "/config.js" {
		h.serveConfigJS(w, r)
		return
	}

	// Handle notification center requests
	if strings.HasPrefix(r.URL.Path, "/notification-center") || strings.Contains(r.Header.Get("Referer"), "/notification-center") {
		h.serveNotificationCenter(w, r)
		return
	}

	// If  path doesn't start with /api
	if !strings.HasPrefix(r.URL.Path, "/api") {
		h.serveConsole(w, r)
		return
	}

	// Default API root response
	if r.URL.Path == "/api" || r.URL.Path == "/api/" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "api running",
		})
	} else {
		// For unhandled API paths
		http.NotFound(w, r)
	}
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

	configJS := fmt.Sprintf(
		"window.API_ENDPOINT = %q;\nwindow.VERSION = %q;\nwindow.ROOT_EMAIL = %q;\nwindow.IS_INSTALLED = %s;",
		h.apiEndpoint,
		h.version,
		h.rootEmail,
		isInstalledStr,
	)
	w.Write([]byte(configJS))
}

// serveConsole handles serving static files, with a fallback for SPA routing
func (h *RootHandler) serveConsole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create file server for console files
	fs := http.FileServer(http.Dir(h.consoleDir))

	path := h.consoleDir + r.URL.Path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// If the requested file doesn't exist, serve index.html for SPA routing
		r.URL.Path = "/"
	}

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

func (h *RootHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/config.js", h.serveConfigJS)
	// catch all route
	mux.HandleFunc("/", h.Handle)
}
