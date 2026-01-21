package notifuse_mjml

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ConvertJSONToMJML converts an EmailBlock JSON tree to MJML string
func ConvertJSONToMJML(tree EmailBlock) string {
	return convertBlockToMJML(tree, 0, "")
}

// ConvertJSONToMJMLRaw converts an EmailBlock JSON tree to MJML string
// without processing Liquid templates - preserving raw template syntax
func ConvertJSONToMJMLRaw(tree EmailBlock) string {
	return convertBlockToMJMLRaw(tree, 0)
}

// ConvertJSONToMJMLWithData converts an EmailBlock JSON tree to MJML string with template data
func ConvertJSONToMJMLWithData(tree EmailBlock, templateData string) (string, error) {
	// Parse template data once at the beginning
	parsedData, parseErr := parseTemplateDataString(templateData)
	if parseErr != nil {
		return "", fmt.Errorf("template data parsing failed: %v", parseErr)
	}
	return convertBlockToMJMLWithErrorAndParsedData(tree, 0, templateData, parsedData)
}

// convertBlockToMJMLWithErrorAndParsedData recursively converts a single EmailBlock to MJML string with error handling and pre-parsed data
func convertBlockToMJMLWithErrorAndParsedData(block EmailBlock, indentLevel int, templateData string, parsedData map[string]interface{}) (string, error) {
	indent := strings.Repeat("  ", indentLevel)
	tagName := string(block.GetType())
	children := block.GetChildren()

	// Handle self-closing tags that don't have children but may have content
	if len(children) == 0 {
		// Check if the block has content (for mj-text, mj-button, etc.)
		content := getBlockContent(block)

		if content != "" {
			// Process Liquid templating for mj-text, mj-button, mj-title, mj-preview, and mj-raw blocks
			blockType := block.GetType()
			if blockType == MJMLComponentMjText || blockType == MJMLComponentMjButton || blockType == MJMLComponentMjTitle || blockType == MJMLComponentMjPreview || blockType == MJMLComponentMjRaw {
				// Only process Liquid when we have actual template data.
				// When parsedData is nil or empty, preserve Liquid syntax for MJML export (issue #226).
				if parsedData != nil && len(parsedData) > 0 {
					processedContent, err := processLiquidContent(content, parsedData, block.GetID())
					if err != nil {
						// Return error instead of just logging
						return "", fmt.Errorf("liquid processing failed for block %s: %v", block.GetID(), err)
					} else {
						content = processedContent
					}
				}
			}

			// Block with content - don't escape for mj-raw, mj-text, and mj-button (they can contain HTML)
			attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID())
			if blockType == MJMLComponentMjRaw || blockType == MJMLComponentMjText || blockType == MJMLComponentMjButton {
				return fmt.Sprintf("%s<%s%s>%s</%s>", indent, tagName, attributeString, content, tagName), nil
			} else {
				return fmt.Sprintf("%s<%s%s>%s</%s>", indent, tagName, attributeString, escapeContent(content), tagName), nil
			}
		} else {
			// Self-closing block or empty block
			attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID())
			if attributeString != "" {
				return fmt.Sprintf("%s<%s%s />", indent, tagName, attributeString), nil
			} else {
				return fmt.Sprintf("%s<%s />", indent, tagName), nil
			}
		}
	}

	// Block with children
	attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID())
	openTag := fmt.Sprintf("%s<%s%s>", indent, tagName, attributeString)
	closeTag := fmt.Sprintf("%s</%s>", indent, tagName)

	// Process children
	var childrenMJML []string
	for _, child := range children {
		if child != nil {
			childMJML, err := convertBlockToMJMLWithErrorAndParsedData(child, indentLevel+1, templateData, parsedData)
			if err != nil {
				return "", err
			}
			childrenMJML = append(childrenMJML, childMJML)
		}
	}

	return fmt.Sprintf("%s\n%s\n%s", openTag, strings.Join(childrenMJML, "\n"), closeTag), nil
}

// convertBlockToMJML recursively converts a single EmailBlock to MJML string
func convertBlockToMJML(block EmailBlock, indentLevel int, templateData string) string {
	// Parse template data once at the beginning
	parsedData, parseErr := parseTemplateDataString(templateData)
	if parseErr != nil {
		parsedData = nil // Continue with nil data if parsing fails
	}
	return convertBlockToMJMLWithParsedData(block, indentLevel, templateData, parsedData)
}

// convertBlockToMJMLWithParsedData recursively converts a single EmailBlock to MJML string with pre-parsed data
func convertBlockToMJMLWithParsedData(block EmailBlock, indentLevel int, templateData string, parsedData map[string]interface{}) string {
	// Defensive check: ensure the block has a valid BaseBlock pointer
	if block == nil {
		return ""
	}

	// Check if GetType returns empty (indicates invalid/uninitialized block)
	blockType := block.GetType()
	if blockType == "" {
		return ""
	}

	indent := strings.Repeat("  ", indentLevel)
	tagName := string(blockType)
	children := block.GetChildren()

	// Handle self-closing tags that don't have children but may have content
	if len(children) == 0 {
		// Check if the block has content (for mj-text, mj-button, etc.)
		content := getBlockContent(block)

		if content != "" {
			// Process Liquid templating for mj-text, mj-button, mj-title, mj-preview, and mj-raw blocks
			blockType := block.GetType()
			if blockType == MJMLComponentMjText || blockType == MJMLComponentMjButton || blockType == MJMLComponentMjTitle || blockType == MJMLComponentMjPreview || blockType == MJMLComponentMjRaw {
				// Only process Liquid when we have actual template data.
				// When parsedData is nil or empty, preserve Liquid syntax for MJML export (issue #226).
				if parsedData != nil && len(parsedData) > 0 {
					processedContent, err := processLiquidContent(content, parsedData, block.GetID())
					if err != nil {
						// Log error but continue with original content
						fmt.Printf("Warning: Liquid processing failed for block %s: %v\n", block.GetID(), err)
					} else {
						content = processedContent
					}
				}
			}

			// Block with content - don't escape for mj-raw, mj-text, and mj-button (they can contain HTML)
			attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID())
			if blockType == MJMLComponentMjRaw || blockType == MJMLComponentMjText || blockType == MJMLComponentMjButton {
				return fmt.Sprintf("%s<%s%s>%s</%s>", indent, tagName, attributeString, content, tagName)
			} else {
				return fmt.Sprintf("%s<%s%s>%s</%s>", indent, tagName, attributeString, escapeContent(content), tagName)
			}
		} else {
			// Self-closing block or empty block
			attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID())
			if attributeString != "" {
				return fmt.Sprintf("%s<%s%s />", indent, tagName, attributeString)
			} else {
				return fmt.Sprintf("%s<%s />", indent, tagName)
			}
		}
	}

	// Block with children
	attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID())
	openTag := fmt.Sprintf("%s<%s%s>", indent, tagName, attributeString)
	closeTag := fmt.Sprintf("%s</%s>", indent, tagName)

	// Process children
	var childrenMJML []string
	for _, child := range children {
		if child != nil {
			childrenMJML = append(childrenMJML, convertBlockToMJMLWithParsedData(child, indentLevel+1, templateData, parsedData))
		}
	}

	return fmt.Sprintf("%s\n%s\n%s", openTag, strings.Join(childrenMJML, "\n"), closeTag)
}

// convertBlockToMJMLRaw recursively converts a single EmailBlock to MJML string
// without any Liquid template processing - preserving raw template syntax
func convertBlockToMJMLRaw(block EmailBlock, indentLevel int) string {
	// Defensive check: ensure the block has a valid BaseBlock pointer
	if block == nil {
		return ""
	}

	// Check if GetType returns empty (indicates invalid/uninitialized block)
	blockType := block.GetType()
	if blockType == "" {
		return ""
	}

	indent := strings.Repeat("  ", indentLevel)
	tagName := string(blockType)
	children := block.GetChildren()

	// Handle self-closing tags that don't have children but may have content
	if len(children) == 0 {
		// Check if the block has content (for mj-text, mj-button, etc.)
		content := getBlockContent(block)

		if content != "" {
			// Do NOT process Liquid - keep content raw
			// Block with content - don't escape for mj-raw, mj-text, and mj-button (they can contain HTML)
			attributeString := formatAttributes(block.GetAttributes())
			if blockType == MJMLComponentMjRaw || blockType == MJMLComponentMjText || blockType == MJMLComponentMjButton {
				return fmt.Sprintf("%s<%s%s>%s</%s>", indent, tagName, attributeString, content, tagName)
			} else {
				return fmt.Sprintf("%s<%s%s>%s</%s>", indent, tagName, attributeString, escapeContent(content), tagName)
			}
		} else {
			// Self-closing block or empty block
			attributeString := formatAttributes(block.GetAttributes())
			if attributeString != "" {
				return fmt.Sprintf("%s<%s%s />", indent, tagName, attributeString)
			} else {
				return fmt.Sprintf("%s<%s />", indent, tagName)
			}
		}
	}

	// Block with children
	attributeString := formatAttributes(block.GetAttributes())
	openTag := fmt.Sprintf("%s<%s%s>", indent, tagName, attributeString)
	closeTag := fmt.Sprintf("%s</%s>", indent, tagName)

	// Process children
	var childrenMJML []string
	for _, child := range children {
		if child != nil {
			childrenMJML = append(childrenMJML, convertBlockToMJMLRaw(child, indentLevel+1))
		}
	}

	return fmt.Sprintf("%s\n%s\n%s", openTag, strings.Join(childrenMJML, "\n"), closeTag)
}

// ProcessLiquidTemplate processes Liquid templating in any content (public function)
func ProcessLiquidTemplate(content string, templateData map[string]interface{}, context string) (string, error) {
	return processLiquidContent(content, templateData, context)
}

// parseTemplateDataString parses JSON string to map[string]interface{} for internal MJML functions
func parseTemplateDataString(templateData string) (map[string]interface{}, error) {
	if templateData == "" {
		return make(map[string]interface{}), nil
	}

	var jsonData map[string]interface{}
	err := json.Unmarshal([]byte(templateData), &jsonData)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON in templateData: %w", err)
	}
	return jsonData, nil
}

// processLiquidContent processes Liquid templating in content with security protections
func processLiquidContent(content string, templateData map[string]interface{}, blockID string) (string, error) {
	// Check if content contains Liquid templating markup
	if !strings.Contains(content, "{{") && !strings.Contains(content, "{%") {
		return content, nil // No Liquid markup found, return original content
	}

	// Clean non-breaking spaces and other invisible characters from template variables
	content = cleanLiquidTemplate(content)

	// Create secure Liquid engine with timeout and size protections
	engine := NewSecureLiquidEngine()

	// Use provided template data or initialize empty map if nil
	var jsonData map[string]interface{}
	if templateData != nil {
		jsonData = templateData
	} else {
		jsonData = make(map[string]interface{})
	}

	// Render the content with Liquid (with security protections)
	renderedContent, err := engine.RenderWithTimeout(content, jsonData)
	if err != nil {
		return content, fmt.Errorf("liquid rendering error in block (ID: %s): %w", blockID, err)
	}

	return renderedContent, nil
}

// cleanLiquidTemplate removes non-breaking spaces and other invisible characters from Liquid template variables
func cleanLiquidTemplate(content string) string {
	// Replace non-breaking spaces (\u00a0) with regular spaces within {{ }} and {% %} blocks
	// This regex finds Liquid template variables and removes non-breaking spaces from them
	liquidVarRegex := regexp.MustCompile(`(\{\{[^}]*\}\}|\{%[^%]*%\})`)

	return liquidVarRegex.ReplaceAllStringFunc(content, func(match string) string {
		// Remove non-breaking spaces (\u00a0) and other invisible characters
		cleaned := strings.ReplaceAll(match, "\u00a0", "")  // Non-breaking space
		cleaned = strings.ReplaceAll(cleaned, "\u200b", "") // Zero-width space
		cleaned = strings.ReplaceAll(cleaned, "\u2060", "") // Word joiner
		cleaned = strings.ReplaceAll(cleaned, "\ufeff", "") // Byte order mark
		return cleaned
	})
}

// getBlockContent extracts content from a block
func getBlockContent(block EmailBlock) string {
	content := block.GetContent()
	if content != nil {
		return *content
	}
	return ""
}

// formatAttributes formats attributes object into MJML attribute string
func formatAttributes(attributes map[string]interface{}) string {
	return formatAttributesWithLiquid(attributes, nil, "")
}

// formatAttributesWithLiquid formats attributes object into MJML attribute string with liquid processing
func formatAttributesWithLiquid(attributes map[string]interface{}, templateData map[string]interface{}, blockID string) string {
	if len(attributes) == 0 {
		return ""
	}

	var attrPairs []string
	for key, value := range attributes {
		if shouldIncludeAttribute(value) {
			if attr := formatSingleAttributeWithLiquid(key, value, templateData, blockID); attr != "" {
				attrPairs = append(attrPairs, attr)
			}
		}
	}

	return strings.Join(attrPairs, "")
}

// shouldIncludeAttribute determines if an attribute value should be included in the output
func shouldIncludeAttribute(value interface{}) bool {
	if value == nil {
		return false
	}

	switch v := value.(type) {
	case string:
		return v != ""
	case *string:
		return v != nil && *v != ""
	case bool:
		return true // Include boolean attributes regardless of value
	case *bool:
		return v != nil
	case int, int32, int64, float32, float64:
		return true // Include numeric values
	default:
		return fmt.Sprintf("%v", value) != ""
	}
}

// formatSingleAttribute formats a single attribute key-value pair
func formatSingleAttribute(key string, value interface{}) string {
	return formatSingleAttributeWithLiquid(key, value, nil, "")
}

// formatSingleAttributeWithLiquid formats a single attribute key-value pair with liquid processing
func formatSingleAttributeWithLiquid(key string, value interface{}, templateData map[string]interface{}, blockID string) string {
	// Convert camelCase to kebab-case for MJML attributes
	kebabKey := camelToKebab(key)

	// Handle different value types
	switch v := value.(type) {
	case bool:
		if v {
			return fmt.Sprintf(" %s", kebabKey)
		}
		return ""
	case *bool:
		if v != nil && *v {
			return fmt.Sprintf(" %s", kebabKey)
		}
		return ""
	case string:
		if v == "" {
			return ""
		}
		processedValue := processAttributeValue(v, kebabKey, templateData, blockID)
		escapedValue := escapeAttributeValue(processedValue, kebabKey)
		return fmt.Sprintf(` %s="%s"`, kebabKey, escapedValue)
	case *string:
		if v == nil || *v == "" {
			return ""
		}
		processedValue := processAttributeValue(*v, kebabKey, templateData, blockID)
		escapedValue := escapeAttributeValue(processedValue, kebabKey)
		return fmt.Sprintf(` %s="%s"`, kebabKey, escapedValue)
	default:
		// Handle other types (int, float, etc.) by converting to string
		strValue := fmt.Sprintf("%v", value)
		if strValue == "" {
			return ""
		}
		processedValue := processAttributeValue(strValue, kebabKey, templateData, blockID)
		escapedValue := escapeAttributeValue(processedValue, kebabKey)
		return fmt.Sprintf(` %s="%s"`, kebabKey, escapedValue)
	}
}

// processAttributeValue processes attribute values through liquid templating if applicable
func processAttributeValue(value, attributeKey string, templateData map[string]interface{}, blockID string) string {
	// Process liquid templates for URL-related attributes and alt text
	isURLAttribute := attributeKey == "href" || attributeKey == "src" || attributeKey == "action" ||
		attributeKey == "background-url" || strings.HasSuffix(attributeKey, "-url")

	// Alt attribute for images - users commonly personalize this
	isAltAttribute := attributeKey == "alt"

	// If templateData is nil/empty or this isn't a processable attribute, return as-is.
	// This preserves Liquid syntax (e.g., {{ postImage }}) when no template data is provided,
	// which is important for MJML export where we want to keep the raw Liquid syntax.
	// Without this check, Liquid variables would render as empty strings, breaking URLs (issue #226).
	if templateData == nil || len(templateData) == 0 || (!isURLAttribute && !isAltAttribute) {
		return value
	}

	// Check if value contains Liquid syntax before processing
	hasLiquidSyntax := strings.Contains(value, "{{") || strings.Contains(value, "{%")

	// Process liquid content for URL attributes
	processedValue, err := processLiquidContent(value, templateData, fmt.Sprintf("%s.%s", blockID, attributeKey))
	if err != nil {
		// If liquid processing fails, return original value and log warning
		fmt.Printf("Warning: Liquid processing failed for attribute %s in block %s: %v\n", attributeKey, blockID, err)
		return value
	}

	// Issue #226 fix: If the original value contained Liquid syntax but rendered to empty,
	// it means the template variable is not defined in the template data.
	// Show a helpful debug message instead of an empty string to help users identify missing variables.
	// This also prevents broken URLs like src="" which would cause MJML compilation errors.
	if hasLiquidSyntax && strings.TrimSpace(processedValue) == "" {
		// Extract variable name from Liquid syntax for clearer error message
		// Handles {{ varName }} and {{ object.property }} patterns
		varName := extractLiquidVariableName(value)
		if varName != "" {
			return fmt.Sprintf("[undefined: %s]", varName)
		}
		return "[undefined variable]"
	}

	return processedValue
}

// liquidVarRegex matches Liquid variable expressions like {{ varName }} or {{ object.property }}
// Compiled once at package level for performance
var liquidVarRegex = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*(?:\|[^}]*)?\}\}`)

// extractLiquidVariableName extracts the variable name from a Liquid template expression
// e.g., "{{ postImage }}" -> "postImage", "{{ contact.email }}" -> "contact.email"
func extractLiquidVariableName(value string) string {
	matches := liquidVarRegex.FindStringSubmatch(value)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// camelToKebab converts camelCase to kebab-case
func camelToKebab(str string) string {
	// Use regex to find capital letters and replace them with hyphen + lowercase
	re := regexp.MustCompile("([A-Z])")
	return re.ReplaceAllStringFunc(str, func(match string) string {
		return "-" + strings.ToLower(match)
	})
}

// escapeAttributeValue escapes attribute values for safe XML/MJML output
// All ampersands must be escaped as &amp; per XML specification
// The MJML compiler will handle converting them back to & in the final HTML
func escapeAttributeValue(value string, attributeName string) string {
	// Always escape ampersands first, even in URLs
	// MJML is XML and must follow XML escaping rules
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	value = strings.ReplaceAll(value, "'", "&#39;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	return value
}

// escapeContent escapes content for safe HTML output
func escapeContent(content string) string {
	content = strings.ReplaceAll(content, "&", "&amp;")
	content = strings.ReplaceAll(content, "<", "&lt;")
	content = strings.ReplaceAll(content, ">", "&gt;")
	return content
}

// ConvertToMJMLString is a convenience function that converts an EmailBlock to MJML
// and wraps it in a complete MJML document structure if needed
func ConvertToMJMLString(block EmailBlock) (string, error) {
	return ConvertToMJMLStringWithData(block, "")
}

// ConvertToMJMLStringWithData converts an EmailBlock to MJML with template data
func ConvertToMJMLStringWithData(block EmailBlock, templateData string) (string, error) {
	if block == nil {
		return "", fmt.Errorf("block cannot be nil")
	}

	// If the root block is not MJML, we need to validate the structure
	if block.GetType() != MJMLComponentMjml {
		return "", fmt.Errorf("root block must be of type 'mjml', got '%s'", block.GetType())
	}

	// Validate the email structure before converting
	if err := ValidateEmailStructure(block); err != nil {
		return "", fmt.Errorf("invalid email structure: %w", err)
	}

	return ConvertJSONToMJMLWithData(block, templateData)
}

// ConvertToMJMLWithOptions provides additional options for MJML conversion
type MJMLConvertOptions struct {
	Validate      bool   // Whether to validate the structure before converting
	PrettyPrint   bool   // Whether to format with proper indentation (always true for now)
	IncludeXMLTag bool   // Whether to include XML declaration at the beginning
	TemplateData  string // JSON string containing template data for Liquid processing
}

// ConvertToMJMLWithOptions converts an EmailBlock to MJML string with additional options
func ConvertToMJMLWithOptions(block EmailBlock, options MJMLConvertOptions) (string, error) {
	if block == nil {
		return "", fmt.Errorf("block cannot be nil")
	}

	// Validate if requested
	if options.Validate {
		if err := ValidateEmailStructure(block); err != nil {
			return "", fmt.Errorf("validation failed: %w", err)
		}
	}

	// Convert to MJML with template data
	mjml, err := ConvertJSONToMJMLWithData(block, options.TemplateData)
	if err != nil {
		return "", fmt.Errorf("mjml conversion failed: %w", err)
	}

	// Add XML declaration if requested
	if options.IncludeXMLTag {
		mjml = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" + mjml
	}

	return mjml, nil
}
