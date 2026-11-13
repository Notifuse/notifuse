package domain

import (
	"encoding/json"
	"math"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A field sent with the wrong JSON type is refused, naming the field, rather than
// stored as empty.
func TestFromJSON_RefusesAFieldOfTheWrongType(t *testing.T) {
	stringFields := []string{
		"timezone", "language", "first_name", "last_name", "full_name", "phone",
		"address_line_1", "address_line_2", "country", "postcode", "state", "job_title",
		"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
		"tracking_opens_consent", "tracking_clicks_consent", "tracking_consent_source",
	}
	numberFields := []string{"custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5"}
	timeFields := []string{
		"custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
		"tracking_consent_updated_at",
	}

	check := func(t *testing.T, field string, value interface{}, wantErr string) {
		t.Helper()
		payload, err := json.Marshal(map[string]interface{}{"email": "a@example.com", field: value})
		require.NoError(t, err)

		contact, err := FromJSON(payload)

		require.Error(t, err)
		assert.Nil(t, contact)
		assert.Contains(t, err.Error(), field)
		assert.Contains(t, err.Error(), wantErr)
	}
	for _, field := range stringFields {
		t.Run(field, func(t *testing.T) { check(t, field, 42, "expected string") })
	}
	for _, field := range numberFields {
		t.Run(field, func(t *testing.T) { check(t, field, "forty-two", "expected number") })
	}
	for _, field := range timeFields {
		t.Run(field, func(t *testing.T) { check(t, field, true, "expected string") })
	}
}

func TestFromJSON_RefusesAnUnknownClicksConsent(t *testing.T) {
	_, err := FromJSON([]byte(`{"email": "a@example.com", "tracking_clicks_consent": "maybe"}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tracking_clicks_consent")
}

func TestContact_Validate_RefusesAnUnknownOpensConsent(t *testing.T) {
	contact := &Contact{Email: "a@example.com", TrackingOpensConsent: &NullableString{String: "maybe"}}

	err := contact.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tracking_opens_consent")
}

func TestUpsertContactRequest_Validate_RequiresTheContact(t *testing.T) {
	_, _, err := (&UpsertContactRequest{WorkspaceID: "ws-1"}).Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "contact field is required")
}

func TestGetContactsRequest_SegmentsAndConsentFilters(t *testing.T) {
	t.Run("query: segments are read", func(t *testing.T) {
		var req GetContactsRequest
		params := url.Values{"workspace_id": {"ws-1"}, "segments[]": {"seg-a", "seg-b"}}

		require.NoError(t, req.FromQueryParams(params))
		assert.Equal(t, []string{"seg-a", "seg-b"}, req.Segments)
	})

	t.Run("query: an unknown clicks consent is refused", func(t *testing.T) {
		var req GetContactsRequest
		err := req.FromQueryParams(url.Values{"workspace_id": {"ws-1"}, "tracking_clicks_consent": {"maybe"}})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid tracking_clicks_consent")
	})

	for _, field := range []string{"tracking_opens_consent", "tracking_clicks_consent"} {
		t.Run("body: an unknown "+field+" is refused", func(t *testing.T) {
			req := GetContactsRequest{WorkspaceID: "ws-1"}
			if field == "tracking_opens_consent" {
				req.TrackingOpensConsent = "maybe"
			} else {
				req.TrackingClicksConsent = "maybe"
			}

			err := req.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid "+field)
		})
	}
}

// Merge copies each optional field the other contact sets.
func TestContact_Merge_CopiesOptionalFields(t *testing.T) {
	str := func(s string) *NullableString { return &NullableString{String: s} }
	updatedAt := &NullableTime{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	other := &Contact{
		ExternalID:               str("ext-1"),
		Timezone:                 str("Europe/Paris"),
		Language:                 str("fr"),
		LastName:                 str("Doe"),
		FullName:                 str("Jane Doe"),
		Phone:                    str("+33100000000"),
		AddressLine2:             str("Floor 2"),
		Postcode:                 str("75001"),
		State:                    str("IDF"),
		JobTitle:                 str("CTO"),
		TrackingConsentSource:    str("api"),
		TrackingConsentUpdatedAt: updatedAt,
	}
	contact := &Contact{Email: "a@example.com"}

	contact.Merge(other)

	assert.Equal(t, "a@example.com", contact.Email, "an empty email on the other contact keeps this one")
	assert.Equal(t, other.ExternalID, contact.ExternalID)
	assert.Equal(t, other.Timezone, contact.Timezone)
	assert.Equal(t, other.Language, contact.Language)
	assert.Equal(t, other.LastName, contact.LastName)
	assert.Equal(t, other.FullName, contact.FullName)
	assert.Equal(t, other.Phone, contact.Phone)
	assert.Equal(t, other.AddressLine2, contact.AddressLine2)
	assert.Equal(t, other.Postcode, contact.Postcode)
	assert.Equal(t, other.State, contact.State)
	assert.Equal(t, other.JobTitle, contact.JobTitle)
	assert.Equal(t, other.TrackingConsentSource, contact.TrackingConsentSource)
	assert.Equal(t, updatedAt, contact.TrackingConsentUpdatedAt)
}

func TestContact_ToMapOfAny_RefusesNaN(t *testing.T) {
	contact := &Contact{Email: "a@example.com", CustomNumber1: &NullableFloat64{Float64: math.NaN()}}

	_, err := contact.ToMapOfAny()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal contact")
}
