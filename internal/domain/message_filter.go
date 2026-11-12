package domain

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/asaskevich/govalidator"
)

// Values of the origin filter of messages.list and messages.count: what sent the message.
const (
	MessageOriginBroadcast     = "broadcast"
	MessageOriginAutomation    = "automation"
	MessageOriginTransactional = "transactional"
	// MessageOriginOther is a message sent by none of the three: a template test or a
	// double opt-in confirmation.
	MessageOriginOther = "other"
)

// MessageOrigins are the accepted values of the origin filter.
var MessageOrigins = []string{MessageOriginBroadcast, MessageOriginAutomation, MessageOriginTransactional, MessageOriginOther}

// MessageFilter holds the filters of messages.list, shared with messages.count: every field
// of MessageListParams but Cursor and Limit, with the same names, types and JSON tags.
type MessageFilter struct {
	ID             string `json:"id,omitempty"`
	ExternalID     string `json:"external_id,omitempty"`
	ListID         string `json:"list_id,omitempty"`
	Channel        string `json:"channel,omitempty"`
	ContactEmail   string `json:"contact_email,omitempty"`
	BroadcastID    string `json:"broadcast_id,omitempty"`
	TemplateID     string `json:"template_id,omitempty"`
	ClickedURL     string `json:"clicked_url,omitempty"`
	IsSent         *bool  `json:"is_sent,omitempty"`
	IsDelivered    *bool  `json:"is_delivered,omitempty"`
	IsFailed       *bool  `json:"is_failed,omitempty"`
	IsOpened       *bool  `json:"is_opened,omitempty"`
	IsClicked      *bool  `json:"is_clicked,omitempty"`
	IsBounced      *bool  `json:"is_bounced,omitempty"`
	IsComplained   *bool  `json:"is_complained,omitempty"`
	IsUnsubscribed *bool  `json:"is_unsubscribed,omitempty"`

	TransactionalNotificationID string `json:"transactional_notification_id,omitempty"`
	Origin                      string `json:"origin,omitempty"`

	SentAfter     *time.Time `json:"sent_after,omitempty"`
	SentBefore    *time.Time `json:"sent_before,omitempty"`
	UpdatedAfter  *time.Time `json:"updated_after,omitempty"`
	UpdatedBefore *time.Time `json:"updated_before,omitempty"`
	CreatedAfter  *time.Time `json:"created_after,omitempty"`
	CreatedBefore *time.Time `json:"created_before,omitempty"`
}

// FromQuery reads the filters from HTTP query parameters. It only parses: Validate checks
// the values.
func (f *MessageFilter) FromQuery(query url.Values) error {
	f.ID = query.Get("id")
	f.ExternalID = query.Get("external_id")
	f.ListID = query.Get("list_id")
	f.Channel = query.Get("channel")
	f.ContactEmail = query.Get("contact_email")
	f.BroadcastID = query.Get("broadcast_id")
	f.TransactionalNotificationID = query.Get("transactional_notification_id")
	f.TemplateID = query.Get("template_id")
	f.ClickedURL = query.Get("clicked_url")
	f.Origin = query.Get("origin")

	bools := []struct {
		name   string
		target **bool
	}{
		{"is_sent", &f.IsSent},
		{"is_delivered", &f.IsDelivered},
		{"is_failed", &f.IsFailed},
		{"is_opened", &f.IsOpened},
		{"is_clicked", &f.IsClicked},
		{"is_bounced", &f.IsBounced},
		{"is_complained", &f.IsComplained},
		{"is_unsubscribed", &f.IsUnsubscribed},
	}
	for _, b := range bools {
		if raw := query.Get(b.name); raw != "" {
			var value bool
			if err := json.Unmarshal([]byte(raw), &value); err != nil {
				return fmt.Errorf("invalid %s value: %s", b.name, raw)
			}
			*b.target = &value
		}
	}

	times := []struct {
		name   string
		target **time.Time
	}{
		{"sent_after", &f.SentAfter},
		{"sent_before", &f.SentBefore},
		{"updated_after", &f.UpdatedAfter},
		{"updated_before", &f.UpdatedBefore},
		{"created_after", &f.CreatedAfter},
		{"created_before", &f.CreatedBefore},
	}
	for _, t := range times {
		if err := parseTimeParam(query, t.name, t.target); err != nil {
			return err
		}
	}
	return nil
}

// Validate checks the filter values. It never changes them.
func (f MessageFilter) Validate() error {
	if f.Channel != "" && !govalidator.IsIn(f.Channel, "email", "sms", "push") {
		return fmt.Errorf("invalid channel type: %s", f.Channel)
	}

	if f.ContactEmail != "" && !govalidator.IsEmail(f.ContactEmail) {
		return fmt.Errorf("invalid contact email format")
	}

	// clicked_url is matched inside clicked_links, which no index covers, so it only
	// narrows a broadcast's messages; PostgreSQL plans it like the other filters of
	// this list.
	if f.ClickedURL != "" {
		if f.BroadcastID == "" {
			return fmt.Errorf("clicked_url requires broadcast_id")
		}
		if len(f.ClickedURL) > MaxClickedURLLength {
			return fmt.Errorf("clicked_url must be at most %d characters", MaxClickedURLLength)
		}
	}

	if f.Origin != "" && !govalidator.IsIn(f.Origin, MessageOrigins...) {
		return fmt.Errorf("origin must be broadcast, automation, transactional or other")
	}

	// Note: BroadcastID and TemplateID are not validated as UUIDs
	// They can be any non-empty string format

	ranges := []struct {
		after, before *time.Time
		message       string
	}{
		{f.SentAfter, f.SentBefore, "sent_after must be before sent_before"},
		{f.UpdatedAfter, f.UpdatedBefore, "updated_after must be before updated_before"},
		{f.CreatedAfter, f.CreatedBefore, "created_after must be before created_before"},
	}
	for _, r := range ranges {
		if r.after != nil && r.before != nil && r.after.After(*r.before) {
			return fmt.Errorf("%s", r.message)
		}
	}
	return nil
}

// RequireBound refuses a filter that would make messages.count read the whole table: it
// needs created_after, or a filter that an index narrows. list_id and external_id are not
// indexed for this.
func (f MessageFilter) RequireBound() error {
	if f.CreatedAfter != nil {
		return nil
	}
	for _, indexed := range []string{f.ID, f.ContactEmail, f.BroadcastID, f.TransactionalNotificationID, f.TemplateID} {
		if indexed != "" {
			return nil
		}
	}
	return fmt.Errorf("messages.count needs created_after or an indexed filter")
}

// ParseMessageCountQuery reads the filters of messages.count: cursor and limit are ignored,
// the values are validated and the filter must be bounded.
func ParseMessageCountQuery(query url.Values) (MessageFilter, error) {
	var f MessageFilter
	if err := f.FromQuery(query); err != nil {
		return MessageFilter{}, err
	}
	if err := f.Validate(); err != nil {
		return MessageFilter{}, err
	}
	if err := f.RequireBound(); err != nil {
		return MessageFilter{}, err
	}
	return f, nil
}

// Filter returns the filter fields of the list parameters.
func (p MessageListParams) Filter() MessageFilter {
	return MessageFilter{
		ID:                          p.ID,
		ExternalID:                  p.ExternalID,
		ListID:                      p.ListID,
		Channel:                     p.Channel,
		ContactEmail:                p.ContactEmail,
		BroadcastID:                 p.BroadcastID,
		TemplateID:                  p.TemplateID,
		ClickedURL:                  p.ClickedURL,
		IsSent:                      p.IsSent,
		IsDelivered:                 p.IsDelivered,
		IsFailed:                    p.IsFailed,
		IsOpened:                    p.IsOpened,
		IsClicked:                   p.IsClicked,
		IsBounced:                   p.IsBounced,
		IsComplained:                p.IsComplained,
		IsUnsubscribed:              p.IsUnsubscribed,
		TransactionalNotificationID: p.TransactionalNotificationID,
		Origin:                      p.Origin,
		SentAfter:                   p.SentAfter,
		SentBefore:                  p.SentBefore,
		UpdatedAfter:                p.UpdatedAfter,
		UpdatedBefore:               p.UpdatedBefore,
		CreatedAfter:                p.CreatedAfter,
		CreatedBefore:               p.CreatedBefore,
	}
}

// setFilter copies the filter fields into the list parameters.
func (p *MessageListParams) setFilter(f MessageFilter) {
	p.ID = f.ID
	p.ExternalID = f.ExternalID
	p.ListID = f.ListID
	p.Channel = f.Channel
	p.ContactEmail = f.ContactEmail
	p.BroadcastID = f.BroadcastID
	p.TemplateID = f.TemplateID
	p.ClickedURL = f.ClickedURL
	p.IsSent = f.IsSent
	p.IsDelivered = f.IsDelivered
	p.IsFailed = f.IsFailed
	p.IsOpened = f.IsOpened
	p.IsClicked = f.IsClicked
	p.IsBounced = f.IsBounced
	p.IsComplained = f.IsComplained
	p.IsUnsubscribed = f.IsUnsubscribed
	p.TransactionalNotificationID = f.TransactionalNotificationID
	p.Origin = f.Origin
	p.SentAfter = f.SentAfter
	p.SentBefore = f.SentBefore
	p.UpdatedAfter = f.UpdatedAfter
	p.UpdatedBefore = f.UpdatedBefore
	p.CreatedAfter = f.CreatedAfter
	p.CreatedBefore = f.CreatedBefore
}

// MessageCounts are the counts of messages.count, each following the is_*=true filter of
// the same name.
type MessageCounts struct {
	Total      int64 `json:"total"`
	Delivered  int64 `json:"delivered"`
	Opened     int64 `json:"opened"`
	Clicked    int64 `json:"clicked"`
	Bounced    int64 `json:"bounced"`
	Failed     int64 `json:"failed"`
	Complained int64 `json:"complained"`
}
