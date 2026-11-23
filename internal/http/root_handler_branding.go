package http

import (
	"encoding/json"
	"encoding/xml"
	"net/http"

	"github.com/Notifuse/notifuse/pkg/branding"
)

// The console paths the instance brand reaches before React starts. They are served by Go,
// ahead of the console's file server, because their content depends on the brand and the
// licence rather than on the build.
const (
	instanceManifestPath = "/console/instance.webmanifest"
	legacyManifestPath   = "/console/site.webmanifest"
	browserConfigPath    = "/console/browserconfig.xml"
)

// SetBranding supplies the identity to show, asked on every request so a licence that expires
// while the process runs shows Notifuse from the next page load. nil shows the default identity.
func (h *RootHandler) SetBranding(effective func() branding.Resolved) {
	h.branding = effective
}

func (h *RootHandler) effectiveBranding() branding.Resolved {
	if h.branding == nil {
		return branding.Resolve(branding.Branding{})
	}
	return h.branding()
}

// allowGetHead answers 405 with an Allow header to anything but GET and HEAD, and reports
// whether the request may go on.
func allowGetHead(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
	return false
}

func setNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// serveBrandingAsset serves the three brand-dependent console paths and reports whether the
// path was one of them.
func (h *RootHandler) serveBrandingAsset(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case instanceManifestPath:
		h.serveInstanceManifest(w, r)
	case legacyManifestPath:
		if !allowGetHead(w, r) {
			return true
		}
		setNoStore(w)
		http.Redirect(w, r, instanceManifestPath, http.StatusTemporaryRedirect)
	case browserConfigPath:
		h.serveBrowserConfig(w, r)
	default:
		return false
	}
	return true
}

type manifestIcon struct {
	Src   string `json:"src"`
	Sizes string `json:"sizes,omitempty"`
	Type  string `json:"type,omitempty"`
}

type webManifest struct {
	Name            string         `json:"name"`
	ShortName       string         `json:"short_name"`
	StartURL        string         `json:"start_url"`
	Scope           string         `json:"scope"`
	Icons           []manifestIcon `json:"icons"`
	ThemeColor      string         `json:"theme_color"`
	BackgroundColor string         `json:"background_color"`
	Display         string         `json:"display"`
}

// instanceManifest builds the web app manifest. Local icons carry their known size and type; a
// configured icon URL carries neither, since nothing here fetches it to find out.
func instanceManifest(b branding.Resolved) webManifest {
	m := webManifest{
		Name:            b.Name,
		ShortName:       b.Name,
		StartURL:        "/console/",
		Scope:           "/console/",
		ThemeColor:      "#ffffff",
		BackgroundColor: "#ffffff",
		Display:         "standalone",
	}
	switch {
	case !b.Custom:
		m.Icons = []manifestIcon{
			{Src: "/console/android-chrome-192x192.png", Sizes: "192x192", Type: "image/png"},
			{Src: "/console/android-chrome-384x384.png", Sizes: "384x384", Type: "image/png"},
		}
	case b.IconURL != "":
		// The neutral icon follows as a fallback with a declared size: Chrome only counts an
		// icon whose size it knows when it decides whether the console can be installed.
		m.Icons = []manifestIcon{
			{Src: b.IconURL},
			{Src: branding.NeutralIconPath, Sizes: "192x192", Type: "image/png"},
		}
	default:
		// branding.NeutralIconPath is a 192 × 192 PNG shipped with the console.
		m.Icons = []manifestIcon{{Src: branding.NeutralIconPath, Sizes: "192x192", Type: "image/png"}}
	}
	return m
}

func (h *RootHandler) serveInstanceManifest(w http.ResponseWriter, r *http.Request) {
	if !allowGetHead(w, r) {
		return
	}
	body, err := json.Marshal(instanceManifest(h.effectiveBranding()))
	if err != nil {
		WriteJSONError(w, "Failed to build the manifest", http.StatusInternalServerError)
		return
	}
	setNoStore(w)
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}

type browserConfigXML struct {
	XMLName       xml.Name `xml:"browserconfig"`
	MSApplication struct {
		Tile struct {
			Square150 struct {
				Src string `xml:"src,attr"`
			} `xml:"square150x150logo"`
			TileColor string `xml:"TileColor"`
		} `xml:"tile"`
	} `xml:"msapplication"`
}

// serveBrowserConfig serves the Windows tile configuration. In default mode it is the Notifuse
// tile; a custom brand answers 204, which turns the legacy tiles off, because their format needs a
// fixed-size image this version does not generate.
func (h *RootHandler) serveBrowserConfig(w http.ResponseWriter, r *http.Request) {
	if !allowGetHead(w, r) {
		return
	}
	setNoStore(w)
	if h.effectiveBranding().Custom {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var cfg browserConfigXML
	cfg.MSApplication.Tile.Square150.Src = "/console/mstile-150x150.png"
	cfg.MSApplication.Tile.TileColor = "#da532c"
	body, err := xml.MarshalIndent(cfg, "", "  ")
	if err != nil {
		WriteJSONError(w, "Failed to build the browser configuration", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
}

// instanceBrandingJS is the window.INSTANCE_BRANDING statement of /config.js. encoding/json
// escapes <, > and & by default, so no brand text can close the script it sits in.
func instanceBrandingJS(b branding.Resolved) string {
	data, err := json.Marshal(b)
	if err != nil {
		data, _ = json.Marshal(branding.Resolve(branding.Branding{}))
	}
	return "window.INSTANCE_BRANDING = " + string(data) + ";"
}
