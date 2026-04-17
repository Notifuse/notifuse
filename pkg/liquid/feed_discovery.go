package liquid

import (
	"html"
	"strings"
)

// InjectFeedDiscoveryTags inserts RSS and JSON Feed autodiscovery <link>
// elements before the closing </head> in the rendered HTML. If </head> is
// not found, the HTML is returned unchanged.
//
// On category pages pass categorySlug to include the per-category feed links
// alongside the main feed. Pass empty string for non-category pages.
func InjectFeedDiscoveryTags(rendered, blogTitle, categorySlug string) string {
	idx := strings.Index(strings.ToLower(rendered), "</head>")
	if idx == -1 {
		return rendered
	}

	escapedTitle := html.EscapeString(blogTitle)

	var tags strings.Builder
	tags.WriteString(`<link rel="alternate" type="application/rss+xml" title="`)
	tags.WriteString(escapedTitle)
	tags.WriteString(` — RSS" href="/feed.xml">`)
	tags.WriteString("\n")
	tags.WriteString(`<link rel="alternate" type="application/feed+json" title="`)
	tags.WriteString(escapedTitle)
	tags.WriteString(` — JSON Feed" href="/feed.json">`)
	tags.WriteString("\n")

	if categorySlug != "" {
		tags.WriteString(`<link rel="alternate" type="application/rss+xml" title="`)
		tags.WriteString(escapedTitle)
		tags.WriteString(" — ")
		tags.WriteString(html.EscapeString(categorySlug))
		tags.WriteString(` RSS" href="/`)
		tags.WriteString(categorySlug)
		tags.WriteString(`/feed.xml">`)
		tags.WriteString("\n")
		tags.WriteString(`<link rel="alternate" type="application/feed+json" title="`)
		tags.WriteString(escapedTitle)
		tags.WriteString(" — ")
		tags.WriteString(html.EscapeString(categorySlug))
		tags.WriteString(` JSON Feed" href="/`)
		tags.WriteString(categorySlug)
		tags.WriteString(`/feed.json">`)
		tags.WriteString("\n")
	}

	return rendered[:idx] + tags.String() + rendered[idx:]
}
