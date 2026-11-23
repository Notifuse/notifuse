package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/dkim"
)

const (
	// dkimKeysTTL is how long the keys of a workspace are reused before they are read
	// again. A change made on this instance is seen at once (Invalidate); one made on
	// another instance within this delay.
	dkimKeysTTL = 60 * time.Second
	// dkimKeysMaxWorkspaces bounds the workspaces whose keys are held.
	dkimKeysMaxWorkspaces = 4096
)

// DKIMKeyStore resolves the key a send is signed with. It holds the DKIM keys of each
// workspace for dkimKeysTTL, read without loading the workspace, and the parsed private
// keys (dkim.KeyCache). DKIMService invalidates a workspace's entry when it changes its
// keys.
type DKIMKeyStore struct {
	repo      domain.WorkspaceRepository
	secretKey string
	signers   *dkim.KeyCache
	now       func() time.Time

	mu      sync.Mutex
	entries map[string]dkimKeysEntry
}

type dkimKeysEntry struct {
	keys   []domain.DKIMKey
	readAt time.Time
}

// NewDKIMKeyStore returns a store reading the keys from repo and opening them with the
// instance secret key.
func NewDKIMKeyStore(repo domain.WorkspaceRepository, secretKey string) *DKIMKeyStore {
	return &DKIMKeyStore{
		repo:      repo,
		secretKey: secretKey,
		signers:   dkim.NewKeyCache(dkim.KeyCacheSize),
		now:       time.Now,
		entries:   map[string]dkimKeysEntry{},
	}
}

// Keys returns the DKIM keys of a workspace, from the cache when it is fresh.
func (s *DKIMKeyStore) Keys(ctx context.Context, workspaceID string) ([]domain.DKIMKey, error) {
	now := s.now()
	s.mu.Lock()
	entry, ok := s.entries[workspaceID]
	s.mu.Unlock()
	if ok && now.Sub(entry.readAt) < dkimKeysTTL {
		return entry.keys, nil
	}

	keys, err := s.repo.GetWorkspaceDKIMKeys(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, held := s.entries[workspaceID]; !held && len(s.entries) >= dkimKeysMaxWorkspaces {
		for id, e := range s.entries {
			if now.Sub(e.readAt) >= dkimKeysTTL {
				delete(s.entries, id)
			}
		}
		for id := range s.entries {
			if len(s.entries) < dkimKeysMaxWorkspaces {
				break
			}
			delete(s.entries, id)
		}
	}
	s.entries[workspaceID] = dkimKeysEntry{keys: keys, readAt: now}
	return keys, nil
}

// Invalidate drops the cached keys of a workspace.
func (s *DKIMKeyStore) Invalidate(workspaceID string) {
	s.mu.Lock()
	delete(s.entries, workspaceID)
	s.mu.Unlock()
}

// SigningKey returns the key that signs an email of workspaceID sent from fromAddress,
// or nil when the exact domain of the sender has no published key. snapshot, when
// non-nil, is the keys the caller already read: no lookup is made then, even when it is
// empty. Any failure is wrapped in dkim.ErrSign: an email that should be signed is never
// sent unsigned instead.
func (s *DKIMKeyStore) SigningKey(ctx context.Context, workspaceID string, snapshot *[]domain.DKIMKey, fromAddress string) (*domain.DKIMSigningKey, error) {
	var keys []domain.DKIMKey
	if snapshot != nil {
		keys = *snapshot
	} else {
		read, err := s.Keys(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to read the DKIM keys: %v", dkim.ErrSign, err)
		}
		keys = read
	}

	key := domain.PublishedDKIMKeyFor(keys, fromAddress)
	if key == nil {
		return nil, nil
	}
	private, err := s.signers.Get(workspaceID, key.EncryptedPrivateKey, s.secretKey)
	if err != nil {
		return nil, fmt.Errorf("DKIM key of %s: %w", key.Domain, err)
	}
	return &domain.DKIMSigningKey{Domain: key.Domain, Selector: key.Selector, Key: private}, nil
}
