package blogfeed

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderJSON_ValidJSON(t *testing.T) {
	out, err := RenderJSON(sampleFeed())
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &parsed), "output is not valid JSON")
}

func TestRenderJSON_TopLevelFields(t *testing.T) {
	out, err := RenderJSON(sampleFeed())
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &parsed))

	assert.Equal(t, "https://jsonfeed.org/version/1.1", parsed["version"])
	assert.Equal(t, "Example Blog", parsed["title"])
	assert.Equal(t, "https://blog.example.com", parsed["home_page_url"])
	assert.Equal(t, "https://blog.example.com/feed.xml", parsed["feed_url"])
	assert.Equal(t, "en", parsed["language"])
	assert.Equal(t, "https://blog.example.com/logo.png", parsed["icon"])
	assert.Equal(t, "https://blog.example.com/icon.png", parsed["favicon"])
}

func TestRenderJSON_ItemFields(t *testing.T) {
	out, err := RenderJSON(sampleFeed())
	require.NoError(t, err)

	var parsed struct {
		Items []struct {
			ID            string `json:"id"`
			URL           string `json:"url"`
			Title         string `json:"title"`
			ContentHTML   string `json:"content_html"`
			Summary       string `json:"summary"`
			DatePublished string `json:"date_published"`
			DateModified  string `json:"date_modified"`
			Image         string `json:"image"`
			Tags          []string `json:"tags"`
			Authors       []struct {
				Name string `json:"name"`
			} `json:"authors"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Len(t, parsed.Items, 1)

	it := parsed.Items[0]
	assert.Equal(t, "post-001", it.ID)
	assert.Equal(t, "https://blog.example.com/tech/hello-world", it.URL)
	assert.Equal(t, "Hello World", it.Title)
	assert.Equal(t, "<p>Full body HTML</p>", it.ContentHTML)
	assert.Equal(t, "A warm greeting.", it.Summary)
	assert.Equal(t, "2026-04-14T10:00:00Z", it.DatePublished)
	assert.Equal(t, "2026-04-15T11:00:00Z", it.DateModified)
	assert.Equal(t, "https://blog.example.com/img/hero.jpg", it.Image)
	assert.Equal(t, []string{"Tech"}, it.Tags)
	require.Len(t, it.Authors, 2)
	assert.Equal(t, "Alice", it.Authors[0].Name)
	assert.Equal(t, "Bob", it.Authors[1].Name)
}

func TestRenderJSON_EmptyFeed(t *testing.T) {
	f := sampleFeed()
	f.Items = nil
	out, err := RenderJSON(f)
	require.NoError(t, err)

	var parsed struct {
		Items []interface{} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Empty(t, parsed.Items)
}

func TestRenderJSON_EmptyOptionalFields(t *testing.T) {
	f := sampleFeed()
	f.Items[0].Authors = nil
	f.Items[0].Excerpt = ""
	f.Items[0].FeaturedImageURL = ""
	f.Items[0].ContentHTML = ""

	out, err := RenderJSON(f)
	require.NoError(t, err)

	var parsed struct {
		Items []struct {
			Authors     []interface{} `json:"authors"`
			Summary     string        `json:"summary"`
			Image       string        `json:"image"`
			ContentHTML string        `json:"content_html"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Len(t, parsed.Items, 1)

	it := parsed.Items[0]
	assert.Nil(t, it.Authors, "nil Authors should omit the field, not emit null")
	assert.Empty(t, it.Summary)
	assert.Empty(t, it.Image)
	assert.Empty(t, it.ContentHTML)
}

func TestRenderJSON_UnicodeTitle(t *testing.T) {
	f := sampleFeed()
	f.Items[0].Title = "Héllo 👋 مرحبا"
	out, err := RenderJSON(f)
	require.NoError(t, err)

	var parsed struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Equal(t, "Héllo 👋 مرحبا", parsed.Items[0].Title)
}
