package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAnnotationServiceTest(t *testing.T) (
	*mocks.MockAnnotationRepository,
	*mocks.MockWorkspaceRepository,
	*mocks.MockAuthService,
	*AnnotationService,
	*gomock.Controller,
) {
	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockAnnotationRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	service := NewAnnotationService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger)

	return mockRepo, mockWorkspaceRepo, mockAuthService, service, ctrl
}

// annotationUserWorkspace builds a member (not an owner — owners bypass every
// permission check) with the given web analytics access.
func annotationUserWorkspace(workspaceID string, read, write bool) *domain.UserWorkspace {
	return &domain.UserWorkspace{
		WorkspaceID: workspaceID,
		UserID:      "user123",
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceWebAnalytics: domain.ResourcePermissions{
				Read:  read,
				Write: write,
			},
		},
	}
}

func TestAnnotationService_Create_RequiresWebAnalyticsWrite(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, false), nil)

	// The repository must not be reached at all: a read-only member is refused
	// before any workspace database is opened.
	mockRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	annotation, err := service.CreateAnnotation(ctx, &domain.CreateAnnotationRequest{
		WorkspaceID: workspaceID,
		AnnotatedAt: time.Now().UTC(),
		Title:       "Launch",
	})

	require.Error(t, err)
	assert.Nil(t, annotation)

	var permErr *domain.PermissionError
	require.True(t, errors.As(err, &permErr), "expected a *domain.PermissionError, got %T: %v", err, err)
	assert.Equal(t, domain.PermissionResourceWebAnalytics, permErr.Resource)
	assert.Equal(t, domain.PermissionTypeWrite, permErr.Permission)
	assert.Contains(t, permErr.Message, "web analytics")
}

func TestAnnotationService_List_RequiresWebAnalyticsRead(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, false, false), nil)

	mockRepo.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	annotations, err := service.ListAnnotations(ctx, &domain.ListAnnotationsRequest{WorkspaceID: workspaceID})

	require.Error(t, err)
	assert.Nil(t, annotations)

	var permErr *domain.PermissionError
	require.True(t, errors.As(err, &permErr), "expected a *domain.PermissionError, got %T: %v", err, err)
	assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)
}

func TestAnnotationService_Get_RequiresWebAnalyticsRead(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, false, false), nil)

	mockRepo.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	annotation, err := service.GetAnnotation(ctx, &domain.GetAnnotationRequest{WorkspaceID: workspaceID, ID: "a1"})

	require.Error(t, err)
	assert.Nil(t, annotation)

	var permErr *domain.PermissionError
	require.True(t, errors.As(err, &permErr), "expected a *domain.PermissionError, got %T: %v", err, err)
	assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)
}

func TestAnnotationService_Create_ForcesManualSource(t *testing.T) {
	workspaceID := "ws123"

	t.Run("a created annotation is always manual with no source id", func(t *testing.T) {
		mockRepo, mockWorkspaceRepo, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
		defer ctrl.Finish()

		ctx := context.Background()
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)
		mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
			ID:       workspaceID,
			Settings: domain.WorkspaceSettings{Timezone: "Europe/Paris"},
		}, nil).AnyTimes()

		var stored *domain.Annotation
		mockRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Annotation) error {
				stored = a
				return nil
			})

		annotation, err := service.CreateAnnotation(ctx, &domain.CreateAnnotationRequest{
			WorkspaceID: workspaceID,
			AnnotatedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
			Title:       "Launch",
			Source:      domain.AnnotationSourceManual,
		})

		require.NoError(t, err)
		require.NotNil(t, annotation)
		require.NotNil(t, stored)
		assert.Equal(t, domain.AnnotationSourceManual, stored.Source)
		assert.Nil(t, stored.SourceID)
		assert.False(t, stored.IsSystem())
		// 32-char hyphenless uuid, matching the id shape of the other workspace tables.
		assert.Len(t, stored.ID, 32)
		assert.NotContains(t, stored.ID, "-")
	})

	t.Run("a client asking for a system source is refused, not downgraded", func(t *testing.T) {
		mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
		defer ctrl.Finish()

		ctx := context.Background()
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)

		mockRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		annotation, err := service.CreateAnnotation(ctx, &domain.CreateAnnotationRequest{
			WorkspaceID: workspaceID,
			AnnotatedAt: time.Now().UTC(),
			Title:       "Fake broadcast",
			Source:      domain.AnnotationSourceBroadcast,
		})

		require.Error(t, err)
		assert.Nil(t, annotation)

		var vErr domain.ValidationError
		require.True(t, errors.As(err, &vErr), "expected a domain.ValidationError, got %T: %v", err, err)
	})
}

func TestAnnotationService_Create_DefaultsColorAndTimezone(t *testing.T) {
	mockRepo, mockWorkspaceRepo, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{Timezone: "Asia/Tokyo"},
	}, nil)

	var stored *domain.Annotation
	mockRepo.EXPECT().
		Create(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, a *domain.Annotation) error {
			stored = a
			return nil
		})

	annotation, err := service.CreateAnnotation(ctx, &domain.CreateAnnotationRequest{
		WorkspaceID: workspaceID,
		AnnotatedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
		Title:       "Launch",
	})

	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, domain.AnnotationDefaultColor, stored.Color)
	assert.Equal(t, "Asia/Tokyo", stored.Timezone)
	assert.Equal(t, stored, annotation)
}

func TestAnnotationService_Create_KeepsRequestedColorAndTimezone(t *testing.T) {
	mockRepo, mockWorkspaceRepo, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)
	// An explicit timezone must short-circuit the workspace lookup entirely.
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)

	var stored *domain.Annotation
	mockRepo.EXPECT().
		Create(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, a *domain.Annotation) error {
			stored = a
			return nil
		})

	_, err := service.CreateAnnotation(ctx, &domain.CreateAnnotationRequest{
		WorkspaceID: workspaceID,
		AnnotatedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
		Title:       "Launch",
		Color:       "#ff0000",
		Timezone:    "America/New_York",
	})

	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "#ff0000", stored.Color)
	assert.Equal(t, "America/New_York", stored.Timezone)
}

func TestAnnotationService_Create_DefaultsTimezoneToUTCWhenWorkspaceLookupFails(t *testing.T) {
	testCases := []struct {
		name      string
		workspace *domain.Workspace
		err       error
	}{
		{name: "lookup errors", workspace: nil, err: errors.New("connection refused")},
		{name: "lookup returns nil workspace", workspace: nil, err: nil},
		{
			name:      "stored timezone is not a real zone",
			workspace: &domain.Workspace{ID: "ws123", Settings: domain.WorkspaceSettings{Timezone: "Mars/Phobos"}},
			err:       nil,
		},
		{
			name:      "stored timezone is empty",
			workspace: &domain.Workspace{ID: "ws123", Settings: domain.WorkspaceSettings{}},
			err:       nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo, mockWorkspaceRepo, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
			defer ctrl.Finish()

			ctx := context.Background()
			workspaceID := "ws123"

			mockAuthService.EXPECT().
				AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
				Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)
			mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(tc.workspace, tc.err)

			var stored *domain.Annotation
			mockRepo.EXPECT().
				Create(gomock.Any(), workspaceID, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, a *domain.Annotation) error {
					stored = a
					return nil
				})

			_, err := service.CreateAnnotation(ctx, &domain.CreateAnnotationRequest{
				WorkspaceID: workspaceID,
				AnnotatedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
				Title:       "Launch",
			})

			// A workspace lookup failure must not lose the annotation: timezone is
			// display intent only, so it falls back rather than aborting.
			require.NoError(t, err)
			require.NotNil(t, stored)
			assert.Equal(t, "UTC", stored.Timezone)
		})
	}
}

func TestAnnotationService_Create_ValidationError(t *testing.T) {
	testCases := []struct {
		name string
		req  *domain.CreateAnnotationRequest
	}{
		{
			name: "bad colour",
			req: &domain.CreateAnnotationRequest{
				WorkspaceID: "ws123",
				AnnotatedAt: time.Now().UTC(),
				Title:       "Launch",
				Color:       "red",
			},
		},
		{
			name: "missing title",
			req: &domain.CreateAnnotationRequest{
				WorkspaceID: "ws123",
				AnnotatedAt: time.Now().UTC(),
			},
		},
		{
			name: "zero annotated_at",
			req: &domain.CreateAnnotationRequest{
				WorkspaceID: "ws123",
				Title:       "Launch",
			},
		},
		{
			name: "unknown timezone",
			req: &domain.CreateAnnotationRequest{
				WorkspaceID: "ws123",
				AnnotatedAt: time.Now().UTC(),
				Title:       "Launch",
				Timezone:    "Mars/Phobos",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
			defer ctrl.Finish()

			ctx := context.Background()
			mockAuthService.EXPECT().
				AuthenticateUserForWorkspace(gomock.Any(), tc.req.WorkspaceID).
				Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(tc.req.WorkspaceID, true, true), nil)

			mockRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			annotation, err := service.CreateAnnotation(ctx, tc.req)

			require.Error(t, err)
			assert.Nil(t, annotation)

			// The handler asserts on the value type, so the service must return a
			// domain.ValidationError and not a bare fmt.Errorf.
			var vErr domain.ValidationError
			require.True(t, errors.As(err, &vErr), "expected a domain.ValidationError, got %T: %v", err, err)
		})
	}
}

func TestAnnotationService_List_ValidationError(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"
	start := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	end := start.Add(-24 * time.Hour)

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)

	mockRepo.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	annotations, err := service.ListAnnotations(ctx, &domain.ListAnnotationsRequest{
		WorkspaceID: workspaceID,
		Start:       &start,
		End:         &end,
	})

	require.Error(t, err)
	assert.Nil(t, annotations)

	var vErr domain.ValidationError
	require.True(t, errors.As(err, &vErr), "expected a domain.ValidationError, got %T: %v", err, err)
}

func TestAnnotationService_List_PassesFilterAndClampsLimit(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, false), nil)

	expected := []*domain.Annotation{{ID: "a1", Title: "Launch"}}

	var filter domain.AnnotationFilter
	mockRepo.EXPECT().
		List(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, f domain.AnnotationFilter) ([]*domain.Annotation, error) {
			filter = f
			return expected, nil
		})

	annotations, err := service.ListAnnotations(ctx, &domain.ListAnnotationsRequest{
		WorkspaceID: workspaceID,
		Start:       &start,
		End:         &end,
		Sources:     []string{domain.AnnotationSourceManual},
		Limit:       100000,
	})

	require.NoError(t, err)
	assert.Equal(t, expected, annotations)
	require.NotNil(t, filter.Start)
	require.NotNil(t, filter.End)
	assert.Equal(t, start, *filter.Start)
	assert.Equal(t, end, *filter.End)
	assert.Equal(t, []string{domain.AnnotationSourceManual}, filter.Sources)
	assert.Equal(t, domain.AnnotationMaxListLimit, filter.Limit)
}

func TestAnnotationService_List_RepositoryError(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, false), nil)
	mockRepo.EXPECT().
		List(gomock.Any(), workspaceID, gomock.Any()).
		Return(nil, errors.New("database is down"))

	annotations, err := service.ListAnnotations(ctx, &domain.ListAnnotationsRequest{WorkspaceID: workspaceID})

	require.Error(t, err)
	assert.Nil(t, annotations)
	// Not a validation error and not a not-found: the handler must answer 500.
	var vErr domain.ValidationError
	assert.False(t, errors.As(err, &vErr))
	var nf *domain.ErrNotFound
	assert.False(t, errors.As(err, &nf))
}

func TestAnnotationService_Get_NotFound(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, false), nil)
	mockRepo.EXPECT().
		Get(gomock.Any(), workspaceID, "missing").
		Return(nil, &domain.ErrNotFound{Entity: "annotation", ID: "missing"})

	annotation, err := service.GetAnnotation(ctx, &domain.GetAnnotationRequest{WorkspaceID: workspaceID, ID: "missing"})

	require.Error(t, err)
	assert.Nil(t, annotation)

	// The wrapping must stay unwrappable: the handler answers 404 through errors.As.
	var nf *domain.ErrNotFound
	require.True(t, errors.As(err, &nf), "expected a *domain.ErrNotFound, got %T: %v", err, err)
	assert.Equal(t, "annotation", nf.Entity)
}

func TestAnnotationService_Update_NotFound(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)
	mockRepo.EXPECT().
		Get(gomock.Any(), workspaceID, "missing").
		Return(nil, &domain.ErrNotFound{Entity: "annotation", ID: "missing"})
	mockRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	annotation, err := service.UpdateAnnotation(ctx, &domain.UpdateAnnotationRequest{
		WorkspaceID: workspaceID,
		ID:          "missing",
		AnnotatedAt: time.Now().UTC(),
		Title:       "Launch",
	})

	require.Error(t, err)
	assert.Nil(t, annotation)

	var nf *domain.ErrNotFound
	require.True(t, errors.As(err, &nf), "expected a *domain.ErrNotFound, got %T: %v", err, err)
}

func TestAnnotationService_Delete_NotFound(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)
	mockRepo.EXPECT().
		Delete(gomock.Any(), workspaceID, "missing").
		Return(&domain.ErrNotFound{Entity: "annotation", ID: "missing"})

	err := service.DeleteAnnotation(ctx, &domain.DeleteAnnotationRequest{WorkspaceID: workspaceID, ID: "missing"})

	require.Error(t, err)

	var nf *domain.ErrNotFound
	require.True(t, errors.As(err, &nf), "expected a *domain.ErrNotFound, got %T: %v", err, err)
}

func TestAnnotationService_Update_PreservesSource(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "broadcast42"
	createdAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)
	mockRepo.EXPECT().
		Get(gomock.Any(), workspaceID, "a1").
		Return(&domain.Annotation{
			ID:          "a1",
			AnnotatedAt: createdAt,
			Timezone:    "Asia/Tokyo",
			Title:       "Summer sale",
			Color:       domain.AnnotationBroadcastColor,
			Source:      domain.AnnotationSourceBroadcast,
			SourceID:    &broadcastID,
			CreatedAt:   createdAt,
			UpdatedAt:   createdAt,
		}, nil)

	var stored *domain.Annotation
	mockRepo.EXPECT().
		Update(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, a *domain.Annotation) error {
			stored = a
			return nil
		})

	newAt := time.Date(2026, 8, 2, 8, 0, 0, 0, time.UTC)
	annotation, err := service.UpdateAnnotation(ctx, &domain.UpdateAnnotationRequest{
		WorkspaceID: workspaceID,
		ID:          "a1",
		AnnotatedAt: newAt,
		Title:       "Summer sale (reworded)",
	})

	require.NoError(t, err)
	require.NotNil(t, stored)
	// The edit changes presentation only; the origin is reloaded from storage so a
	// client cannot promote a manual row or steal another broadcast's slot.
	assert.Equal(t, domain.AnnotationSourceBroadcast, stored.Source)
	require.NotNil(t, stored.SourceID)
	assert.Equal(t, broadcastID, *stored.SourceID)
	assert.Equal(t, "Summer sale (reworded)", stored.Title)
	assert.Equal(t, newAt, stored.AnnotatedAt)
	// Unset optional fields fall back to what is stored rather than to the defaults.
	assert.Equal(t, domain.AnnotationBroadcastColor, stored.Color)
	assert.Equal(t, "Asia/Tokyo", stored.Timezone)
	assert.Equal(t, createdAt, stored.CreatedAt)
	assert.True(t, stored.UpdatedAt.After(createdAt))
	assert.Equal(t, stored, annotation)
}

func TestAnnotationService_Update_OverridesStoredFields(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"
	createdAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)
	mockRepo.EXPECT().
		Get(gomock.Any(), workspaceID, "a1").
		Return(&domain.Annotation{
			ID:          "a1",
			AnnotatedAt: createdAt,
			Timezone:    "Asia/Tokyo",
			Title:       "Summer sale",
			Description: "Stored description",
			Color:       "#111111",
			Source:      domain.AnnotationSourceManual,
			CreatedAt:   createdAt,
			UpdatedAt:   createdAt,
		}, nil)

	var stored *domain.Annotation
	mockRepo.EXPECT().
		Update(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, a *domain.Annotation) error {
			stored = a
			return nil
		})

	newAt := time.Date(2026, 8, 2, 8, 0, 0, 0, time.UTC)
	annotation, err := service.UpdateAnnotation(ctx, &domain.UpdateAnnotationRequest{
		WorkspaceID: workspaceID,
		ID:          "a1",
		AnnotatedAt: newAt,
		Timezone:    "Europe/Paris",
		Title:       "Autumn sale",
		Description: "Requested description",
		Color:       "#ff0000",
	})

	require.NoError(t, err)
	require.NotNil(t, stored)
	// Every field the request supplies wins over the stored row: the fall-back only
	// applies to an omitted field. Swapping either precedence would make an edit
	// silently revert to the value the operator just replaced.
	assert.Equal(t, "#ff0000", stored.Color)
	assert.Equal(t, "Europe/Paris", stored.Timezone)
	assert.Equal(t, "Requested description", stored.Description)
	assert.Equal(t, "Autumn sale", stored.Title)
	assert.Equal(t, newAt, stored.AnnotatedAt)
	assert.Equal(t, createdAt, stored.CreatedAt)
	assert.Equal(t, stored, annotation)
}

func TestAnnotationService_Update_ClearsDescription(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"
	createdAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)
	mockRepo.EXPECT().
		Get(gomock.Any(), workspaceID, "a1").
		Return(&domain.Annotation{
			ID:          "a1",
			AnnotatedAt: createdAt,
			Timezone:    "Asia/Tokyo",
			Title:       "Summer sale",
			Description: "Stored description",
			Color:       "#111111",
			Source:      domain.AnnotationSourceManual,
			CreatedAt:   createdAt,
			UpdatedAt:   createdAt,
		}, nil)

	var stored *domain.Annotation
	mockRepo.EXPECT().
		Update(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, a *domain.Annotation) error {
			stored = a
			return nil
		})

	annotation, err := service.UpdateAnnotation(ctx, &domain.UpdateAnnotationRequest{
		WorkspaceID: workspaceID,
		ID:          "a1",
		AnnotatedAt: createdAt,
		Title:       "Summer sale",
	})

	require.NoError(t, err)
	require.NotNil(t, stored)
	// Description does NOT fall back to the stored value, and the asymmetry with
	// color and timezone is deliberate: empty is a description an operator can
	// legitimately want, so sending it is the only way to clear one. Color and
	// timezone have no such empty form — Annotation.Validate rejects both blank —
	// which is why those two fall back instead. Making the three "consistent"
	// would leave a description permanently uneditable once set.
	assert.Empty(t, stored.Description)
	assert.Equal(t, "#111111", stored.Color)
	assert.Equal(t, "Asia/Tokyo", stored.Timezone)
	assert.Equal(t, stored, annotation)
}

func TestAnnotationService_Update_ValidationError(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)
	mockRepo.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	annotation, err := service.UpdateAnnotation(ctx, &domain.UpdateAnnotationRequest{
		WorkspaceID: workspaceID,
		ID:          "a1",
		AnnotatedAt: time.Now().UTC(),
		Title:       "Launch",
		Color:       "#GGGGGG",
	})

	require.Error(t, err)
	assert.Nil(t, annotation)

	var vErr domain.ValidationError
	require.True(t, errors.As(err, &vErr), "expected a domain.ValidationError, got %T: %v", err, err)
}

func TestAnnotationService_Delete_AllowsSystemRows(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user123"}, annotationUserWorkspace(workspaceID, true, true), nil)
	// No pre-read of the row: a broadcast annotation is deletable like any other,
	// because its broadcast has already started and cannot un-start.
	mockRepo.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().Delete(gomock.Any(), workspaceID, "a1").Return(nil)

	require.NoError(t, service.DeleteAnnotation(ctx, &domain.DeleteAnnotationRequest{
		WorkspaceID: workspaceID,
		ID:          "a1",
	}))
}

// --- Broadcast subscriber -------------------------------------------------

func TestAnnotationService_RegisterWithEventBus(t *testing.T) {
	t.Run("subscribes to the sending-started event", func(t *testing.T) {
		_, _, _, service, ctrl := setupAnnotationServiceTest(t)
		defer ctrl.Finish()

		mockEventBus := mocks.NewMockEventBus(ctrl)
		mockEventBus.EXPECT().Subscribe(domain.EventBroadcastSendingStarted, gomock.Any())

		service.RegisterWithEventBus(mockEventBus)
	})

	t.Run("a nil bus is refused rather than panicking", func(t *testing.T) {
		_, _, _, service, ctrl := setupAnnotationServiceTest(t)
		defer ctrl.Finish()

		assert.NotPanics(t, func() { service.RegisterWithEventBus(nil) })
	})
}

func TestAnnotationService_HandleBroadcastSendingStarted_CreatesAnnotation(t *testing.T) {
	mockRepo, mockWorkspaceRepo, _, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"
	broadcastID := "broadcast42"
	startedAt := time.Date(2026, 8, 15, 9, 30, 0, 0, time.UTC)

	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{Timezone: "Asia/Tokyo"},
	}, nil)

	var stored *domain.Annotation
	mockRepo.EXPECT().
		CreateFromSource(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, a *domain.Annotation) (bool, error) {
			stored = a
			return true, nil
		})

	service.HandleBroadcastSendingStarted(ctx, domain.EventPayload{
		Type:        domain.EventBroadcastSendingStarted,
		WorkspaceID: workspaceID,
		EntityID:    broadcastID,
		Data: map[string]interface{}{
			"broadcast_name": "Summer sale",
			"started_at":     startedAt.Format(time.RFC3339),
		},
	})

	require.NotNil(t, stored)
	assert.Equal(t, domain.AnnotationSourceBroadcast, stored.Source)
	require.NotNil(t, stored.SourceID)
	assert.Equal(t, broadcastID, *stored.SourceID)
	assert.Equal(t, "Summer sale", stored.Title)
	assert.True(t, stored.AnnotatedAt.Equal(startedAt), "expected %v, got %v", startedAt, stored.AnnotatedAt)
	assert.Equal(t, domain.AnnotationBroadcastColor, stored.Color)
	assert.Equal(t, "Asia/Tokyo", stored.Timezone)
	assert.True(t, stored.IsSystem())
	assert.NoError(t, stored.Validate())
}

func TestAnnotationService_HandleBroadcastSendingStarted_TruncatesLongBroadcastName(t *testing.T) {
	mockRepo, mockWorkspaceRepo, _, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	// 150 CJK characters: 450 bytes. A byte-slicing truncation would keep 100 bytes
	// — 33 characters and a rune cut in half — so this pins the rune-based one.
	name := strings.Repeat("測", 150)

	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{Timezone: "UTC"},
	}, nil)

	var stored *domain.Annotation
	mockRepo.EXPECT().
		CreateFromSource(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, a *domain.Annotation) (bool, error) {
			stored = a
			return true, nil
		})

	service.HandleBroadcastSendingStarted(ctx, domain.EventPayload{
		WorkspaceID: workspaceID,
		EntityID:    "broadcast42",
		Data:        map[string]interface{}{"broadcast_name": name},
	})

	require.NotNil(t, stored)
	assert.Equal(t, domain.AnnotationMaxTitleLength, utf8.RuneCountInString(stored.Title))
	assert.Equal(t, strings.Repeat("測", domain.AnnotationMaxTitleLength), stored.Title)
	assert.True(t, utf8.ValidString(stored.Title), "truncation cut a rune in half")
	assert.NoError(t, stored.Validate())
}

func TestAnnotationService_HandleBroadcastSendingStarted_Idempotent(t *testing.T) {
	mockRepo, mockWorkspaceRepo, _, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{Timezone: "UTC"},
	}, nil)

	// created=false is the ON CONFLICT path: the broadcast was already annotated by
	// an earlier run that republished. It is a no-op, not a failure.
	mockRepo.EXPECT().
		CreateFromSource(gomock.Any(), workspaceID, gomock.Any()).
		Return(false, nil)

	assert.NotPanics(t, func() {
		service.HandleBroadcastSendingStarted(ctx, domain.EventPayload{
			WorkspaceID: workspaceID,
			EntityID:    "broadcast42",
			Data:        map[string]interface{}{"broadcast_name": "Summer sale"},
		})
	})
}

func TestAnnotationService_HandleBroadcastSendingStarted_MissingStartedAt(t *testing.T) {
	testCases := []struct {
		name string
		data map[string]interface{}
	}{
		{name: "absent", data: map[string]interface{}{"broadcast_name": "Summer sale"}},
		{name: "empty", data: map[string]interface{}{"broadcast_name": "Summer sale", "started_at": ""}},
		{name: "unparseable", data: map[string]interface{}{"broadcast_name": "Summer sale", "started_at": "yesterday"}},
		{name: "not a string", data: map[string]interface{}{"broadcast_name": "Summer sale", "started_at": 1755250000}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo, mockWorkspaceRepo, _, service, ctrl := setupAnnotationServiceTest(t)
			defer ctrl.Finish()

			ctx := context.Background()
			workspaceID := "ws123"

			mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
				ID:       workspaceID,
				Settings: domain.WorkspaceSettings{Timezone: "UTC"},
			}, nil)

			var stored *domain.Annotation
			mockRepo.EXPECT().
				CreateFromSource(gomock.Any(), workspaceID, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, a *domain.Annotation) (bool, error) {
					stored = a
					return true, nil
				})

			before := time.Now().UTC().Add(-time.Second)
			service.HandleBroadcastSendingStarted(ctx, domain.EventPayload{
				WorkspaceID: workspaceID,
				EntityID:    "broadcast42",
				Data:        tc.data,
			})
			after := time.Now().UTC().Add(time.Second)

			require.NotNil(t, stored)
			// Falls back to now rather than to the zero time, which Validate rejects.
			assert.False(t, stored.AnnotatedAt.IsZero())
			assert.True(t, stored.AnnotatedAt.After(before) && stored.AnnotatedAt.Before(after),
				"expected a timestamp near now, got %v", stored.AnnotatedAt)
			assert.NoError(t, stored.Validate())
		})
	}
}

func TestAnnotationService_HandleBroadcastSendingStarted_MissingBroadcastName(t *testing.T) {
	mockRepo, mockWorkspaceRepo, _, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{Timezone: "UTC"},
	}, nil)

	var stored *domain.Annotation
	mockRepo.EXPECT().
		CreateFromSource(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, a *domain.Annotation) (bool, error) {
			stored = a
			return true, nil
		})

	service.HandleBroadcastSendingStarted(ctx, domain.EventPayload{
		WorkspaceID: workspaceID,
		EntityID:    "broadcast42",
	})

	require.NotNil(t, stored)
	// A blank title would fail validation and lose the annotation entirely.
	assert.NotEmpty(t, stored.Title)
	assert.NoError(t, stored.Validate())
}

func TestAnnotationService_HandleBroadcastSendingStarted_IncompletePayload(t *testing.T) {
	testCases := []struct {
		name    string
		payload domain.EventPayload
	}{
		{name: "no workspace", payload: domain.EventPayload{EntityID: "broadcast42"}},
		{name: "no entity", payload: domain.EventPayload{WorkspaceID: "ws123"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo, mockWorkspaceRepo, _, service, ctrl := setupAnnotationServiceTest(t)
			defer ctrl.Finish()

			mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)
			mockRepo.EXPECT().CreateFromSource(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			assert.NotPanics(t, func() {
				service.HandleBroadcastSendingStarted(context.Background(), tc.payload)
			})
		})
	}
}

func TestAnnotationService_HandleBroadcastSendingStarted_RepoError(t *testing.T) {
	mockRepo, mockWorkspaceRepo, _, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "ws123"

	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{Timezone: "UTC"},
	}, nil)
	mockRepo.EXPECT().
		CreateFromSource(gomock.Any(), workspaceID, gomock.Any()).
		Return(false, errors.New("database is down"))

	// Logged and swallowed: the handler returns nothing, and an annotation must
	// never be able to fail a send.
	assert.NotPanics(t, func() {
		service.HandleBroadcastSendingStarted(ctx, domain.EventPayload{
			WorkspaceID: workspaceID,
			EntityID:    "broadcast42",
			Data:        map[string]interface{}{"broadcast_name": "Summer sale"},
		})
	})
}

func TestAnnotationService_HandleBroadcastSendingStarted_DoesNotAuthenticate(t *testing.T) {
	mockRepo, mockWorkspaceRepo, mockAuthService, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	workspaceID := "ws123"

	// The bus calls this on behalf of the platform: there is no user in the
	// context and authorize has no system-call bypass, so any authentication
	// attempt here would drop every automatic annotation.
	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).Times(0)

	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{Timezone: "UTC"},
	}, nil)
	mockRepo.EXPECT().CreateFromSource(gomock.Any(), workspaceID, gomock.Any()).Return(true, nil)

	service.HandleBroadcastSendingStarted(context.Background(), domain.EventPayload{
		WorkspaceID: workspaceID,
		EntityID:    "broadcast42",
		Data:        map[string]interface{}{"broadcast_name": "Summer sale"},
	})
}

func TestAnnotationService_HandleBroadcastSendingStarted_SurvivesCancelledPublisherContext(t *testing.T) {
	mockRepo, mockWorkspaceRepo, _, service, ctrl := setupAnnotationServiceTest(t)
	defer ctrl.Finish()

	workspaceID := "ws123"

	// The publisher's context can be gone before the detached handler runs; the
	// annotation must still be written.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), workspaceID).
		DoAndReturn(func(handlerCtx context.Context, _ string) (*domain.Workspace, error) {
			require.NoError(t, handlerCtx.Err(), "handler context inherited the publisher's cancellation")
			return &domain.Workspace{ID: workspaceID, Settings: domain.WorkspaceSettings{Timezone: "UTC"}}, nil
		})

	var stored *domain.Annotation
	mockRepo.EXPECT().
		CreateFromSource(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(handlerCtx context.Context, _ string, a *domain.Annotation) (bool, error) {
			require.NoError(t, handlerCtx.Err(), "handler context inherited the publisher's cancellation")
			stored = a
			return true, nil
		})

	service.HandleBroadcastSendingStarted(ctx, domain.EventPayload{
		WorkspaceID: workspaceID,
		EntityID:    "broadcast42",
		Data:        map[string]interface{}{"broadcast_name": "Summer sale"},
	})

	require.NotNil(t, stored)
}
