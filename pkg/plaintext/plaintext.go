// Package plaintext turns an email's HTML into the text/plain alternative sent with it.
//
// The output is meant to be read by a person in a text-only mail client, so it keeps
// what a reader needs (paragraphs, lists, link destinations) and drops what only a
// browser uses (styles, scripts, hidden preheaders, tracking pixels).
package plaintext

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// separator is what an <hr> becomes.
const separator = "------------------------------"

// skipped elements never reach the text: they hold markup, code or form controls,
// not something a reader of the email is meant to read.
var skipped = map[atom.Atom]bool{
	atom.Head: true, atom.Title: true, atom.Style: true, atom.Script: true,
	atom.Noscript: true, atom.Iframe: true, atom.Template: true, atom.Svg: true,
	atom.Math: true, atom.Object: true, atom.Input: true, atom.Select: true,
	atom.Textarea: true,
}

// blocks start and end a line.
var blocks = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Table: true, atom.Tbody: true, atom.Thead: true,
	atom.Tfoot: true, atom.Tr: true, atom.Td: true, atom.Th: true, atom.H1: true,
	atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Ul: true, atom.Ol: true, atom.Li: true, atom.Blockquote: true,
	atom.Section: true, atom.Article: true, atom.Header: true, atom.Footer: true,
	atom.Center: true, atom.Pre: true, atom.Address: true, atom.Dl: true, atom.Dt: true,
	atom.Dd: true, atom.Figure: true, atom.Figcaption: true, atom.Main: true, atom.Nav: true,
}

// paragraphs are followed by a blank line rather than a single line break.
var paragraphs = map[atom.Atom]bool{
	atom.P: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true,
	atom.H5: true, atom.H6: true, atom.Table: true, atom.Tr: true, atom.Blockquote: true,
	atom.Pre: true,
}

// invisible removes the characters email builders use to pad the hidden preheader
// and that would show as noise, or nothing at all, in a text client.
var invisible = strings.NewReplacer(
	"\u200b", "", "\u200c", "", "\u200d", "", "\u034f", "", "\u00ad", "", "\ufeff", "",
	"\u00a0", " ",
)

// FromHTML returns the plain-text version of an HTML email, or "" when the HTML holds
// nothing to read. It never fails: the parser recovers from malformed markup, and the
// one input it refuses (elements nested deeper than it allows) yields "", which sends the
// email as HTML only.
func FromHTML(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(strings.ToValidUTF8(src, "�")))
	if err != nil {
		return ""
	}
	w := &writer{}
	w.walkChildren(doc)
	return w.finish()
}

type list struct {
	ordered bool
	count   int
}

type writer struct {
	b            strings.Builder
	line         strings.Builder // the line being written, without its prefix
	pendingSpace bool            // collapsed whitespace waiting for the next word
	afterURL     bool            // a URL was just written: the next word must not touch it
	lists        []list
	quoteDepth   int
	preDepth     int
	itemDepth    int    // open list items: a paragraph inside one ends a line, not a block
	marker       string // the list marker waiting for the first word of its item
	continuation string // what starts the other lines of the current item, under its text
}

// write appends s to the current line, inserting the space that collapsed whitespace,
// or a URL just written, calls for.
func (w *writer) write(s string) {
	if s == "" {
		return
	}
	if w.line.Len() == 0 {
		if w.marker != "" {
			w.line.WriteString(w.marker)
			w.marker = ""
		} else {
			w.line.WriteString(w.continuation)
		}
	}
	punctuation := len(s) - len(strings.TrimLeft(s, trailingPunctuation))
	attached := w.afterURL && punctuation > 0 && (punctuation == len(s) || isSpace(s[punctuation]))
	separate := w.pendingSpace || (w.afterURL && !attached)
	if w.line.Len() > 0 && separate && !strings.HasSuffix(w.line.String(), " ") && !strings.HasPrefix(s, " ") {
		w.line.WriteByte(' ')
	}
	w.pendingSpace = false
	// Punctuation alone after a URL leaves the URL needing a space before the next word.
	w.afterURL = attached && punctuation == len(s)
	w.line.WriteString(s)
}

// trailingPunctuation may follow a URL without a space when a space or the end of the
// text comes next: autolinkers leave it out of the link, and a space before it would
// read oddly.
const trailingPunctuation = ".,;:!?)]\"'”’»"

// writeURL appends a URL and remembers that the next word must be separated from it,
// so that a client's autolinker never reads two links, or a link and a word, as one.
func (w *writer) writeURL(s string) {
	w.write(s)
	w.afterURL = true
}

// newline ends the current line, if it has anything on it.
func (w *writer) newline() {
	if w.line.Len() > 0 {
		w.flushLine()
	}
	w.pendingSpace = false
	w.afterURL = false
}

// blankLine ends the current line and leaves an empty line after it.
func (w *writer) blankLine() {
	w.newline()
	w.b.WriteByte('\n')
}

func (w *writer) flushLine() {
	if w.quoteDepth > 0 {
		w.b.WriteString(strings.Repeat("> ", w.quoteDepth))
	}
	w.b.WriteString(w.line.String())
	w.b.WriteByte('\n')
	w.line.Reset()
}

// finish trims every line, keeps at most one blank line in a row and drops leading and
// trailing blank lines.
func (w *writer) finish() string {
	w.newline()
	var out []string
	blank := false
	for _, l := range strings.Split(w.b.String(), "\n") {
		l = strings.TrimRight(l, " \t")
		if strings.TrimSpace(l) == "" {
			if len(out) > 0 {
				blank = true
			}
			continue
		}
		if blank {
			out = append(out, "")
			blank = false
		}
		out = append(out, l)
	}
	out = dropRepeatedURLs(out)
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// dropRepeatedURLs removes a line that is nothing but a URL when the next line links to
// the same URL with a label, as social blocks do: an icon link, then the same link as text.
func dropRepeatedURLs(lines []string) []string {
	var kept []string
	for i, l := range lines {
		if isWebURL(l) && !strings.Contains(l, " ") {
			next := i + 1
			if next < len(lines) && lines[next] == "" {
				next++
			}
			if next < len(lines) && strings.HasSuffix(lines[next], "("+l+")") {
				continue
			}
		}
		if l == "" && (len(kept) == 0 || kept[len(kept)-1] == "") {
			continue
		}
		kept = append(kept, l)
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	return kept
}

func (w *writer) walkChildren(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.walk(c)
	}
}

func (w *writer) walk(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		w.text(n.Data)
		return
	case html.ElementNode:
		w.element(n)
		return
	case html.CommentNode, html.DoctypeNode:
		return
	}
	w.walkChildren(n)
}

func (w *writer) text(data string) {
	data = invisible.Replace(data)
	if w.preDepth > 0 {
		lines := strings.Split(data, "\n")
		for i, l := range lines {
			if i > 0 {
				w.flushLine()
			}
			w.line.WriteString(l)
		}
		return
	}
	if strings.TrimSpace(data) == "" {
		if data != "" {
			w.pendingSpace = true
		}
		return
	}
	if isSpace(data[0]) {
		w.pendingSpace = true
	}
	w.write(strings.Join(strings.Fields(data), " "))
	if isSpace(data[len(data)-1]) {
		w.pendingSpace = true
	}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\t' || c == '\r' || c == '\f'
}

func (w *writer) element(n *html.Node) {
	if skipped[n.DataAtom] || isHidden(n) {
		return
	}
	switch n.DataAtom {
	case atom.Br:
		if w.line.Len() == 0 {
			// A second line break in a row leaves an empty line.
			w.b.WriteByte('\n')
		}
		w.newline()
		return
	case atom.Hr:
		w.blankLine()
		w.write(separator)
		w.blankLine()
		return
	case atom.Img:
		if alt := strings.TrimSpace(attr(n, "alt")); alt != "" {
			w.write(collapse(alt))
		}
		return
	case atom.A:
		w.link(n)
		return
	case atom.Ul, atom.Ol:
		w.newline()
		l := list{ordered: n.DataAtom == atom.Ol}
		if start, err := strconv.Atoi(strings.TrimSpace(attr(n, "start"))); err == nil && l.ordered {
			l.count = start - 1
		}
		w.lists = append(w.lists, l)
		w.walkChildren(n)
		w.lists = w.lists[:len(w.lists)-1]
		w.newline()
		if len(w.lists) == 0 {
			w.blankLine()
		}
		return
	case atom.Li:
		w.newline()
		indent, marker := "", "- "
		if depth := len(w.lists); depth > 0 {
			indent = strings.Repeat("  ", depth-1)
			l := &w.lists[depth-1]
			l.count++
			if l.ordered {
				marker = strconv.Itoa(l.count) + ". "
			}
		}
		// The marker waits for the item's first word, so a paragraph or a div inside the
		// item does not leave it alone on its line.
		outer := w.continuation
		w.marker = indent + marker
		w.continuation = strings.Repeat(" ", len(w.marker))
		w.itemDepth++
		w.walkChildren(n)
		w.newline()
		w.itemDepth--
		w.marker = ""
		w.continuation = outer
		return
	case atom.Blockquote:
		w.blankLine()
		w.quoteDepth++
		w.walkChildren(n)
		w.newline()
		w.quoteDepth--
		w.blankLine()
		return
	case atom.Pre:
		w.blankLine()
		w.preDepth++
		w.walkChildren(n)
		w.preDepth--
		w.blankLine()
		return
	}

	if blocks[n.DataAtom] {
		w.newline()
	}
	w.walkChildren(n)
	if paragraphs[n.DataAtom] && w.itemDepth == 0 {
		w.blankLine()
	} else if blocks[n.DataAtom] {
		w.newline()
	}
}

// link writes an anchor as "label (url)", or just the URL when the label adds nothing.
// Only web, mail and phone links carry a destination a reader can use; anything else
// keeps its label alone.
func (w *writer) link(n *html.Node) {
	href := strings.TrimSpace(attr(n, "href"))
	label := linkLabel(n)
	lower := strings.ToLower(href)

	switch {
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		if label == "" || label == href || isWebURL(label) {
			w.writeURL(href)
			return
		}
		w.write(label)
		w.pendingSpace = true
		w.writeURL("(" + href + ")")
	case strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "tel:"):
		target := href[strings.Index(href, ":")+1:]
		if i := strings.IndexByte(target, '?'); i >= 0 {
			target = target[:i]
		}
		if label == "" || strings.EqualFold(label, target) {
			w.writeURL(target)
			return
		}
		w.write(label)
		w.pendingSpace = true
		w.writeURL("(" + target + ")")
	default:
		w.write(label)
	}
}

// linkLabel is the text a reader sees for a link: its text, or the alt text of the
// image it wraps.
func linkLabel(n *html.Node) string {
	sub := &writer{}
	sub.walkChildren(n)
	return collapse(sub.finish())
}

func isWebURL(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// isHidden reports whether the element is hidden for every reader. mso-hide:all is
// deliberately not one of them: it hides in Outlook only, and bulletproof buttons put
// it on the very link other clients show.
func isHidden(n *html.Node) bool {
	for _, a := range n.Attr {
		switch a.Key {
		case "hidden":
			return true
		case "style":
			style := strings.ToLower(strings.Join(strings.Fields(a.Val), ""))
			if strings.Contains(style, "display:none") || strings.Contains(style, "visibility:hidden") {
				return true
			}
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
