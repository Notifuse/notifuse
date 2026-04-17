package blogfeed

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleFeed() *domain.BlogFeed {
	return &domain.BlogFeed{
		Meta: domain.BlogFeedMeta{
			Title:       "Example Blog",
			Description: "Tech articles",
			SiteURL:     "https://blog.example.com",
			FeedURL:     "https://blog.example.com/feed.xml",
			SelfURL:     "https://blog.example.com/feed.xml",
			Language:    "en",
			LogoURL:     "https://blog.example.com/logo.png",
			IconURL:     "https://blog.example.com/icon.png",
			UpdatedAt:   time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
			ETag:        `W/"abcdef1234567890"`,
		},
		Items: []domain.BlogFeedItem{
			{
				GUID:             "post-001",
				Title:            "Hello World",
				URL:              "https://blog.example.com/tech/hello-world",
				CategorySlug:     "tech",
				CategoryName:     "Tech",
				ContentHTML:      "<p>Full body HTML</p>",
				Excerpt:          "A warm greeting.",
				Authors:          []domain.BlogAuthor{{Name: "Alice"}, {Name: "Bob"}},
				FeaturedImageURL: "https://blog.example.com/img/hero.jpg",
				PublishedAt:      time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
				UpdatedAt:        time.Date(2026, 4, 15, 11, 0, 0, 0, time.UTC),
			},
		},
	}
}

func TestRenderRSS_WellFormedXML(t *testing.T) {
	out, err := RenderRSS(sampleFeed())
	require.NoError(t, err)
	// Must be parseable by encoding/xml without error.
	var parsed struct{}
	require.NoError(t, xml.Unmarshal(out, &parsed), "output is not well-formed XML")
}

func TestRenderRSS_NamespacesPresent(t *testing.T) {
	out, err := RenderRSS(sampleFeed())
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `xmlns:atom="http://www.w3.org/2005/Atom"`)
	assert.Contains(t, s, `xmlns:content="http://purl.org/rss/1.0/modules/content/"`)
	assert.Contains(t, s, `xmlns:dc="http://purl.org/dc/elements/1.1/"`)
	assert.Contains(t, s, `xmlns:media="http://search.yahoo.com/mrss/"`)
}

func TestRenderRSS_ChannelElements(t *testing.T) {
	out, err := RenderRSS(sampleFeed())
	require.NoError(t, err)
	s := string(out)

	assert.Contains(t, s, "<title>Example Blog</title>")
	assert.Contains(t, s, "<link>https://blog.example.com</link>")
	assert.Contains(t, s, "<language>en</language>")
	assert.Contains(t, s, "<lastBuildDate>")
	assert.Contains(t, s, `rel="self"`)
	assert.Contains(t, s, `type="application/rss+xml"`)
	// Channel image
	assert.Contains(t, s, "<url>https://blog.example.com/logo.png</url>")
}

func TestRenderRSS_ItemElements(t *testing.T) {
	out, err := RenderRSS(sampleFeed())
	require.NoError(t, err)
	s := string(out)

	assert.Contains(t, s, "<title>Hello World</title>")
	assert.Contains(t, s, "<link>https://blog.example.com/tech/hello-world</link>")
	assert.Contains(t, s, `isPermaLink="false"`)
	assert.Contains(t, s, "post-001")
	assert.Contains(t, s, "<description>A warm greeting.</description>")
	// content:encoded CDATA
	assert.Contains(t, s, "<![CDATA[<p>Full body HTML</p>]]>")
	// category
	assert.Contains(t, s, "<category>Tech</category>")
	// dc:creator
	assert.Contains(t, s, "Alice")
	assert.Contains(t, s, "Bob")
	// media
	assert.Contains(t, s, `url="https://blog.example.com/img/hero.jpg"`)
	assert.Contains(t, s, `medium="image"`)
}

func TestRenderRSS_DateFormatRFC822(t *testing.T) {
	out, err := RenderRSS(sampleFeed())
	require.NoError(t, err)
	s := string(out)
	// RFC 1123Z: "Mon, 02 Jan 2006 15:04:05 -0700"
	assert.Contains(t, s, "Tue, 14 Apr 2026 10:00:00 +0000")
}

func TestRenderRSS_SpecialCharsInTitle(t *testing.T) {
	f := sampleFeed()
	f.Items[0].Title = `Test <"title"> & more 👋`
	out, err := RenderRSS(f)
	require.NoError(t, err)
	// encoding/xml escapes by default.
	var parsed struct{}
	require.NoError(t, xml.Unmarshal(out, &parsed))
	s := string(out)
	assert.Contains(t, s, "&amp;")
	assert.NotContains(t, s, `<"title">`) // must be escaped
}

func TestRenderRSS_EmptyFeed(t *testing.T) {
	f := &domain.BlogFeed{
		Meta: domain.BlogFeedMeta{
			Title:    "Empty",
			SiteURL:  "https://example.com",
			SelfURL:  "https://example.com/feed.xml",
			Language: "en",
		},
	}
	out, err := RenderRSS(f)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "<item>")
	var parsed struct{}
	require.NoError(t, xml.Unmarshal(out, &parsed))
}

func TestRenderRSS_EmptyOptionalFields(t *testing.T) {
	f := sampleFeed()
	f.Items[0].Authors = nil
	f.Items[0].Excerpt = ""
	f.Items[0].FeaturedImageURL = ""
	f.Items[0].ContentHTML = ""

	out, err := RenderRSS(f)
	require.NoError(t, err)
	s := string(out)

	// Well-formed XML
	var parsed struct{}
	require.NoError(t, xml.Unmarshal(out, &parsed))

	// No empty dc:creator, media:content, or media:thumbnail tags
	assert.NotContains(t, s, "<dc:creator></dc:creator>")
	assert.NotContains(t, s, `<media:content url=""`)
	assert.NotContains(t, s, `<media:thumbnail url=""`)
	// content:encoded should be omitted when empty
	assert.NotContains(t, s, "content:encoded")
}

func TestRenderRSS_AbsoluteURLs(t *testing.T) {
	out, err := RenderRSS(sampleFeed())
	require.NoError(t, err)
	s := string(out)
	for _, attr := range []string{"href=", "url=", "<link>"} {
		idx := strings.Index(s, attr)
		if idx == -1 {
			continue
		}
		after := s[idx:]
		end := 80
		if end > len(after) {
			end = len(after)
		}
		assert.NotContains(t, after[:end], `="/`, "found relative URL near %q", attr)
	}
}
