package http

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func coverageRootHandler(repo domain.WorkspaceRepository) *RootHandler {
	isInstalled := true
	return NewRootHandler(
		"console_test", "notification_center_test", logger.NewLogger(),
		"https://api.example.com", "1.0", "root@example.com", &isInstalled,
		false, "", 0, "off",
		repo, nil, nil,
		nil, "",
		false,
	)
}

func TestCoverage_DetectWorkspaceByHost(t *testing.T) {
	t.Run("nil repository returns nil without panicking", func(t *testing.T) {
		h := coverageRootHandler(nil)
		assert.Nil(t, h.detectWorkspaceByHost(context.Background(), "blog.example.com"))
	})

	t.Run("repository error returns nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockWorkspaceRepository(ctrl)
		repo.EXPECT().GetWorkspaceByCustomDomain(gomock.Any(), "blog.example.com").
			Return(nil, errors.New("db down"))
		h := coverageRootHandler(repo)
		assert.Nil(t, h.detectWorkspaceByHost(context.Background(), "blog.example.com"))
	})

	t.Run("no matching workspace returns nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockWorkspaceRepository(ctrl)
		repo.EXPECT().GetWorkspaceByCustomDomain(gomock.Any(), "unknown.example.com").
			Return(nil, nil)
		h := coverageRootHandler(repo)
		assert.Nil(t, h.detectWorkspaceByHost(context.Background(), "unknown.example.com"))
	})

	t.Run("matched workspace is returned verbatim and host is forwarded", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		want := &domain.Workspace{ID: "ws42", Name: "Acme"}
		repo := mocks.NewMockWorkspaceRepository(ctrl)
		repo.EXPECT().GetWorkspaceByCustomDomain(gomock.Any(), "blog.acme.com").Return(want, nil)
		h := coverageRootHandler(repo)
		got := h.detectWorkspaceByHost(context.Background(), "blog.acme.com")
		require.NotNil(t, got)
		assert.Same(t, want, got)
		assert.Equal(t, "ws42", got.ID)
		assert.Equal(t, "Acme", got.Name)
	})

	t.Run("empty host is forwarded to the repository as-is", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mocks.NewMockWorkspaceRepository(ctrl)
		repo.EXPECT().GetWorkspaceByCustomDomain(gomock.Any(), "").Return(nil, nil)
		h := coverageRootHandler(repo)
		assert.Nil(t, h.detectWorkspaceByHost(context.Background(), ""))
	})
}
