package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
)

// TrackingWorkspaceLoader reads a workspace's encrypted secret key and custom endpoint
// URL, returning *domain.ErrWorkspaceNotFound when there is no such workspace.
type TrackingWorkspaceLoader func(ctx context.Context, workspaceID string) (encryptedSecretKey, customEndpointURL string, err error)

const (
	// trackingKnownTTL bounds how stale a workspace's endpoint host can be, and how long
	// a workspace re-created under the same ID keeps its old key here.
	trackingKnownTTL = 5 * time.Minute
	// trackingUnknownTTL is how long an ID with no workspace is not queried again.
	trackingUnknownTTL = 5 * time.Second
	// trackingErrorBackoff is how long an ID whose lookup failed, with nothing cached to
	// serve, is not queried again, so a database outage is not amplified.
	trackingErrorBackoff = 2 * time.Second
	// trackingRefreshInterval is how often a bad tag may force a workspace to be re-read.
	trackingRefreshInterval = 30 * time.Second
	// trackingLoadTimeout bounds a coalesced load, which no single caller's context owns.
	trackingLoadTimeout = 5 * time.Second
	// trackingMaxNegativeEntries caps the unknown and failed IDs kept, since anyone can
	// name random IDs. Past it they are all dropped; real workspaces are kept.
	trackingMaxNegativeEntries = 10_000
)

type trackingEntry struct {
	status    trackingtoken.WorkspaceStatus
	workspace trackingtoken.Workspace
	loadedAt  time.Time
	// expires is when a known or unknown entry must be re-read.
	expires time.Time
	// retryAfter holds back a re-read after a failed one.
	retryAfter time.Time
}

// TrackingWorkspaceSource tells the email tracking routes the signing key and endpoint
// host of a workspace, from a narrow query and a cache. It is safe for concurrent use.
//
// Unauthenticated traffic costs at most one primary-key query per distinct ID per
// trackingUnknownTTL for IDs with no workspace, per trackingKnownTTL for real ones, and
// one more per real workspace per trackingRefreshInterval for bad tags.
type TrackingWorkspaceSource struct {
	load TrackingWorkspaceLoader
	keys []string
	now  func() time.Time

	mu      sync.Mutex
	entries map[string]*trackingEntry
	// negatives counts the unknown and failed entries, for the cap.
	negatives int
	group     singleflight.Group
}

// NewTrackingWorkspaceSource builds a source over load, decrypting secrets with
// secretKey, then each of previousSecretKeys.
func NewTrackingWorkspaceSource(load TrackingWorkspaceLoader, secretKey string, previousSecretKeys []string) *TrackingWorkspaceSource {
	return &TrackingWorkspaceSource{
		load:    load,
		keys:    append([]string{secretKey}, previousSecretKeys...),
		now:     time.Now,
		entries: make(map[string]*trackingEntry),
	}
}

// Lookup returns the workspace's tracking data from the cache, loading it when missing
// or expired. When a reload fails transiently, an expired known entry is still served.
func (s *TrackingWorkspaceSource) Lookup(ctx context.Context, workspaceID string) (trackingtoken.Workspace, trackingtoken.WorkspaceStatus) {
	// An ID that cannot name a workspace is unknown without a query, and never cached:
	// it comes from the request and could be of any size.
	if !trackingtoken.ValidWorkspaceID(workspaceID) {
		return trackingtoken.Workspace{}, trackingtoken.WorkspaceUnknown
	}
	now := s.now()
	s.mu.Lock()
	e := s.entries[workspaceID]
	if e != nil && s.servable(e, now) {
		ws, status := e.workspace, e.status
		s.mu.Unlock()
		return ws, status
	}
	s.mu.Unlock()
	return s.reload(ctx, workspaceID)
}

// Refresh re-reads a known workspace after its key failed to verify a tag, at most once
// per trackingRefreshInterval, and not while a failed read is backing off; otherwise it
// answers as Lookup.
func (s *TrackingWorkspaceSource) Refresh(ctx context.Context, workspaceID string) (trackingtoken.Workspace, trackingtoken.WorkspaceStatus) {
	if !trackingtoken.ValidWorkspaceID(workspaceID) {
		return trackingtoken.Workspace{}, trackingtoken.WorkspaceUnknown
	}
	now := s.now()
	s.mu.Lock()
	e := s.entries[workspaceID]
	if e != nil && e.status == trackingtoken.WorkspaceKnown &&
		(now.Sub(e.loadedAt) < trackingRefreshInterval || now.Before(e.retryAfter)) {
		ws := e.workspace
		s.mu.Unlock()
		return ws, trackingtoken.WorkspaceKnown
	}
	if e != nil && e.status != trackingtoken.WorkspaceKnown && s.servable(e, now) {
		ws, status := e.workspace, e.status
		s.mu.Unlock()
		return ws, status
	}
	s.mu.Unlock()
	return s.reload(ctx, workspaceID)
}

// servable reports whether e can be answered without a query. Caller holds s.mu.
func (s *TrackingWorkspaceSource) servable(e *trackingEntry, now time.Time) bool {
	switch e.status {
	case trackingtoken.WorkspaceUnavailable:
		return now.Before(e.retryAfter)
	case trackingtoken.WorkspaceKnown:
		// An expired entry whose reload just failed is served until the backoff ends.
		return now.Before(e.expires) || now.Before(e.retryAfter)
	default:
		return now.Before(e.expires)
	}
}

type trackingLoadResult struct {
	workspace trackingtoken.Workspace
	status    trackingtoken.WorkspaceStatus
}

func (s *TrackingWorkspaceSource) reload(ctx context.Context, workspaceID string) (trackingtoken.Workspace, trackingtoken.WorkspaceStatus) {
	v, _, _ := s.group.Do(workspaceID, func() (any, error) {
		// One disconnecting client must not fail the others waiting on this load.
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), trackingLoadTimeout)
		defer cancel()
		encrypted, endpoint, err := s.load(loadCtx, workspaceID)
		return s.store(workspaceID, encrypted, endpoint, err), nil
	})
	r := v.(trackingLoadResult)
	return r.workspace, r.status
}

// store records the outcome of a load and returns what to answer.
func (s *TrackingWorkspaceSource) store(workspaceID, encrypted, endpoint string, loadErr error) trackingLoadResult {
	now := s.now()
	var notFound *domain.ErrWorkspaceNotFound

	var next *trackingEntry
	switch {
	case loadErr == nil:
		if ws, ok := s.decode(encrypted, endpoint); ok {
			next = &trackingEntry{status: trackingtoken.WorkspaceKnown, workspace: ws, loadedAt: now, expires: now.Add(trackingKnownTTL)}
		} else {
			// A secret no listed key opens verifies nothing: as good as no workspace.
			next = &trackingEntry{status: trackingtoken.WorkspaceUnknown, loadedAt: now, expires: now.Add(trackingUnknownTTL)}
		}
	case errors.As(loadErr, &notFound):
		next = &trackingEntry{status: trackingtoken.WorkspaceUnknown, loadedAt: now, expires: now.Add(trackingUnknownTTL)}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.entries[workspaceID]
	if next == nil {
		// Transient failure: keep serving what we had, or hold back the next query.
		if prev != nil && prev.status == trackingtoken.WorkspaceKnown {
			prev.retryAfter = now.Add(trackingErrorBackoff)
			return trackingLoadResult{workspace: prev.workspace, status: trackingtoken.WorkspaceKnown}
		}
		next = &trackingEntry{status: trackingtoken.WorkspaceUnavailable, loadedAt: now, retryAfter: now.Add(trackingErrorBackoff)}
	}
	s.put(workspaceID, prev, next)
	return trackingLoadResult{workspace: next.workspace, status: next.status}
}

// put replaces prev with next under workspaceID, keeping the negative count and cap.
// Caller holds s.mu.
func (s *TrackingWorkspaceSource) put(workspaceID string, prev, next *trackingEntry) {
	if prev != nil && prev.status != trackingtoken.WorkspaceKnown {
		s.negatives--
	}
	if next.status != trackingtoken.WorkspaceKnown {
		if s.negatives >= trackingMaxNegativeEntries {
			for id, e := range s.entries {
				if e.status != trackingtoken.WorkspaceKnown {
					delete(s.entries, id)
				}
			}
			s.negatives = 0
		}
		s.negatives++
	}
	s.entries[workspaceID] = next
}

func (s *TrackingWorkspaceSource) decode(encrypted, endpoint string) (trackingtoken.Workspace, bool) {
	if encrypted == "" {
		return trackingtoken.Workspace{}, false
	}
	secret, _, err := crypto.DecryptWithKeys(encrypted, s.keys)
	if err != nil || secret == "" {
		return trackingtoken.Workspace{}, false
	}
	key, err := trackingtoken.DeriveKey(secret)
	if err != nil {
		return trackingtoken.Workspace{}, false
	}
	return trackingtoken.Workspace{Key: key, EndpointHost: trackingtoken.HostOf(endpoint)}, true
}
