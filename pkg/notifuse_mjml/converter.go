package notifuse_mjml

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// liquidEnginePool reuses SecureLiquidEngine instances across renders.
// Creating an engine registers all standard tags on a fresh environment,
// which is wasteful when rendering many blocks (e.g. during a broadcast).
// A pool is used instead of a shared instance because liquid.Environment
// caches strainer classes in an unsynchronized map during rendering, so a
// single engine must not be used from multiple goroutines concurrently.
var liquidEnginePool = sync.Pool{
	New: func() interface{} {
		return NewSecureLiquidEngine()
	},
}

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
	return convertBlockToMJMLWithErrorAndParsedData(tree, 0, templateData, parsedData, false)
}

// ConvertJSONToMJMLWithDataProtected is ConvertJSONToMJMLWithData but neutralizes the
// Liquid delimiters in everything it renders per-block, so a following whole-document
// Liquid pass cannot execute Liquid that came from data (a contact field, a feed
// value) rather than from the template. The caller must call restoreLiquidDelimiters
// on the result after that pass. Used only by CompileTemplate.
func ConvertJSONToMJMLWithDataProtected(tree EmailBlock, templateData string) (string, error) {
	parsedData, parseErr := parseTemplateDataString(templateData)
	if parseErr != nil {
		return "", fmt.Errorf("template data parsing failed: %v", parseErr)
	}
	return convertBlockToMJMLWithErrorAndParsedData(tree, 0, templateData, parsedData, true)
}

// convertBlockToMJMLWithErrorAndParsedData recursively converts a single EmailBlock to MJML string with error handling and pre-parsed data
func convertBlockToMJMLWithErrorAndParsedData(block EmailBlock, indentLevel int, templateData string, parsedData map[string]interface{}, protectLiquid bool) (string, error) {
	// mj-liquid: output content directly, no wrapping tags
	// Content is raw MJML+Liquid processed in the whole-string Liquid pass
	if block.GetType() == MJMLComponentMjLiquid {
		content := getBlockContent(block)
		return content, nil
	}

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
				if len(parsedData) > 0 {
					processedContent, err := processLiquidContent(content, parsedData, block.GetID())
					if err != nil {
						// Return error instead of just logging
						return "", fmt.Errorf("liquid processing failed for block %s: %v", block.GetID(), err)
					} else {
						content = processedContent
						if protectLiquid {
							content = protectRenderedLiquidDelimiters(content)
						}
					}
				}
			}

			// Block with content - don't escape for mj-raw, mj-text, and mj-button (they can contain HTML per MJML spec)
			attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID(), protectLiquid)
			if blockType == MJMLComponentMjRaw || blockType == MJMLComponentMjText || blockType == MJMLComponentMjButton {
				return fmt.Sprintf("%s<%s%s>%s</%s>", indent, tagName, attributeString, content, tagName), nil
			} else {
				return fmt.Sprintf("%s<%s%s>%s</%s>", indent, tagName, attributeString, escapeContent(content), tagName), nil
			}
		} else {
			// Self-closing block or empty block
			attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID(), protectLiquid)
			if attributeString != "" {
				return fmt.Sprintf("%s<%s%s />", indent, tagName, attributeString), nil
			} else {
				return fmt.Sprintf("%s<%s />", indent, tagName), nil
			}
		}
	}

	// Block with children
	attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID(), protectLiquid)
	openTag := fmt.Sprintf("%s<%s%s>", indent, tagName, attributeString)
	closeTag := fmt.Sprintf("%s</%s>", indent, tagName)

	// Process children
	var childrenMJML []string
	for _, child := range children {
		if child != nil {
			childMJML, err := convertBlockToMJMLWithErrorAndParsedData(child, indentLevel+1, templateData, parsedData, protectLiquid)
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
// convertBlockToMJMLWithParsedData is the no-data walk variant (used by
// ConvertJSONToMJML). parsedData is always nil here, so it never renders Liquid and
// needs no delimiter protection; the data path uses convertBlockToMJMLWithErrorAndParsedData.
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

	if blockType == MJMLComponentMjLiquid {
		content := getBlockContent(block)
		return content
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
				if len(parsedData) > 0 {
					processedContent, err := processLiquidContent(content, parsedData, block.GetID())
					if err != nil {
						// Log error but continue with original content
						fmt.Printf("Warning: Liquid processing failed for block %s: %v\n", block.GetID(), err)
					} else {
						content = processedContent
					}
				}
			}

			// Block with content - don't escape for mj-raw, mj-text, and mj-button (they can contain HTML per MJML spec)
			attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID(), false)
			if blockType == MJMLComponentMjRaw || blockType == MJMLComponentMjText || blockType == MJMLComponentMjButton {
				return fmt.Sprintf("%s<%s%s>%s</%s>", indent, tagName, attributeString, content, tagName)
			} else {
				return fmt.Sprintf("%s<%s%s>%s</%s>", indent, tagName, attributeString, escapeContent(content), tagName)
			}
		} else {
			// Self-closing block or empty block
			attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID(), false)
			if attributeString != "" {
				return fmt.Sprintf("%s<%s%s />", indent, tagName, attributeString)
			} else {
				return fmt.Sprintf("%s<%s />", indent, tagName)
			}
		}
	}

	// Block with children
	attributeString := formatAttributesWithLiquid(block.GetAttributes(), parsedData, block.GetID(), false)
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

	if blockType == MJMLComponentMjLiquid {
		content := getBlockContent(block)
		return content
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
			// Block with content - don't escape for mj-raw, mj-text, and mj-button (they can contain HTML per MJML spec)
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

	// Undo what rich text editors do to Liquid markup: invisible characters,
	// curly or encoded quotes, encoded operators (see cleanLiquidTemplate)
	content = cleanLiquidTemplate(content)

	// Get a secure Liquid engine (timeout and size protections) from the pool
	engine := liquidEnginePool.Get().(*SecureLiquidEngine)

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
		// Do not return the engine to the pool on error: on timeout the
		// render goroutine is still running and still using the engine's
		// environment, so repooling it would hand a busy engine to the next
		// caller. Dropping the engine is cheap relative to that risk.
		return content, fmt.Errorf("liquid rendering error in block (ID: %s): %w", blockID, err)
	}

	liquidEnginePool.Put(engine)
	return renderedContent, nil
}

// liquidMarkupRegex finds Liquid template variables ({{ }}) and tags ({% %}) roughly
// the way Liquid's tokenizer delimits them: a variable ends at its first "}", a tag at
// its first "%}", so a tag may contain "%" (date: '%Y', contains '%').
var liquidMarkupRegex = regexp.MustCompile(`(?s)\{\{[^}]*\}\}|\{%.*?%\}`)

// Private-Use-Area sentinels stand in for the Liquid delimiters while an
// already-rendered string passes through the whole-document Liquid pass, so that pass
// cannot execute Liquid that came from data (a contact field, a feed value) rather
// than from the template. They cannot occur in legitimate email content.
const (
	liquidSentinelOpenVar  = "\uE000" // {{
	liquidSentinelCloseVar = "\uE001" // }}
	liquidSentinelOpenTag  = "\uE002" // {%
	liquidSentinelCloseTag = "\uE003" // %}
)

// liquidSentinelStripper removes any pre-existing sentinel from a value before it is
// protected, so crafted data cannot smuggle one in and have it forged into a real
// delimiter on restore.
var liquidSentinelStripper = strings.NewReplacer(
	liquidSentinelOpenVar, "", liquidSentinelCloseVar, "",
	liquidSentinelOpenTag, "", liquidSentinelCloseTag, "",
)

var liquidDelimiterProtector = strings.NewReplacer(
	"{{", liquidSentinelOpenVar, "}}", liquidSentinelCloseVar,
	"{%", liquidSentinelOpenTag, "%}", liquidSentinelCloseTag,
)

var liquidDelimiterRestorer = strings.NewReplacer(
	liquidSentinelOpenVar, "{{", liquidSentinelCloseVar, "}}",
	liquidSentinelOpenTag, "{%", liquidSentinelCloseTag, "%}",
)

// protectRenderedLiquidDelimiters makes every Liquid delimiter in an already-rendered
// string inert; restoreLiquidDelimiters reverses it after the whole-document pass. All
// four delimiters are protected, not just the openers: a stray "%}" left in rendered
// data must not pair with an mj-liquid "{%" across a cross-block conditional.
func protectRenderedLiquidDelimiters(s string) string {
	return liquidDelimiterProtector.Replace(liquidSentinelStripper.Replace(s))
}

func restoreLiquidDelimiters(s string) string {
	return liquidDelimiterRestorer.Replace(s)
}

// liquidRawTagRegex recognizes the tag that opens a raw body, which Liquid outputs
// verbatim.
var liquidRawTagRegex = regexp.MustCompile(`^\{%-?\s*raw\s*-?%\}$`)

// isLiquidEndRawTag reports whether markup ends a raw body, the way liquidgo's raw tag
// decides it: the markup between {% and %}, trimmed, is "endraw" or starts with
// "endraw " ({%- endraw -%} is not an end there, so the body runs on).
func isLiquidEndRawTag(markup string) bool {
	if !strings.HasPrefix(markup, "{%") {
		return false
	}
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(markup, "{%"), "%}"))
	return name == "endraw" || strings.HasPrefix(name, "endraw ")
}

// cleanLiquidTemplate undoes what rich text editors do to the Liquid markup typed
// into them (see cleanLiquidMarkup). The body of a {% raw %} block is copied
// unchanged, since Liquid outputs it verbatim.
func cleanLiquidTemplate(content string) string {
	locs := liquidMarkupRegex.FindAllStringIndex(content, -1)
	if len(locs) == 0 {
		return content
	}
	var b strings.Builder
	b.Grow(len(content))
	last := 0
	inRaw := false
	for _, loc := range locs {
		cleaned := cleanLiquidMarkup(content[loc[0]:loc[1]])
		if inRaw {
			if !isLiquidEndRawTag(cleaned) {
				continue
			}
			inRaw = false
		} else {
			inRaw = liquidRawTagRegex.MatchString(cleaned)
		}
		b.WriteString(content[last:loc[0]])
		b.WriteString(cleaned)
		last = loc[1]
	}
	b.WriteString(content[last:])
	return b.String()
}

// cleanLiquidMarkup undoes what rich text editors do to a single {{ }} or {% %}
// markup: see cleanLiquidSpaces and normalizeLiquidMarkup.
func cleanLiquidMarkup(markup string) string {
	return normalizeLiquidMarkup(cleanLiquidSpaces(markup))
}

// cleanLiquidSpaces turns every non-breaking space, however it is spelled (U+00A0,
// &nbsp;, &#160;, &#xA0;…), into a space and removes zero-width characters, string
// literals included. Deleting a non-breaking space instead would glue `if` to its
// condition.
func cleanLiquidSpaces(markup string) string {
	if !needsLiquidNormalization(markup) {
		return markup
	}
	var b strings.Builder
	b.Grow(len(markup))
	for i := 0; i < len(markup); {
		r, n := readLiquidUnit(markup[i:])
		switch r {
		case '\u00a0':
			b.WriteByte(' ')
		case '\u200b', '\u2060', '\ufeff': // zero-width space, word joiner, byte order mark
		default:
			b.WriteString(markup[i : i+n])
		}
		i += n
	}
	return b.String()
}

// liquidQuotes maps each character that can stand for a string delimiter in Liquid
// markup to its ASCII quote: the console's text editor curled typed quotes (Tiptap
// Typography), word processors curl them on paste, and German typography opens a
// quotation with a low quote. Entity spellings (&#39;, &rsquo;…) are decoded first.
var liquidQuotes = map[rune]byte{
	'\'': '\'', '\u2018': '\'', '\u2019': '\'', '\u201a': '\'', '\u201b': '\'',
	'"': '"', '\u201c': '"', '\u201d': '"', '\u201e': '"', '\u201f': '"',
}

// liquidOperators maps each character that can stand for a comparison operator in
// Liquid markup to the operator: Typography turns a typed != into ≠, word processors
// produce ≥ and ≤, and HTML serialization encodes a typed > or < as an entity.
var liquidOperators = map[rune]string{
	'\u2260': "!=", '\u2265': ">=", '\u2264': "<=", '>': ">", '<': "<",
}

// normalizeLiquidMarkup restores the ASCII syntax of a single {{ }} or {% %} markup.
// Liquid reports none of these spellings as an error: a curly-quoted string parses
// as an unknown variable, so `default: ‘Guest’` renders nothing, and `≠` or `&gt;`
// reduce a comparison to a truthiness test of its left operand.
//
// A literal opened by any quote other than an ASCII one was curled by an editor from
// what the user typed in ASCII: it closes on the next quote of its kind, and the
// apostrophes the editor curled inside it are straightened, so “O’Brien” compares
// equal to O'Brien. A literal opened by an ASCII quote, raw or encoded, keeps its
// characters, but where it closes is ambiguous: 'the dogs’ toys' holds an apostrophe,
// while 'Guest’ is a pair the editor half-curled. The first reading closes such a
// literal only on an ASCII quote; when that leaves a literal open, the second also
// closes it on a curly quote that ends a word. A markup neither reading closes is
// returned unchanged, since rewriting one of its quotes would hand Liquid a lone
// quote, which it cannot parse.
func normalizeLiquidMarkup(markup string) string {
	if !needsLiquidNormalization(markup) {
		return markup
	}
	for _, curlyCloses := range [...]bool{false, true} {
		if normalized, ok := rewriteLiquidMarkup(markup, curlyCloses); ok {
			return normalized
		}
	}
	return markup
}

// rewriteLiquidMarkup is one reading of normalizeLiquidMarkup; ok is false when the
// markup ends inside a literal. Without curlyCloses, a literal opened by an ASCII quote
// is copied verbatim up to its ASCII closer, or read like a curled one when it has
// none; with it, such a literal also closes on a closing curly quote of its kind that
// ends a word.
func rewriteLiquidMarkup(markup string, curlyCloses bool) (normalized string, ok bool) {
	var b strings.Builder
	b.Grow(len(markup))
	var open byte     // ASCII quote of the literal being copied, 0 outside one
	verbatim := false // the literal was opened by an ASCII quote and is copied verbatim
	for i := 0; i < len(markup); {
		r, n := readLiquidUnit(markup[i:])
		quote, isQuote := liquidQuotes[r]
		switch {
		case open == 0 && isQuote:
			open = quote
			verbatim = r == rune(quote) && (curlyCloses || hasLiquidASCIIQuote(markup[i+n:], quote))
			b.WriteByte(quote)
		case open == 0:
			if operator, ok := liquidOperators[r]; ok {
				b.WriteString(operator)
			} else {
				b.WriteString(markup[i : i+n])
			}
		case verbatim:
			if r == rune(open) || (curlyCloses && quote == open && endsWordWithClosingQuote(r, markup[i+n:])) {
				b.WriteByte(open)
				open = 0
			} else {
				b.WriteString(markup[i : i+n])
			}
		case isQuote && (quote == open || (quote == '\'' && r != '\'')):
			// The closing quote, or an apostrophe the editor curled. Other quotes,
			// such as “hi” or an encoded &quot; inside ‘…’, are kept: straightened,
			// a double quote would end the HTML attribute the value renders into.
			if quote == open {
				open = 0
			}
			b.WriteByte(quote)
		default:
			b.WriteString(markup[i : i+n])
		}
		i += n
	}
	return b.String(), open == 0
}

// endsWordWithClosingQuote reports whether r is a closing curly quote that is not
// followed by a letter or a digit, as a closing delimiter is and the apostrophe of
// "It’s" is not.
func endsWordWithClosingQuote(r rune, rest string) bool {
	if r != '\u2019' && r != '\u201d' {
		return false
	}
	if rest == "" {
		return true
	}
	next, _ := readLiquidUnit(rest)
	return !unicode.IsLetter(next) && !unicode.IsDigit(next)
}

// needsLiquidNormalization reports whether markup holds anything normalizeLiquidMarkup
// rewrites: every such spelling is an entity or a non-ASCII character.
func needsLiquidNormalization(markup string) bool {
	for i := 0; i < len(markup); i++ {
		if markup[i] == '&' || markup[i] >= utf8.RuneSelf {
			return true
		}
	}
	return false
}

// maxLiquidEntityLength bounds the HTML entities readLiquidUnit decodes; every
// spelling of a quote or an operator is shorter (&#x201D;, &rsquo;).
const maxLiquidEntityLength = 10

// readLiquidUnit returns the character s starts with and the length of its spelling,
// decoding an HTML entity so that &#39;, &#x27; and &apos; all read as '.
func readLiquidUnit(s string) (rune, int) {
	if s[0] == '&' {
		if end := strings.IndexByte(s[:min(len(s), maxLiquidEntityLength)], ';'); end > 1 {
			entity := s[:end+1]
			if decoded := html.UnescapeString(entity); decoded != entity {
				if r, size := utf8.DecodeRuneInString(decoded); size == len(decoded) {
					return r, len(entity)
				}
			}
		}
		return '&', 1
	}
	return utf8.DecodeRuneInString(s)
}

// hasLiquidASCIIQuote reports whether s contains the ASCII quote q, raw or encoded.
func hasLiquidASCIIQuote(s string, q byte) bool {
	for i := 0; i < len(s); {
		r, n := readLiquidUnit(s[i:])
		if r == rune(q) {
			return true
		}
		i += n
	}
	return false
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
	return formatAttributesWithLiquid(attributes, nil, "", false)
}

// formatAttributesWithLiquid formats attributes object into MJML attribute string with liquid processing
func formatAttributesWithLiquid(attributes map[string]interface{}, templateData map[string]interface{}, blockID string, protectLiquid bool) string {
	if len(attributes) == 0 {
		return ""
	}

	var attrPairs []string
	for key, value := range attributes {
		// Skip editor-only attributes that are not valid MJML
		if key == "visibility" {
			continue
		}
		if shouldIncludeAttribute(value) {
			if attr := formatSingleAttributeWithLiquid(key, value, templateData, blockID, protectLiquid); attr != "" {
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
	return formatSingleAttributeWithLiquid(key, value, nil, "", false)
}

// formatSingleAttributeWithLiquid formats a single attribute key-value pair with liquid processing
func formatSingleAttributeWithLiquid(key string, value interface{}, templateData map[string]interface{}, blockID string, protectLiquid bool) string {
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
		processedValue := processAttributeValue(v, kebabKey, templateData, blockID, protectLiquid)
		escapedValue := escapeAttributeValue(processedValue, kebabKey)
		return fmt.Sprintf(` %s="%s"`, kebabKey, escapedValue)
	case *string:
		if v == nil || *v == "" {
			return ""
		}
		processedValue := processAttributeValue(*v, kebabKey, templateData, blockID, protectLiquid)
		escapedValue := escapeAttributeValue(processedValue, kebabKey)
		return fmt.Sprintf(` %s="%s"`, kebabKey, escapedValue)
	case map[string]interface{}:
		// Visual-editor box-model objects (e.g. inner-padding stored as
		// {top, right, bottom, left}) must compile to a CSS shorthand, never to
		// Go's "map[...]" formatting which strict clients like Gmail reject.
		cssValue, ok := formatBoxModelMap(v)
		if !ok || cssValue == "" {
			return ""
		}
		processedValue := processAttributeValue(cssValue, kebabKey, templateData, blockID, protectLiquid)
		escapedValue := escapeAttributeValue(processedValue, kebabKey)
		return fmt.Sprintf(` %s="%s"`, kebabKey, escapedValue)
	default:
		// Handle other types (int, float, etc.) by converting to string
		strValue := fmt.Sprintf("%v", value)
		if strValue == "" {
			return ""
		}
		processedValue := processAttributeValue(strValue, kebabKey, templateData, blockID, protectLiquid)
		escapedValue := escapeAttributeValue(processedValue, kebabKey)
		return fmt.Sprintf(` %s="%s"`, kebabKey, escapedValue)
	}
}

// boxModelSides are the side keys a visual-editor padding/margin object may carry.
var boxModelSides = []string{"top", "right", "bottom", "left"}

// formatBoxModelMap converts a visual-editor box-model object such as
// {"top": "0px", "bottom": "0px"} into a CSS shorthand string ("0px",
// "10px 25px" or "1px 2px 3px 4px"). Missing sides default to "0px", mirroring
// the console's PaddingInput shorthand logic. It returns ok=false when the map is
// not a box-model object (or is empty), so the caller can skip the attribute
// entirely instead of leaking Go's "map[...]" formatting into the MJML/CSS.
func formatBoxModelMap(m map[string]interface{}) (string, bool) {
	hasSide := false
	for _, side := range boxModelSides {
		if _, ok := m[side]; ok {
			hasSide = true
			break
		}
	}
	if !hasSide {
		return "", false
	}

	sideValue := func(key string) string {
		if v, ok := m[key]; ok && v != nil {
			if s := strings.TrimSpace(fmt.Sprintf("%v", v)); s != "" {
				return s
			}
		}
		return "0px"
	}

	top := sideValue("top")
	right := sideValue("right")
	bottom := sideValue("bottom")
	left := sideValue("left")

	switch {
	case top == right && right == bottom && bottom == left:
		return top, true
	case top == bottom && right == left:
		return top + " " + right, true
	default:
		return top + " " + right + " " + bottom + " " + left, true
	}
}

// processAttributeValue processes attribute values through liquid templating if applicable
func processAttributeValue(value, attributeKey string, templateData map[string]interface{}, blockID string, protectLiquid bool) string {
	// Process liquid templates for URL-related attributes and alt text
	isURLAttribute := attributeKey == "href" || attributeKey == "src" || attributeKey == "action" ||
		attributeKey == "background-url" || strings.HasSuffix(attributeKey, "-url")

	// Alt attribute for images - users commonly personalize this
	isAltAttribute := attributeKey == "alt"

	// If templateData is nil/empty or this isn't a processable attribute, return as-is.
	// This preserves Liquid syntax (e.g., {{ postImage }}) when no template data is provided,
	// which is important for MJML export where we want to keep the raw Liquid syntax.
	// Without this check, Liquid variables would render as empty strings, breaking URLs (issue #226).
	if len(templateData) == 0 || (!isURLAttribute && !isAltAttribute) {
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

	if protectLiquid {
		return protectRenderedLiquidDelimiters(processedValue)
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

// upperCaseRegex finds capital letters for camelCase to kebab-case conversion
var upperCaseRegex = regexp.MustCompile("([A-Z])")

// camelToKebab converts camelCase to kebab-case
func camelToKebab(str string) string {
	// Replace capital letters with hyphen + lowercase
	return upperCaseRegex.ReplaceAllStringFunc(str, func(match string) string {
		return "-" + strings.ToLower(match)
	})
}

// escapeAttributeValue escapes attribute values for safe XML/MJML output
// All ampersands must be escaped as &amp; per XML specification
// The MJML compiler will handle converting them back to & in the final HTML
func escapeAttributeValue(value string, attributeName string) string {
	// The MJML renderer (gomjml) decodes the named XML entities &lt; &gt; &quot; &apos;
	// back to raw characters in a pre-pass *before* its XML parse, and then writes
	// attribute values to the final HTML without re-escaping them. A value escaped the
	// ordinary XML way (" -> &quot;) therefore reaches the attribute as a raw " < or >,
	// which closes the attribute and fails the compile, or emits broken HTML. Escaping
	// the ampersand of each entity a second time (&amp;quot;) survives that pre-pass:
	// gomjml preserves &amp;, the XML parser turns &amp; back into &, and the literal
	// entity &quot; is what lands in the HTML, which the browser decodes to ". The
	// apostrophe uses the numeric &#39;, which gomjml does not pre-decode, and a raw '
	// inside a double-quoted attribute needs no further escaping. See the gomjml
	// write-up (double-decoding + unescaped attribute output); drop the doubling once a
	// renderer release escapes attribute output.
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "\"", "&amp;quot;")
	value = strings.ReplaceAll(value, "'", "&#39;")
	value = strings.ReplaceAll(value, "<", "&amp;lt;")
	value = strings.ReplaceAll(value, ">", "&amp;gt;")
	return value
}

// escapeContent escapes content for safe insertion as element text content.
// Angle brackets use numeric character references (&#60;/&#62;) rather than
// &lt;/&gt; because the MJML parser pre-decodes the named entities back to raw
// brackets before XML parsing, which would turn escaped text into markup and
// fail the compile; numeric references survive that pre-decode untouched.
// Keep in sync with escapeXMLContent in template_compilation.go.
func escapeContent(content string) string {
	content = strings.ReplaceAll(content, "&", "&amp;")
	content = strings.ReplaceAll(content, "<", "&#60;")
	content = strings.ReplaceAll(content, ">", "&#62;")
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
