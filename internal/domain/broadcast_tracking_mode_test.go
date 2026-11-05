package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroadcast_Validate_TrackingMode(t *testing.T) {
	for _, mode := range []string{"", "inherit", domain.BroadcastTrackingModeNoTracking} {
		b := createValidBroadcast()
		b.TrackingMode = mode
		assert.NoError(t, b.Validate(), mode)
	}

	inherit := createValidBroadcast()
	inherit.TrackingMode = "inherit"
	require.NoError(t, inherit.Validate())
	assert.Equal(t, "", inherit.TrackingMode, "inherit is stored as its meaning")

	for _, mode := range []string{"disabled", "never"} {
		b := createValidBroadcast()
		b.TrackingMode = mode
		assert.Error(t, b.Validate(), mode)
	}
}

func TestBroadcast_Validate_NoTrackingRefusesAutoWinner(t *testing.T) {
	b := createValidBroadcastWithTest()
	b.TrackingMode = domain.BroadcastTrackingModeNoTracking
	assert.Error(t, b.Validate())

	manual := createValidBroadcastWithTest()
	manual.TestSettings.AutoSendWinner = false
	manual.TrackingMode = domain.BroadcastTrackingModeNoTracking
	assert.NoError(t, manual.Validate())
}

func TestCreateBroadcastRequest_TrackingMode(t *testing.T) {
	var req domain.CreateBroadcastRequest
	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"workspace123","name":"n","audience":{"list":"list123"},"tracking_mode":"no_tracking"}`), &req))
	b, err := req.Validate()
	require.NoError(t, err)
	assert.Equal(t, domain.BroadcastTrackingModeNoTracking, b.TrackingMode)
}

func TestUpdateBroadcastRequest_TrackingMode(t *testing.T) {
	decode := func(t *testing.T, body string) *domain.UpdateBroadcastRequest {
		t.Helper()
		var req domain.UpdateBroadcastRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		return &req
	}
	const base = `"workspace_id":"workspace123","id":"broadcast123","name":"n"`

	t.Run("absent keeps the stored mode", func(t *testing.T) {
		stored := createValidBroadcast()
		stored.TrackingMode = domain.BroadcastTrackingModeNoTracking
		b, err := decode(t, `{`+base+`}`).Validate(&stored)
		require.NoError(t, err)
		assert.Equal(t, domain.BroadcastTrackingModeNoTracking, b.TrackingMode)
	})

	t.Run("sent on a draft applies", func(t *testing.T) {
		stored := createValidBroadcast()
		b, err := decode(t, `{`+base+`,"tracking_mode":"no_tracking"}`).Validate(&stored)
		require.NoError(t, err)
		assert.Equal(t, domain.BroadcastTrackingModeNoTracking, b.TrackingMode)
	})

	t.Run("inherit clears it", func(t *testing.T) {
		stored := createValidBroadcast()
		stored.TrackingMode = domain.BroadcastTrackingModeNoTracking
		b, err := decode(t, `{`+base+`,"tracking_mode":"inherit"}`).Validate(&stored)
		require.NoError(t, err)
		assert.Equal(t, "", b.TrackingMode)
	})

	t.Run("a change on a paused broadcast is refused", func(t *testing.T) {
		stored := createValidBroadcast()
		stored.Status = domain.BroadcastStatusPaused
		_, err := decode(t, `{`+base+`,"tracking_mode":"no_tracking"}`).Validate(&stored)
		assert.Error(t, err)
	})

	t.Run("resending the same mode on a paused broadcast is accepted", func(t *testing.T) {
		stored := createValidBroadcast()
		stored.Status = domain.BroadcastStatusPaused
		stored.TrackingMode = domain.BroadcastTrackingModeNoTracking
		_, err := decode(t, `{`+base+`,"tracking_mode":"no_tracking"}`).Validate(&stored)
		assert.NoError(t, err)
	})

	t.Run("disabled is refused", func(t *testing.T) {
		stored := createValidBroadcast()
		_, err := decode(t, `{`+base+`,"tracking_mode":"disabled"}`).Validate(&stored)
		assert.Error(t, err)
	})
}
