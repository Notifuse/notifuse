package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renewedAfterTree() *domain.TreeNode {
	return diagLeaf(&domain.DimensionFilter{FieldName: "custom_json_1", FieldType: "time", Operator: "after_date",
		JSONPath: []string{"renewed_at"}, StringValues: []string{"2026-01-01T00:00:00Z"}})
}

func TestSegmentService_PreviewSegmentValueError(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (*SegmentService, *mocks.MockSegmentRepository, *mocks.MockWorkspaceRepository) {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockSegmentRepository(ctrl)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		svc := NewSegmentService(repo, workspaceRepo, mocks.NewMockTaskService(ctrl), newPermissiveAuthService(ctrl), newSegmentTestLogger(ctrl))
		return svc, repo, workspaceRepo
	}
	conversionErr := fmt.Errorf("failed to execute preview count query: %w", outOfRange)

	t.Run("a value that does not convert names the contact", func(t *testing.T) {
		svc, repo, workspaceRepo := setup(t)
		db, mock := newDiagnosisMock(t)
		repo.EXPECT().PreviewSegment(ctx, "w1", gomock.Any(), gomock.Any(), 10).Return(0, conversionErr)
		workspaceRepo.EXPECT().GetConnection(ctx, "w1").Return(db, nil)
		mock.ExpectQuery(`SELECT email, ` + renewedText + ` FROM contacts WHERE ` + renewedText + ` IS NOT NULL AND NOT pg_input_is_valid(` + renewedText + `, 'timestamptz') AND ` + renewedText + ` = $1 LIMIT 1`).
			WithArgs("2026-02-30").
			WillReturnRows(sqlmock.NewRows([]string{"email", "v"}).AddRow("jane@example.com", "2026-02-30"))

		response, err := svc.PreviewSegment(ctx, "w1", renewedAfterTree(), 10)
		assert.Nil(t, response)
		var valueErr *domain.SegmentValueError
		require.True(t, errors.As(err, &valueErr), "got %v", err)
		assert.Equal(t, &domain.SegmentValueError{Column: "custom_json_1", Path: []string{"renewed_at"},
			Type: domain.SegmentValueTypeDate, Email: "jane@example.com", Value: "2026-02-30", Cause: outOfRange.Message}, valueErr)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no filter that casts a JSON value: the error as before", func(t *testing.T) {
		svc, repo, workspaceRepo := setup(t)
		db, _ := newDiagnosisMock(t)
		repo.EXPECT().PreviewSegment(ctx, "w1", gomock.Any(), gomock.Any(), 10).Return(0, conversionErr)
		workspaceRepo.EXPECT().GetConnection(ctx, "w1").Return(db, nil)

		_, err := svc.PreviewSegment(ctx, "w1", createTestTree(), 10)
		require.Error(t, err)
		var valueErr *domain.SegmentValueError
		assert.False(t, errors.As(err, &valueErr))
		assert.Contains(t, err.Error(), "failed to preview segment")
	})

	t.Run("another database error is not diagnosed", func(t *testing.T) {
		svc, repo, _ := setup(t)
		repo.EXPECT().PreviewSegment(ctx, "w1", gomock.Any(), gomock.Any(), 10).
			Return(0, fmt.Errorf("wrapped: %w", &pq.Error{Code: "22021", Message: "invalid byte sequence"}))

		_, err := svc.PreviewSegment(ctx, "w1", renewedAfterTree(), 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to preview segment")
	})

	t.Run("no connection for the diagnosis: the error as before", func(t *testing.T) {
		svc, repo, workspaceRepo := setup(t)
		repo.EXPECT().PreviewSegment(ctx, "w1", gomock.Any(), gomock.Any(), 10).Return(0, conversionErr)
		workspaceRepo.EXPECT().GetConnection(ctx, "w1").Return(nil, errors.New("no db"))

		_, err := svc.PreviewSegment(ctx, "w1", renewedAfterTree(), 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to preview segment")
	})
}

// segmentWithLastError is a segment whose last build failed on a contact's value.
func segmentWithLastError() *domain.Segment {
	at := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	return &domain.Segment{
		ID: "seg_1", Name: "Renewed", Color: "#000", Tree: renewedAfterTree(), Timezone: "UTC", Version: 2,
		Status: string(domain.SegmentStatusActive),
		LastError: &domain.SegmentLastError{
			Kind: domain.SegmentLastErrorKindValueConversion, Column: "custom_json_1", Path: []string{"renewed_at"},
			Type: domain.SegmentValueTypeDate, Email: "jane@example.com", Value: "2026-02-30",
			Cause: outOfRange.Message, Source: domain.SegmentLastErrorSourceBuild,
		},
		LastErrorAt: &at,
	}
}

func redactedLastError() *domain.SegmentLastError {
	return &domain.SegmentLastError{
		Kind: domain.SegmentLastErrorKindValueConversion, Column: "custom_json_1", Path: []string{"renewed_at"},
		Type: domain.SegmentValueTypeDate, Source: domain.SegmentLastErrorSourceBuild,
	}
}

func TestSegmentService_LastErrorRedaction(t *testing.T) {
	ctx := context.Background()
	member := func(contactsRead bool) *domain.UserWorkspace {
		return &domain.UserWorkspace{UserID: "user-1", WorkspaceID: "w1", Role: "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceSegments: {Read: true, Write: true},
				domain.PermissionResourceContacts: {Read: contactsRead},
			}}
	}
	setup := func(t *testing.T, grant *domain.UserWorkspace) (*SegmentService, *mocks.MockSegmentRepository) {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockSegmentRepository(ctrl)
		auth := mocks.NewMockAuthService(ctrl)
		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "w1").Return(ctx, &domain.User{ID: "user-1"}, grant, nil).AnyTimes()
		svc := NewSegmentService(repo, mocks.NewMockWorkspaceRepository(ctrl), mocks.NewMockTaskService(ctrl), auth, newSegmentTestLogger(ctrl))
		return svc, repo
	}

	for _, tc := range []struct {
		name         string
		contactsRead bool
		want         *domain.SegmentLastError
	}{
		{"without contacts:read the contact is left out", false, redactedLastError()},
		{"with contacts:read the error is whole", true, segmentWithLastError().LastError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := setup(t, member(tc.contactsRead))

			repo.EXPECT().GetSegmentByID(gomock.Any(), "w1", "seg_1").Return(segmentWithLastError(), nil)
			repo.EXPECT().GetSegmentContactCount(gomock.Any(), "w1", "seg_1").Return(3, nil)
			segment, err := svc.GetSegment(ctx, &domain.GetSegmentRequest{WorkspaceID: "w1", ID: "seg_1"})
			require.NoError(t, err)
			assert.Equal(t, tc.want, segment.LastError)
			assert.NotNil(t, segment.LastErrorAt, "when it failed is kept")

			repo.EXPECT().GetSegments(gomock.Any(), "w1", false).Return([]*domain.Segment{segmentWithLastError(), {ID: "seg-2"}}, nil)
			segments, err := svc.ListSegments(ctx, &domain.GetSegmentsRequest{WorkspaceID: "w1"})
			require.NoError(t, err)
			require.Len(t, segments, 2)
			assert.Equal(t, tc.want, segments[0].LastError)
			assert.Nil(t, segments[1].LastError)

		})
	}
}

func TestSegmentService_UpdateSegmentLastError(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (*SegmentService, *mocks.MockSegmentRepository, *mocks.MockTaskService) {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockSegmentRepository(ctrl)
		tasks := mocks.NewMockTaskService(ctrl)
		svc := NewSegmentService(repo, mocks.NewMockWorkspaceRepository(ctrl), tasks, newPermissiveAuthService(ctrl), newSegmentTestLogger(ctrl))
		return svc, repo, tasks
	}

	// Every update carries a tree and moves the version, so the row's error is cleared by the
	// repository's CASE and the segment returned carries none. A write that keeps the version (and
	// the error) is pinned against a real database by the integration test.
	t.Run("a tree change returns no last error", func(t *testing.T) {
		svc, repo, tasks := setup(t)
		repo.EXPECT().GetSegmentByID(gomock.Any(), "w1", "seg_1").Return(segmentWithLastError(), nil)
		repo.EXPECT().UpdateSegment(gomock.Any(), "w1", gomock.Any()).Return(nil)
		tasks.EXPECT().CreateTask(gomock.Any(), "w1", gomock.Any()).Return(errors.New("not now"))

		updated, err := svc.UpdateSegment(ctx, &domain.UpdateSegmentRequest{WorkspaceID: "w1", ID: "seg_1",
			Name: "Renewed", Color: "#000", Timezone: "UTC", Tree: createTestTree()})
		require.NoError(t, err)
		assert.Equal(t, int64(3), updated.Version)
		assert.Nil(t, updated.LastError)
		assert.Nil(t, updated.LastErrorAt)
	})

}
