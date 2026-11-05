package domain

import (
	"fmt"
	"testing"

	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func consentPtr(value string) *NullableString {
	if value == "" {
		return nil
	}
	return &NullableString{String: value}
}

func TestResolveEmailTracking_TruthTable(t *testing.T) {
	consents := []string{"", TrackingConsentGranted, TrackingConsentDenied}
	unknowns := []string{UnknownConsentTrack, UnknownConsentNoTrack}
	modes := []string{"", notifuse_mjml.TrackingModeInherit, notifuse_mjml.TrackingModeNoTracking, notifuse_mjml.TrackingModeDisabled}

	allows := func(consent, unknown string) bool {
		switch consent {
		case TrackingConsentGranted:
			return true
		case TrackingConsentDenied:
			return false
		}
		return unknown == UnknownConsentTrack
	}

	for _, opensEnabled := range []bool{true, false} {
		for _, clicksEnabled := range []bool{true, false} {
			for _, unknown := range unknowns {
				for _, mode := range modes {
					for _, co := range consents {
						for _, cc := range consents {
							name := fmt.Sprintf("opens=%v/clicks=%v/unknown=%s/mode=%q/co=%q/cc=%q", opensEnabled, clicksEnabled, unknown, mode, co, cc)
							t.Run(name, func(t *testing.T) {
								policy := EmailTrackingPolicy{OpensEnabled: opensEnabled, ClicksEnabled: clicksEnabled, UnknownConsent: unknown}
								contact := &Contact{Email: "a@example.com", TrackingOpensConsent: consentPtr(co), TrackingClicksConsent: consentPtr(cc)}

								got := ResolveEmailTracking(policy, contact, mode)

								inherit := mode == "" || mode == notifuse_mjml.TrackingModeInherit
								want := EmailTrackingDecision{
									Opens:    inherit && opensEnabled && allows(co, unknown),
									Clicks:   inherit && clicksEnabled && allows(cc, unknown),
									Identify: inherit && allows(cc, unknown),
									UTM:      mode != notifuse_mjml.TrackingModeDisabled,
								}
								assert.Equal(t, want, got)
							})
						}
					}
				}
			}
		}
	}
}

func TestResolveEmailTracking_NilContactIsUnknown(t *testing.T) {
	track := ResolveEmailTracking(EmailTrackingPolicy{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: UnknownConsentTrack}, nil, "")
	assert.Equal(t, EmailTrackingDecision{Opens: true, Clicks: true, Identify: true, UTM: true}, track)

	noTrack := ResolveEmailTracking(EmailTrackingPolicy{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: UnknownConsentNoTrack}, nil, "")
	assert.Equal(t, EmailTrackingDecision{UTM: true}, noTrack)
}

func TestResolveEmailTracking_EmptyUnknownConsentTracks(t *testing.T) {
	got := ResolveEmailTracking(EmailTrackingPolicy{OpensEnabled: true, ClicksEnabled: true}, &Contact{Email: "a@example.com"}, "")
	assert.True(t, got.Opens)
	assert.True(t, got.Clicks)
}

func TestResolveEmailTracking_ExplicitNullIsUnknown(t *testing.T) {
	contact := &Contact{
		Email:                 "a@example.com",
		TrackingOpensConsent:  &NullableString{IsNull: true},
		TrackingClicksConsent: &NullableString{IsNull: true},
	}
	got := ResolveEmailTracking(EmailTrackingPolicy{OpensEnabled: true, ClicksEnabled: true, UnknownConsent: UnknownConsentNoTrack}, contact, "")
	assert.False(t, got.Opens)
	assert.False(t, got.Clicks)
}

func TestEmailTrackingDecision_Apply(t *testing.T) {
	ts := notifuse_mjml.TrackingSettings{TrackingMode: notifuse_mjml.TrackingModeInherit}
	EmailTrackingDecision{Opens: true, Clicks: false, Identify: false, UTM: true}.Apply(&ts)

	assert.True(t, ts.EnableTracking)
	assert.False(t, ts.SuppressOpens)
	assert.True(t, ts.SuppressClicks)
	assert.True(t, ts.SuppressIdentify)
	assert.Equal(t, notifuse_mjml.TrackingModeInherit, ts.TrackingMode)

	none := notifuse_mjml.TrackingSettings{}
	EmailTrackingDecision{Identify: true, UTM: true}.Apply(&none)
	assert.False(t, none.EnableTracking)
	assert.False(t, none.SuppressIdentify)
}

func TestTrackedFlagsFromSettings(t *testing.T) {
	opens, clicks := TrackedFlagsFromSettings(notifuse_mjml.TrackingSettings{EnableTracking: true, SuppressClicks: true})
	require.NotNil(t, opens)
	require.NotNil(t, clicks)
	assert.True(t, *opens)
	assert.False(t, *clicks)

	opens, clicks = TrackedFlagsFromSettings(notifuse_mjml.TrackingSettings{EnableTracking: true, TrackingMode: notifuse_mjml.TrackingModeDisabled})
	assert.False(t, *opens)
	assert.False(t, *clicks)

	opens, clicks = TrackedFlagsFromSettings(notifuse_mjml.TrackingSettings{})
	assert.False(t, *opens)
	assert.False(t, *clicks)
}

func TestEmailTrackingDecision_TrackedFlags(t *testing.T) {
	opens, clicks := EmailTrackingDecision{Opens: true}.TrackedFlags()
	assert.True(t, *opens)
	assert.False(t, *clicks)
}

func TestValidateTrackingConsentValue(t *testing.T) {
	assert.NoError(t, ValidateTrackingConsentValue(""))
	assert.NoError(t, ValidateTrackingConsentValue(TrackingConsentGranted))
	assert.NoError(t, ValidateTrackingConsentValue(TrackingConsentDenied))
	assert.Error(t, ValidateTrackingConsentValue("unknown"))
	assert.Error(t, ValidateTrackingConsentValue("GRANTED"))
}
