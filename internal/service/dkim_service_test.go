package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/dkim"
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

const dkimServiceSecret = "dkim-service-test-secret-32-bytes!"

var (
	dkimServiceKeyOnce sync.Once
	dkimServiceKeyPEM  string
	dkimServiceKeyPub  string
)

func dkimServiceKey(t *testing.T) (string, string) {
	t.Helper()
	dkimServiceKeyOnce.Do(func() {
		var err error
		dkimServiceKeyPEM, dkimServiceKeyPub, err = dkim.GenerateRSA2048()
		require.NoError(t, err)
	})
	return dkimServiceKeyPEM, dkimServiceKeyPub
}

// fakeDKIMRepo holds one workspace's keys and integrations in memory and applies
// UpdateDKIMKeys the way the repository does. Other methods are not implemented.
type fakeDKIMRepo struct {
	domain.WorkspaceRepository

	mu           sync.Mutex
	keys         []domain.DKIMKey
	integrations domain.Integrations
	writes       int
	touches      int
	reads        int
	lists        int
	// beforeUpdate runs before UpdateDKIMKeys reads the keys, to simulate a writer
	// that commits during a lookup.
	beforeUpdate func(r *fakeDKIMRepo)
}

func (r *fakeDKIMRepo) GetByID(_ context.Context, id string) (*domain.Workspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return &domain.Workspace{ID: id, Settings: domain.WorkspaceSettings{DKIMKeys: append([]domain.DKIMKey{}, r.keys...)}, Integrations: r.integrations}, nil
}

func (r *fakeDKIMRepo) GetWorkspaceDKIMKeys(_ context.Context, _ string) ([]domain.DKIMKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	return append([]domain.DKIMKey{}, r.keys...), nil
}

func (r *fakeDKIMRepo) ListWorkspacesDKIMMetadata(_ context.Context) ([]domain.WorkspaceDKIMMetadata, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lists++
	if len(r.keys) == 0 {
		return nil, nil
	}
	keys := make([]domain.DKIMKey, len(r.keys))
	for i, k := range r.keys {
		keys[i] = k.Public()
	}
	return []domain.WorkspaceDKIMMetadata{{WorkspaceID: "ws-1", Keys: keys}}, nil
}

func (r *fakeDKIMRepo) UpdateDKIMKeys(_ context.Context, _ string, mutate domain.DKIMKeysMutator) ([]domain.DKIMKey, error) {
	if r.beforeUpdate != nil {
		hook := r.beforeUpdate
		r.beforeUpdate = nil
		hook(r)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	m, err := mutate(append([]domain.DKIMKey{}, r.keys...), r.integrations)
	if err != nil {
		return nil, err
	}
	if !m.Changed {
		return r.keys, nil
	}
	if err := domain.CheckDKIMKeys(mustJSON(map[string]any{"dkim_keys": m.Keys}), dkimServiceSecret); err != nil {
		return nil, err
	}
	r.keys = m.Keys
	r.writes++
	if m.TouchUpdatedAt {
		r.touches++
	}
	return r.keys, nil
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

type staticResolver struct {
	mu      sync.Mutex
	records map[string][]string
	err     error
}

func (s *staticResolver) set(name string, records ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if records == nil {
		delete(s.records, name)
		return
	}
	s.records[name] = records
}

func (s *staticResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	records, ok := s.records[strings.TrimSuffix(name, ".")]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	return records, nil
}

type dkimServiceFixture struct {
	svc      *DKIMService
	repo     *fakeDKIMRepo
	store    *DKIMKeyStore
	resolver *staticResolver
	now      time.Time
}

func newDKIMServiceFixture(t *testing.T, role string, permissions domain.UserPermissions) *dkimServiceFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
		DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{UserID: "u1", WorkspaceID: "ws-1", Role: role, Permissions: permissions}, nil
		}).AnyTimes()

	f := &dkimServiceFixture{
		repo:     &fakeDKIMRepo{},
		resolver: &staticResolver{records: map[string][]string{}},
		now:      time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC),
	}
	f.store = NewDKIMKeyStore(f.repo, dkimServiceSecret)
	f.svc = NewDKIMService(f.repo, auth, f.store, f.resolver, dkimServiceSecret, logger.NewLoggerWithLevel("disabled"))
	f.svc.now = func() time.Time { return f.now }
	f.svc.generate = func() (string, string, error) {
		privPEM, pub := dkimServiceKey(t)
		return privPEM, pub, nil
	}
	return f
}

func (f *dkimServiceFixture) publish(key domain.DKIMKey) {
	f.resolver.set(dkim.RecordName(key.Selector, key.Domain), dkim.FormatDNSValue(key.PublicKey))
}

func smtpIntegration(id string, senders ...string) domain.Integration {
	in := domain.Integration{ID: id, Name: "Relay " + id, Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP}}
	for _, s := range senders {
		in.EmailProvider.Senders = append(in.EmailProvider.Senders, domain.EmailSender{Email: s})
	}
	return in
}

func TestDKIMService_CreateDomain(t *testing.T) {
	ctx := context.Background()

	t.Run("an owner creates a pending key", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		f.repo.integrations = domain.Integrations{smtpIntegration("smtp1", "news@acme.io")}

		view, err := f.svc.CreateDomain(ctx, "ws-1", " Acme.IO ")
		require.NoError(t, err)

		assert.Equal(t, "acme.io", view.Domain)
		assert.Equal(t, domain.DKIMKeyStatusPending, view.Status)
		assert.Len(t, view.Selector, 30)
		assert.True(t, strings.HasPrefix(view.Selector, "notifuse202611"))
		assert.Equal(t, view.Selector+"._domainkey.acme.io", view.DNSRecord.Name)
		assert.Equal(t, []domain.DKIMDomainUser{{IntegrationID: "smtp1", IntegrationName: "Relay smtp1"}}, view.UsedBy)
		assert.NotContains(t, string(mustJSON(view)), "private")

		require.Len(t, f.repo.keys, 1)
		stored := f.repo.keys[0]
		assert.NotEmpty(t, stored.ID)
		assert.Equal(t, int64(1), stored.Revision)
		_, err = dkim.OpenPrivateKey(stored.EncryptedPrivateKey, dkimServiceSecret)
		assert.NoError(t, err, "stored encrypted with the instance secret key")
		assert.Equal(t, 1, f.repo.touches)
	})

	t.Run("a member cannot", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "member", domain.UserPermissions{domain.PermissionResourceWorkspace: {Read: true, Write: true}})
		_, err := f.svc.CreateDomain(ctx, "ws-1", "acme.io")
		var unauthorized *domain.ErrUnauthorized
		assert.ErrorAs(t, err, &unauthorized)
		assert.Empty(t, f.repo.keys)
	})

	t.Run("an invalid domain is refused", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		_, err := f.svc.CreateDomain(ctx, "ws-1", "https://acme.io")
		var invalid *domain.ErrDKIMInvalidDomain
		assert.ErrorAs(t, err, &invalid)
	})

	t.Run("a second key for a domain is refused", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		_, err := f.svc.CreateDomain(ctx, "ws-1", "acme.io")
		require.NoError(t, err)
		_, err = f.svc.CreateDomain(ctx, "ws-1", "ACME.io.")
		var conflict *domain.ErrDKIMConflict
		assert.ErrorAs(t, err, &conflict)
		assert.Len(t, f.repo.keys, 1)
	})

	t.Run("the cap is enforced", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		for i := 0; i < domain.MaxDKIMKeysPerWorkspace; i++ {
			f.repo.keys = append(f.repo.keys, domain.DKIMKey{Domain: string(rune('a'+i%26)) + strings.Repeat("x", i) + ".io"})
		}
		_, err := f.svc.CreateDomain(ctx, "ws-1", "acme.io")
		var conflict *domain.ErrDKIMConflict
		assert.ErrorAs(t, err, &conflict)
	})

	t.Run("the store sees the new key at once", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		keys, err := f.store.Keys(ctx, "ws-1")
		require.NoError(t, err)
		require.Empty(t, keys)

		_, err = f.svc.CreateDomain(ctx, "ws-1", "acme.io")
		require.NoError(t, err)
		keys, err = f.store.Keys(ctx, "ws-1")
		require.NoError(t, err)
		assert.Len(t, keys, 1)
	})
}

func TestDKIMService_RegenerateKey(t *testing.T) {
	ctx := context.Background()

	for _, status := range []domain.DKIMKeyStatus{domain.DKIMKeyStatusPending, domain.DKIMKeyStatusMismatch} {
		t.Run("replaces a "+string(status)+" key", func(t *testing.T) {
			f := newDKIMServiceFixture(t, "owner", nil)
			created := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
			f.repo.keys = []domain.DKIMKey{{ID: "old", Revision: 7, Domain: "acme.io", Selector: "notifuse202610old", Status: status, CreatedAt: created, ConsecutiveFailures: 2}}

			view, err := f.svc.RegenerateKey(ctx, "ws-1", "acme.io")
			require.NoError(t, err)

			stored := f.repo.keys[0]
			assert.NotEqual(t, "old", stored.ID)
			assert.Equal(t, int64(1), stored.Revision)
			assert.NotEqual(t, "notifuse202610old", stored.Selector)
			assert.Equal(t, stored.Selector, view.Selector)
			assert.Equal(t, domain.DKIMKeyStatusPending, stored.Status)
			assert.Zero(t, stored.ConsecutiveFailures)
			assert.Equal(t, f.now, stored.CreatedAt, "a new key: its age drives the pace of the checks")
			assert.NotEqual(t, created, stored.CreatedAt)
		})
	}

	t.Run("a published key is refused", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		f.repo.keys = []domain.DKIMKey{{ID: "old", Domain: "acme.io", Status: domain.DKIMKeyStatusPublished}}
		_, err := f.svc.RegenerateKey(ctx, "ws-1", "acme.io")
		var conflict *domain.ErrDKIMConflict
		assert.ErrorAs(t, err, &conflict)
		assert.Equal(t, "old", f.repo.keys[0].ID)
	})

	t.Run("an unknown domain", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		_, err := f.svc.RegenerateKey(ctx, "ws-1", "acme.io")
		var notFound *domain.ErrDKIMDomainNotFound
		assert.ErrorAs(t, err, &notFound)
	})
}

func TestDKIMService_VerifyDomain(t *testing.T) {
	ctx := context.Background()

	t.Run("a published record publishes the key at once", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		view, err := f.svc.CreateDomain(ctx, "ws-1", "acme.io")
		require.NoError(t, err)
		f.publish(f.repo.keys[0])

		got, err := f.svc.VerifyDomain(ctx, "ws-1", "acme.io")
		require.NoError(t, err)
		assert.Equal(t, domain.DKIMCheckResultOK, got.Check.Result)
		assert.Equal(t, domain.DKIMKeyStatusPublished, got.Domain.Status)
		assert.False(t, got.StaleConflict)
		assert.Equal(t, view.Selector, got.Domain.Selector)
		assert.Equal(t, domain.DKIMKeyStatusPublished, f.repo.keys[0].Status)
	})

	t.Run("no record keeps it pending", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		_, err := f.svc.CreateDomain(ctx, "ws-1", "acme.io")
		require.NoError(t, err)

		got, err := f.svc.VerifyDomain(ctx, "ws-1", "acme.io")
		require.NoError(t, err)
		assert.Equal(t, domain.DKIMCheckResultNotFound, got.Check.Result)
		assert.Equal(t, domain.DKIMKeyStatusPending, got.Domain.Status)
	})

	t.Run("a key changed during the lookup is not overwritten", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		_, err := f.svc.CreateDomain(ctx, "ws-1", "acme.io")
		require.NoError(t, err)
		f.publish(f.repo.keys[0])
		f.repo.beforeUpdate = func(r *fakeDKIMRepo) {
			r.mu.Lock()
			r.keys[0].ID = "regenerated"
			r.keys[0].Revision = 1
			r.keys[0].Selector = "notifuse202611ffffffffffffffff"
			r.mu.Unlock()
		}
		writes := f.repo.writes

		got, err := f.svc.VerifyDomain(ctx, "ws-1", "acme.io")
		require.NoError(t, err)
		assert.True(t, got.StaleConflict)
		assert.Equal(t, domain.DKIMCheckResultOK, got.Check.Result, "what the lookup found is still reported")
		assert.Equal(t, domain.DKIMKeyStatusPending, got.Domain.Status, "the stored state is returned")
		assert.Equal(t, "notifuse202611ffffffffffffffff", got.Domain.Selector)
		assert.Equal(t, writes, f.repo.writes)
	})

	t.Run("a published key stops signing only at the third failure an hour apart, with a warning", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		_, err := f.svc.CreateDomain(ctx, "ws-1", "acme.io")
		require.NoError(t, err)
		key := f.repo.keys[0]
		f.publish(key)
		_, err = f.svc.VerifyDomain(ctx, "ws-1", "acme.io")
		require.NoError(t, err)
		f.resolver.set(dkim.RecordName(key.Selector, key.Domain))

		ctrl := gomock.NewController(t)
		log := pkgmocks.NewMockLogger(ctrl)
		log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().Warn(gomock.Any()).Times(1)
		f.svc.logger = log

		for i, after := range []time.Duration{time.Hour, 10 * time.Minute, 50 * time.Minute} {
			f.now = f.now.Add(after)
			got, err := f.svc.VerifyDomain(ctx, "ws-1", "acme.io")
			require.NoError(t, err)
			assert.Equal(t, domain.DKIMKeyStatusPublished, got.Domain.Status, "check %d", i)
		}
		assert.Equal(t, 2, f.repo.keys[0].ConsecutiveFailures)

		f.now = f.now.Add(time.Hour)
		got, err := f.svc.VerifyDomain(ctx, "ws-1", "acme.io")
		require.NoError(t, err)
		assert.Equal(t, domain.DKIMKeyStatusPending, got.Domain.Status)
		assert.NotNil(t, got.Domain.DowngradedAt)
	})

	t.Run("a member cannot", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "member", domain.UserPermissions{domain.PermissionResourceWorkspace: {Read: true, Write: true}})
		_, err := f.svc.VerifyDomain(ctx, "ws-1", "acme.io")
		var unauthorized *domain.ErrUnauthorized
		assert.ErrorAs(t, err, &unauthorized)
	})
}

func TestDKIMService_DeleteDomain(t *testing.T) {
	ctx := context.Background()

	for _, status := range []domain.DKIMKeyStatus{domain.DKIMKeyStatusPending, domain.DKIMKeyStatusMismatch, domain.DKIMKeyStatusPublished} {
		t.Run("refused while an SMTP sender uses a "+string(status)+" key", func(t *testing.T) {
			f := newDKIMServiceFixture(t, "owner", nil)
			f.repo.keys = []domain.DKIMKey{{Domain: "acme.io", Status: status}}
			f.repo.integrations = domain.Integrations{smtpIntegration("smtp1", "news@acme.io")}

			err := f.svc.DeleteDomain(ctx, "ws-1", "acme.io")
			var conflict *domain.ErrDKIMConflict
			require.ErrorAs(t, err, &conflict)
			assert.Equal(t, []domain.DKIMDomainUser{{IntegrationID: "smtp1", IntegrationName: "Relay smtp1"}}, conflict.UsedBy)
			assert.Len(t, f.repo.keys, 1)
		})
	}

	t.Run("allowed when only an API provider uses the domain", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		f.repo.keys = []domain.DKIMKey{{Domain: "acme.io"}, {Domain: "other.io"}}
		f.repo.integrations = domain.Integrations{{ID: "ses", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{
			Kind: domain.EmailProviderKindSES, Senders: []domain.EmailSender{{Email: "news@acme.io"}},
		}}}

		require.NoError(t, f.svc.DeleteDomain(ctx, "ws-1", "acme.io"))
		require.Len(t, f.repo.keys, 1)
		assert.Equal(t, "other.io", f.repo.keys[0].Domain)
	})

	t.Run("an unknown domain", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "owner", nil)
		err := f.svc.DeleteDomain(ctx, "ws-1", "acme.io")
		var notFound *domain.ErrDKIMDomainNotFound
		assert.ErrorAs(t, err, &notFound)
	})
}

func TestDKIMService_ListDomains(t *testing.T) {
	ctx := context.Background()

	t.Run("a member with read permission lists the domains and their users, without keys", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "member", domain.UserPermissions{domain.PermissionResourceWorkspace: {Read: true}})
		f.repo.keys = []domain.DKIMKey{
			{Domain: "acme.io", Selector: "s1", PublicKey: "PUB", EncryptedPrivateKey: "SECRET", Status: domain.DKIMKeyStatusPublished},
			{Domain: "other.io", Selector: "s2", EncryptedPrivateKey: "SECRET"},
		}
		f.repo.integrations = domain.Integrations{smtpIntegration("smtp1", "news@acme.io")}

		views, err := f.svc.ListDomains(ctx, "ws-1")
		require.NoError(t, err)
		require.Len(t, views, 2)
		assert.Len(t, views[0].UsedBy, 1)
		assert.Equal(t, []domain.DKIMDomainUser{}, views[1].UsedBy)
		assert.Equal(t, "v=DKIM1; k=rsa; p=PUB", views[0].DNSRecord.Value)
		assert.NotContains(t, string(mustJSON(views)), "SECRET")
	})

	t.Run("a member without workspace permission cannot", func(t *testing.T) {
		f := newDKIMServiceFixture(t, "member", domain.UserPermissions{})
		_, err := f.svc.ListDomains(ctx, "ws-1")
		var unauthorized *domain.ErrUnauthorized
		assert.ErrorAs(t, err, &unauthorized)
	})

	t.Run("an authentication failure is returned", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		auth := mocks.NewMockAuthService(ctrl)
		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").Return(nil, nil, nil, errors.New("no session"))
		svc := NewDKIMService(&fakeDKIMRepo{}, auth, nil, nil, dkimServiceSecret, logger.NewLoggerWithLevel("disabled"))
		_, err := svc.ListDomains(ctx, "ws-1")
		assert.ErrorContains(t, err, "no session")
	})
}
