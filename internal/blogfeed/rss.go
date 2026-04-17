package blogfeed

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

const (
	nsAtom    = "http://www.w3.org/2005/Atom"
	nsContent = "http://purl.org/rss/1.0/modules/content/"
	nsDC      = "http://purl.org/dc/elements/1.1/"
	nsMedia   = "http://search.yahoo.com/mrss/"
)

type rssRoot struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	NsAtom  string     `xml:"xmlns:atom,attr"`
	NsCont  string     `xml:"xmlns:content,attr"`
	NsDC    string     `xml:"xmlns:dc,attr"`
	NsMedia string     `xml:"xmlns:media,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string        `xml:"title"`
	Link          string        `xml:"link"`
	Description   string        `xml:"description"`
	Language      string        `xml:"language"`
	LastBuildDate string        `xml:"lastBuildDate"`
	AtomSelfLink  rssAtomLink   `xml:"atom:link"`
	Image         *rssImage     `xml:"image,omitempty"`
	Items         []rssItem     `xml:"item"`
}

type rssAtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssImage struct {
	URL   string `xml:"url"`
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

type rssItem struct {
	Title          string          `xml:"title"`
	Link           string          `xml:"link"`
	GUID           rssGUID         `xml:"guid"`
	PubDate        string          `xml:"pubDate"`
	Description    string          `xml:"description"`
	ContentEncoded *rssCDATA       `xml:"content:encoded,omitempty"`
	Categories     []string        `xml:"category,omitempty"`
	Creators       []rssDCCreator  `xml:"dc:creator,omitempty"`
	MediaContent   *rssMediaContent `xml:"media:content,omitempty"`
	MediaThumb     *rssMediaThumb  `xml:"media:thumbnail,omitempty"`
}

type rssGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

type rssDCCreator struct {
	XMLName xml.Name `xml:"dc:creator"`
	Value   string   `xml:",chardata"`
}

type rssCDATA struct {
	XMLName xml.Name
	Value   string `xml:",cdata"`
}

type rssMediaContent struct {
	XMLName xml.Name `xml:"media:content"`
	URL     string   `xml:"url,attr"`
	Medium  string   `xml:"medium,attr"`
}

type rssMediaThumb struct {
	XMLName xml.Name `xml:"media:thumbnail"`
	URL     string   `xml:"url,attr"`
}

// RenderRSS serializes a BlogFeed as RSS 2.0 XML.
func RenderRSS(feed *domain.BlogFeed) ([]byte, error) {
	channel := rssChannel{
		Title:         feed.Meta.Title,
		Link:          feed.Meta.SiteURL,
		Description:   feed.Meta.Description,
		Language:      feed.Meta.Language,
		LastBuildDate: formatRFC822(feed.Meta.UpdatedAt),
		AtomSelfLink: rssAtomLink{
			Href: feed.Meta.SelfURL,
			Rel:  "self",
			Type: "application/rss+xml",
		},
	}

	if feed.Meta.LogoURL != "" || feed.Meta.IconURL != "" {
		imgURL := feed.Meta.LogoURL
		if imgURL == "" {
			imgURL = feed.Meta.IconURL
		}
		channel.Image = &rssImage{
			URL:   imgURL,
			Title: feed.Meta.Title,
			Link:  feed.Meta.SiteURL,
		}
	}

	for _, item := range feed.Items {
		ri := rssItem{
			Title:       item.Title,
			Link:        item.URL,
			GUID:        rssGUID{IsPermaLink: "false", Value: item.GUID},
			PubDate:     formatRFC822(item.PublishedAt),
			Description: item.Excerpt,
		}

		if item.ContentHTML != "" && item.ContentHTML != item.Excerpt {
			ri.ContentEncoded = &rssCDATA{
				XMLName: xml.Name{Local: "content:encoded"},
				Value:   item.ContentHTML,
			}
		}

		if item.CategoryName != "" {
			ri.Categories = []string{item.CategoryName}
		}

		for _, a := range item.Authors {
			ri.Creators = append(ri.Creators, rssDCCreator{Value: a.Name})
		}

		if item.FeaturedImageURL != "" {
			ri.MediaContent = &rssMediaContent{URL: item.FeaturedImageURL, Medium: "image"}
			ri.MediaThumb = &rssMediaThumb{URL: item.FeaturedImageURL}
		}

		channel.Items = append(channel.Items, ri)
	}

	root := rssRoot{
		Version: "2.0",
		NsAtom:  nsAtom,
		NsCont:  nsContent,
		NsDC:    nsDC,
		NsMedia: nsMedia,
		Channel: channel,
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("rss encode: %w", err)
	}
	return buf.Bytes(), nil
}

func formatRFC822(t time.Time) string {
	return t.UTC().Format(time.RFC1123Z)
}
