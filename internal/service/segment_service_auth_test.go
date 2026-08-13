package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPermissiveAuthService returns an auth mock that admits everyone, for the
// tests that are about segment behaviour rather than about who is asking.
// TestSegmentService_RejectsNonMembers below is what pins the boundary.
func newPermissiveAuthService(ctrl *gomock.Controller) *mocks.MockAuthService {
	m := mocks.NewMockAuthService(ctrl)
	m.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "user-1"}, &domain.UserWorkspace{
				UserID:      "user-1",
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil
		}).AnyTimes()
	return m
}

// TestSegmentService_RejectsNonMembers pins the workspace boundary for segments.
//
// Every one of these methods takes workspace_id straight from the request, and
// per-database isolation does not by itself establish a right to that database.
// GetSegmentContacts matters most: it returns contact email addresses.
//
// The assertion that matters is that NO repository method is reached: the mocks
// below carry no EXPECT() calls, so gomock fails if any of them is touched.
func TestSegmentService_RejectsNonMembers(t *testing.T) {
	const victimWorkspace = "victim-workspace"

	cases := []struct {
		name string
		call func(context.Context, *SegmentService) error
	}{
		{"CreateSegment", func(ctx context.Context, s *SegmentService) error {
			_, err := s.CreateSegment(ctx, &domain.CreateSegmentRequest{WorkspaceID: victimWorkspace})
			return err
		}},
		{"GetSegment", func(ctx context.Context, s *SegmentService) error {
			_, err := s.GetSegment(ctx, &domain.GetSegmentRequest{WorkspaceID: victimWorkspace, ID: "seg-1"})
			return err
		}},
		{"ListSegments", func(ctx context.Context, s *SegmentService) error {
			_, err := s.ListSegments(ctx, &domain.GetSegmentsRequest{WorkspaceID: victimWorkspace})
			return err
		}},
		{"UpdateSegment", func(ctx context.Context, s *SegmentService) error {
			_, err := s.UpdateSegment(ctx, &domain.UpdateSegmentRequest{WorkspaceID: victimWorkspace, ID: "seg-1"})
			return err
		}},
		{"DeleteSegment", func(ctx context.Context, s *SegmentService) error {
			return s.DeleteSegment(ctx, &domain.DeleteSegmentRequest{WorkspaceID: victimWorkspace, ID: "seg-1"})
		}},
		{"RebuildSegment", func(ctx context.Context, s *SegmentService) error {
			return s.RebuildSegment(ctx, victimWorkspace, "seg-1")
		}},
		{"PreviewSegment", func(ctx context.Context, s *SegmentService) error {
			_, err := s.PreviewSegment(ctx, victimWorkspace, &domain.TreeNode{Kind: "leaf"}, 10)
			return err
		}},
		{"GetSegmentContacts", func(ctx context.Context, s *SegmentService) error {
			_, err := s.GetSegmentContacts(ctx, victimWorkspace, "seg-1", 50, 0)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockSegmentRepository(ctrl)
			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockTaskService := mocks.NewMockTaskService(ctrl)
			mockLogger := newSegmentTestLogger(ctrl)

			mockAuthService := mocks.NewMockAuthService(ctrl)
			mockAuthService.EXPECT().
				AuthenticateUserForWorkspace(gomock.Any(), victimWorkspace).
				Return(nil, nil, nil, errors.New("user is not a member of the workspace"))

			service := NewSegmentService(mockRepo, mockWorkspaceRepo, mockTaskService, mockAuthService, mockLogger)

			err := tc.call(context.Background(), service)
			require.Error(t, err, "a non-member must not be served")
			assert.Contains(t, err.Error(), "failed to authenticate user")
		})
	}
}

// newSegmentTestLogger mirrors the permissive logger setup the other segment
// tests build inline.
func newSegmentTestLogger(ctrl *gomock.Controller) *pkgmocks.MockLogger {
	m := pkgmocks.NewMockLogger(ctrl)
	m.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(m).AnyTimes()
	m.EXPECT().WithFields(gomock.Any()).Return(m).AnyTimes()
	m.EXPECT().Info(gomock.Any()).AnyTimes()
	m.EXPECT().Warn(gomock.Any()).AnyTimes()
	m.EXPECT().Debug(gomock.Any()).AnyTimes()
	m.EXPECT().Error(gomock.Any()).AnyTimes()
	return m
}
