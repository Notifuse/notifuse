package domain

import "time"

// BlogFeedItem is one post as represented in an RSS or JSON feed.
//
// GUID must be the immutable post ID so slug/URL changes don't duplicate the
// item in aggregators.
type BlogFeedItem struct {
	GUID             string
	Title            string
	URL              string
	CategorySlug     string
	CategoryName     string
	ContentHTML      string
	Excerpt          string
	Authors          []BlogAuthor
	FeaturedImageURL string
	PublishedAt      time.Time
	UpdatedAt        time.Time
}

// BlogFeedMeta is the channel-level (RSS) / top-level (JSON Feed) metadata.
type BlogFeedMeta struct {
	Title       string
	Description string
	SiteURL     string
	FeedURL     string
	SelfURL     string
	Language    string
	IconURL     string
	LogoURL     string
	UpdatedAt   time.Time
	ETag        string
}

// BlogFeed is the materialized feed payload passed to the renderer.
type BlogFeed struct {
	Meta  BlogFeedMeta
	Items []BlogFeedItem
}
