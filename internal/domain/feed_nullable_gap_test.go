package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGapTestRecipientFeedRequestValidate pins every guard of the test-feed
// endpoint in order: required IDs first, then the URL rules (https + host),
// then per-header validation propagated unwrapped.
func TestGapTestRecipientFeedRequestValidate(t *testing.T) {
	cases := []struct {
		name string
		req  TestRecipientFeedRequest
		want string
	}{
		{"workspace first", TestRecipientFeedRequest{}, "workspace_id is required"},
		{"broadcast second", TestRecipientFeedRequest{WorkspaceID: "ws"}, "broadcast_id is required"},
		{"url third", TestRecipientFeedRequest{WorkspaceID: "ws", BroadcastID: "b"}, "url is required"},
		{"unparseable url", TestRecipientFeedRequest{WorkspaceID: "ws", BroadcastID: "b", URL: "%zz"}, "url: invalid URL: parse \"%zz\": invalid URL escape \"%zz\""},
		{"http is not https", TestRecipientFeedRequest{WorkspaceID: "ws", BroadcastID: "b", URL: "http://feed.example.com/x"}, "url must use https scheme"},
		{"host required", TestRecipientFeedRequest{WorkspaceID: "ws", BroadcastID: "b", URL: "https:///path"}, "url: URL must have a host"},
		{"header name", TestRecipientFeedRequest{WorkspaceID: "ws", BroadcastID: "b", URL: "https://feed.example.com", Headers: []DataFeedHeader{{Value: "v"}}}, "header name is required"},
		{"header value", TestRecipientFeedRequest{WorkspaceID: "ws", BroadcastID: "b", URL: "https://feed.example.com", Headers: []DataFeedHeader{{Name: "X-A"}}}, "header value is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.EqualError(t, tc.req.Validate(), tc.want)
		})
	}

	valid := TestRecipientFeedRequest{WorkspaceID: "ws", BroadcastID: "b", URL: "https://feed.example.com/daily?x=1",
		Headers: []DataFeedHeader{{Name: "Authorization", Value: "Bearer k"}}}
	assert.NoError(t, valid.Validate())
}

// TestGapBuildRecipientFeedContactNulls: NULL custom fields must not leak into
// the feed payload as zero values — numbers/datetimes/JSON become nil pointers
// (omitted), while null strings collapse to "".
func TestGapBuildRecipientFeedContactNulls(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	c := &Contact{
		Email:           "a@example.com",
		FirstName:       &NullableString{String: "Ann", IsNull: false},
		LastName:        &NullableString{IsNull: true},
		CustomNumber1:   &NullableFloat64{Float64: 42.5},
		CustomNumber2:   &NullableFloat64{Float64: 9, IsNull: true},
		CustomDatetime1: &NullableTime{Time: now},
		CustomDatetime2: &NullableTime{Time: now, IsNull: true},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	got := BuildRecipientFeedContact(c)
	assert.Equal(t, "a@example.com", got.Email)
	assert.Equal(t, "Ann", got.FirstName)
	assert.Equal(t, "", got.LastName, "NULL string must collapse to empty, not a stale value")
	require.NotNil(t, got.CustomNumber1)
	assert.Equal(t, 42.5, *got.CustomNumber1)
	assert.Nil(t, got.CustomNumber2, "NULL number must be omitted, not sent as 0 or 9")
	require.NotNil(t, got.CustomDatetime1)
	assert.Equal(t, "2026-01-02T03:04:05Z", *got.CustomDatetime1)
	assert.Nil(t, got.CustomDatetime2)
	assert.Equal(t, "2026-01-02T03:04:05Z", got.CreatedAt)
	assert.Nil(t, got.CustomJSON1, "unset JSON fields stay nil")
}

// TestGapNullableTimeScan pins the scanner contract: []byte timestamps from
// drivers, invalid bytes error, unsupported Go types name the type, and Value
// round-trips NULL.
func TestGapNullableTimeScan(t *testing.T) {
	var nt NullableTime
	require.NoError(t, nt.Scan(nil))
	assert.True(t, nt.IsNull)
	assert.True(t, nt.Time.IsZero())

	// time.Time is the driver-native shape and scans cleanly.
	stamp := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	require.NoError(t, nt.Scan(stamp))
	assert.False(t, nt.IsNull)
	assert.Equal(t, stamp, nt.Time)

	// []byte input defers to sql.NullTime, which cannot scan byte slices:
	// the driver error propagates unwrapped rather than being swallowed.
	err2 := nt.Scan([]byte("2026-03-04 05:06:07"))
	require.EqualError(t, err2, "unsupported Scan, storing driver.Value type []uint8 into type *time.Time")
	err := nt.Scan(12345)
	require.EqualError(t, err, "cannot scan int into NullableTime")

	v, err := NullableTime{IsNull: true}.Value()
	require.NoError(t, err)
	assert.Nil(t, v)
	v, err = NullableTime{Time: nt.Time}.Value()
	require.NoError(t, err)
	assert.Equal(t, nt.Time, v)
}
