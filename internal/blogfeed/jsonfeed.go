package blogfeed

import (
	"encoding/json"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
)

type jsonFeed struct {
	Version     string          `json:"version"`
	Title       string          `json:"title"`
	HomePageURL string          `json:"home_page_url,omitempty"`
	FeedURL     string          `json:"feed_url,omitempty"`
	Description string          `json:"description,omitempty"`
	Language    string          `json:"language,omitempty"`
	Icon        string          `json:"icon,omitempty"`
	Favicon     string          `json:"favicon,omitempty"`
	Items       []jsonFeedItem  `json:"items"`
}

type jsonFeedItem struct {
	ID            string           `json:"id"`
	URL           string           `json:"url,omitempty"`
	Title         string           `json:"title,omitempty"`
	ContentHTML   string           `json:"content_html,omitempty"`
	Summary       string           `json:"summary,omitempty"`
	DatePublished string           `json:"date_published,omitempty"`
	DateModified  string           `json:"date_modified,omitempty"`
	Authors       []jsonFeedAuthor `json:"authors,omitempty"`
	Image         string           `json:"image,omitempty"`
	Tags          []string         `json:"tags,omitempty"`
}

type jsonFeedAuthor struct {
	Name string `json:"name"`
}

// RenderJSON serializes a BlogFeed as JSON Feed 1.1.
func RenderJSON(feed *domain.BlogFeed) ([]byte, error) {
	jf := jsonFeed{
		Version:     "https://jsonfeed.org/version/1.1",
		Title:       feed.Meta.Title,
		HomePageURL: feed.Meta.SiteURL,
		FeedURL:     feed.Meta.FeedURL,
		Description: feed.Meta.Description,
		Language:    feed.Meta.Language,
		Icon:        feed.Meta.LogoURL,
		Favicon:     feed.Meta.IconURL,
		Items:       make([]jsonFeedItem, 0, len(feed.Items)),
	}

	for _, item := range feed.Items {
		ji := jsonFeedItem{
			ID:            item.GUID,
			URL:           item.URL,
			Title:         item.Title,
			ContentHTML:   item.ContentHTML,
			Summary:       item.Excerpt,
			DatePublished: item.PublishedAt.UTC().Format("2006-01-02T15:04:05Z"),
			DateModified:  item.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			Image:         item.FeaturedImageURL,
		}

		for _, a := range item.Authors {
			ji.Authors = append(ji.Authors, jsonFeedAuthor{Name: a.Name})
		}

		if item.CategoryName != "" {
			ji.Tags = []string{item.CategoryName}
		}

		jf.Items = append(jf.Items, ji)
	}

	out, err := json.MarshalIndent(jf, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("jsonfeed encode: %w", err)
	}
	return out, nil
}
