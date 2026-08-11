package notifuse_mjml

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/preslavrachev/gomjml/mjml"
)

// htmlVoidElements are HTML elements that must be self-closing in XML
var htmlVoidElements = []string{
	"area", "base", "br", "col", "embed", "hr", "img", "input",
	"link", "meta", "param", "source", "track", "wbr",
}

// htmlEntityToCodepoint maps HTML named entities to their Unicode code points
// Only entities not predefined in XML (amp, lt, gt, quot, apos) need conversion
var htmlEntityToCodepoint = map[string]int{
	// Whitespace and formatting
	"nbsp": 160, "ensp": 8194, "emsp": 8195, "thinsp": 8201,
	// Punctuation
	"bull": 8226, "hellip": 8230, "mdash": 8212, "ndash": 8211,
	"lsquo": 8216, "rsquo": 8217, "ldquo": 8220, "rdquo": 8221,
	"laquo": 171, "raquo": 187,
	// Symbols
	"copy": 169, "reg": 174, "trade": 8482, "sect": 167, "para": 182,
	"deg": 176, "plusmn": 177, "times": 215, "divide": 247,
	"micro": 181, "middot": 183,
	// Currency
	"euro": 8364, "pound": 163, "yen": 165, "cent": 162,
	// Arrows
	"larr": 8592, "rarr": 8594, "uarr": 8593, "darr": 8595, "harr": 8596,
	// Spanish/French punctuation
	"iexcl": 161, "iquest": 191,
}

// preprocessMjmlForXML preprocesses MJML string to fix common HTML vs XML incompatibilities
// This is necessary because gomjml uses a strict XML parser
func preprocessMjmlForXML(mjmlString string) string {
	processed := mjmlString

	// Step 1: Convert HTML void tags to self-closing XML format
	// HTML allows <br>, <hr>, <img>, etc. without closing slash
	// XML requires self-closing: <br/>, <hr/>, <img/>
	// Match: <br>, <br >, <hr>, <img src="...">, etc.
	// Don't match: <br/>, <br />
	voidTagPattern := regexp.MustCompile(
		`(?i)<(` + strings.Join(htmlVoidElements, "|") + `)(\s[^>]*)?>`,
	)
	processed = voidTagPattern.ReplaceAllStringFunc(processed, func(match string) string {
		// Check if already self-closing (ends with /> or / >)
		trimmed := strings.TrimSpace(match)
		if strings.HasSuffix(trimmed, "/>") {
			return match
		}

		// Extract tag name and attributes using submatch
		parts := voidTagPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		tagName := parts[1]
		attrs := ""
		if len(parts) > 2 && parts[2] != "" {
			attrs = strings.TrimRight(parts[2], " ")
		}
		return "<" + tagName + attrs + "/>"
	})

	// Step 2: Convert HTML named entities to XML numeric entities
	// XML only predefines: &amp; &lt; &gt; &quot; &apos;
	// HTML entities like &nbsp; must be converted to &#160;
	entityPattern := regexp.MustCompile(`&([a-zA-Z]+);`)
	processed = entityPattern.ReplaceAllStringFunc(processed, func(match string) string {
		// Extract entity name (without & and ;)
		entityName := strings.ToLower(match[1 : len(match)-1])

		// Preserve XML predefined entities
		if entityName == "amp" || entityName == "lt" || entityName == "gt" ||
			entityName == "quot" || entityName == "apos" {
			return match
		}

		// Convert known HTML entities to numeric
		if codepoint, ok := htmlEntityToCodepoint[entityName]; ok {
			return fmt.Sprintf("&#%d;", codepoint)
		}

		// Unknown entity - leave as-is
		return match
	})

	return processed
}

// MapOfAny represents a map of string to any value, used for template data
type MapOfAny map[string]any

// Per-notification tracking modes. The zero value ("") and TrackingModeInherit
// both follow the workspace tracking flag; TrackingModeDisabled suppresses all
// rewriting (redirect, pixel, and UTM parameters) because opted-out
// notifications carry single-use auth URLs that must never be modified.
const (
	TrackingModeInherit  = "inherit"
	TrackingModeDisabled = "disabled"
)

// WebIdentifyQueryParam is the URL parameter a tracked link carries so the web
// analytics SDK can adopt the recipient's identity with no customer code: the
// SDK reads it on landing, strips it from the address bar and sends it on the
// next beat (web_analytics_sdk/src/sdk.ts).
//
// The literal lives here rather than in internal/domain because that package
// imports this one — the link-rewriting pass below is the only Go code that
// writes the parameter, and the reverse import would be a cycle.
const WebIdentifyQueryParam = "nf_id"

// ValidateTrackingMode rejects unknown tracking mode values.
func ValidateTrackingMode(mode string) error {
	switch mode {
	case "", TrackingModeInherit, TrackingModeDisabled:
		return nil
	default:
		return fmt.Errorf("invalid tracking_mode %q: must be %q or %q", mode, TrackingModeInherit, TrackingModeDisabled)
	}
}

type TrackingSettings struct {
	EnableTracking bool `json:"enable_tracking"`
	// TrackingMode is the per-notification tri-state tracking preference; see
	// the TrackingMode* constants. Stored canonically: inherit is the absent value.
	TrackingMode string `json:"tracking_mode,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	UTMSource    string `json:"utm_source,omitempty"`
	UTMMedium    string `json:"utm_medium,omitempty"`
	UTMCampaign  string `json:"utm_campaign,omitempty"`
	UTMContent   string `json:"utm_content,omitempty"`
	UTMTerm      string `json:"utm_term,omitempty"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	MessageID    string `json:"message_id,omitempty"`
	// IdentifyToken is the encrypted identity minted for THIS recipient and
	// appended to tracked links as nf_id. It is a bearer credential: whoever
	// holds it is treated as that contact, so it is excluded from the JSON —
	// TrackingSettings is persisted as transactional_notifications.tracking_settings
	// and a per-recipient credential must never reach the database.
	IdentifyToken string `json:"-"`
	// IdentifyAllowedHosts are the workspace's web analytics allowed domains,
	// the only hosts the token above may be handed to. Request-scoped like the
	// token, hence also excluded from the persisted JSON.
	IdentifyAllowedHosts []string `json:"-"`
}

// IsZero reports whether no field is set. Callers use it to tell "no tracking
// settings supplied" from a real change; IdentifyAllowedHosts makes the struct
// non-comparable, so `== TrackingSettings{}` is not available for that check.
func (t TrackingSettings) IsZero() bool {
	return !t.EnableTracking &&
		t.TrackingMode == "" &&
		t.Endpoint == "" &&
		t.UTMSource == "" &&
		t.UTMMedium == "" &&
		t.UTMCampaign == "" &&
		t.UTMContent == "" &&
		t.UTMTerm == "" &&
		t.WorkspaceID == "" &&
		t.MessageID == "" &&
		t.IdentifyToken == "" &&
		len(t.IdentifyAllowedHosts) == 0
}

// Value implements the driver.Valuer interface for database storage
func (t TrackingSettings) Value() (driver.Value, error) {
	return json.Marshal(t)
}

// Scan implements the sql.Scanner interface for database retrieval
func (t *TrackingSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed for TrackingSettings")
	}

	return json.Unmarshal(v, t)
}

// isNonTrackableURL checks if a URL should not have click tracking applied.
// This includes special protocol links (mailto, tel, sms, etc.), template placeholders,
// and anchor links that should not be redirected through the tracking endpoint.
func isNonTrackableURL(urlStr string) bool {
	if urlStr == "" {
		return true
	}

	// Skip template placeholders (Liquid syntax)
	if strings.Contains(urlStr, "{{") || strings.Contains(urlStr, "{%") {
		return true
	}

	// Skip anchor-only links
	if strings.HasPrefix(urlStr, "#") {
		return true
	}

	// Skip special protocol links that should not be tracked
	lowerURL := strings.ToLower(urlStr)
	nonTrackableProtocols := []string{
		"mailto:",
		"tel:",
		"sms:",
		"javascript:",
		"data:",
		"blob:",
		"file:",
	}

	for _, protocol := range nonTrackableProtocols {
		if strings.HasPrefix(lowerURL, protocol) {
			return true
		}
	}

	return false
}

// applyUTMParameters appends the configured UTM parameters to sourceURL and
// returns the result. The URL is returned unchanged when it is empty, a Liquid
// placeholder, a mailto:/tel: link, cannot be parsed, or already carries any
// utm_* query parameter.
func (t *TrackingSettings) applyUTMParameters(sourceURL string) string {
	if sourceURL == "" || strings.Contains(sourceURL, "{{") || strings.Contains(sourceURL, "{%") ||
		strings.HasPrefix(sourceURL, "mailto:") || strings.HasPrefix(sourceURL, "tel:") {
		return sourceURL
	}

	parsedURL, err := url.Parse(sourceURL)
	if err != nil {
		return sourceURL
	}

	queryParams := parsedURL.Query()

	// If the URL already has UTM parameters, leave them untouched
	for key := range queryParams {
		if strings.HasPrefix(strings.ToLower(key), "utm_") {
			return sourceURL
		}
	}

	if t.UTMSource != "" {
		queryParams.Add("utm_source", t.UTMSource)
	}
	if t.UTMMedium != "" {
		queryParams.Add("utm_medium", t.UTMMedium)
	}
	if t.UTMCampaign != "" {
		queryParams.Add("utm_campaign", t.UTMCampaign)
	}
	if t.UTMContent != "" {
		queryParams.Add("utm_content", t.UTMContent)
	}
	if t.UTMTerm != "" {
		queryParams.Add("utm_term", t.UTMTerm)
	}
	parsedURL.RawQuery = queryParams.Encode()

	return parsedURL.String()
}

// MatchesAllowedHost reports whether hostname is covered by allowedHosts, using
// the workspace web analytics allowed-domains semantics: "*.example.com" covers
// the apex as well as any subdomain, comparison is case-insensitive and
// surrounding whitespace is tolerated.
//
// Unlike the workspace-level check, an empty list matches NOTHING here: this
// gate decides whether a per-recipient identity credential is appended to a
// link, so an unconfigured allowlist has to mean "no host" rather than "every
// host on the internet". For the same reason a wildcard over a bare TLD
// ("*.com") matches nothing at all.
func MatchesAllowedHost(hostname string, allowedHosts []string) bool {
	host := strings.ToLower(strings.TrimSpace(hostname))
	if host == "" {
		return false
	}
	for _, d := range allowedHosts {
		allowed := strings.ToLower(strings.TrimSpace(d))
		if allowed == "" {
			continue
		}
		if wild, ok := strings.CutPrefix(allowed, "*."); ok {
			// "*.com" matches nothing: a wildcard over a bare TLD would hand
			// this recipient's identity to every link in the email that happens
			// to point at that TLD, which is a different order of mistake from
			// the over-broad origin it would let through on the beat side. The
			// entry is skipped rather than treated as an error because this is
			// stored workspace configuration: one saved before it was validated
			// must lose its match without taking the rest of the allowlist with
			// it. The test is "the suffix contains a dot", not a public suffix
			// lookup, so "*.example.com" and "*.example.co.uk" keep working
			// (and the over-wide "*.co.uk" still gets through).
			if !strings.Contains(wild, ".") {
				continue
			}
			if host == wild || strings.HasSuffix(host, "."+wild) {
				return true
			}
			continue
		}
		if host == allowed {
			return true
		}
	}
	return false
}

// applyIdentityParam appends the per-recipient identity token to sourceURL as
// the nf_id parameter and returns the result. Like applyUTMParameters, the URL
// is returned unchanged when it is empty, a Liquid placeholder, a mailto:/tel:
// link, cannot be parsed, or already carries the parameter.
//
// It is additionally a no-op unless a token was minted AND the destination host
// is on the allowlist: the token identifies one contact to whoever holds it, so
// it may only be handed to the sites the workspace declared, never to whatever
// third-party link the email happens to contain.
//
// The parameter is appended by raw string surgery, so every other byte of the
// link survives exactly as its author wrote it. Rebuilding the URL through
// url.Values instead loses data: Encode() round-trips the query through a
// key/value map, which silently drops pairs it cannot split ("sid=1;2"),
// re-escapes bytes that were written literally ("discount=50%off") and reorders
// what is left. These are hand-built customer links — an order-dependent
// signature or a legacy semicolon separator has to come out the other side
// intact. internal/service/email_service.go's stripQueryParam does the same
// surgery for the inverse operation, and the two agree on what a pair is: split
// on '&', the key is what precedes the first '='.
func (t *TrackingSettings) applyIdentityParam(sourceURL string) string {
	if t.IdentifyToken == "" || len(t.IdentifyAllowedHosts) == 0 {
		return sourceURL
	}

	if sourceURL == "" || strings.Contains(sourceURL, "{{") || strings.Contains(sourceURL, "{%") ||
		strings.HasPrefix(sourceURL, "mailto:") || strings.HasPrefix(sourceURL, "tel:") {
		return sourceURL
	}

	// url.Parse is used to read the host for the allowlist gate and to reject
	// input that is not a URL at all. The string it would rebuild is never used.
	parsedURL, err := url.Parse(sourceURL)
	if err != nil {
		return sourceURL
	}

	if !MatchesAllowedHost(parsedURL.Hostname(), t.IdentifyAllowedHosts) {
		return sourceURL
	}

	// The query is what sits between the first '?' and the fragment. The pair is
	// appended at the end of the query and never after '#': a parameter in the
	// fragment stays in the browser and would never reach the destination.
	head, fragment := sourceURL, ""
	if hash := strings.IndexByte(sourceURL, '#'); hash >= 0 {
		head, fragment = sourceURL[:hash], sourceURL[hash:]
	}

	// Only the value we add is escaped; the token is ours, everything else in
	// the query is left byte-for-byte as it arrived.
	pair := WebIdentifyQueryParam + "=" + url.QueryEscape(t.IdentifyToken)

	mark := strings.IndexByte(head, '?')
	if mark < 0 {
		return head + "?" + pair + fragment
	}

	query := head[mark+1:]

	// A link that already carries an identity keeps it: the author may have
	// built the URL themselves, and overwriting it would silently change who
	// the landing page attributes the visit to.
	for _, existing := range strings.Split(query, "&") {
		key := existing
		if eq := strings.IndexByte(existing, '='); eq >= 0 {
			key = existing[:eq]
		}
		if key == WebIdentifyQueryParam {
			return sourceURL
		}
	}

	if query == "" {
		// "…/page?" already carries the separator the pair needs.
		return head + pair + fragment
	}

	return head + "&" + pair + fragment
}

func (t *TrackingSettings) GetTrackingURL(sourceURL string) string {
	// The explicit per-notification opt-out suppresses everything, including
	// UTM rewriting (single-use auth URLs must never be modified).
	if t.TrackingMode == TrackingModeDisabled {
		return sourceURL
	}

	// Ignore if URL is empty, a placeholder, mailto:, tel:, or already tracked (basic check)
	if sourceURL == "" || strings.Contains(sourceURL, "{{") || strings.Contains(sourceURL, "{%") || strings.HasPrefix(sourceURL, "mailto:") || strings.HasPrefix(sourceURL, "tel:") {
		return sourceURL
	}

	// Append UTM parameters to the destination URL
	destinationURL := t.applyUTMParameters(sourceURL)

	if !t.EnableTracking {
		return destinationURL
	}

	// parse endpoint and add the UTM-augmented destination URL to the query params
	parsedEndpoint, err := url.Parse(t.Endpoint)
	if err != nil {
		return sourceURL
	}
	endpointParams := parsedEndpoint.Query()
	endpointParams.Add("url", destinationURL)
	parsedEndpoint.RawQuery = endpointParams.Encode()

	return parsedEndpoint.String()
}

// CompileTemplateRequest represents the request for compiling a template
type CompileTemplateRequest struct {
	WorkspaceID            string           `json:"workspace_id"`
	MessageID              string           `json:"message_id"`
	VisualEditorTree       EmailBlock       `json:"visual_editor_tree"`
	MjmlSource             *string          `json:"mjml_source,omitempty"`
	Subject                *string          `json:"subject,omitempty"`         // Email subject; processed through Liquid using TemplateData
	SubjectPreview         *string          `json:"subject_preview,omitempty"` // Email subject preview (inbox preview text); processed through Liquid
	TemplateData           MapOfAny         `json:"test_data,omitempty"`
	TrackingSettings       TrackingSettings `json:"tracking_settings,omitempty"`
	Channel                string           `json:"channel,omitempty"`                  // "email" or "web"
	PreserveLiquid         bool             `json:"preserve_liquid,omitempty"`          // When true, skip Liquid template processing and preserve raw syntax
	SubjectPreviewOverride *string          `json:"subject_preview_override,omitempty"` // Override mj-preview content before compilation
}

// UnmarshalJSON implements custom JSON unmarshaling for CompileTemplateRequest
func (r *CompileTemplateRequest) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with the same fields but using json.RawMessage for VisualEditorTree
	type Alias CompileTemplateRequest
	aux := &struct {
		*Alias
		VisualEditorTree json.RawMessage `json:"visual_editor_tree"`
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Unmarshal the VisualEditorTree using our custom function
	if len(aux.VisualEditorTree) > 0 {
		block, err := UnmarshalEmailBlock(aux.VisualEditorTree)
		if err != nil {
			// If MjmlSource is provided, we can skip visual_editor_tree parsing errors
			if r.MjmlSource != nil && *r.MjmlSource != "" {
				return nil
			}
			return fmt.Errorf("failed to unmarshal visual_editor_tree: %w", err)
		}
		r.VisualEditorTree = block
	}

	return nil
}

// Validate ensures that the compile template request has all required fields
func (r *CompileTemplateRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("invalid compile template request: workspace_id is required")
	}
	if r.MessageID == "" {
		return fmt.Errorf("invalid compile template request: message_id is required")
	}

	// Accept either MjmlSource or VisualEditorTree
	if r.MjmlSource != nil && *r.MjmlSource != "" {
		// MjmlSource is provided, no need to validate VisualEditorTree
		return nil
	}

	// Basic validation for the tree root kind
	if r.VisualEditorTree == nil || r.VisualEditorTree.GetType() != MJMLComponentMjml {
		return fmt.Errorf("invalid compile template request: visual_editor_tree must have type 'mjml'")
	}
	if r.VisualEditorTree.GetChildren() == nil {
		return fmt.Errorf("invalid compile template request: visual_editor_tree root block must have children")
	}

	return nil
}

// CompileTemplateResponse represents the response from compiling a template
type CompileTemplateResponse struct {
	Success        bool        `json:"success"`
	MJML           *string     `json:"mjml,omitempty"`            // Pointer, omit if nil
	HTML           *string     `json:"html,omitempty"`            // Pointer, omit if nil
	Subject        *string     `json:"subject,omitempty"`         // Rendered email subject (Liquid processed); omit if not provided in request
	SubjectPreview *string     `json:"subject_preview,omitempty"` // Rendered email subject preview (Liquid processed); omit if not provided in request
	Error          *mjml.Error `json:"error,omitempty"`           // Pointer, omit if nil
	TemplateData   MapOfAny    `json:"test_data,omitempty"`       // Effective template data used for rendering (includes the injected workspace object); omit if empty
}

// renderSubjectField applies the same Liquid rules used for the body to a header
// field such as Subject or SubjectPreview. Returns the rendered value (or the
// original when Liquid processing is skipped) and any error wrapped as *mjml.Error
// so the caller can surface it on the response.
func renderSubjectField(value *string, data MapOfAny, channel string, preserveLiquid bool, contextLabel string) (*string, *mjml.Error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	if preserveLiquid || channel == "web" || len(data) == 0 {
		v := *value
		return &v, nil
	}
	rendered, err := ProcessLiquidTemplate(*value, data, contextLabel)
	if err != nil {
		return nil, &mjml.Error{Message: err.Error()}
	}
	return &rendered, nil
}

// GenerateEmailRedirectionEndpoint generates the email redirection endpoint URL
// Uses encrypted path tokens (/r/{token}) to avoid pixel blocker detection.
// Falls back to legacy query params (/visit?mid=...) if encryption fails.
func GenerateEmailRedirectionEndpoint(workspaceID string, messageID string, apiEndpoint string, destinationURL string, sentTimestamp int64) string {
	// Try encrypted format: /r/{token}
	plaintext := fmt.Sprintf("%s\n%s\n%d\n%s", messageID, workspaceID, sentTimestamp, destinationURL)
	token, err := crypto.EncryptTrackingToken(plaintext)
	if err == nil {
		return fmt.Sprintf("%s/r/%s", apiEndpoint, token)
	}

	// Fallback to legacy query params
	encodedMID := url.QueryEscape(messageID)
	encodedWID := url.QueryEscape(workspaceID)
	encodedURL := url.QueryEscape(destinationURL)
	return fmt.Sprintf("%s/visit?mid=%s&wid=%s&ts=%d&url=%s",
		apiEndpoint, encodedMID, encodedWID, sentTimestamp, encodedURL)
}

// GenerateHTMLOpenTrackingPixel generates the HTML for the open tracking pixel.
// Uses encrypted path tokens (/t/{token}) to avoid pixel blocker detection.
// Falls back to legacy query params (/opens?mid=...) if encryption fails.
func GenerateHTMLOpenTrackingPixel(workspaceID string, messageID string, apiEndpoint string, sentTimestamp int64) string {
	// Try encrypted format: /t/{token}
	plaintext := fmt.Sprintf("%s\n%s\n%d", messageID, workspaceID, sentTimestamp)
	token, err := crypto.EncryptTrackingToken(plaintext)
	var pixelURL string
	if err == nil {
		pixelURL = fmt.Sprintf("%s/t/%s", apiEndpoint, token)
	} else {
		// Fallback to legacy query params
		encodedMID := url.QueryEscape(messageID)
		encodedWID := url.QueryEscape(workspaceID)
		pixelURL = fmt.Sprintf("%s/opens?mid=%s&wid=%s&ts=%d",
			apiEndpoint, encodedMID, encodedWID, sentTimestamp)
	}
	return fmt.Sprintf(`<table border="0" cellpadding="0" cellspacing="0" role="presentation" width="100%%"><tr><td><img src="%s" alt="" style="border:0;margin:0;padding:0;"></td></tr></table>`, pixelURL)
}

// CompileTemplate compiles a visual editor tree to MJML and HTML
func CompileTemplate(req CompileTemplateRequest) (resp *CompileTemplateResponse, err error) {
	var mjmlString string

	// Render Subject and SubjectPreview through Liquid before any body work, so
	// the rendered values can be returned even when the body fails to compile.
	// A malformed Liquid expression in the subject short-circuits the response.
	renderedSubject, subjectErr := renderSubjectField(req.Subject, req.TemplateData, req.Channel, req.PreserveLiquid, "email_subject")
	if subjectErr != nil {
		return &CompileTemplateResponse{Success: false, Error: subjectErr}, nil
	}
	renderedSubjectPreview, previewErr := renderSubjectField(req.SubjectPreview, req.TemplateData, req.Channel, req.PreserveLiquid, "email_subject_preview")
	if previewErr != nil {
		return &CompileTemplateResponse{Success: false, Subject: renderedSubject, Error: previewErr}, nil
	}

	// If MjmlSource is provided (code mode), use it directly.
	// Note: Channel filtering is not applied in code mode — code mode users
	// control their own MJML structure directly.
	if req.MjmlSource != nil && *req.MjmlSource != "" {
		mjmlString = *req.MjmlSource

		// Process Liquid templates if template data is provided and PreserveLiquid is false
		if !req.PreserveLiquid && len(req.TemplateData) > 0 {
			processed, err := ProcessLiquidTemplate(mjmlString, req.TemplateData, "mjml-source")
			if err != nil {
				return &CompileTemplateResponse{
					Success:        false,
					Subject:        renderedSubject,
					SubjectPreview: renderedSubjectPreview,
					Error: &mjml.Error{
						Message: err.Error(),
					},
				}, nil
			}
			mjmlString = processed
		}

		// Apply the subject_preview override after Liquid, so the escaping applies to
		// the rendered value rather than to the Liquid syntax. Escaping first would
		// leave the rendered value unescaped (a bare & fails the XML parse) and would
		// corrupt comparisons like {% if a > b %} by entity-encoding the operator.
		// Unconditional, unlike the visual-mode fallback below: code mode has no tree
		// pass to place the override, so it must also replace an existing mj-preview,
		// which overrideMjPreviewInSource does.
		if req.SubjectPreviewOverride != nil && *req.SubjectPreviewOverride != "" {
			renderedOverride, overrideErr := renderSubjectField(req.SubjectPreviewOverride, req.TemplateData, req.Channel, req.PreserveLiquid, "email_subject_preview_override")
			if overrideErr != nil {
				return &CompileTemplateResponse{
					Success:        false,
					Subject:        renderedSubject,
					SubjectPreview: renderedSubjectPreview,
					Error:          overrideErr,
				}, nil
			}
			mjmlString = overrideMjPreviewInSource(mjmlString, *renderedOverride)
		}
	} else {
		// Visual editor mode: convert JSON tree to MJML

		tree := req.VisualEditorTree

		// Apply subject_preview override in the tree before conversion.
		// WARNING: updateBlockContent mutates the caller's tree in place (the mj-preview block).
		// Callers pass VisualEditorTree by pointer; this is safe only because templates are loaded
		// fresh per send and compiled sequentially. If a shared template cache is introduced, clone
		// the tree before concurrent compilation to avoid a data race on the mj-preview block.
		if req.SubjectPreviewOverride != nil && *req.SubjectPreviewOverride != "" {
			updateBlockContent(tree, MJMLComponentMjPreview, *req.SubjectPreviewOverride)
		}

		// If PreserveLiquid is true, skip all Liquid processing and return raw MJML
		// This is used for MJML export where we want to preserve Liquid syntax like {{contact.external_id}}
		if req.PreserveLiquid {
			mjmlString = ConvertJSONToMJMLRaw(tree)
		} else {
			// Prepare template data JSON string
			// Note: Web channel doesn't use template data (no contact personalization)
			var templateDataStr string
			if len(req.TemplateData) > 0 && req.Channel != "web" {
				jsonDataBytes, err := json.Marshal(req.TemplateData)
				if err != nil {
					return &CompileTemplateResponse{
						Success:        false,
						MJML:           nil,
						HTML:           nil,
						Subject:        renderedSubject,
						SubjectPreview: renderedSubjectPreview,
						Error: &mjml.Error{
							Message: fmt.Sprintf("failed to marshal template data: %v", err),
						},
					}, nil
				}
				templateDataStr = string(jsonDataBytes)
			}

			// Compile tree to MJML using our pkg/mjml function with template data
			if templateDataStr != "" {
				var err error
				mjmlString, err = ConvertJSONToMJMLWithData(tree, templateDataStr)
				if err != nil {
					return &CompileTemplateResponse{
						Success:        false,
						MJML:           nil,
						HTML:           nil,
						Subject:        renderedSubject,
						SubjectPreview: renderedSubjectPreview,
						Error: &mjml.Error{
							Message: err.Error(),
						},
					}, nil
				}
			} else {
				mjmlString = ConvertJSONToMJML(tree)
			}
		}
	}

	// Whole-string Liquid pass for visual editor mode.
	// Processes raw Liquid from mj-liquid blocks. Existing block content was already
	// Liquid-processed per-block during tree walk, so the second pass is a no-op for them.
	if req.MjmlSource == nil && !req.PreserveLiquid && len(req.TemplateData) > 0 && req.Channel != "web" {
		processed, liquidErr := ProcessLiquidTemplate(mjmlString, req.TemplateData, "visual-editor-whole")
		if liquidErr != nil {
			return &CompileTemplateResponse{
				Success:        false,
				Subject:        renderedSubject,
				SubjectPreview: renderedSubjectPreview,
				Error:          &mjml.Error{Message: liquidErr.Error()},
			}, nil
		}
		mjmlString = processed
	}

	// For visual editor mode: if subject_preview override was requested but the tree
	// didn't contain an mj-preview block, fall back to injecting it in the MJML string.
	// The preview text is Liquid-rendered first, mirroring the per-block rendering an
	// mj-preview block in the tree receives: the whole-string pass above has already
	// run, so rendering here (rather than injecting raw Liquid) is what gets the
	// expression evaluated exactly once, and XML-escaping applies to the rendered
	// value instead of the Liquid syntax.
	// The condition tests for a *paired* tag on purpose: that is the signal the tree
	// walk already placed the override in a real mj-preview block. A self-closing
	// <mj-preview /> means it did not — an mj-liquid block can emit a bare tag the
	// walk never sees — so it falls through here and overrideMjPreviewInSource fills
	// it rather than injecting a second element.
	if req.MjmlSource == nil && req.SubjectPreviewOverride != nil && *req.SubjectPreviewOverride != "" {
		if !mjPreviewTagRegexp.MatchString(mjmlString) {
			renderedOverride, overrideErr := renderSubjectField(req.SubjectPreviewOverride, req.TemplateData, req.Channel, req.PreserveLiquid, "email_subject_preview_override")
			if overrideErr != nil {
				return &CompileTemplateResponse{
					Success:        false,
					Subject:        renderedSubject,
					SubjectPreview: renderedSubjectPreview,
					Error:          overrideErr,
				}, nil
			}
			mjmlString = overrideMjPreviewInSource(mjmlString, *renderedOverride)
		}
	}

	// Preprocess MJML to fix HTML vs XML incompatibilities
	// gomjml uses a strict XML parser that doesn't accept HTML void tags (<br>) or HTML entities (&nbsp;)
	preprocessedMjml := preprocessMjmlForXML(mjmlString)

	// Compile MJML to HTML using gomjml library
	htmlResult, err := mjml.Render(preprocessedMjml)
	if err != nil {
		// Return the response struct with Success=false and the Error details
		return &CompileTemplateResponse{
			Success:        false,
			MJML:           &mjmlString, // Include original MJML for context if desired
			HTML:           nil,
			Subject:        renderedSubject,
			SubjectPreview: renderedSubjectPreview,
			Error: &mjml.Error{
				Message: err.Error(),
			},
		}, nil
	}

	// Decode HTML entities in href attributes to fix broken URLs with query parameters
	// The MJML-to-HTML compiler doesn't always decode &amp; back to & in href attributes
	htmlResult = decodeHTMLEntitiesInURLAttributes(htmlResult)

	// Skip tracking for web channel
	if req.Channel == "web" {
		return &CompileTemplateResponse{
			Success:        true,
			MJML:           &mjmlString,
			HTML:           &htmlResult, // No tracking applied for web
			Subject:        renderedSubject,
			SubjectPreview: renderedSubjectPreview,
			Error:          nil,
		}, nil
	}

	// Apply link tracking to the HTML output (email channel only).
	// Tracking failures are usually user-content issues (e.g. malformed href in
	// an mj-button), so surface them as structured compile errors like every
	// other failure path rather than as a Go error — that way the rendered
	// subject/subject_preview reach the client instead of being dropped.
	trackedHTML, err := TrackLinks(htmlResult, req.TrackingSettings)
	if err != nil {
		return &CompileTemplateResponse{
			Success:        false,
			MJML:           &mjmlString,
			HTML:           nil,
			Subject:        renderedSubject,
			SubjectPreview: renderedSubjectPreview,
			Error: &mjml.Error{
				Message: err.Error(),
			},
		}, nil
	}

	// Return successful response
	return &CompileTemplateResponse{
		Success:        true,
		MJML:           &mjmlString,
		HTML:           &trackedHTML,
		Subject:        renderedSubject,
		SubjectPreview: renderedSubjectPreview,
		Error:          nil,
	}, nil
}

// decodeHTMLEntitiesInURLAttributes decodes HTML entities (&amp;, &quot;, etc.)
// in href, src, and other URL attributes to ensure clickable links work correctly.
// The MJML-to-HTML compiler doesn't always decode these entities properly in attributes,
// which breaks URLs with query parameters (e.g., ?action=confirm&email=... becomes &amp;email=...)
func decodeHTMLEntitiesInURLAttributes(html string) string {
	// Pattern matches href="...", src="...", action="..." attributes
	// Captures: (attribute=") (url content) (")
	urlAttrRegex := regexp.MustCompile(`((?:href|src|action)=["'])([^"']+)(["'])`)

	return urlAttrRegex.ReplaceAllStringFunc(html, func(match string) string {
		parts := urlAttrRegex.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match // Return original if parsing fails
		}

		beforeURL := parts[1]  // href=" or src=" or action="
		encodedURL := parts[2] // the URL with HTML entities
		afterURL := parts[3]   // closing "

		// Decode common HTML entities that appear in URLs
		// Note: We only decode entities that are safe to decode in URL context
		decodedURL := encodedURL
		decodedURL = strings.ReplaceAll(decodedURL, "&amp;", "&")
		decodedURL = strings.ReplaceAll(decodedURL, "&quot;", "\"")
		decodedURL = strings.ReplaceAll(decodedURL, "&#39;", "'")
		decodedURL = strings.ReplaceAll(decodedURL, "&lt;", "<")
		decodedURL = strings.ReplaceAll(decodedURL, "&gt;", ">")

		return beforeURL + decodedURL + afterURL
	})
}

func TrackLinks(htmlString string, trackingSettings TrackingSettings) (updatedHTML string, err error) {
	// The per-notification opt-out suppresses everything, including UTM
	// rewriting: opted-out notifications carry single-use auth URLs that must
	// not be modified in any way, even when a caller sets UTM fields.
	if trackingSettings.TrackingMode == TrackingModeDisabled {
		return htmlString, nil
	}

	// If tracking is disabled and there is nothing else to write onto the links,
	// return the original HTML. An identity token counts: a workspace can run web
	// analytics with click tracking off, and the recipient still has to be
	// identified on landing.
	if !trackingSettings.EnableTracking && trackingSettings.UTMSource == "" &&
		trackingSettings.UTMMedium == "" && trackingSettings.UTMCampaign == "" &&
		trackingSettings.UTMContent == "" && trackingSettings.UTMTerm == "" &&
		trackingSettings.IdentifyToken == "" {
		return htmlString, nil
	}

	// Use regex to find and replace href attributes in <a> tags
	// This regex matches: <a ...href="url"... > or <a ...href='url'... >
	hrefRegex := regexp.MustCompile(`(<a[^>]*\s+href=["'])([^"']+)(["'][^>]*>)`)

	updatedHTML = hrefRegex.ReplaceAllStringFunc(htmlString, func(match string) string {
		// Extract the parts: opening tag with href=", URL, closing " and rest of tag
		parts := hrefRegex.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match // Return original if parsing fails
		}

		beforeURL := parts[1]   // <a ...href="
		originalURL := parts[2] // the URL
		afterURL := parts[3]    // "...>

		// Skip tracking for special protocol links (mailto, tel, sms, etc.)
		// These should not be wrapped in a redirect as it breaks their functionality
		if isNonTrackableURL(originalURL) {
			return match // Return original link unchanged
		}

		// Append UTM parameters to the destination URL
		destinationURL := trackingSettings.applyUTMParameters(originalURL)

		// Then the identity token, on the destination URL rather than on the
		// redirect built below. That is the whole reason for the ordering: the
		// redirect encrypts the destination it will send the recipient to, so a
		// parameter added after it exists would never reach the landing page.
		//
		// It buys no confidentiality, and none is claimed. With click tracking
		// off there is no redirect at all and the token sits in the href as
		// plain text; with it on, the /r/ token is obfuscation aimed at
		// pixel-blockers, not a secret — crypto.EncryptTrackingToken uses a
		// hardcoded key, so anyone with the source can read it back. The token
		// is therefore readable by anyone who can read the email, which is
		// acceptable because the email is addressed to that contact. The
		// exposure that remains is forwarding: whoever receives the forward is
		// taken for the original recipient until the token expires.
		destinationURL = trackingSettings.applyIdentityParam(destinationURL)

		trackedURL := destinationURL

		if trackingSettings.EnableTracking {
			// Use current Unix timestamp (seconds) for bot detection.
			// The UTM-augmented destination URL is what gets encrypted into the
			// token, so the redirect target preserves the UTM parameters.
			sentTimestamp := time.Now().Unix()
			trackedURL = GenerateEmailRedirectionEndpoint(trackingSettings.WorkspaceID, trackingSettings.MessageID, trackingSettings.Endpoint, destinationURL, sentTimestamp)
		}

		// Return the updated tag
		return beforeURL + trackedURL + afterURL
	})

	if trackingSettings.EnableTracking {
		// Insert tracking pixel before </body>. The pixel is wrapped in a <table>
		// by GenerateHTMLOpenTrackingPixel to look like a structural layout element
		// rather than a standalone tracking pixel.
		sentTimestamp := time.Now().Unix()
		trackingPixel := GenerateHTMLOpenTrackingPixel(trackingSettings.WorkspaceID, trackingSettings.MessageID, trackingSettings.Endpoint, sentTimestamp)

		bodyCloseRegex := regexp.MustCompile(`(?i)(<\/body>)`)
		if bodyCloseRegex.MatchString(updatedHTML) {
			updatedHTML = bodyCloseRegex.ReplaceAllString(updatedHTML, trackingPixel+"$1")
		} else {
			updatedHTML = updatedHTML + trackingPixel
		}
	}

	return updatedHTML, nil
}

// mjPreviewTagRegexp matches a paired <mj-preview ...>...</mj-preview> in MJML
// source, including one carrying attributes. The [^>]* cannot cross the tag
// boundary, so content such as <br/> never confuses the match.
var mjPreviewTagRegexp = regexp.MustCompile(`(?is)(<mj-preview\b[^>]*>)([\s\S]*?)(</mj-preview\s*>)`)

// mjPreviewSelfClosingTagRegexp matches a self-closing <mj-preview ... />, the
// form the converter emits for an mj-preview block with no content.
var mjPreviewSelfClosingTagRegexp = regexp.MustCompile(`(?is)(<mj-preview\b[^>]*?)\s*/>`)

// mjHeadTagRegexp matches the opening <mj-head...> tag.
var mjHeadTagRegexp = regexp.MustCompile(`(?i)<mj-head[^>]*>`)

// mjmlRootTagRegexp matches the opening <mjml...> tag.
var mjmlRootTagRegexp = regexp.MustCompile(`(?i)<mjml[^>]*>`)

// overrideMjPreviewInSource replaces or injects <mj-preview> in raw MJML source.
// Content is XML-escaped for safe insertion.
// Fallback order: replace existing → inject after <mj-head> → create <mj-head> after <mjml>.
func overrideMjPreviewInSource(mjmlSource string, previewText string) string {
	escaped := escapeXMLContent(previewText)

	// Expand a self-closing <mj-preview /> into a paired tag holding the text,
	// keeping any attributes it carried.
	if mjPreviewSelfClosingTagRegexp.MatchString(mjmlSource) {
		return mjPreviewSelfClosingTagRegexp.ReplaceAllString(mjmlSource,
			"${1}>"+escapeRegexpReplacement(escaped)+"</mj-preview>")
	}

	// Replace existing <mj-preview> content
	if mjPreviewTagRegexp.MatchString(mjmlSource) {
		return mjPreviewTagRegexp.ReplaceAllString(mjmlSource, "${1}"+escapeRegexpReplacement(escaped)+"${3}")
	}

	// No <mj-preview> — inject after <mj-head>
	newTag := "<mj-preview>" + escaped + "</mj-preview>"
	loc := mjHeadTagRegexp.FindStringIndex(mjmlSource)
	if loc != nil {
		return mjmlSource[:loc[1]] + "\n    " + newTag + mjmlSource[loc[1]:]
	}

	// No <mj-head> — create one after <mjml>
	loc = mjmlRootTagRegexp.FindStringIndex(mjmlSource)
	if loc != nil {
		return mjmlSource[:loc[1]] + "\n  <mj-head>\n    " + newTag + "\n  </mj-head>" + mjmlSource[loc[1]:]
	}

	// No <mjml> tag found; return as-is
	return mjmlSource
}

// escapeXMLContent escapes &, <, > for safe insertion as XML element text
// content. Angle brackets use numeric character references (&#60;/&#62;) rather
// than &lt;/&gt; because the MJML parser pre-decodes the named entities back to
// raw brackets before XML parsing, which would turn escaped text into markup
// and fail the compile; numeric references survive that pre-decode untouched.
func escapeXMLContent(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&#60;")
	s = strings.ReplaceAll(s, ">", "&#62;")
	return s
}

// escapeRegexpReplacement escapes $ signs so they are treated literally by ReplaceAllString.
func escapeRegexpReplacement(s string) string {
	return strings.ReplaceAll(s, "$", "$$")
}

// updateBlockContent traverses the block tree and sets the content of all blocks
// matching the given type. Used to override mj-preview content before compilation.
func updateBlockContent(block EmailBlock, blockType MJMLComponentType, content string) {
	if block == nil {
		return
	}
	if block.GetType() == blockType {
		block.SetContent(&content)
	}
	for _, child := range block.GetChildren() {
		updateBlockContent(child, blockType, content)
	}
}
