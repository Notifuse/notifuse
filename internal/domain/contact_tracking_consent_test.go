package domain

import (
	"database/sql"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromJSON_TrackingConsentFields(t *testing.T) {
	contact, err := FromJSON(`{
		"email": "a@example.com",
		"tracking_opens_consent": "granted",
		"tracking_clicks_consent": "denied",
		"tracking_consent_source": "legacy_crm",
		"tracking_consent_updated_at": "2026-04-20T10:00:00Z"
	}`)
	require.NoError(t, err)

	require.NotNil(t, contact.TrackingOpensConsent)
	assert.Equal(t, TrackingConsentGranted, contact.TrackingOpensConsent.String)
	require.NotNil(t, contact.TrackingClicksConsent)
	assert.Equal(t, TrackingConsentDenied, contact.TrackingClicksConsent.String)
	require.NotNil(t, contact.TrackingConsentSource)
	assert.Equal(t, "legacy_crm", contact.TrackingConsentSource.String)
	require.NotNil(t, contact.TrackingConsentUpdatedAt)
	assert.True(t, contact.TrackingConsentUpdatedAt.Time.Equal(time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)))
}

func TestFromJSON_TrackingConsentExplicitNullAndAbsent(t *testing.T) {
	contact, err := FromJSON(`{"email": "a@example.com", "tracking_opens_consent": null}`)
	require.NoError(t, err)

	require.NotNil(t, contact.TrackingOpensConsent)
	assert.True(t, contact.TrackingOpensConsent.IsNull)
	assert.Nil(t, contact.TrackingClicksConsent)
	assert.Nil(t, contact.TrackingConsentSource)
	assert.Nil(t, contact.TrackingConsentUpdatedAt)
}

func TestFromJSON_TrackingConsentRejectsInvalidValue(t *testing.T) {
	_, err := FromJSON(`{"email": "a@example.com", "tracking_opens_consent": "yes"}`)
	assert.Error(t, err)

	_, err = FromJSON(`{"email": "a@example.com", "tracking_clicks_consent": true}`)
	assert.Error(t, err)
}

func TestContactValidate_TrackingConsent(t *testing.T) {
	valid := &Contact{Email: "a@example.com", TrackingOpensConsent: &NullableString{String: TrackingConsentGranted}, TrackingClicksConsent: &NullableString{IsNull: true}}
	assert.NoError(t, valid.Validate())

	badValue := &Contact{Email: "a@example.com", TrackingClicksConsent: &NullableString{String: "maybe"}}
	assert.Error(t, badValue.Validate())

	longSource := &Contact{Email: "a@example.com", TrackingConsentSource: &NullableString{String: strings.Repeat("s", MaxTrackingConsentSourceLength+1)}}
	assert.Error(t, longSource.Validate())
}

func TestContactMerge_TrackingConsentFields(t *testing.T) {
	when := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	existing := &Contact{
		Email:                    "a@example.com",
		TrackingOpensConsent:     &NullableString{String: TrackingConsentGranted},
		TrackingClicksConsent:    &NullableString{String: TrackingConsentGranted},
		TrackingConsentSource:    &NullableString{String: TrackingConsentSourceAPI},
		TrackingConsentUpdatedAt: &NullableTime{Time: when},
	}

	existing.Merge(&Contact{Email: "a@example.com", TrackingClicksConsent: &NullableString{String: TrackingConsentDenied}})

	assert.Equal(t, TrackingConsentGranted, existing.TrackingOpensConsent.String, "absent field is kept")
	assert.Equal(t, TrackingConsentDenied, existing.TrackingClicksConsent.String)
	assert.Equal(t, TrackingConsentSourceAPI, existing.TrackingConsentSource.String)
	assert.True(t, existing.TrackingConsentUpdatedAt.Time.Equal(when))

	existing.Merge(&Contact{Email: "a@example.com", TrackingOpensConsent: &NullableString{IsNull: true}})
	assert.True(t, existing.TrackingOpensConsent.IsNull)
}

func TestTrackingConsentChanged(t *testing.T) {
	granted := &NullableString{String: TrackingConsentGranted}
	denied := &NullableString{String: TrackingConsentDenied}
	explicitNull := &NullableString{IsNull: true}

	cases := []struct {
		name          string
		before, after *Contact
		want          bool
	}{
		{"both unknown, nil and nil", &Contact{}, &Contact{}, false},
		{"nil and explicit null are the same unknown", &Contact{}, &Contact{TrackingOpensConsent: explicitNull, TrackingClicksConsent: explicitNull}, false},
		{"same value", &Contact{TrackingOpensConsent: granted}, &Contact{TrackingOpensConsent: &NullableString{String: TrackingConsentGranted}}, false},
		{"opens changed", &Contact{TrackingOpensConsent: granted}, &Contact{TrackingOpensConsent: denied}, true},
		{"clicks set from unknown", &Contact{}, &Contact{TrackingClicksConsent: granted}, true},
		{"back to unknown", &Contact{TrackingClicksConsent: denied}, &Contact{TrackingClicksConsent: explicitNull}, true},
		{"nil before", nil, &Contact{TrackingOpensConsent: granted}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, TrackingConsentChanged(tc.before, tc.after))
		})
	}
}

func TestContact_HasTrackingConsentValue(t *testing.T) {
	assert.False(t, (&Contact{}).HasTrackingConsentValue())
	assert.False(t, (&Contact{TrackingOpensConsent: &NullableString{IsNull: true}}).HasTrackingConsentValue())
	assert.True(t, (&Contact{TrackingClicksConsent: &NullableString{String: TrackingConsentDenied}}).HasTrackingConsentValue())
}

func TestContact_ProvidesTrackingConsent(t *testing.T) {
	assert.False(t, (&Contact{}).ProvidesTrackingConsent())
	assert.True(t, (&Contact{TrackingOpensConsent: &NullableString{IsNull: true}}).ProvidesTrackingConsent())
	assert.True(t, (&Contact{TrackingClicksConsent: &NullableString{String: TrackingConsentDenied}}).ProvidesTrackingConsent())
}

func TestContact_ClearTrackingConsent(t *testing.T) {
	c := &Contact{
		TrackingOpensConsent:     &NullableString{String: TrackingConsentGranted},
		TrackingClicksConsent:    &NullableString{String: TrackingConsentGranted},
		TrackingConsentSource:    &NullableString{String: "x"},
		TrackingConsentUpdatedAt: &NullableTime{Time: time.Now()},
	}
	c.ClearTrackingConsent()
	assert.Nil(t, c.TrackingOpensConsent)
	assert.Nil(t, c.TrackingClicksConsent)
	assert.Nil(t, c.TrackingConsentSource)
	assert.Nil(t, c.TrackingConsentUpdatedAt)
}

// trackingScanRow scans the contactColumns-ordered values into the destinations
// ScanContact passes, so the test pins the column order end to end.
type trackingScanRow struct{ values []interface{} }

func (r trackingScanRow) Scan(dest ...interface{}) error {
	for i := range dest {
		switch d := dest[i].(type) {
		case *string:
			*d = r.values[i].(string)
		case *sql.NullString:
			*d = r.values[i].(sql.NullString)
		case *sql.NullFloat64:
			*d = sql.NullFloat64{}
		case *sql.NullTime:
			*d = r.values[i].(sql.NullTime)
		case *[]byte:
			*d = nil
		case *time.Time:
			*d = r.values[i].(time.Time)
		}
	}
	return nil
}

func TestScanContact_ReadsTrackingConsentColumns(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	values := []interface{}{"a@example.com"}
	for i := 0; i < 13; i++ { // external_id .. job_title
		values = append(values, sql.NullString{})
	}
	for i := 0; i < 5; i++ { // custom_string
		values = append(values, sql.NullString{})
	}
	for i := 0; i < 5; i++ { // custom_number
		values = append(values, nil)
	}
	for i := 0; i < 5; i++ { // custom_datetime
		values = append(values, sql.NullTime{})
	}
	for i := 0; i < 5; i++ { // custom_json
		values = append(values, nil)
	}
	values = append(values, now, now, now, now)
	values = append(values,
		sql.NullString{String: TrackingConsentGranted, Valid: true},
		sql.NullString{String: TrackingConsentDenied, Valid: true},
		sql.NullTime{Time: now, Valid: true},
		sql.NullString{String: TrackingConsentSourceImport, Valid: true},
	)

	contact, err := ScanContact(trackingScanRow{values: values})
	require.NoError(t, err)

	require.NotNil(t, contact.TrackingOpensConsent)
	assert.Equal(t, TrackingConsentGranted, contact.TrackingOpensConsent.String)
	require.NotNil(t, contact.TrackingClicksConsent)
	assert.Equal(t, TrackingConsentDenied, contact.TrackingClicksConsent.String)
	require.NotNil(t, contact.TrackingConsentUpdatedAt)
	assert.True(t, contact.TrackingConsentUpdatedAt.Time.Equal(now))
	require.NotNil(t, contact.TrackingConsentSource)
	assert.Equal(t, TrackingConsentSourceImport, contact.TrackingConsentSource.String)
}

func TestGetContactsRequest_TrackingConsentFilters(t *testing.T) {
	params := url.Values{}
	params.Set("workspace_id", "ws")
	params.Set("tracking_opens_consent", "unknown")
	params.Set("tracking_clicks_consent", "denied")

	var req GetContactsRequest
	require.NoError(t, req.FromQueryParams(params))
	assert.Equal(t, "unknown", req.TrackingOpensConsent)
	assert.Equal(t, "denied", req.TrackingClicksConsent)

	params.Set("tracking_opens_consent", "maybe")
	var bad GetContactsRequest
	assert.Error(t, bad.FromQueryParams(params))
}

func TestContact_DefaultTrackingConsentSource(t *testing.T) {
	c := &Contact{TrackingOpensConsent: &NullableString{String: TrackingConsentGranted}}
	c.DefaultTrackingConsentSource(TrackingConsentSourceAPI)
	require.NotNil(t, c.TrackingConsentSource)
	assert.Equal(t, TrackingConsentSourceAPI, c.TrackingConsentSource.String)

	own := &Contact{TrackingOpensConsent: &NullableString{IsNull: true}, TrackingConsentSource: &NullableString{String: "legacy_crm"}}
	own.DefaultTrackingConsentSource(TrackingConsentSourceAPI)
	assert.Equal(t, "legacy_crm", own.TrackingConsentSource.String)

	none := &Contact{}
	none.DefaultTrackingConsentSource(TrackingConsentSourceAPI)
	assert.Nil(t, none.TrackingConsentSource, "no consent, no source")
}

func TestContact_ImposeTrackingConsentSource(t *testing.T) {
	c := &Contact{
		TrackingClicksConsent:    &NullableString{String: TrackingConsentDenied},
		TrackingConsentSource:    &NullableString{String: TrackingConsentSourceConsole},
		TrackingConsentUpdatedAt: &NullableTime{Time: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	c.ImposeTrackingConsentSource(TrackingConsentSourceSubscribeForm)
	require.NotNil(t, c.TrackingConsentSource)
	assert.Equal(t, TrackingConsentSourceSubscribeForm, c.TrackingConsentSource.String)
	assert.Nil(t, c.TrackingConsentUpdatedAt)
	assert.Equal(t, TrackingConsentDenied, c.TrackingClicksConsent.String)

	none := &Contact{TrackingConsentSource: &NullableString{String: "x"}, TrackingConsentUpdatedAt: &NullableTime{Time: time.Now()}}
	none.ImposeTrackingConsentSource(TrackingConsentSourceSubscribeForm)
	assert.Nil(t, none.TrackingConsentSource)
	assert.Nil(t, none.TrackingConsentUpdatedAt)
}

func TestTrackingConsentSourceForUser(t *testing.T) {
	assert.Equal(t, TrackingConsentSourceAPI, TrackingConsentSourceForUser(&User{Type: UserTypeAPIKey}))
	assert.Equal(t, TrackingConsentSourceConsole, TrackingConsentSourceForUser(&User{Type: UserTypeUser}))
	assert.Equal(t, TrackingConsentSourceConsole, TrackingConsentSourceForUser(nil))
}
