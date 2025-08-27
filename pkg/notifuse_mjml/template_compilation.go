package notifuse_mjml

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/preslavrachev/gomjml/mjml"
)

// MapOfAny represents a map of string to any value, used for template data
type MapOfAny map[string]any

// CompileError represents an error that occurred during MJML compilation
type CompileError struct {
	Message string `json:"message"`
}

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
	Error   *CompileError `json:"error,omitempty"` // Pointer, omit if nil
}

// GenerateEmailRedirectionEndpoint generates the email redirection endpoint URL
func GenerateEmailRedirectionEndpoint(workspaceID string, messageID string, apiEndpoint string, destinationURL string) string {
	// URL encode the parameters to handle special characters
	encodedMID := url.QueryEscape(messageID)
	encodedWID := url.QueryEscape(workspaceID)
	encodedURL := url.QueryEscape(destinationURL)
	return fmt.Sprintf("%s/visit?mid=%s&wid=%s&url=%s",
		apiEndpoint, encodedMID, encodedWID, encodedURL)
}

func GenerateHTMLOpenTrackingPixel(workspaceID string, messageID string, apiEndpoint string) string {
	// URL encode the parameters to handle special characters
	encodedMID := url.QueryEscape(messageID)
	encodedWID := url.QueryEscape(workspaceID)
	pixelURL := fmt.Sprintf("%s/opens?mid=%s&wid=%s",
		apiEndpoint, encodedMID, encodedWID)
	return fmt.Sprintf(`<img src="%s" alt="" width="1" height="1">`, pixelURL)
}

// CompileTemplate compiles a visual editor tree to MJML and HTML
func CompileTemplate(req CompileTemplateRequest) (resp *CompileTemplateResponse, err error) {
	// Prepare template data JSON string
	var templateDataStr string
	if len(req.TemplateData) > 0 {
		jsonDataBytes, err := json.Marshal(req.TemplateData)
		if err != nil {
			return &CompileTemplateResponse{
				Success: false,
				MJML:    nil,
				HTML:    nil,
				Error: &CompileError{
					Message: fmt.Sprintf("failed to marshal template data: %v", err),
				},
			}, nil
		}
		templateDataStr = string(jsonDataBytes)
	}

	// Compile tree to MJML using our pkg/mjml function with template data
	var mjmlString string
	if templateDataStr != "" {
		var err error
		mjmlString, err = ConvertJSONToMJMLWithData(req.VisualEditorTree, templateDataStr)
		if err != nil {
			return &CompileTemplateResponse{
				Success: false,
				MJML:    nil,
				HTML:    nil,
				Error: &CompileError{
					Message: err.Error(),
				},
			}, nil
		}
	} else {
		mjmlString = ConvertJSONToMJML(req.VisualEditorTree)
	}

	// Compile MJML to HTML using gomjml library
	htmlResult, err := mjml.Render(mjmlString)
	if err != nil {
		// Return the response struct with Success=false and the Error details
		return &CompileTemplateResponse{
			Success: false,
			MJML:    &mjmlString, // Include original MJML for context if desired
			HTML:    nil,
			Error: &CompileError{
				Message: err.Error(),
			},
		}, nil
	}

	// Apply link tracking to the HTML output
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

		// Apply tracking to the URL
		trackedURL := trackingSettings.GetTrackingURL(originalURL)

		if trackingSettings.EnableTracking {
			trackedURL = GenerateEmailRedirectionEndpoint(trackingSettings.WorkspaceID, trackingSettings.MessageID, trackingSettings.Endpoint, originalURL)
		}

		// Return the updated tag
		return beforeURL + trackedURL + afterURL
	})

	if trackingSettings.EnableTracking {
		// Insert tracking pixel at the end of the body tag
		trackingPixel := GenerateHTMLOpenTrackingPixel(trackingSettings.WorkspaceID, trackingSettings.MessageID, trackingSettings.Endpoint)

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
