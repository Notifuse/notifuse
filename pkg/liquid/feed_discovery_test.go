package liquid

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInjectFeedDiscoveryTags_HomePage(t *testing.T) {
	in := `<html><head><title>Blog</title></head><body></body></html>`
	out := InjectFeedDiscoveryTags(in, "My Blog", "")

	assert.Contains(t, out, `type="application/rss+xml"`)
	assert.Contains(t, out, `href="/feed.xml"`)
	assert.Contains(t, out, `type="application/feed+json"`)
	assert.Contains(t, out, `href="/feed.json"`)
	// No category-specific links on a home page.
	assert.NotContains(t, out, "/tech/feed.xml")
}

func TestInjectFeedDiscoveryTags_CategoryPage(t *testing.T) {
	in := `<html><head><title>Blog</title></head><body></body></html>`
	out := InjectFeedDiscoveryTags(in, "My Blog", "tech")

	// Main feed links.
	assert.Contains(t, out, `href="/feed.xml"`)
	assert.Contains(t, out, `href="/feed.json"`)
	// Category-specific links.
	assert.Contains(t, out, `href="/tech/feed.xml"`)
	assert.Contains(t, out, `href="/tech/feed.json"`)
}

func TestInjectFeedDiscoveryTags_NoHead(t *testing.T) {
	in := `<div>no head tag here</div>`
	out := InjectFeedDiscoveryTags(in, "Blog", "")
	assert.Equal(t, in, out)
}

func TestInjectFeedDiscoveryTags_HTMLEscapesTitle(t *testing.T) {
	in := `<html><head></head><body></body></html>`
	out := InjectFeedDiscoveryTags(in, `Blog "with" <special> & chars`, "")
	assert.Contains(t, out, `Blog &#34;with&#34; &lt;special&gt; &amp; chars`)
}
