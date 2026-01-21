package notifuse_mjml

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

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

type TrackingSettings struct {
	EnableTracking bool   `json:"enable_tracking"`
	Endpoint       string `json:"endpoint,omitempty"`
	UTMSource      string `json:"utm_source,omitempty"`
	UTMMedium      string `json:"utm_medium,omitempty"`
	UTMCampaign    string `json:"utm_campaign,omitempty"`
	UTMContent     string `json:"utm_content,omitempty"`
	UTMTerm        string `json:"utm_term,omitempty"`
	WorkspaceID    string `json:"workspace_id,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
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

func (t *TrackingSettings) GetTrackingURL(sourceURL string) string {
	// Ignore if URL is empty, a placeholder, mailto:, tel:, or already tracked (basic check)
	if sourceURL == "" || strings.Contains(sourceURL, "{{") || strings.Contains(sourceURL, "{%") || strings.HasPrefix(sourceURL, "mailto:") || strings.HasPrefix(sourceURL, "tel:") {
		return sourceURL
	}

	// parse sourceURL to get the domain
	parsedURL, err := url.Parse(sourceURL)
	if err != nil {
		return sourceURL
	}

	// Get existing query parameters
	queryParams := parsedURL.Query()

	// Check if URL already has UTM parameters - if yes, don't modify them
	hasExistingUTM := false
	for key := range queryParams {
		if strings.HasPrefix(strings.ToLower(key), "utm_") {
			hasExistingUTM = true
			break
		}
	}

	// Add UTM parameters to the URL if no existing UTM parameters
	if !hasExistingUTM {
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
	}

	if !t.EnableTracking {
		return parsedURL.String()
	}

	// parse endpoint and add url to the query params
	parsedEndpoint, err := url.Parse(t.Endpoint)
	if err != nil {
		return sourceURL
	}
	endpointParams := parsedEndpoint.Query()
	endpointParams.Add("url", parsedURL.String()) // Use the URL with UTM parameters
	parsedEndpoint.RawQuery = endpointParams.Encode()

	return parsedEndpoint.String()
}

// CompileTemplateRequest represents the request for compiling a template
type CompileTemplateRequest struct {
	WorkspaceID      string           `json:"workspace_id"`
	MessageID        string           `json:"message_id"`
	VisualEditorTree EmailBlock       `json:"visual_editor_tree"`
	TemplateData     MapOfAny         `json:"test_data,omitempty"`
	TrackingSettings TrackingSettings `json:"tracking_settings,omitempty"`
	Channel          string           `json:"channel,omitempty"`          // "email" or "web" - filters blocks by visibility
	PreserveLiquid   bool             `json:"preserve_liquid,omitempty"`  // When true, skip Liquid template processing and preserve raw syntax
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
	// Basic validation for the tree root kind
	if r.VisualEditorTree.GetType() != MJMLComponentMjml {
		return fmt.Errorf("invalid compile template request: visual_editor_tree must have type 'mjml'")
	}
	if r.VisualEditorTree.GetChildren() == nil {
		return fmt.Errorf("invalid compile template request: visual_editor_tree root block must have children")
	}

	return nil
}

// CompileTemplateResponse represents the response from compiling a template
type CompileTemplateResponse struct {
	Success bool          `json:"success"`
	MJML    *string       `json:"mjml,omitempty"`  // Pointer, omit if nil
	HTML    *string       `json:"html,omitempty"`  // Pointer, omit if nil
	Error   *mjml.Error `json:"error,omitempty"` // Pointer, omit if nil
}

// GenerateEmailRedirectionEndpoint generates the email redirection endpoint URL
func GenerateEmailRedirectionEndpoint(workspaceID string, messageID string, apiEndpoint string, destinationURL string, sentTimestamp int64) string {
	// URL encode the parameters to handle special characters
	encodedMID := url.QueryEscape(messageID)
	encodedWID := url.QueryEscape(workspaceID)
	encodedURL := url.QueryEscape(destinationURL)
	return fmt.Sprintf("%s/visit?mid=%s&wid=%s&ts=%d&url=%s",
		apiEndpoint, encodedMID, encodedWID, sentTimestamp, encodedURL)
}

func GenerateHTMLOpenTrackingPixel(workspaceID string, messageID string, apiEndpoint string, sentTimestamp int64) string {
	// URL encode the parameters to handle special characters
	encodedMID := url.QueryEscape(messageID)
	encodedWID := url.QueryEscape(workspaceID)
	pixelURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s&ts=%d",
		apiEndpoint, encodedMID, encodedWID, sentTimestamp)
	return fmt.Sprintf(`<img src="%s" alt="" width="1" height="1">`, pixelURL)
}

// CompileTemplate compiles a visual editor tree to MJML and HTML
func CompileTemplate(req CompileTemplateRequest) (resp *CompileTemplateResponse, err error) {
	// Apply channel filtering if specified
	tree := req.VisualEditorTree
	if req.Channel != "" {
		tree = FilterBlocksByChannel(req.VisualEditorTree, req.Channel)
	}

	var mjmlString string

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
					Success: false,
					MJML:    nil,
					HTML:    nil,
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
					Success: false,
					MJML:    nil,
					HTML:    nil,
					Error: &mjml.Error{
						Message: err.Error(),
					},
				}, nil
			}
		} else {
			mjmlString = ConvertJSONToMJML(tree)
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
			Success: false,
			MJML:    &mjmlString, // Include original MJML for context if desired
			HTML:    nil,
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
			Success: true,
			MJML:    &mjmlString,
			HTML:    &htmlResult, // No tracking applied for web
			Error:   nil,
		}, nil
	}

	// Apply link tracking to the HTML output (email channel only)
	trackedHTML, err := TrackLinks(htmlResult, req.TrackingSettings)
	if err != nil {
		return nil, err
	}

	// Return successful response
	return &CompileTemplateResponse{
		Success: true,
		MJML:    &mjmlString,
		HTML:    &trackedHTML,
		Error:   nil,
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
	// If tracking is disabled and no UTM parameters to add, return original HTML
	if !trackingSettings.EnableTracking && trackingSettings.UTMSource == "" &&
		trackingSettings.UTMMedium == "" && trackingSettings.UTMCampaign == "" &&
		trackingSettings.UTMContent == "" && trackingSettings.UTMTerm == "" {
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

		// Apply tracking to the URL
		trackedURL := trackingSettings.GetTrackingURL(originalURL)

		if trackingSettings.EnableTracking {
			// Use current Unix timestamp (seconds) for bot detection
			sentTimestamp := time.Now().Unix()
			trackedURL = GenerateEmailRedirectionEndpoint(trackingSettings.WorkspaceID, trackingSettings.MessageID, trackingSettings.Endpoint, originalURL, sentTimestamp)
		}

		// Return the updated tag
		return beforeURL + trackedURL + afterURL
	})

	if trackingSettings.EnableTracking {
		// Insert tracking pixel at the end of the body tag
		// Use current Unix timestamp (seconds) for bot detection
		sentTimestamp := time.Now().Unix()
		trackingPixel := GenerateHTMLOpenTrackingPixel(trackingSettings.WorkspaceID, trackingSettings.MessageID, trackingSettings.Endpoint, sentTimestamp)

		// Find the closing </body> tag and insert the pixel before it
		bodyCloseRegex := regexp.MustCompile(`(?i)(<\/body>)`)
		if bodyCloseRegex.MatchString(updatedHTML) {
			updatedHTML = bodyCloseRegex.ReplaceAllString(updatedHTML, trackingPixel+"$1")
		} else {
			// Fallback: if no closing body tag found, append to the end
			updatedHTML = updatedHTML + trackingPixel
		}
	}

	return updatedHTML, nil
}
