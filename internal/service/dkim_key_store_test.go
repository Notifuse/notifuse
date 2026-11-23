package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/dkim"
)

type erroringDKIMRepo struct {
	domain.WorkspaceRepository
	err error
}

func (r *erroringDKIMRepo) GetWorkspaceDKIMKeys(context.Context, string) ([]domain.DKIMKey, error) {
	return nil, r.err
}

func sealedServiceKey(t *testing.T) string {
	t.Helper()
	privPEM, _ := dkimServiceKey(t)
	sealed, err := dkim.EncryptPrivateKey(privPEM, dkimServiceSecret)
	require.NoError(t, err)
	return sealed
}

func TestDKIMKeyStore_Keys(t *testing.T) {
	ctx := context.Background()
	repo := &fakeDKIMRepo{keys: []domain.DKIMKey{{Domain: "acme.io"}}}
	store := NewDKIMKeyStore(repo, dkimServiceSecret)
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		_, err := store.Keys(ctx, "ws-1")
		require.NoError(t, err)
	}
	assert.Equal(t, 1, repo.reads, "read once within the TTL")

	now = now.Add(dkimKeysTTL)
	_, err := store.Keys(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, 2, repo.reads, "read again once stale")

	store.Invalidate("ws-1")
	_, err = store.Keys(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, 3, repo.reads, "read again after Invalidate")
}

// invalidatingRepo calls Invalidate while a read is in flight, as a DKIM change committed
// during a send's lookup would.
type invalidatingRepo struct {
	fakeDKIMRepo
	store *DKIMKeyStore
}

func (r *invalidatingRepo) GetWorkspaceDKIMKeys(ctx context.Context, id string) ([]domain.DKIMKey, error) {
	keys, err := r.fakeDKIMRepo.GetWorkspaceDKIMKeys(ctx, id)
	r.store.Invalidate(id)
	return keys, err
}

func TestDKIMKeyStore_AReadOverlappingAnInvalidateIsNotKept(t *testing.T) {
	ctx := context.Background()
	repo := &invalidatingRepo{fakeDKIMRepo: fakeDKIMRepo{keys: []domain.DKIMKey{{Domain: "acme.io"}}}}
	store := NewDKIMKeyStore(repo, dkimServiceSecret)
	repo.store = store

	keys, err := store.Keys(ctx, "ws-1")
	require.NoError(t, err)
	assert.Len(t, keys, 1, "the read is still used")
	_, err = store.Keys(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, 2, repo.reads, "but not cached: the next call reads again")
}

func TestDKIMKeyStore_IsBounded(t *testing.T) {
	ctx := context.Background()
	store := NewDKIMKeyStore(&fakeDKIMRepo{}, dkimServiceSecret)
	for i := 0; i < dkimKeysMaxWorkspaces+50; i++ {
		_, err := store.Keys(ctx, "ws-"+time.Duration(i).String())
		require.NoError(t, err)
	}
	assert.LessOrEqual(t, len(store.entries), dkimKeysMaxWorkspaces)
}

func TestDKIMKeyStore_SigningKey(t *testing.T) {
	ctx := context.Background()
	sealed := sealedServiceKey(t)
	published := domain.DKIMKey{Domain: "acme.io", Selector: "sel", Status: domain.DKIMKeyStatusPublished, EncryptedPrivateKey: sealed}

	t.Run("a snapshot is used as is, even empty, with no lookup", func(t *testing.T) {
		repo := &fakeDKIMRepo{keys: []domain.DKIMKey{published}}
		store := NewDKIMKeyStore(repo, dkimServiceSecret)

		empty := []domain.DKIMKey{}
		key, err := store.SigningKey(ctx, "ws-1", &empty, "news@acme.io")
		require.NoError(t, err)
		assert.Nil(t, key)

		snapshot := []domain.DKIMKey{published}
		key, err = store.SigningKey(ctx, "ws-1", &snapshot, "news@acme.io")
		require.NoError(t, err)
		require.NotNil(t, key)
		assert.Equal(t, "acme.io", key.Domain)
		assert.Equal(t, "sel", key.Selector)
		assert.NotNil(t, key.Key)
		assert.Zero(t, repo.reads)
	})

	t.Run("without a snapshot the keys are read", func(t *testing.T) {
		repo := &fakeDKIMRepo{keys: []domain.DKIMKey{published}}
		store := NewDKIMKeyStore(repo, dkimServiceSecret)
		key, err := store.SigningKey(ctx, "ws-1", nil, "Acme <news@ACME.io>")
		require.NoError(t, err)
		assert.NotNil(t, key)
		assert.Equal(t, 1, repo.reads)
	})

	t.Run("no key for pending, mismatch, a subdomain or another domain", func(t *testing.T) {
		keys := []domain.DKIMKey{
			{Domain: "pending.io", Status: domain.DKIMKeyStatusPending, EncryptedPrivateKey: sealed},
			{Domain: "mismatch.io", Status: domain.DKIMKeyStatusMismatch, EncryptedPrivateKey: sealed},
			published,
		}
		store := NewDKIMKeyStore(&fakeDKIMRepo{}, dkimServiceSecret)
		for _, from := range []string{"a@pending.io", "a@mismatch.io", "a@support.acme.io", "a@other.io"} {
			key, err := store.SigningKey(ctx, "ws-1", &keys, from)
			require.NoError(t, err, from)
			assert.Nil(t, key, from)
		}
	})

	t.Run("a key that does not open is a signing error", func(t *testing.T) {
		broken := published
		broken.EncryptedPrivateKey = "zz"
		keys := []domain.DKIMKey{broken}
		_, err := NewDKIMKeyStore(&fakeDKIMRepo{}, dkimServiceSecret).SigningKey(ctx, "ws-1", &keys, "news@acme.io")
		assert.True(t, errors.Is(err, dkim.ErrSign))
	})

	t.Run("a read failure is a signing error", func(t *testing.T) {
		store := NewDKIMKeyStore(&erroringDKIMRepo{err: errors.New("db down")}, dkimServiceSecret)
		_, err := store.SigningKey(ctx, "ws-1", nil, "news@acme.io")
		assert.True(t, errors.Is(err, dkim.ErrSign))
	})
}
