package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeUpdateSettings(t *testing.T, settingsJSON string) WorkspaceSettings {
	t.Helper()
	var req UpdateWorkspaceRequest
	require.NoError(t, json.Unmarshal([]byte(`{"id":"ws","name":"n","settings":`+settingsJSON+`}`), &req))
	return req.Settings
}

func TestWorkspaceSettings_TrackingPolicy(t *testing.T) {
	legacyOn := WorkspaceSettings{EmailTrackingEnabled: true}
	assert.Equal(t, EmailTrackingPolicy{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: UnknownConsentTrack}, legacyOn.TrackingPolicy())

	legacyOff := WorkspaceSettings{}
	assert.Equal(t, EmailTrackingPolicy{UnknownConsent: UnknownConsentTrack}, legacyOff.TrackingPolicy())

	split := WorkspaceSettings{EmailTrackingEnabled: true, EmailTracking: &EmailTrackingSettings{OpensEnabled: false, ClicksEnabled: true, UnknownConsent: UnknownConsentNoTrack}}
	assert.Equal(t, EmailTrackingPolicy{OpensEnabled: false, ClicksEnabled: true, UnknownConsent: UnknownConsentNoTrack}, split.TrackingPolicy())
}

func TestDefaultEmailTrackingSettings(t *testing.T) {
	assert.Equal(t, &EmailTrackingSettings{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: UnknownConsentTrack}, DefaultEmailTrackingSettings())
}

func TestEmailTrackingSettings_Validate(t *testing.T) {
	assert.NoError(t, (&EmailTrackingSettings{UnknownConsent: UnknownConsentTrack}).Validate())
	assert.NoError(t, (&EmailTrackingSettings{UnknownConsent: UnknownConsentNoTrack}).Validate())
	assert.Error(t, (&EmailTrackingSettings{}).Validate())
	assert.Error(t, (&EmailTrackingSettings{UnknownConsent: "ask"}).Validate())
}

func TestUpdateWorkspaceRequest_EmailTrackingNullRefused(t *testing.T) {
	settings := decodeUpdateSettings(t, `{"timezone":"UTC","default_language":"en","languages":["en"],"email_tracking":null}`)
	err := settings.Validate("passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "email_tracking")
}

func TestUpdateWorkspaceRequest_EmailTrackingInvalidUnknownRefused(t *testing.T) {
	settings := decodeUpdateSettings(t, `{"timezone":"UTC","default_language":"en","languages":["en"],"email_tracking":{"opens_enabled":true,"clicks_enabled":true,"unknown_consent":"maybe"}}`)
	assert.Error(t, settings.Validate("passphrase"))
}

func TestPreserveOmitted_EmailTrackingReconciliation(t *testing.T) {
	storedSplit := WorkspaceSettings{
		EmailTrackingEnabled: true,
		EmailTracking:        &EmailTrackingSettings{OpensEnabled: false, ClicksEnabled: true, UnknownConsent: UnknownConsentNoTrack},
	}
	storedLegacy := WorkspaceSettings{EmailTrackingEnabled: true}

	cases := []struct {
		name        string
		stored      WorkspaceSettings
		body        string
		wantEnabled bool
		want        *EmailTrackingSettings
	}{
		{
			name:        "new body decides and enabled follows",
			stored:      storedSplit,
			body:        `{"email_tracking_enabled":true,"email_tracking":{"opens_enabled":false,"clicks_enabled":false,"unknown_consent":"track"}}`,
			wantEnabled: false,
			want:        &EmailTrackingSettings{OpensEnabled: false, ClicksEnabled: false, UnknownConsent: UnknownConsentTrack},
		},
		{
			name:        "old client resends the stored object and flips the single switch",
			stored:      storedSplit,
			body:        `{"email_tracking_enabled":false,"email_tracking":{"opens_enabled":false,"clicks_enabled":true,"unknown_consent":"no_track"}}`,
			wantEnabled: false,
			want:        &EmailTrackingSettings{OpensEnabled: false, ClicksEnabled: false, UnknownConsent: UnknownConsentNoTrack},
		},
		{
			name:        "old client sends only the single switch",
			stored:      storedSplit,
			body:        `{"email_tracking_enabled":false}`,
			wantEnabled: false,
			want:        &EmailTrackingSettings{OpensEnabled: false, ClicksEnabled: false, UnknownConsent: UnknownConsentNoTrack},
		},
		{
			name:        "single switch on a workspace that never saved the split",
			stored:      WorkspaceSettings{},
			body:        `{"email_tracking_enabled":true}`,
			wantEnabled: true,
			want:        &EmailTrackingSettings{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: UnknownConsentTrack},
		},
		{
			name:        "single switch unchanged keeps the split",
			stored:      storedSplit,
			body:        `{"email_tracking_enabled":true}`,
			wantEnabled: true,
			want:        storedSplit.EmailTracking,
		},
		{
			name:        "neither key keeps both",
			stored:      storedSplit,
			body:        `{"website_url":"https://example.com"}`,
			wantEnabled: true,
			want:        storedSplit.EmailTracking,
		},
		{
			name:        "neither key on a legacy workspace stays legacy",
			stored:      storedLegacy,
			body:        `{}`,
			wantEnabled: true,
			want:        nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := decodeUpdateSettings(t, tc.body)
			settings.PreserveOmitted(tc.stored)

			assert.Equal(t, tc.wantEnabled, settings.EmailTrackingEnabled)
			assert.Equal(t, tc.want, settings.EmailTracking)
		})
	}
}

func TestPreserveOmitted_EmailTrackingNeverAliasesStored(t *testing.T) {
	stored := WorkspaceSettings{
		EmailTrackingEnabled: true,
		EmailTracking:        &EmailTrackingSettings{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: UnknownConsentTrack},
	}
	settings := decodeUpdateSettings(t, `{}`)
	settings.PreserveOmitted(stored)

	require.NotNil(t, settings.EmailTracking)
	assert.NotSame(t, stored.EmailTracking, settings.EmailTracking)
	settings.EmailTracking.OpensEnabled = false
	assert.True(t, stored.EmailTracking.OpensEnabled, "the stored value must not change")
}

func TestPreserveOmitted_EmailTrackingAppliedToGoAssembledSettings(t *testing.T) {
	// Settings built in Go record no omitted keys; a nil EmailTracking still
	// means "not said" rather than "clear it".
	stored := WorkspaceSettings{
		EmailTrackingEnabled: true,
		EmailTracking:        &EmailTrackingSettings{OpensEnabled: true, ClicksEnabled: false, UnknownConsent: UnknownConsentTrack},
	}
	settings := WorkspaceSettings{EmailTrackingEnabled: true}
	settings.PreserveOmitted(stored)
	assert.Equal(t, stored.EmailTracking, settings.EmailTracking)
}
