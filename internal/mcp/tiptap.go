package mcp

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// A blog post's body (D26) is a Tiptap JSON document. The tools check it here,
// then render the HTML and the plain text its web template stores, as the
// console's jsonToHtml and extractTextContent do
// (console/src/components/blog/utils.ts). The public page shows that HTML, so
// every text and attribute value is escaped as it is written, and each
// attribute the console writes unescaped is checked against a pattern first.

// maxTiptapDepth bounds how deeply nodes nest: a list in a list item in a
// blockquote is four levels, and the editor never needs dozens.
const maxTiptapDepth = 32

// tiptapBlocks are the node types a document, a blockquote or a list item
// holds; tiptapInline those a paragraph or a heading holds.
var (
	tiptapBlocks = []string{"paragraph", "heading", "blockquote", "bulletList", "orderedList", "codeBlock", "horizontalRule", "image", "youtube"}
	tiptapInline = []string{"text", "hardBreak"}
	tiptapMarks  = []string{"bold", "italic", "underline", "strike", "code", "link", "textStyle", "highlight", "subscript", "superscript"}
)

// tiptapChildren is what each node type may contain; a type it does not list
// is a leaf.
var tiptapChildren = map[string][]string{
	"doc":         tiptapBlocks,
	"paragraph":   tiptapInline,
	"heading":     tiptapInline,
	"blockquote":  tiptapBlocks,
	"listItem":    tiptapBlocks,
	"bulletList":  {"listItem"},
	"orderedList": {"listItem"},
	"codeBlock":   {"text"},
}

var (
	textAligns = []string{"left", "center", "right", "justify"}
	// A CSS color as the editor writes one: a name, #hex, rgb() or var().
	// No quote, colon, semicolon or slash, so it cannot leave its declaration.
	cssColorPattern  = regexp.MustCompile(`^[#A-Za-z0-9(),.%\s-]{0,64}$`)
	youtubeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	dimensionPattern = regexp.MustCompile(`^[0-9]{1,6}(\.[0-9]{1,4})?(px|%)?$`)
	digitsPattern    = regexp.MustCompile(`^[0-9]{1,9}$`)
)

// checkTiptapDocument checks a body before anything is written: a doc root,
// known node and mark types in the places the editor puts them, no empty text
// (the editor refuses to load one), link URLs in http, https or mailto, image
// URLs in http or https, and attribute values that render safely.
func checkTiptapDocument(doc map[string]any) *ToolError {
	if doc["type"] != "doc" {
		return validationError(`body: the root must be {"type":"doc","content":[...]}`)
	}
	if _, ok := doc["content"].([]any); !ok {
		return validationError("body.content: must be an array of block nodes")
	}
	if message := checkTiptapNode(doc, "body", 0); message != "" {
		return validationError("%s", message)
	}
	return nil
}

func checkTiptapNode(node map[string]any, path string, depth int) string {
	if depth > maxTiptapDepth {
		return fmt.Sprintf("%s: nodes nest more than %d levels deep", path, maxTiptapDepth)
	}
	nodeType, _ := node["type"].(string)
	for key := range node {
		if !slices.Contains([]string{"type", "attrs", "content", "marks", "text"}, key) {
			return fmt.Sprintf("%s: unknown key %q; a node has type, attrs, content, and marks and text for a text node", path, key)
		}
	}
	attrs := map[string]any{}
	if raw, present := node["attrs"]; present && raw != nil {
		object, ok := raw.(map[string]any)
		if !ok {
			return path + ".attrs: must be an object"
		}
		attrs = object
	}

	if nodeType == "text" {
		if _, present := node["content"]; present {
			return path + ": a text node has no content"
		}
		text, ok := node["text"].(string)
		if !ok || text == "" {
			return path + ".text: a text node needs a non-empty text"
		}
		marks, present := node["marks"]
		if !present || marks == nil {
			return ""
		}
		list, ok := marks.([]any)
		if !ok {
			return path + ".marks: must be an array"
		}
		for i, item := range list {
			mark, ok := item.(map[string]any)
			if !ok {
				return fmt.Sprintf("%s.marks[%d]: must be an object", path, i)
			}
			if message := checkTiptapMark(mark, fmt.Sprintf("%s.marks[%d]", path, i)); message != "" {
				return message
			}
		}
		return ""
	}
	if _, present := node["text"]; present {
		return path + ": only a text node has text"
	}
	if _, present := node["marks"]; present {
		return path + ": only a text node has marks"
	}
	if message := checkTiptapAttrs(nodeType, attrs, path+".attrs"); message != "" {
		return message
	}

	raw, present := node["content"]
	allowed, container := tiptapChildren[nodeType]
	if !container {
		if present {
			return fmt.Sprintf("%s: a %s node has no content", path, nodeType)
		}
		return ""
	}
	if !present || raw == nil {
		return ""
	}
	children, ok := raw.([]any)
	if !ok {
		return path + ".content: must be an array"
	}
	for i, item := range children {
		childPath := fmt.Sprintf("%s.content[%d]", path, i)
		child, ok := item.(map[string]any)
		if !ok {
			return childPath + ": must be a node object"
		}
		childType, _ := child["type"].(string)
		if !slices.Contains(allowed, childType) {
			return fmt.Sprintf("%s: a %s node cannot hold a %q node; it holds %s", childPath, nodeType, childType, strings.Join(allowed, ", "))
		}
		if nodeType == "codeBlock" {
			if _, marked := child["marks"]; marked {
				return childPath + ": text in a code block has no marks"
			}
		}
		if message := checkTiptapNode(child, childPath, depth+1); message != "" {
			return message
		}
	}
	return ""
}

// checkTiptapAttrs checks the attributes the renderer writes; any other
// attribute is kept in the document and never rendered.
func checkTiptapAttrs(nodeType string, attrs map[string]any, path string) string {
	switch nodeType {
	case "paragraph", "heading", "blockquote":
		if message := checkEnum(attrs, "textAlign", textAligns, path); message != "" {
			return message
		}
		for _, key := range []string{"backgroundColor", "color"} {
			if message := checkColor(attrs, key, path); message != "" {
				return message
			}
		}
		if nodeType == "heading" {
			if level, present := attrs["level"]; present && level != nil {
				n, ok := level.(float64)
				if !ok || n != float64(int(n)) || n < 1 || n > 6 {
					return path + ".level: must be an integer from 1 to 6"
				}
			}
		}
	case "orderedList":
		return checkCount(attrs, "start", path)
	case "codeBlock":
		return checkString(attrs, "language", path)
	case "image":
		src, _ := attrs["src"].(string)
		if !hasScheme(src, "http://", "https://") {
			return path + ".src: an image needs an http or https URL"
		}
		for _, key := range []string{"alt", "title", "align", "caption"} {
			if message := checkString(attrs, key, path); message != "" {
				return message
			}
		}
		for _, key := range []string{"width", "height"} {
			if message := checkDimension(attrs, key, path); message != "" {
				return message
			}
		}
		if value, present := attrs["showCaption"]; present && value != nil && value != true && value != false && value != "true" && value != "false" {
			return path + ".showCaption: must be true or false"
		}
	case "youtube":
		src, _ := attrs["src"].(string)
		if !youtubeIDPattern.MatchString(src) {
			return path + `.src: must be a YouTube video ID of 11 characters, such as "dQw4w9WgXcQ", not a URL`
		}
		for _, key := range []string{"align", "caption"} {
			if message := checkString(attrs, key, path); message != "" {
				return message
			}
		}
		for _, key := range []string{"width", "height"} {
			if message := checkDimension(attrs, key, path); message != "" {
				return message
			}
		}
		if message := checkCount(attrs, "start", path); message != "" {
			return message
		}
		for _, key := range []string{"cc", "loop", "controls", "modestbranding", "showCaption"} {
			value, present := attrs[key]
			if !present || value == nil {
				continue
			}
			switch value {
			case true, false, "true", "false", float64(0), float64(1), "0", "1":
			default:
				return fmt.Sprintf("%s.%s: must be true or false", path, key)
			}
		}
	}
	return ""
}

func checkTiptapMark(mark map[string]any, path string) string {
	markType, _ := mark["type"].(string)
	if !slices.Contains(tiptapMarks, markType) {
		return fmt.Sprintf("%s: unknown mark %q; marks are %s", path, markType, strings.Join(tiptapMarks, ", "))
	}
	for key := range mark {
		if key != "type" && key != "attrs" {
			return fmt.Sprintf("%s: unknown key %q; a mark has type and attrs", path, key)
		}
	}
	attrs := map[string]any{}
	if raw, present := mark["attrs"]; present && raw != nil {
		object, ok := raw.(map[string]any)
		if !ok {
			return path + ".attrs: must be an object"
		}
		attrs = object
	}
	switch markType {
	case "link":
		href, _ := attrs["href"].(string)
		if !hasScheme(href, "http://", "https://", "mailto:") {
			return path + ".attrs.href: a link needs an http, https or mailto URL"
		}
		return checkString(attrs, "target", path+".attrs")
	case "textStyle", "highlight":
		return checkColor(attrs, "color", path+".attrs")
	}
	return ""
}

// hasScheme reports whether a URL starts with one of the schemes, in any
// case, with nothing before it and no control character anywhere: a browser
// drops tabs and newlines from a URL, which would let one hide a scheme.
func hasScheme(value string, schemes ...string) bool {
	if strings.ContainsFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return false
	}
	lower := strings.ToLower(value)
	for _, scheme := range schemes {
		if strings.HasPrefix(lower, scheme) && len(value) > len(scheme) {
			_, err := url.Parse(value)
			return err == nil
		}
	}
	return false
}

func checkString(attrs map[string]any, key, path string) string {
	if value, present := attrs[key]; present && value != nil {
		if _, ok := value.(string); !ok {
			return fmt.Sprintf("%s.%s: must be a string", path, key)
		}
	}
	return ""
}

func checkEnum(attrs map[string]any, key string, values []string, path string) string {
	if value, present := attrs[key]; present && value != nil && value != "" {
		text, ok := value.(string)
		if !ok || !slices.Contains(values, text) {
			return fmt.Sprintf("%s.%s: must be one of %s", path, key, strings.Join(values, ", "))
		}
	}
	return ""
}

func checkColor(attrs map[string]any, key, path string) string {
	if value, present := attrs[key]; present && value != nil {
		text, ok := value.(string)
		if !ok || !cssColorPattern.MatchString(text) {
			return fmt.Sprintf(`%s.%s: must be a CSS color such as "#ff0000" or "rgb(255, 0, 0)"`, path, key)
		}
	}
	return ""
}

// checkDimension accepts a width or a height as the editor writes one: a
// number, or a number in a string with px or %.
func checkDimension(attrs map[string]any, key, path string) string {
	value, present := attrs[key]
	if !present || value == nil {
		return ""
	}
	switch v := value.(type) {
	case float64:
		if v >= 0 && v <= 100000 {
			return ""
		}
	case string:
		if v == "" || dimensionPattern.MatchString(v) {
			return ""
		}
	}
	return fmt.Sprintf("%s.%s: must be a number of pixels", path, key)
}

// checkCount accepts a list's first number or a video's start second: a whole
// number, or one in a string.
func checkCount(attrs map[string]any, key, path string) string {
	value, present := attrs[key]
	if !present || value == nil {
		return ""
	}
	switch v := value.(type) {
	case float64:
		if v >= 0 && v <= 1e9 && v == float64(int64(v)) {
			return ""
		}
	case string:
		if v == "" || digitsPattern.MatchString(v) {
			return ""
		}
	}
	return fmt.Sprintf("%s.%s: must be a whole number", path, key)
}

// renderTiptapHTML is jsonToHtml for a document checkTiptapDocument accepted.
func renderTiptapHTML(doc map[string]any) string {
	if _, ok := doc["content"].([]any); !ok {
		return ""
	}
	var b strings.Builder
	writeTiptapNode(&b, doc)
	return b.String()
}

// tiptapPlainText is extractTextContent: every text, the children of a node
// joined with a space, trimmed.
func tiptapPlainText(doc map[string]any) string {
	if _, ok := doc["content"].([]any); !ok {
		return ""
	}
	return strings.TrimFunc(tiptapText(doc), isJSSpace)
}

func tiptapText(node map[string]any) string {
	if node["type"] == "text" {
		text, _ := node["text"].(string)
		return text
	}
	children, ok := node["content"].([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(children))
	for _, child := range children {
		object, _ := child.(map[string]any)
		parts = append(parts, tiptapText(object))
	}
	return strings.Join(parts, " ")
}

// isJSSpace is the white space JavaScript's String.prototype.trim removes.
func isJSSpace(r rune) bool {
	switch {
	case r >= '\t' && r <= '\r', r == ' ', r == 0xa0, r == 0x1680, r >= 0x2000 && r <= 0x200a,
		r == 0x2028, r == 0x2029, r == 0x202f, r == 0x205f, r == 0x3000, r == 0xfeff:
		return true
	}
	return false
}

var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#039;")

// escapeHTML is the console's escapeHtml, entity for entity.
func escapeHTML(text string) string {
	return htmlEscaper.Replace(text)
}

func tiptapChildNodes(node map[string]any) []map[string]any {
	raw, _ := node["content"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if child, ok := item.(map[string]any); ok {
			out = append(out, child)
		}
	}
	return out
}

func nodeAttrs(node map[string]any) map[string]any {
	attrs, _ := node["attrs"].(map[string]any)
	if attrs == nil {
		return map[string]any{}
	}
	return attrs
}

// truthy is JavaScript's truthiness for a decoded JSON value.
func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return v != ""
	default:
		return true
	}
}

// jsString is String(value) for a decoded JSON value.
func jsString(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// orDefault is JavaScript's value || fallback.
func orDefault(value any, fallback any) any {
	if truthy(value) {
		return value
	}
	return fallback
}

func writeTiptapNode(b *strings.Builder, node map[string]any) {
	nodeType, _ := node["type"].(string)
	if nodeType == "text" {
		b.WriteString(tiptapMarkedText(node))
		return
	}
	var inner strings.Builder
	for _, child := range tiptapChildNodes(node) {
		writeTiptapNode(&inner, child)
	}
	content := inner.String()
	attrs := nodeAttrs(node)

	switch nodeType {
	case "doc":
		b.WriteString(content)
	case "paragraph":
		if content == "" {
			content = "<br>"
		}
		b.WriteString("<p" + styleAttr(attrs) + ">" + content + "</p>")
	case "heading":
		level := jsString(orDefault(attrs["level"], float64(2)))
		b.WriteString("<h" + level + styleAttr(attrs) + ">" + content + "</h" + level + ">")
	case "blockquote":
		b.WriteString("<blockquote" + styleAttr(attrs) + ">" + content + "</blockquote>")
	case "bulletList":
		b.WriteString("<ul>" + content + "</ul>")
	case "orderedList":
		start := ""
		if truthy(attrs["start"]) {
			start = ` start="` + escapeHTML(jsString(attrs["start"])) + `"`
		}
		b.WriteString("<ol" + start + ">" + content + "</ol>")
	case "listItem":
		b.WriteString("<li>" + content + "</li>")
	case "codeBlock":
		language := jsString(orDefault(attrs["language"], "plaintext"))
		lines := []string{}
		for _, child := range tiptapChildNodes(node) {
			text, _ := child["text"].(string)
			lines = append(lines, text)
		}
		b.WriteString(`<pre><code class="language-` + escapeHTML(language) + `">` + escapeHTML(strings.Join(lines, "\n")) + "</code></pre>")
	case "horizontalRule":
		b.WriteString("<hr>")
	case "hardBreak":
		b.WriteString("<br>")
	case "image":
		writeImage(b, attrs)
	case "youtube":
		writeYoutube(b, attrs)
	default:
		b.WriteString(content)
	}
}

func styleAttr(attrs map[string]any) string {
	var styles []string
	if align := attrs["textAlign"]; truthy(align) && align != "left" {
		styles = append(styles, "text-align: "+jsString(align))
	}
	if truthy(attrs["backgroundColor"]) {
		styles = append(styles, "background-color: "+jsString(attrs["backgroundColor"]))
	}
	if truthy(attrs["color"]) {
		styles = append(styles, "color: "+jsString(attrs["color"]))
	}
	if len(styles) == 0 {
		return ""
	}
	return ` style="` + escapeHTML(strings.Join(styles, "; ")) + `"`
}

func tiptapMarkedText(node map[string]any) string {
	text, _ := node["text"].(string)
	out := escapeHTML(text)
	marks, _ := node["marks"].([]any)
	for _, item := range marks {
		mark, _ := item.(map[string]any)
		attrs := nodeAttrs(mark)
		switch mark["type"] {
		case "bold":
			out = "<strong>" + out + "</strong>"
		case "italic":
			out = "<em>" + out + "</em>"
		case "underline":
			out = "<u>" + out + "</u>"
		case "strike":
			out = "<s>" + out + "</s>"
		case "code":
			out = "<code>" + out + "</code>"
		case "link":
			href := escapeHTML(jsString(orDefault(attrs["href"], "#")))
			target := ""
			if truthy(attrs["target"]) {
				target = ` target="` + escapeHTML(jsString(attrs["target"])) + `"`
			}
			out = `<a href="` + href + `"` + target + ">" + out + "</a>"
		case "textStyle":
			if truthy(attrs["color"]) {
				out = `<span style="color: ` + escapeHTML(jsString(attrs["color"])) + `">` + out + "</span>"
			}
		case "highlight":
			color := jsString(orDefault(attrs["color"], "#ffff00"))
			out = `<mark style="background-color: ` + escapeHTML(color) + `">` + out + "</mark>"
		case "subscript":
			out = "<sub>" + out + "</sub>"
		case "superscript":
			out = "<sup>" + out + "</sup>"
		}
	}
	return out
}

func writeImage(b *strings.Builder, attrs map[string]any) {
	src := escapeHTML(jsString(orDefault(attrs["src"], "")))
	alt := escapeHTML(jsString(orDefault(attrs["alt"], "")))
	out := `src="` + src + `" alt="` + alt + `"`
	if truthy(attrs["title"]) {
		out += ` title="` + escapeHTML(jsString(attrs["title"])) + `"`
	}
	for _, attr := range []struct{ key, name string }{
		{"width", "data-width"}, {"height", "data-height"}, {"align", "data-align"}, {"caption", "data-caption"},
	} {
		if truthy(attrs[attr.key]) {
			out += " " + attr.name + `="` + escapeHTML(jsString(attrs[attr.key])) + `"`
		}
	}
	// The console writes this one whenever the key is there, null included.
	if value, present := attrs["showCaption"]; present {
		out += ` data-show-caption="` + escapeHTML(jsString(value)) + `"`
	}
	b.WriteString("<img " + out + " />")
}

func isOn(value any) bool {
	return value == true || value == "true" || value == float64(1) || value == "1"
}

func isOff(value any) bool {
	return value == false || value == "false" || value == float64(0) || value == "0"
}

func writeYoutube(b *strings.Builder, attrs map[string]any) {
	videoID := jsString(orDefault(attrs["src"], ""))
	width := jsString(orDefault(attrs["width"], float64(640)))
	height := jsString(orDefault(attrs["height"], float64(360)))
	align := jsString(orDefault(attrs["align"], "left"))
	cc, loop, modestBranding := isOn(attrs["cc"]), isOn(attrs["loop"]), isOn(attrs["modestbranding"])
	controls := !isOff(attrs["controls"])
	start := 0
	if truthy(attrs["start"]) {
		// parseInt of a whole number checkTiptapAttrs accepted.
		start, _ = strconv.Atoi(jsString(attrs["start"]))
	}
	showCaption := attrs["showCaption"] == true || attrs["showCaption"] == "true"
	caption := jsString(orDefault(attrs["caption"], ""))

	// URLSearchParams' order. The values are digits or a video ID, which
	// it leaves as they are.
	var query []string
	if cc {
		query = append(query, "cc_load_policy=1")
	}
	if loop {
		query = append(query, "loop=1", "playlist="+url.QueryEscape(videoID))
	}
	if !controls {
		query = append(query, "controls=0")
	}
	if modestBranding {
		query = append(query, "modestbranding=1")
	}
	if start > 0 {
		query = append(query, "start="+strconv.Itoa(start))
	}
	src := "https://www.youtube-nocookie.com/embed/" + videoID
	if len(query) > 0 {
		src += "?" + strings.Join(query, "&")
	}

	div := `data-youtube-video data-align="` + escapeHTML(align) + `" data-width="` + escapeHTML(width) + `"`
	if showCaption {
		div += ` data-show-caption="true"`
	}
	if caption != "" {
		div += ` data-caption="` + escapeHTML(caption) + `"`
	}
	if cc {
		div += ` data-cc="true"`
	}
	if loop {
		div += ` data-loop="true"`
	}
	if !controls {
		div += ` data-controls="false"`
	}
	if modestBranding {
		div += ` data-modestbranding="true"`
	}
	if start > 0 {
		div += ` data-start="` + strconv.Itoa(start) + `"`
	}
	b.WriteString("<div " + div + `><iframe src="` + escapeHTML(src) + `" width="` + escapeHTML(width) + `" height="` + escapeHTML(height) +
		`" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe></div>`)
}
