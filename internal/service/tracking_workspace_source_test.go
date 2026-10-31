package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
)

const (
	trackingTestSecretKey   = "current-secret-key-for-tracking-tests"
	trackingTestPreviousKey = "previous-secret-key-for-tracking-tests"
)

type fakeTrackingRow struct {
	encrypted string
	endpoint  string
	err       error
}

type fakeTrackingLoader struct {
	mu    sync.Mutex
	rows  map[string]fakeTrackingRow
	calls map[string]int
	// gate, when set, blocks every load until closed.
	gate chan struct{}
}

func newFakeTrackingLoader() *fakeTrackingLoader {
	return &fakeTrackingLoader{rows: map[string]fakeTrackingRow{}, calls: map[string]int{}}
}

func (f *fakeTrackingLoader) set(id string, row fakeTrackingRow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[id] = row
}

func (f *fakeTrackingLoader) count(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[id]
}

func (f *fakeTrackingLoader) load(ctx context.Context, id string) (string, string, error) {
	if f.gate != nil {
		<-f.gate
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[id]++
	row, ok := f.rows[id]
	if !ok {
		return "", "", &domain.ErrWorkspaceNotFound{WorkspaceID: id}
	}
	return row.encrypted, row.endpoint, row.err
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func encryptedSecret(t *testing.T, secret, key string) string {
	t.Helper()
	enc, err := crypto.EncryptString(secret, key)
	require.NoError(t, err)
	return enc
}

func newTestTrackingSource(loader *fakeTrackingLoader) (*TrackingWorkspaceSource, *fakeClock) {
	clock := &fakeClock{t: time.Unix(1_730_000_000, 0)}
	s := NewTrackingWorkspaceSource(loader.load, trackingTestSecretKey, []string{trackingTestPreviousKey})
	s.now = clock.now
	return s, clock
}

func TestTrackingWorkspaceSource_KnownWorkspaceIsCached(t *testing.T) {
	loader := newFakeTrackingLoader()
	loader.set("ws-1", fakeTrackingRow{encrypted: encryptedSecret(t, "ws-secret", trackingTestSecretKey)})
	s, clock := newTestTrackingSource(loader)
	ctx := context.Background()

	ws, status := s.Lookup(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceKnown, status)
	assert.Equal(t, domain.TrackingSigningKey("ws-secret"), ws.Key)
	assert.Equal(t, "", ws.EndpointHost)

	clock.advance(trackingKnownTTL - time.Second)
	_, status = s.Lookup(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceKnown, status)
	assert.Equal(t, 1, loader.count("ws-1"), "no query within the TTL")

	clock.advance(2 * time.Second)
	_, status = s.Lookup(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceKnown, status)
	assert.Equal(t, 2, loader.count("ws-1"), "reloaded once expired")
}

func TestTrackingWorkspaceSource_StaleEntryServedOnTransientError(t *testing.T) {
	loader := newFakeTrackingLoader()
	loader.set("ws-1", fakeTrackingRow{encrypted: encryptedSecret(t, "ws-secret", trackingTestSecretKey)})
	s, clock := newTestTrackingSource(loader)
	ctx := context.Background()

	_, status := s.Lookup(ctx, "ws-1")
	require.Equal(t, trackingtoken.WorkspaceKnown, status)

	clock.advance(trackingKnownTTL + time.Second)
	loader.set("ws-1", fakeTrackingRow{err: errors.New("connection refused")})
	ws, status := s.Lookup(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceKnown, status, "an outage must not cost verified links")
	assert.Equal(t, domain.TrackingSigningKey("ws-secret"), ws.Key)

	// Within the backoff, the stale entry is served without a query.
	_, status = s.Lookup(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceKnown, status)
	assert.Equal(t, 2, loader.count("ws-1"))

	clock.advance(trackingErrorBackoff + time.Second)
	loader.set("ws-1", fakeTrackingRow{encrypted: encryptedSecret(t, "new-secret", trackingTestSecretKey)})
	ws, status = s.Lookup(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceKnown, status)
	assert.Equal(t, domain.TrackingSigningKey("new-secret"), ws.Key, "replaced once a reload succeeds")
	assert.Equal(t, 3, loader.count("ws-1"))
}

func TestTrackingWorkspaceSource_UnknownIsCachedBriefly(t *testing.T) {
	loader := newFakeTrackingLoader()
	s, clock := newTestTrackingSource(loader)
	ctx := context.Background()

	for i := range 50 {
		_, status := s.Lookup(ctx, fmt.Sprintf("random-%d", i))
		assert.Equal(t, trackingtoken.WorkspaceUnknown, status)
	}
	for i := range 50 {
		_, status := s.Lookup(ctx, fmt.Sprintf("random-%d", i))
		assert.Equal(t, trackingtoken.WorkspaceUnknown, status)
		assert.Equal(t, 1, loader.count(fmt.Sprintf("random-%d", i)), "one query per ID within the TTL")
	}

	clock.advance(trackingUnknownTTL + time.Second)
	_, status := s.Lookup(ctx, "random-0")
	assert.Equal(t, trackingtoken.WorkspaceUnknown, status)
	assert.Equal(t, 2, loader.count("random-0"))
}

func TestTrackingWorkspaceSource_NegativeEntriesAreCapped(t *testing.T) {
	loader := newFakeTrackingLoader()
	loader.set("ws-1", fakeTrackingRow{encrypted: encryptedSecret(t, "ws-secret", trackingTestSecretKey)})
	s, _ := newTestTrackingSource(loader)
	ctx := context.Background()

	_, status := s.Lookup(ctx, "ws-1")
	require.Equal(t, trackingtoken.WorkspaceKnown, status)
	for i := range trackingMaxNegativeEntries + 1 {
		s.Lookup(ctx, fmt.Sprintf("random-%d", i))
	}
	s.mu.Lock()
	size, negatives := len(s.entries), s.negatives
	s.mu.Unlock()
	assert.LessOrEqual(t, negatives, trackingMaxNegativeEntries)
	assert.LessOrEqual(t, size, trackingMaxNegativeEntries+1)

	_, status = s.Lookup(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceKnown, status)
	assert.Equal(t, 1, loader.count("ws-1"), "real workspaces survive the cap")
}

func TestTrackingWorkspaceSource_TransientErrorWithoutEntry(t *testing.T) {
	loader := newFakeTrackingLoader()
	loader.set("ws-1", fakeTrackingRow{err: errors.New("connection refused")})
	s, clock := newTestTrackingSource(loader)
	ctx := context.Background()

	_, status := s.Lookup(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceUnavailable, status)
	_, status = s.Lookup(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceUnavailable, status)
	assert.Equal(t, 1, loader.count("ws-1"), "no re-query within the backoff")

	clock.advance(trackingErrorBackoff + time.Second)
	loader.set("ws-1", fakeTrackingRow{encrypted: encryptedSecret(t, "ws-secret", trackingTestSecretKey)})
	_, status = s.Lookup(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceKnown, status)
	assert.Equal(t, 2, loader.count("ws-1"))
}

func TestTrackingWorkspaceSource_ConcurrentLookupsCoalesce(t *testing.T) {
	loader := newFakeTrackingLoader()
	loader.set("ws-1", fakeTrackingRow{encrypted: encryptedSecret(t, "ws-secret", trackingTestSecretKey)})
	loader.gate = make(chan struct{})
	s, _ := newTestTrackingSource(loader)

	// The first caller's context is cancelled while the load is in flight.
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var known atomic.Int32
	start := func(ctx context.Context) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, status := s.Lookup(ctx, "ws-1"); status == trackingtoken.WorkspaceKnown {
				known.Add(1)
			}
		}()
	}
	start(firstCtx)
	// Let the first caller enter the load before the others join it.
	time.Sleep(20 * time.Millisecond)
	for range 20 {
		start(context.Background())
	}
	time.Sleep(20 * time.Millisecond)
	cancelFirst()
	close(loader.gate)
	wg.Wait()

	assert.Equal(t, 1, loader.count("ws-1"), "one query for concurrent lookups")
	assert.Equal(t, int32(21), known.Load(), "cancelling the first caller fails nobody")
}

func TestTrackingWorkspaceSource_RefreshIsRateLimited(t *testing.T) {
	loader := newFakeTrackingLoader()
	loader.set("ws-1", fakeTrackingRow{encrypted: encryptedSecret(t, "ws-secret", trackingTestSecretKey)})
	s, clock := newTestTrackingSource(loader)
	ctx := context.Background()

	_, status := s.Lookup(ctx, "ws-1")
	require.Equal(t, trackingtoken.WorkspaceKnown, status)

	clock.advance(trackingRefreshInterval + time.Second)
	loader.set("ws-1", fakeTrackingRow{encrypted: encryptedSecret(t, "recreated-secret", trackingTestSecretKey)})
	ws, status := s.Refresh(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceKnown, status)
	assert.Equal(t, domain.TrackingSigningKey("recreated-secret"), ws.Key)
	assert.Equal(t, 2, loader.count("ws-1"))

	clock.advance(time.Second)
	_, _ = s.Refresh(ctx, "ws-1")
	_, _ = s.Refresh(ctx, "ws-1")
	assert.Equal(t, 2, loader.count("ws-1"), "at most one refresh per interval")
}

func TestTrackingWorkspaceSource_SecretDecryption(t *testing.T) {
	loader := newFakeTrackingLoader()
	loader.set("previous", fakeTrackingRow{encrypted: encryptedSecret(t, "ws-secret", trackingTestPreviousKey)})
	loader.set("undecryptable", fakeTrackingRow{encrypted: encryptedSecret(t, "ws-secret", "some-other-key-entirely")})
	loader.set("empty", fakeTrackingRow{encrypted: ""})
	loader.set("empty-plaintext", fakeTrackingRow{encrypted: encryptedSecret(t, "", trackingTestSecretKey)})
	s, _ := newTestTrackingSource(loader)
	ctx := context.Background()

	ws, status := s.Lookup(ctx, "previous")
	assert.Equal(t, trackingtoken.WorkspaceKnown, status, "a previous SECRET_KEY still opens the secret")
	assert.Equal(t, domain.TrackingSigningKey("ws-secret"), ws.Key)

	for _, id := range []string{"undecryptable", "empty", "empty-plaintext"} {
		ws, status := s.Lookup(ctx, id)
		assert.Equal(t, trackingtoken.WorkspaceUnknown, status, id)
		assert.Empty(t, ws.Key, id)
	}

	_, status = s.Lookup(ctx, "")
	assert.Equal(t, trackingtoken.WorkspaceUnknown, status)
}

func TestTrackingWorkspaceSource_EndpointHost(t *testing.T) {
	cases := map[string]string{
		"https://Track.Brand.com:8443/base": "track.brand.com",
		"https://track.brand.com/":          "track.brand.com",
		"track.brand.com":                   "track.brand.com",
		"":                                  "",
		"https://[::1":                      "",
	}
	for endpoint, want := range cases {
		loader := newFakeTrackingLoader()
		loader.set("ws-1", fakeTrackingRow{encrypted: encryptedSecret(t, "ws-secret", trackingTestSecretKey), endpoint: endpoint})
		s, _ := newTestTrackingSource(loader)
		ws, status := s.Lookup(context.Background(), "ws-1")
		require.Equal(t, trackingtoken.WorkspaceKnown, status)
		assert.Equal(t, want, ws.EndpointHost, endpoint)
	}
}

func TestTrackingWorkspaceSource_InvalidIDsAreNeverQueriedNorCached(t *testing.T) {
	loader := newFakeTrackingLoader()
	s, _ := newTestTrackingSource(loader)
	ctx := context.Background()

	for _, id := range []string{"", strings.Repeat("a", 64), strings.Repeat("x", 750_000), "ws 1", "ws/1", "wś1"} {
		_, status := s.Lookup(ctx, id)
		assert.Equal(t, trackingtoken.WorkspaceUnknown, status)
		_, status = s.Refresh(ctx, id)
		assert.Equal(t, trackingtoken.WorkspaceUnknown, status)
	}
	loader.mu.Lock()
	queried := len(loader.calls)
	loader.mu.Unlock()
	assert.Zero(t, queried, "no query for an ID that cannot name a workspace")
	s.mu.Lock()
	assert.Empty(t, s.entries, "nothing cached for it either")
	s.mu.Unlock()
}

func TestTrackingWorkspaceSource_RefreshRespectsErrorBackoff(t *testing.T) {
	loader := newFakeTrackingLoader()
	loader.set("ws-1", fakeTrackingRow{encrypted: encryptedSecret(t, "ws-secret", trackingTestSecretKey)})
	s, clock := newTestTrackingSource(loader)
	ctx := context.Background()

	_, status := s.Lookup(ctx, "ws-1")
	require.Equal(t, trackingtoken.WorkspaceKnown, status)

	// The database goes down after the refresh interval: one failed reload...
	clock.advance(trackingRefreshInterval + time.Second)
	loader.set("ws-1", fakeTrackingRow{err: errors.New("connection refused")})
	ws, status := s.Refresh(ctx, "ws-1")
	assert.Equal(t, trackingtoken.WorkspaceKnown, status, "the cached workspace is still served")
	assert.Equal(t, domain.TrackingSigningKey("ws-secret"), ws.Key)
	require.Equal(t, 2, loader.count("ws-1"))

	// ...then every bad tag within the backoff is answered without a query.
	for range 20 {
		_, status = s.Refresh(ctx, "ws-1")
		assert.Equal(t, trackingtoken.WorkspaceKnown, status)
	}
	assert.Equal(t, 2, loader.count("ws-1"), "an outage must not be amplified by bad tags")

	clock.advance(trackingErrorBackoff + time.Second)
	_, _ = s.Refresh(ctx, "ws-1")
	assert.Equal(t, 3, loader.count("ws-1"), "retried once the backoff is over")
}
