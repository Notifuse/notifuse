package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

type auditServiceFixture struct {
	repo         *mocks.MockAuditLogRepository
	auth         *mocks.MockAuthService
	users        *mocks.MockUserRepository
	workspaces   *mocks.MockWorkspaceRepository
	settings     *mocks.MockSettingRepository
	entitlements *mocks.MockEntitlementProvider
	service      *AuditLogService
	now          time.Time
}

// newAuditServiceFixture wires every dependency as a mock. licensed decides
// what the entitlement provider answers; nil leaves the provider out, which
// must record everything (unwired is not unlicensed). The recording-state
// settings row reads as absent and accepts writes, so tests that do not care
// about markers see no marker.
func newAuditServiceFixture(t *testing.T, licensed *bool) *auditServiceFixture {
	t.Helper()
	f := newAuditServiceFixtureBare(t, licensed)
	f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRecordingSettingKey).
		Return(nil, &domain.ErrSettingNotFound{Key: domain.AuditLogsRecordingSettingKey}).AnyTimes()
	f.settings.EXPECT().Set(gomock.Any(), domain.AuditLogsRecordingSettingKey, gomock.Any()).Return(nil).AnyTimes()
	return f
}

// newAuditServiceFixtureBare is the same without the recording-state defaults,
// for the tests that assert on them.
func newAuditServiceFixtureBare(t *testing.T, licensed *bool) *auditServiceFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	f := &auditServiceFixture{
		repo:       mocks.NewMockAuditLogRepository(ctrl),
		auth:       mocks.NewMockAuthService(ctrl),
		users:      mocks.NewMockUserRepository(ctrl),
		workspaces: mocks.NewMockWorkspaceRepository(ctrl),
		settings:   mocks.NewMockSettingRepository(ctrl),
		now:        time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC),
	}
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	cfg := AuditLogServiceConfig{
		Repo:          f.repo,
		AuthService:   f.auth,
		UserRepo:      f.users,
		WorkspaceRepo: f.workspaces,
		SettingRepo:   f.settings,
		IsRootEmail:   func(email string) bool { return email == "root@example.com" },
		Logger:        mockLogger,
		Now:           func() time.Time { return f.now },
	}
	if licensed != nil {
		f.entitlements = mocks.NewMockEntitlementProvider(ctrl)
		features := []domain.Feature{}
		if *licensed {
			features = append(features, domain.FeatureAuditLogs)
		}
		f.entitlements.EXPECT().Entitlements().Return(domain.Entitlements{Features: features, State: domain.LicenseStateActive}).AnyTimes()
		cfg.Entitlements = f.entitlements
	}
	f.service = NewAuditLogService(cfg)
	return f
}

func auditBool(b bool) *bool { return &b }

func TestAuditLogService_Record(t *testing.T) {
	t.Run("licensed: fills the row and inserts", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(true))
		f.service.Init(context.Background())

		var inserted *domain.AuditLog
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, log *domain.AuditLog) error {
			inserted = log
			return nil
		})

		f.service.Record(context.Background(), &domain.AuditLog{
			Action:     "templates.update",
			ActorType:  domain.AuditActorUser,
			ActorID:    "u1",
			ActorEmail: "ann@example.com",
			Changes:    map[string]domain.AuditChange{"smtp_password": {Old: "a", New: "b"}, "name": {Old: "x", New: "y"}},
			Metadata:   map[string]any{"api_key": "k", "scope": "full"},
		})

		require.NotNil(t, inserted)
		assert.Len(t, inserted.ID, 32)
		assert.Equal(t, f.now, inserted.OccurredAt)
		assert.Equal(t, domain.AuditCategoryTemplates, inserted.Category, "derived from the catalogue")
		assert.Equal(t, domain.AuditOutcomeSuccess, inserted.Outcome)
		assert.Equal(t, domain.AuditRedactedValue, inserted.Changes["smtp_password"].New, "the recorder redacts even when the caller forgot")
		assert.Equal(t, "y", inserted.Changes["name"].New)
		assert.Equal(t, domain.AuditRedactedValue, inserted.Metadata["api_key"])
		assert.Equal(t, "full", inserted.Metadata["scope"])
	})

	t.Run("unlicensed: nothing is written", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(false))
		f.service.Init(context.Background())
		// No Insert expectation: gomock fails on an unexpected call.
		f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.update"})
	})

	t.Run("no provider: records everything", func(t *testing.T) {
		f := newAuditServiceFixture(t, nil)
		f.service.Init(context.Background())
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(nil)
		f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.update"})
	})

	t.Run("a nil or empty event is ignored", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(true))
		f.service.Record(context.Background(), nil)
		f.service.Record(context.Background(), &domain.AuditLog{})
	})

	t.Run("a repository error is swallowed", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(true))
		f.service.Init(context.Background())
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(errors.New("boom"))
		assert.NotPanics(t, func() {
			f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.update"})
		})
	})

	t.Run("a cancelled request context still writes", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(true))
		f.service.Init(context.Background())
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ *domain.AuditLog) error {
			assert.NoError(t, ctx.Err(), "the write runs on a context detached from the request")
			_, hasDeadline := ctx.Deadline()
			assert.True(t, hasDeadline, "but bounded")
			return nil
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		f.service.Record(ctx, &domain.AuditLog{Action: "templates.update"})
	})

	t.Run("completes the actor from the user repository and marks root", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(true))
		f.service.Init(context.Background())
		f.users.EXPECT().GetUserByID(gomock.Any(), "r1").Return(&domain.User{ID: "r1", Email: "root@example.com", Name: "Root", Type: domain.UserTypeUser}, nil)
		var inserted *domain.AuditLog
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, log *domain.AuditLog) error {
			inserted = log
			return nil
		})
		f.service.Record(context.Background(), &domain.AuditLog{Action: "licence.set", ActorType: domain.AuditActorUser, ActorID: "r1"})
		require.NotNil(t, inserted)
		assert.Equal(t, "root@example.com", inserted.ActorEmail)
		assert.Equal(t, "Root", inserted.ActorName)
		assert.Equal(t, domain.AuditRoleRoot, inserted.ActorRole)
		assert.Equal(t, domain.AuditCategoryLicence, inserted.Category)
	})

	t.Run("an api key resolved late is typed as one", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(true))
		f.service.Init(context.Background())
		f.users.EXPECT().GetUserByID(gomock.Any(), "k1").Return(&domain.User{ID: "k1", Email: "zapier@ws.local", Type: domain.UserTypeAPIKey}, nil)
		var inserted *domain.AuditLog
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, log *domain.AuditLog) error {
			inserted = log
			return nil
		})
		f.service.Record(context.Background(), &domain.AuditLog{Action: "user.logout", ActorID: "k1"})
		assert.Equal(t, domain.AuditActorAPIKey, inserted.ActorType)
		assert.Equal(t, domain.AuditAuthAPIKey, inserted.AuthMethod)
	})

	t.Run("a known actor is not looked up again", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(true))
		f.service.Init(context.Background())
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(nil)
		// No GetUserByID expectation.
		f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.update", ActorID: "u1", ActorEmail: "ann@example.com"})
	})
}

func TestAuditLogService_LicenceTransitions(t *testing.T) {
	newSwitchable := func(t *testing.T) (*auditServiceFixture, *bool) {
		f := newAuditServiceFixture(t, nil)
		licensed := true
		ctrl := gomock.NewController(t)
		provider := mocks.NewMockEntitlementProvider(ctrl)
		provider.EXPECT().Entitlements().DoAndReturn(func() domain.Entitlements {
			if licensed {
				return domain.Entitlements{Features: []domain.Feature{domain.FeatureAuditLogs}}
			}
			return domain.CommunityEntitlements()
		}).AnyTimes()
		f.service.entitlements = provider
		return f, &licensed
	}

	t.Run("stopping writes one marker, then nothing", func(t *testing.T) {
		f, licensed := newSwitchable(t)
		f.service.Init(context.Background())

		var actions []string
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, log *domain.AuditLog) error {
			actions = append(actions, log.Action)
			return nil
		}).AnyTimes()

		f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.update"})
		*licensed = false
		f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.delete"})
		f.service.Record(context.Background(), &domain.AuditLog{Action: "lists.delete"})

		assert.Equal(t, []string{"templates.update", domain.AuditActionRecordingStopped}, actions)
	})

	t.Run("resuming writes one marker, then rows again", func(t *testing.T) {
		f, licensed := newSwitchable(t)
		*licensed = false
		f.service.Init(context.Background())

		var actions []string
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, log *domain.AuditLog) error {
			actions = append(actions, log.Action)
			assert.Equal(t, domain.AuditActorSystem, log.ActorType)
			return nil
		}).AnyTimes()

		f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.update"})
		*licensed = true
		f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.delete"})

		assert.Equal(t, []string{domain.AuditActionRecordingResumed, "templates.delete"}, actions)
	})

	t.Run("without Init the first state seen is not a transition", func(t *testing.T) {
		f, _ := newSwitchable(t)
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, log *domain.AuditLog) error {
			assert.Equal(t, "templates.update", log.Action)
			return nil
		}).Times(1)
		f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.update"})
	})
}

func auditMember(workspaceID string, read bool) *domain.UserWorkspace {
	permissions := domain.NewFullPermissions()
	if read {
		permissions[domain.PermissionResourceAuditLogs] = domain.ResourcePermissions{Read: true}
	}
	return &domain.UserWorkspace{UserID: "u1", WorkspaceID: workspaceID, Role: "member", Permissions: permissions}
}

func TestAuditLogService_List(t *testing.T) {
	filter := domain.AuditLogFilter{WorkspaceID: "ws1"}

	t.Run("a member without the grant is refused, even a full-access one", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(false))
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{ID: "u1"}, auditMember("ws1", false), nil)
		_, err := f.service.List(context.Background(), filter)
		var permErr *domain.PermissionError
		require.True(t, errors.As(err, &permErr))
		assert.Equal(t, domain.PermissionResourceAuditLogs, permErr.Resource)
		assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)
	})

	t.Run("a member with the grant reads, whatever the licence says", func(t *testing.T) {
		// Unlicensed on purpose: the read path never consults the provider, and
		// a provider with a zero-call expectation would fail the test if it did.
		f := newAuditServiceFixture(t, nil)
		ctrl := gomock.NewController(t)
		f.service.entitlements = mocks.NewMockEntitlementProvider(ctrl)

		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{ID: "u1"}, auditMember("ws1", true), nil)
		f.repo.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, got domain.AuditLogFilter) ([]*domain.AuditLog, string, error) {
			assert.Equal(t, domain.AuditScopeWorkspace, got.Scope)
			assert.Equal(t, domain.AuditLogDefaultListLimit, got.Limit)
			return []*domain.AuditLog{{ID: "a"}}, "next", nil
		})
		res, err := f.service.List(context.Background(), filter)
		require.NoError(t, err)
		assert.Len(t, res.Logs, 1)
		assert.Equal(t, "next", res.NextCursor)
		assert.True(t, res.HasMore)
	})

	t.Run("an owner reads without the grant", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(false))
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{ID: "u1"}, &domain.UserWorkspace{Role: "owner"}, nil)
		f.repo.EXPECT().List(gomock.Any(), gomock.Any()).Return([]*domain.AuditLog{}, "", nil)
		res, err := f.service.List(context.Background(), filter)
		require.NoError(t, err)
		assert.False(t, res.HasMore)
	})

	t.Run("deployment scope is for root only", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(false))
		f.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(&domain.User{ID: "u1", Email: "ann@example.com"}, nil)
		_, err := f.service.List(context.Background(), domain.AuditLogFilter{Scope: domain.AuditScopeDeployment})
		var unauthorized *domain.ErrUnauthorized
		assert.True(t, errors.As(err, &unauthorized))

		f.auth.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(&domain.User{ID: "r1", Email: "root@example.com"}, nil)
		f.repo.EXPECT().List(gomock.Any(), gomock.Any()).Return([]*domain.AuditLog{}, "", nil)
		_, err = f.service.List(context.Background(), domain.AuditLogFilter{Scope: domain.AuditScopeDeployment})
		assert.NoError(t, err)
	})

	t.Run("an invalid filter is refused before authentication", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(true))
		_, err := f.service.List(context.Background(), domain.AuditLogFilter{})
		assert.Error(t, err)
	})

	t.Run("authentication failures propagate", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(true))
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), nil, nil, errors.New("nope"))
		_, err := f.service.List(context.Background(), filter)
		assert.Error(t, err)
	})
}

func TestAuditLogService_Get(t *testing.T) {
	f := newAuditServiceFixture(t, auditBool(false))
	f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{ID: "u1"}, &domain.UserWorkspace{Role: "owner"}, nil).Times(2)

	f.repo.EXPECT().GetByID(gomock.Any(), "a").Return(&domain.AuditLog{ID: "a", WorkspaceID: "ws1"}, nil)
	log, err := f.service.Get(context.Background(), "", "ws1", "a")
	require.NoError(t, err)
	assert.Equal(t, "a", log.ID)

	// Another workspace's row is a miss in workspace scope, not a leak.
	f.repo.EXPECT().GetByID(gomock.Any(), "b").Return(&domain.AuditLog{ID: "b", WorkspaceID: "ws2"}, nil)
	_, err = f.service.Get(context.Background(), "", "ws1", "b")
	assert.True(t, errors.Is(err, domain.ErrAuditLogNotFound))
}

func TestAuditLogService_Export(t *testing.T) {
	rowsOf := func(n int) []*domain.AuditLog {
		out := make([]*domain.AuditLog, n)
		for i := range out {
			out[i] = &domain.AuditLog{ID: "id" + strings.Repeat("x", i%3), OccurredAt: time.Date(2026, 9, 5, 0, 0, i, 0, time.UTC), WorkspaceID: "ws1",
				Action: "templates.update", Category: "templates", Outcome: "success", StatusCode: 200, ActorType: "user",
				Changes: map[string]domain.AuditChange{"name": {Old: "a", New: "b"}}}
		}
		return out
	}
	streamRows := func(f *auditServiceFixture, rows []*domain.AuditLog) {
		f.repo.EXPECT().Stream(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ domain.AuditLogFilter, fn func(*domain.AuditLog) error) error {
			for _, row := range rows {
				if err := fn(row); err != nil {
					return err
				}
			}
			return nil
		})
	}

	t.Run("csv: header, rows, flush and the audit record annotated", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(false))
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{ID: "u1"}, &domain.UserWorkspace{Role: "owner"}, nil)
		streamRows(f, rowsOf(3))

		record := domain.NewAuditRecord("", "", "")
		ctx := domain.WithAuditRecord(context.Background(), record)
		var out bytes.Buffer
		flushes := 0
		rows, truncated, err := f.service.Export(ctx, domain.ExportAuditLogsRequest{AuditLogFilter: domain.AuditLogFilter{WorkspaceID: "ws1"}}, &out, func() { flushes++ })
		require.NoError(t, err)
		assert.Equal(t, int64(3), rows)
		assert.False(t, truncated)
		assert.Equal(t, 1, flushes, "a final flush")

		parsed, err := csv.NewReader(&out).ReadAll()
		require.NoError(t, err)
		require.Len(t, parsed, 4)
		assert.Equal(t, auditExportColumns, parsed[0])
		assert.Equal(t, "templates.update", parsed[1][3])
		assert.Equal(t, "200", parsed[1][6])
		assert.Contains(t, parsed[1][19], `"name"`)

		event, _ := record.Finalize(200)
		assert.Equal(t, domain.AuditTargetAuditLog, event.TargetType)
		assert.Equal(t, "ws1", event.TargetID)
		assert.Equal(t, "csv", event.Metadata["format"])
		assert.Equal(t, int64(3), event.Metadata["rows"])
		assert.Equal(t, false, event.Metadata["truncated"])
	})

	t.Run("ndjson: one object per line", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(false))
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{ID: "u1"}, &domain.UserWorkspace{Role: "owner"}, nil)
		streamRows(f, rowsOf(2))
		var out bytes.Buffer
		rows, _, err := f.service.Export(context.Background(), domain.ExportAuditLogsRequest{AuditLogFilter: domain.AuditLogFilter{WorkspaceID: "ws1"}, Format: "ndjson"}, &out, nil)
		require.NoError(t, err)
		assert.Equal(t, int64(2), rows)
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		require.Len(t, lines, 2)
		var decoded domain.AuditLog
		require.NoError(t, json.Unmarshal([]byte(lines[0]), &decoded))
		assert.Equal(t, "templates.update", decoded.Action)
	})

	t.Run("stops at the cap and says so", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(false))
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{ID: "u1"}, &domain.UserWorkspace{Role: "owner"}, nil)
		// The stream offers more than the cap; the callback must stop it.
		f.repo.EXPECT().Stream(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ domain.AuditLogFilter, fn func(*domain.AuditLog) error) error {
			row := &domain.AuditLog{ID: "x", Action: "a", ActorType: "user"}
			for i := 0; i < domain.AuditLogExportMaxRows+5; i++ {
				if err := fn(row); err != nil {
					return err
				}
			}
			t.Fatal("the stream was not stopped at the cap")
			return nil
		})
		var out bytes.Buffer
		rows, truncated, err := f.service.Export(context.Background(), domain.ExportAuditLogsRequest{AuditLogFilter: domain.AuditLogFilter{WorkspaceID: "ws1"}, Format: "ndjson"}, &out, nil)
		require.NoError(t, err)
		assert.Equal(t, int64(domain.AuditLogExportMaxRows), rows)
		assert.True(t, truncated)
		assert.Contains(t, out.String(), `"export.truncated"`)
	})

	t.Run("a refused member gets the error before any byte", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(false))
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{ID: "u1"}, auditMember("ws1", false), nil)
		var out bytes.Buffer
		_, _, err := f.service.Export(context.Background(), domain.ExportAuditLogsRequest{AuditLogFilter: domain.AuditLogFilter{WorkspaceID: "ws1"}}, &out, nil)
		require.Error(t, err)
		assert.Empty(t, out.Bytes())
	})
}

func TestAuditLogService_Actions(t *testing.T) {
	f := newAuditServiceFixture(t, auditBool(false))
	assert.Equal(t, domain.AuditActionCatalogue(), f.service.Actions())
}

func TestAuditLogService_DeploymentRetentionDays(t *testing.T) {
	f := newAuditServiceFixture(t, auditBool(true))
	f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRetentionSettingKey).Return(nil, &domain.ErrSettingNotFound{Key: domain.AuditLogsRetentionSettingKey})
	assert.Equal(t, domain.DefaultAuditLogRetentionDays, f.service.DeploymentRetentionDays(context.Background()))

	f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRetentionSettingKey).Return(&domain.Setting{Value: "90"}, nil)
	assert.Equal(t, 90, f.service.DeploymentRetentionDays(context.Background()))

	f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRetentionSettingKey).Return(&domain.Setting{Value: "0"}, nil)
	assert.Equal(t, 0, f.service.DeploymentRetentionDays(context.Background()), "0 keeps forever")

	f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRetentionSettingKey).Return(&domain.Setting{Value: "7"}, nil)
	assert.Equal(t, domain.DefaultAuditLogRetentionDays, f.service.DeploymentRetentionDays(context.Background()), "an out-of-range value falls back")
}

func TestAuditLogService_PurgeExpired(t *testing.T) {
	days := func(n int) *int { return &n }

	t.Run("each workspace by its own retention, then deployment rows and orphans", func(t *testing.T) {
		// Licensed: the purge events are recorded.
		f := newAuditServiceFixture(t, auditBool(true))
		f.service.Init(context.Background())
		f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRetentionSettingKey).Return(&domain.Setting{Value: "365"}, nil)
		f.workspaces.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{
			{ID: "ws-default"},
			{ID: "ws-90", Settings: domain.WorkspaceSettings{AuditLogs: &domain.AuditLogSettings{RetentionDays: days(90)}}},
			{ID: "ws-forever", Settings: domain.WorkspaceSettings{AuditLogs: &domain.AuditLogSettings{RetentionDays: days(0)}}},
		}, nil)

		cutoff := func(d int) time.Time { return f.now.AddDate(0, 0, -d) }
		f.repo.EXPECT().Purge(gomock.Any(), gomock.Eq(ptrTo("ws-default")), cutoff(365)).Return(int64(2), nil)
		f.repo.EXPECT().Purge(gomock.Any(), gomock.Eq(ptrTo("ws-90")), cutoff(90)).Return(int64(0), nil)
		// ws-forever: no Purge call at all.
		f.repo.EXPECT().Purge(gomock.Any(), gomock.Nil(), cutoff(365)).Return(int64(1), nil)
		f.repo.EXPECT().PurgeOrphans(gomock.Any(), []string{"ws-default", "ws-90", "ws-forever"}, cutoff(365)).Return(int64(4), nil)

		var recorded []*domain.AuditLog
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, log *domain.AuditLog) error {
			recorded = append(recorded, log)
			return nil
		}).AnyTimes()

		total, err := f.service.PurgeExpired(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(7), total)

		require.Len(t, recorded, 3, "one audit.purged per scope that deleted something")
		assert.Equal(t, domain.AuditActionPurged, recorded[0].Action)
		assert.Equal(t, "ws-default", recorded[0].WorkspaceID)
		assert.Equal(t, int64(2), recorded[0].Metadata["deleted"])
		assert.Equal(t, 365, recorded[0].Metadata["retention_days"])
		assert.Equal(t, "deployment", recorded[1].Metadata["scope"])
		assert.Equal(t, "orphans", recorded[2].Metadata["scope"])
		assert.Equal(t, domain.AuditActorSystem, recorded[2].ActorType)
	})

	t.Run("a deployment default of 0 purges nothing beyond workspace settings", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(false))
		f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRetentionSettingKey).Return(&domain.Setting{Value: "0"}, nil)
		f.workspaces.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{
			{ID: "ws-30", Settings: domain.WorkspaceSettings{AuditLogs: &domain.AuditLogSettings{RetentionDays: days(30)}}},
		}, nil)
		f.repo.EXPECT().Purge(gomock.Any(), gomock.Eq(ptrTo("ws-30")), f.now.AddDate(0, 0, -30)).Return(int64(0), nil)
		// No deployment purge, no orphan purge, and unlicensed so no Insert.
		total, err := f.service.PurgeExpired(context.Background())
		require.NoError(t, err)
		assert.Zero(t, total)
	})

	t.Run("a failing workspace does not stop the others", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(false))
		f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRetentionSettingKey).Return(nil, errors.New("db"))
		f.workspaces.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "a"}, {ID: "b"}}, nil)
		f.repo.EXPECT().Purge(gomock.Any(), gomock.Eq(ptrTo("a")), gomock.Any()).Return(int64(0), errors.New("boom"))
		f.repo.EXPECT().Purge(gomock.Any(), gomock.Eq(ptrTo("b")), gomock.Any()).Return(int64(3), nil)
		f.repo.EXPECT().Purge(gomock.Any(), gomock.Nil(), gomock.Any()).Return(int64(0), nil)
		f.repo.EXPECT().PurgeOrphans(gomock.Any(), []string{"a", "b"}, gomock.Any()).Return(int64(0), nil)
		// The purge of "b" is recorded even though the fixture is unlicensed.
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(nil)
		total, err := f.service.PurgeExpired(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
	})

	t.Run("no workspace list, no purge at all", func(t *testing.T) {
		f := newAuditServiceFixture(t, auditBool(true))
		f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRetentionSettingKey).Return(&domain.Setting{Value: "365"}, nil)
		f.workspaces.EXPECT().List(gomock.Any()).Return(nil, errors.New("down"))
		// No Purge / PurgeOrphans expectations: an empty keep list would delete
		// every workspace row.
		_, err := f.service.PurgeExpired(context.Background())
		assert.Error(t, err)
	})
}

func ptrTo(s string) *string { return &s }

// The purge never asks the licence. A provider with no expectations fails the
// test on the first call.
func TestAuditLogService_PurgeNeverConsultsTheLicenceForDeleting(t *testing.T) {
	f := newAuditServiceFixture(t, nil)
	ctrl := gomock.NewController(t)
	provider := mocks.NewMockEntitlementProvider(ctrl)
	f.service.entitlements = provider

	f.settings.EXPECT().Get(gomock.Any(), gomock.Any()).Return(&domain.Setting{Value: "365"}, nil)
	f.workspaces.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "a"}}, nil)
	f.repo.EXPECT().Purge(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil).Times(2)
	f.repo.EXPECT().PurgeOrphans(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)

	// Nothing deleted ⇒ nothing recorded ⇒ the provider is never asked.
	_, err := f.service.PurgeExpired(context.Background())
	assert.NoError(t, err)
}

func TestAuditLogService_Init_RemembersTheRecordingState(t *testing.T) {
	t.Run("a lapse while the server was down is marked at boot", func(t *testing.T) {
		f := newAuditServiceFixtureBare(t, auditBool(false))
		f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRecordingSettingKey).Return(&domain.Setting{Value: "true"}, nil)
		var marker *domain.AuditLog
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, log *domain.AuditLog) error {
			marker = log
			return nil
		})
		f.settings.EXPECT().Set(gomock.Any(), domain.AuditLogsRecordingSettingKey, "false").Return(nil)

		f.service.Init(context.Background())

		require.NotNil(t, marker, "the last run was recording, this one is not: say so")
		assert.Equal(t, domain.AuditActionRecordingStopped, marker.Action)
		assert.Equal(t, f.now, marker.OccurredAt, "stamped at boot, not at the next request")

		// And the next Record sees no further transition.
		f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.update"})
	})

	t.Run("a key installed while the server was down is marked at boot", func(t *testing.T) {
		f := newAuditServiceFixtureBare(t, auditBool(true))
		f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRecordingSettingKey).Return(&domain.Setting{Value: "false"}, nil)
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, log *domain.AuditLog) error {
			assert.Equal(t, domain.AuditActionRecordingResumed, log.Action)
			return nil
		})
		f.settings.EXPECT().Set(gomock.Any(), domain.AuditLogsRecordingSettingKey, "true").Return(nil)
		f.service.Init(context.Background())
	})

	t.Run("no stored state: remember the current one, mark nothing", func(t *testing.T) {
		f := newAuditServiceFixtureBare(t, auditBool(true))
		f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRecordingSettingKey).Return(nil, &domain.ErrSettingNotFound{Key: domain.AuditLogsRecordingSettingKey})
		f.settings.EXPECT().Set(gomock.Any(), domain.AuditLogsRecordingSettingKey, "true").Return(nil)
		// No Insert expectation.
		f.service.Init(context.Background())
	})

	t.Run("an unreadable row leaves the state unknown: the first Record seeds it without a marker", func(t *testing.T) {
		f := newAuditServiceFixtureBare(t, auditBool(true))
		f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRecordingSettingKey).Return(nil, errors.New("connection refused"))
		f.service.Init(context.Background())

		f.settings.EXPECT().Set(gomock.Any(), domain.AuditLogsRecordingSettingKey, "true").Return(nil)
		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, log *domain.AuditLog) error {
			assert.Equal(t, "templates.update", log.Action, "no spurious resumed marker")
			return nil
		})
		f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.update"})
	})

	t.Run("a transition at runtime persists the new state", func(t *testing.T) {
		f := newAuditServiceFixtureBare(t, nil)
		licensed := true
		ctrl := gomock.NewController(t)
		provider := mocks.NewMockEntitlementProvider(ctrl)
		provider.EXPECT().Entitlements().DoAndReturn(func() domain.Entitlements {
			if licensed {
				return domain.Entitlements{Features: []domain.Feature{domain.FeatureAuditLogs}}
			}
			return domain.CommunityEntitlements()
		}).AnyTimes()
		f.service.entitlements = provider
		f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRecordingSettingKey).Return(&domain.Setting{Value: "true"}, nil)
		f.service.Init(context.Background())

		f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		f.settings.EXPECT().Set(gomock.Any(), domain.AuditLogsRecordingSettingKey, "false").Return(nil)
		licensed = false
		f.service.Record(context.Background(), &domain.AuditLog{Action: "templates.update"})
	})
}

func TestAuditLogService_PurgeIsRecordedWithoutALicence(t *testing.T) {
	f := newAuditServiceFixture(t, auditBool(false))
	f.settings.EXPECT().Get(gomock.Any(), domain.AuditLogsRetentionSettingKey).Return(&domain.Setting{Value: "365"}, nil)
	f.workspaces.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "ws-1"}}, nil)
	f.repo.EXPECT().Purge(gomock.Any(), gomock.Eq(ptrTo("ws-1")), gomock.Any()).Return(int64(12), nil)
	f.repo.EXPECT().Purge(gomock.Any(), gomock.Nil(), gomock.Any()).Return(int64(0), nil)
	f.repo.EXPECT().PurgeOrphans(gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil)

	var recorded *domain.AuditLog
	f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, log *domain.AuditLog) error {
		recorded = log
		return nil
	})

	_, err := f.service.PurgeExpired(context.Background())
	require.NoError(t, err)
	require.NotNil(t, recorded, "rows vanishing from an unlicensed log are exactly what the marker explains")
	assert.Equal(t, domain.AuditActionPurged, recorded.Action)
	assert.Equal(t, int64(12), recorded.Metadata["deleted"])
}

func TestAuditLogService_Export_NamesTheExportBeforeStreaming(t *testing.T) {
	f := newAuditServiceFixture(t, auditBool(false))
	f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(context.Background(), &domain.User{ID: "u1"}, &domain.UserWorkspace{Role: "owner"}, nil)
	f.repo.EXPECT().Stream(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("connection reset"))

	record := domain.NewAuditRecord("", "", "")
	ctx := domain.WithAuditRecord(context.Background(), record)
	var out bytes.Buffer
	_, _, err := f.service.Export(ctx, domain.ExportAuditLogsRequest{AuditLogFilter: domain.AuditLogFilter{WorkspaceID: "ws1", Actions: []string{"x"}}, Format: "ndjson"}, &out, nil)
	require.Error(t, err)

	event, _ := record.Finalize(200)
	assert.Equal(t, "ndjson", event.Metadata["format"], "set before the first byte, so a broken stream still names the export")
	assert.Equal(t, []string{"x"}, event.Metadata["actions"])
	assert.Equal(t, "ws1", event.TargetID)
}
