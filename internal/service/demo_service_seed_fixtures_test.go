package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// SeedFixtures replaces the fixtures workspace and nothing else. The real
// UserService and WorkspaceService run over strict mock repositories: an
// unexpected Delete, DeleteAll or List fails the test. Most cases stop the seed at
// createDemoWorkspace's root user lookup (made to fail with errRootLookup), so
// reaching it proves the seed went on to create and not reaching it proves it
// stopped.

var errRootLookup = errors.New("root lookup reached")

// fixturesCreatedAt is the creation time of the "demo" workspace in these tests, with
// the nanoseconds PostgreSQL drops, so the marker comparison must truncate to match.
var fixturesCreatedAt = time.Date(2026, 11, 15, 4, 0, 0, 123456789, time.UTC)

var demoRoot = &domain.User{ID: "root-1", Email: "root@example.com", Type: domain.UserTypeAPIKey}

type seedFixturesGraph struct {
	svc           *DemoService
	userRepo      *mocks.MockUserRepository
	workspaceRepo *mocks.MockWorkspaceRepository
	taskRepo      *mocks.MockTaskRepository
	settingRepo   *mocks.MockSettingRepository
	authService   *mocks.MockAuthService
}

func newSeedFixturesGraph(t *testing.T) seedFixturesGraph {
	t.Helper()
	ctrl := gomock.NewController(t)
	log := logger.NewLoggerWithLevel("disabled")

	g := seedFixturesGraph{
		userRepo:      mocks.NewMockUserRepository(ctrl),
		workspaceRepo: mocks.NewMockWorkspaceRepository(ctrl),
		taskRepo:      mocks.NewMockTaskRepository(ctrl),
		settingRepo:   mocks.NewMockSettingRepository(ctrl),
		authService:   mocks.NewMockAuthService(ctrl),
	}
	userService, err := NewUserService(UserServiceConfig{Repository: g.userRepo, Logger: log})
	require.NoError(t, err)
	cfg := &config.Config{RootEmail: "root@example.com"}
	workspaceService := NewWorkspaceService(
		g.workspaceRepo, nil, g.taskRepo, log, mocks.NewMockUserServiceInterface(ctrl), g.authService, nil, cfg,
		mocks.NewMockContactService(ctrl), nil, nil, nil, nil, "", nil, nil, nil, nil,
	)
	g.svc = &DemoService{
		logger:           log,
		config:           cfg,
		userService:      userService,
		workspaceService: workspaceService,
		workspaceRepo:    g.workspaceRepo,
		taskRepo:         g.taskRepo,
		settingRepo:      g.settingRepo,
		authService:      g.authService,
	}
	return g
}

func existingDemo() *domain.Workspace {
	return &domain.Workspace{ID: "demo", CreatedAt: fixturesCreatedAt}
}

func marker(value string) *domain.Setting {
	return &domain.Setting{Key: demoFixturesSettingKey, Value: value}
}

func (g seedFixturesGraph) expectCreateAttempt() *gomock.Call {
	return g.userRepo.EXPECT().GetUserByEmail(gomock.Any(), "root@example.com").Return(nil, errRootLookup).Times(1)
}

// expectOwnerDelete expects WorkspaceService.DeleteWorkspace on "demo" as root:
// tasks first, then the workspace.
func (g seedFixturesGraph) expectOwnerDelete(deleteErr error) []*gomock.Call {
	owner := &domain.UserWorkspace{UserID: demoRoot.ID, WorkspaceID: "demo", Role: "owner"}
	return []*gomock.Call{
		g.userRepo.EXPECT().GetUserByEmail(gomock.Any(), "root@example.com").Return(demoRoot, nil).Times(1),
		g.authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "demo").
			Return(context.Background(), demoRoot, owner, nil).Times(1),
		g.workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").Return(existingDemo(), nil).Times(1),
		g.taskRepo.EXPECT().DeleteAll(gomock.Any(), "demo").Return(nil).Times(1),
		g.workspaceRepo.EXPECT().Delete(gomock.Any(), "demo").Return(deleteErr).Times(1),
	}
}

func TestDemoService_SeedFixtures_NoWorkspaceCreatesIt(t *testing.T) {
	g := newSeedFixturesGraph(t)
	g.workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").
		Return(nil, &domain.ErrWorkspaceNotFound{WorkspaceID: "demo"}).Times(1)
	g.expectCreateAttempt()

	err := g.svc.SeedFixtures(context.Background())

	assert.ErrorIs(t, err, errRootLookup, "with nothing to replace the seed goes straight to creating")
}

func TestDemoService_SeedFixtures_MarkedWorkspaceIsReplaced(t *testing.T) {
	testCases := []struct {
		name   string
		marker string
	}{
		{"final marker: the exact creation time", "2026-11-15T04:00:00.123456Z"},
		// The seed that created it failed between the insert and the final marker.
		{"pending marker: written just before the creation", "2026-11-15T03:59:58.5Z"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := newSeedFixturesGraph(t)
			calls := []*gomock.Call{
				g.workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").Return(existingDemo(), nil).Times(1),
				g.settingRepo.EXPECT().Get(gomock.Any(), demoFixturesSettingKey).Return(marker(tc.marker), nil).Times(1),
			}
			calls = append(calls, g.expectOwnerDelete(nil)...)
			calls = append(calls, g.expectCreateAttempt())
			gomock.InOrder(calls...)

			err := g.svc.SeedFixtures(context.Background())

			assert.ErrorIs(t, err, errRootLookup, "the old fixtures are deleted, then the new ones created")
		})
	}
}

func TestDemoService_SeedFixtures_RefusesAWorkspaceItDidNotCreate(t *testing.T) {
	testCases := []struct {
		name   string
		marker func(*mocks.MockSettingRepository)
	}{
		{"no marker", func(r *mocks.MockSettingRepository) {
			r.EXPECT().Get(gomock.Any(), demoFixturesSettingKey).
				Return(nil, &domain.ErrSettingNotFound{Key: demoFixturesSettingKey}).Times(1)
		}},
		// The fixtures workspace was deleted by hand and a real "demo" created later: the
		// marker still exists but holds the old workspace's creation time.
		{"marker from an earlier workspace", func(r *mocks.MockSettingRepository) {
			r.EXPECT().Get(gomock.Any(), demoFixturesSettingKey).
				Return(marker("2026-11-14T04:00:00.123456Z"), nil).Times(1)
		}},
		{"marker just past the window", func(r *mocks.MockSettingRepository) {
			r.EXPECT().Get(gomock.Any(), demoFixturesSettingKey).
				Return(marker(fixturesCreatedAt.Truncate(time.Microsecond).Add(-demoFixturesMarkerWindow-time.Microsecond).Format(time.RFC3339Nano)), nil).Times(1)
		}},
		{"marker later than the workspace", func(r *mocks.MockSettingRepository) {
			r.EXPECT().Get(gomock.Any(), demoFixturesSettingKey).
				Return(marker("2026-11-15T04:00:01Z"), nil).Times(1)
		}},
		{"marker this code did not write", func(r *mocks.MockSettingRepository) {
			r.EXPECT().Get(gomock.Any(), demoFixturesSettingKey).Return(marker("demo"), nil).Times(1)
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := newSeedFixturesGraph(t)
			g.workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").Return(existingDemo(), nil).Times(1)
			tc.marker(g.settingRepo)
			// No Delete, DeleteAll or GetUserByEmail expected: the strict mocks fail on any.

			err := g.svc.SeedFixtures(context.Background())

			assert.ErrorIs(t, err, ErrDemoWorkspaceNotFixtures)
		})
	}
}

func TestDemoService_SeedFixtures_StopsOnLookupFailures(t *testing.T) {
	t.Run("workspace lookup", func(t *testing.T) {
		g := newSeedFixturesGraph(t)
		sentinel := errors.New("system db down")
		g.workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").Return(nil, sentinel).Times(1)

		err := g.svc.SeedFixtures(context.Background())

		require.ErrorIs(t, err, sentinel)
		assert.ErrorContains(t, err, "failed to look up the fixtures workspace")
	})

	t.Run("marker read", func(t *testing.T) {
		g := newSeedFixturesGraph(t)
		sentinel := errors.New("settings unreadable")
		g.workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").Return(existingDemo(), nil).Times(1)
		g.settingRepo.EXPECT().Get(gomock.Any(), demoFixturesSettingKey).Return(nil, sentinel).Times(1)

		err := g.svc.SeedFixtures(context.Background())

		require.ErrorIs(t, err, sentinel)
		assert.NotErrorIs(t, err, ErrDemoWorkspaceNotFixtures,
			"an unreadable marker is a failure, not a verdict on the workspace")
	})
}

func TestDemoService_SeedFixtures_StopsWhenTheOldFixturesSurvive(t *testing.T) {
	g := newSeedFixturesGraph(t)
	g.workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").Return(existingDemo(), nil).Times(1)
	g.settingRepo.EXPECT().Get(gomock.Any(), demoFixturesSettingKey).
		Return(marker("2026-11-15T04:00:00.123456Z"), nil).Times(1)
	sentinel := errors.New("drop database failed")
	g.expectOwnerDelete(sentinel)
	// No create attempt expected.

	err := g.svc.SeedFixtures(context.Background())

	require.ErrorIs(t, err, sentinel)
	assert.ErrorContains(t, err, "failed to delete the previous fixtures workspace")
}

// The pending marker goes in before CreateWorkspace. When the creation is refused
// because a workspace is already in the way, that workspace is not the seed's own,
// so the marker that was there before is put back.
func TestDemoService_CreateDemoWorkspace_AlreadyExistsRestoresTheMarker(t *testing.T) {
	testCases := []struct {
		name     string
		previous *domain.Setting
	}{
		{"previous marker is put back", marker("2026-11-14T04:00:00Z")},
		{"no previous marker: the pending one is removed", nil},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := newSeedFixturesGraph(t)
			g.userRepo.EXPECT().GetUserByEmail(gomock.Any(), "root@example.com").Return(demoRoot, nil).Times(1)
			g.authService.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(demoRoot, nil).Times(1)
			g.workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").Return(existingDemo(), nil).Times(1)

			var get *gomock.Call
			if tc.previous == nil {
				get = g.settingRepo.EXPECT().Get(gomock.Any(), demoFixturesSettingKey).
					Return(nil, &domain.ErrSettingNotFound{Key: demoFixturesSettingKey}).Times(1)
			} else {
				get = g.settingRepo.EXPECT().Get(gomock.Any(), demoFixturesSettingKey).Return(tc.previous, nil).Times(1)
			}
			pending := g.settingRepo.EXPECT().Set(gomock.Any(), demoFixturesSettingKey, gomock.Any()).Return(nil).Times(1)
			var restore *gomock.Call
			if tc.previous == nil {
				restore = g.settingRepo.EXPECT().Delete(gomock.Any(), demoFixturesSettingKey).Return(nil).Times(1)
			} else {
				restore = g.settingRepo.EXPECT().Set(gomock.Any(), demoFixturesSettingKey, tc.previous.Value).Return(nil).Times(1)
			}
			gomock.InOrder(get, pending, restore)

			err := g.svc.createDemoWorkspace(context.Background())

			require.ErrorIs(t, err, domain.ErrWorkspaceAlreadyExists)
		})
	}
}

// A caller giving up must not leave a created workspace unmarked: the marker writes
// run on a context the request's cancellation does not reach.
func TestDemoService_CreateDemoWorkspace_MarkerOutlivesTheRequest(t *testing.T) {
	g := newSeedFixturesGraph(t)
	ctx, cancel := context.WithCancel(context.Background())
	g.userRepo.EXPECT().GetUserByEmail(gomock.Any(), "root@example.com").Return(demoRoot, nil).Times(1)
	g.settingRepo.EXPECT().Get(gomock.Any(), demoFixturesSettingKey).
		Return(nil, &domain.ErrSettingNotFound{Key: demoFixturesSettingKey}).Times(1)
	g.settingRepo.EXPECT().Set(gomock.Any(), demoFixturesSettingKey, gomock.Any()).
		DoAndReturn(func(markCtx context.Context, _, _ string) error {
			cancel() // the caller gives up as the marker is written
			assert.NoError(t, markCtx.Err(), "the marker write must not see the request's cancellation")
			return errors.New("stop here")
		}).Times(1)

	err := g.svc.createDemoWorkspace(ctx)

	assert.ErrorContains(t, err, "failed to mark the demo workspace as fixtures")
}
