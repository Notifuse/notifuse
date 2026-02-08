package domain

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// DataFeedHeader represents a custom HTTP header for data feed requests
type DataFeedHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Validate validates the data feed header
func (h *DataFeedHeader) Validate() error {
	if h.Name == "" {
		return fmt.Errorf("header name is required")
	}
	if h.Value == "" {
		return fmt.Errorf("header value is required")
	}
	return nil
}

// GlobalFeedSettings defines the configuration for fetching global data
// that will be available to all recipients in a broadcast
type GlobalFeedSettings struct {
	Enabled bool             `json:"enabled"`
	URL     string           `json:"url,omitempty"`
	Headers []DataFeedHeader `json:"headers"` // Always include headers (empty array, not null)
}

// Validate validates the global feed settings
func (g *GlobalFeedSettings) Validate() error {
	// If not enabled, skip validation
	if !g.Enabled {
		return nil
	}

	// URL is required when enabled
	if g.URL == "" {
		return fmt.Errorf("URL is required when global feed is enabled")
	}

	// Use SSRF-safe URL validation
	if err := ValidateFeedURL(g.URL); err != nil {
		return fmt.Errorf("global feed URL: %w", err)
	}

	// Validate headers
	for _, header := range g.Headers {
		if err := header.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// GetTimeout returns the hardcoded timeout of 5 seconds
func (g *GlobalFeedSettings) GetTimeout() int {
	return 5
}

// Value implements the driver.Valuer interface for database serialization
func (g GlobalFeedSettings) Value() (driver.Value, error) {
	return json.Marshal(g)
}

// MarshalJSON implements custom JSON marshaling to ensure Headers is never null
func (g GlobalFeedSettings) MarshalJSON() ([]byte, error) {
	type Alias GlobalFeedSettings
	// Ensure Headers is an empty array, not null
	if g.Headers == nil {
		g.Headers = []DataFeedHeader{}
	}
	return json.Marshal((*Alias)(&g))
}

// Scan implements the sql.Scanner interface for database deserialization
func (g *GlobalFeedSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(v)
	if err := json.Unmarshal(cloned, g); err != nil {
		return err
	}

	// Ensure Headers is never nil to prevent frontend crashes
	if g.Headers == nil {
		g.Headers = []DataFeedHeader{}
	}

	return nil
}

// GlobalFeedBroadcast represents broadcast information sent in the feed request
type GlobalFeedBroadcast struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GlobalFeedList represents list information sent in the feed request
type GlobalFeedList struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GlobalFeedWorkspace represents workspace information sent in the feed request
type GlobalFeedWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GlobalFeedRequestPayload is the JSON body sent to the global feed endpoint
type GlobalFeedRequestPayload struct {
	Broadcast GlobalFeedBroadcast `json:"broadcast"`
	List      GlobalFeedList      `json:"list"`
	Workspace GlobalFeedWorkspace `json:"workspace"`
}

// RecipientFeedSettings defines the configuration for fetching per-recipient data
// On failure: retries up to 2 times (3 total attempts) with 5 second delay
// If all retries fail: broadcast pauses immediately
type RecipientFeedSettings struct {
	Enabled bool             `json:"enabled"`
	URL     string           `json:"url,omitempty"`
	Headers []DataFeedHeader `json:"headers"` // Always include headers (empty array, not null)
}

// Validate validates the recipient feed settings
func (r *RecipientFeedSettings) Validate() error {
	// If not enabled, skip validation
	if !r.Enabled {
		return nil
	}

	// URL is required when enabled
	if r.URL == "" {
		return fmt.Errorf("URL is required when recipient feed is enabled")
	}

	// Use SSRF-safe URL validation
	if err := ValidateFeedURL(r.URL); err != nil {
		return fmt.Errorf("recipient feed URL: %w", err)
	}

	// RecipientFeed requires https only for security
	parsedURL, _ := url.Parse(r.URL)
	if parsedURL.Scheme != "https" {
		return fmt.Errorf("URL must use https scheme")
	}

	// Validate headers
	for _, header := range r.Headers {
		if err := header.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// GetTimeout returns the hardcoded timeout of 5 seconds
func (r *RecipientFeedSettings) GetTimeout() int {
	return 5
}

// GetMaxRetries returns the hardcoded max retries (2 retries = 3 total attempts)
func (r *RecipientFeedSettings) GetMaxRetries() int {
	return 2
}

// GetRetryDelay returns the hardcoded retry delay in milliseconds (5 seconds)
func (r *RecipientFeedSettings) GetRetryDelay() int {
	return 5000
}

// Value implements the driver.Valuer interface for database serialization
func (r RecipientFeedSettings) Value() (driver.Value, error) {
	return json.Marshal(r)
}

// MarshalJSON implements custom JSON marshaling to ensure Headers is never null
func (r RecipientFeedSettings) MarshalJSON() ([]byte, error) {
	type Alias RecipientFeedSettings
	// Ensure Headers is an empty array, not null
	if r.Headers == nil {
		r.Headers = []DataFeedHeader{}
	}
	return json.Marshal((*Alias)(&r))
}

// Scan implements the sql.Scanner interface for database deserialization
func (r *RecipientFeedSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(v)
	if err := json.Unmarshal(cloned, r); err != nil {
		return err
	}

	// Ensure Headers is never nil to prevent frontend crashes
	if r.Headers == nil {
		r.Headers = []DataFeedHeader{}
	}

	return nil
}

// RecipientFeedContact represents contact information sent in the recipient feed request
type RecipientFeedContact struct {
	Email        string `json:"email"`
	ExternalID   string `json:"external_id,omitempty"`
	Timezone     string `json:"timezone,omitempty"`
	Language     string `json:"language,omitempty"`
	FirstName    string `json:"first_name,omitempty"`
	LastName     string `json:"last_name,omitempty"`
	FullName     string `json:"full_name,omitempty"`
	Phone        string `json:"phone,omitempty"`
	AddressLine1 string `json:"address_line_1,omitempty"`
	AddressLine2 string `json:"address_line_2,omitempty"`
	Country      string `json:"country,omitempty"`
	Postcode     string `json:"postcode,omitempty"`
	State        string `json:"state,omitempty"`
	JobTitle     string `json:"job_title,omitempty"`

	// Custom string fields
	CustomString1 string `json:"custom_string_1,omitempty"`
	CustomString2 string `json:"custom_string_2,omitempty"`
	CustomString3 string `json:"custom_string_3,omitempty"`
	CustomString4 string `json:"custom_string_4,omitempty"`
	CustomString5 string `json:"custom_string_5,omitempty"`

	// Custom number fields (using pointer for omitempty with 0 values)
	CustomNumber1 *float64 `json:"custom_number_1,omitempty"`
	CustomNumber2 *float64 `json:"custom_number_2,omitempty"`
	CustomNumber3 *float64 `json:"custom_number_3,omitempty"`
	CustomNumber4 *float64 `json:"custom_number_4,omitempty"`
	CustomNumber5 *float64 `json:"custom_number_5,omitempty"`

	// Custom datetime fields (RFC3339 format strings for JSON)
	CustomDatetime1 *string `json:"custom_datetime_1,omitempty"`
	CustomDatetime2 *string `json:"custom_datetime_2,omitempty"`
	CustomDatetime3 *string `json:"custom_datetime_3,omitempty"`
	CustomDatetime4 *string `json:"custom_datetime_4,omitempty"`
	CustomDatetime5 *string `json:"custom_datetime_5,omitempty"`

	// Custom JSON fields
	CustomJSON1 interface{} `json:"custom_json_1,omitempty"`
	CustomJSON2 interface{} `json:"custom_json_2,omitempty"`
	CustomJSON3 interface{} `json:"custom_json_3,omitempty"`
	CustomJSON4 interface{} `json:"custom_json_4,omitempty"`
	CustomJSON5 interface{} `json:"custom_json_5,omitempty"`

	// Timestamps (RFC3339 format strings)
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// RecipientFeedList represents list information sent in the recipient feed request
type RecipientFeedList struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RecipientFeedBroadcast represents broadcast information sent in the recipient feed request
type RecipientFeedBroadcast struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RecipientFeedWorkspace represents workspace information sent in the recipient feed request
type RecipientFeedWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RecipientFeedRequestPayload is the JSON body sent to the recipient feed endpoint
type RecipientFeedRequestPayload struct {
	Contact   RecipientFeedContact   `json:"contact"`
	Broadcast RecipientFeedBroadcast `json:"broadcast"`
	List      RecipientFeedList      `json:"list"`
	Workspace RecipientFeedWorkspace `json:"workspace"`
}

// DataFeedSettings consolidates all feed configuration and runtime data
// into a single structure for database storage
type DataFeedSettings struct {
	// Global feed configuration
	GlobalFeed *GlobalFeedSettings `json:"global_feed,omitempty"`

	// Global feed response data (persisted after fetch)
	GlobalFeedData MapOfAny `json:"global_feed_data,omitempty"`

	// When global feed was last fetched
	GlobalFeedFetchedAt *time.Time `json:"global_feed_fetched_at,omitempty"`

	// Per-recipient feed configuration
	RecipientFeed *RecipientFeedSettings `json:"recipient_feed,omitempty"`
}

// Value implements the driver.Valuer interface for database serialization
func (d DataFeedSettings) Value() (driver.Value, error) {
	return json.Marshal(d)
}

// Scan implements the sql.Scanner interface for database deserialization
func (d *DataFeedSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	v, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(v)
	if err := json.Unmarshal(cloned, d); err != nil {
		return err
	}

	// Ensure Headers arrays are never nil to prevent frontend crashes
	if d.GlobalFeed != nil && d.GlobalFeed.Headers == nil {
		d.GlobalFeed.Headers = []DataFeedHeader{}
	}
	if d.RecipientFeed != nil && d.RecipientFeed.Headers == nil {
		d.RecipientFeed.Headers = []DataFeedHeader{}
	}

	return nil
}

// Validate validates the data feed settings
func (d *DataFeedSettings) Validate() error {
	if d.GlobalFeed != nil {
		if err := d.GlobalFeed.Validate(); err != nil {
			return fmt.Errorf("global feed: %w", err)
		}
	}

	if d.RecipientFeed != nil {
		if err := d.RecipientFeed.Validate(); err != nil {
			return fmt.Errorf("recipient feed: %w", err)
		}
	}

	return nil
}

// NullableStringValue returns the string value from a NullableString pointer,
// returning empty string if nil or IsNull is true
func NullableStringValue(ns *NullableString) string {
	if ns == nil || ns.IsNull {
		return ""
	}
	return ns.String
}

// BuildRecipientFeedContact builds a RecipientFeedContact from a Contact
func BuildRecipientFeedContact(c *Contact) RecipientFeedContact {
	rfc := RecipientFeedContact{
		Email:         c.Email,
		ExternalID:    NullableStringValue(c.ExternalID),
		Timezone:      NullableStringValue(c.Timezone),
		Language:      NullableStringValue(c.Language),
		FirstName:     NullableStringValue(c.FirstName),
		LastName:      NullableStringValue(c.LastName),
		FullName:      NullableStringValue(c.FullName),
		Phone:         NullableStringValue(c.Phone),
		AddressLine1:  NullableStringValue(c.AddressLine1),
		AddressLine2:  NullableStringValue(c.AddressLine2),
		Country:       NullableStringValue(c.Country),
		Postcode:      NullableStringValue(c.Postcode),
		State:         NullableStringValue(c.State),
		JobTitle:      NullableStringValue(c.JobTitle),
		CustomString1: NullableStringValue(c.CustomString1),
		CustomString2: NullableStringValue(c.CustomString2),
		CustomString3: NullableStringValue(c.CustomString3),
		CustomString4: NullableStringValue(c.CustomString4),
		CustomString5: NullableStringValue(c.CustomString5),
		CreatedAt:     c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     c.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	// Handle custom number fields
	if c.CustomNumber1 != nil && !c.CustomNumber1.IsNull {
		rfc.CustomNumber1 = &c.CustomNumber1.Float64
	}
	if c.CustomNumber2 != nil && !c.CustomNumber2.IsNull {
		rfc.CustomNumber2 = &c.CustomNumber2.Float64
	}
	if c.CustomNumber3 != nil && !c.CustomNumber3.IsNull {
		rfc.CustomNumber3 = &c.CustomNumber3.Float64
	}
	if c.CustomNumber4 != nil && !c.CustomNumber4.IsNull {
		rfc.CustomNumber4 = &c.CustomNumber4.Float64
	}
	if c.CustomNumber5 != nil && !c.CustomNumber5.IsNull {
		rfc.CustomNumber5 = &c.CustomNumber5.Float64
	}

	// Handle custom datetime fields
	if c.CustomDatetime1 != nil && !c.CustomDatetime1.IsNull {
		t := c.CustomDatetime1.Time.Format("2006-01-02T15:04:05Z07:00")
		rfc.CustomDatetime1 = &t
	}
	if c.CustomDatetime2 != nil && !c.CustomDatetime2.IsNull {
		t := c.CustomDatetime2.Time.Format("2006-01-02T15:04:05Z07:00")
		rfc.CustomDatetime2 = &t
	}
	if c.CustomDatetime3 != nil && !c.CustomDatetime3.IsNull {
		t := c.CustomDatetime3.Time.Format("2006-01-02T15:04:05Z07:00")
		rfc.CustomDatetime3 = &t
	}
	if c.CustomDatetime4 != nil && !c.CustomDatetime4.IsNull {
		t := c.CustomDatetime4.Time.Format("2006-01-02T15:04:05Z07:00")
		rfc.CustomDatetime4 = &t
	}
	if c.CustomDatetime5 != nil && !c.CustomDatetime5.IsNull {
		t := c.CustomDatetime5.Time.Format("2006-01-02T15:04:05Z07:00")
		rfc.CustomDatetime5 = &t
	}

	// Handle custom JSON fields
	if c.CustomJSON1 != nil && !c.CustomJSON1.IsNull {
		rfc.CustomJSON1 = c.CustomJSON1.Data
	}
	if c.CustomJSON2 != nil && !c.CustomJSON2.IsNull {
		rfc.CustomJSON2 = c.CustomJSON2.Data
	}
	if c.CustomJSON3 != nil && !c.CustomJSON3.IsNull {
		rfc.CustomJSON3 = c.CustomJSON3.Data
	}
	if c.CustomJSON4 != nil && !c.CustomJSON4.IsNull {
		rfc.CustomJSON4 = c.CustomJSON4.Data
	}
	if c.CustomJSON5 != nil && !c.CustomJSON5.IsNull {
		rfc.CustomJSON5 = c.CustomJSON5.Data
	}

	return rfc
}
