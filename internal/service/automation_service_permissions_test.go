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

// automationServiceCall runs one AutomationService method and returns its error.
type automationServiceCall struct {
	name       string
	permission domain.PermissionType
	call       func(s *AutomationService, ctx context.Context, workspaceID string) error
}

func automationServiceCalls() []automationServiceCall {
	return []automationServiceCall{
		{"Create", domain.PermissionTypeWrite, func(s *AutomationService, ctx context.Context, ws string) error {
			return s.Create(ctx, ws, createTestAutomationService("auto-1", ws))
		}},
		{"Update", domain.PermissionTypeWrite, func(s *AutomationService, ctx context.Context, ws string) error {
			return s.Update(ctx, ws, createTestAutomationService("auto-1", ws))
		}},
		{"Delete", domain.PermissionTypeWrite, func(s *AutomationService, ctx context.Context, ws string) error {
			return s.Delete(ctx, ws, "auto-1")
		}},
		{"Activate", domain.PermissionTypeWrite, func(s *AutomationService, ctx context.Context, ws string) error {
			return s.Activate(ctx, ws, "auto-1")
		}},
		{"Pause", domain.PermissionTypeWrite, func(s *AutomationService, ctx context.Context, ws string) error {
			return s.Pause(ctx, ws, "auto-1")
		}},
		{"Get", domain.PermissionTypeRead, func(s *AutomationService, ctx context.Context, ws string) error {
			_, err := s.Get(ctx, ws, "auto-1")
			return err
		}},
		{"List", domain.PermissionTypeRead, func(s *AutomationService, ctx context.Context, ws string) error {
			_, _, err := s.List(ctx, ws, domain.AutomationFilter{})
			return err
		}},
		{"GetContactNodeExecutions", domain.PermissionTypeRead, func(s *AutomationService, ctx context.Context, ws string) error {
			_, _, err := s.GetContactNodeExecutions(ctx, ws, "auto-1", "contact@example.com")
			return err
		}},
	}
}

// A member without the permission a method needs is refused before the
// repository is touched: the repository mock has no expectation, so any call
// fails the test.
func TestAutomationService_RefusesMembersWithoutPermission(t *testing.T) {
	workspaceID := "workspace-123"
	members := []struct {
		name        string
		permissions domain.UserPermissions
		refused     map[domain.PermissionType]bool
	}{
		{
			name: "read-only on automations",
			permissions: domain.UserPermissions{
				domain.PermissionResourceAutomations: {Read: true, Write: false},
			},
			refused: map[domain.PermissionType]bool{domain.PermissionTypeWrite: true},
		},
		{
			name: "no automations permission",
			permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: {Read: true, Write: true},
			},
			refused: map[domain.PermissionType]bool{domain.PermissionTypeRead: true, domain.PermissionTypeWrite: true},
		},
	}

	for _, member := range members {
		for _, c := range automationServiceCalls() {
			if !member.refused[c.permission] {
				continue
			}
			t.Run(member.name+"/"+c.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				repo := mocks.NewMockAutomationRepository(ctrl)
				authService := mocks.NewMockAuthService(ctrl)
				service := NewAutomationService(repo, authService, pkgmocks.NewMockLogger(ctrl), false)
				ctx := context.Background()
				authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: "user-1"},
					&domain.UserWorkspace{UserID: "user-1", WorkspaceID: workspaceID, Role: "member", Permissions: member.permissions}, nil)

				err := c.call(service, ctx, workspaceID)

				require.Error(t, err)
				var permErr *domain.PermissionError
				require.True(t, errors.As(err, &permErr), "want a permission error, got %v", err)
				assert.Equal(t, domain.PermissionResourceAutomations, permErr.Resource)
				assert.Equal(t, c.permission, permErr.Permission)
			})
		}
	}
}

// A failed authentication is reported as such by each method in the table, with
// no repository call.
func TestAutomationService_AuthenticationFailure(t *testing.T) {
	for _, c := range automationServiceCalls() {
		t.Run(c.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			authService := mocks.NewMockAuthService(ctrl)
			service := NewAutomationService(mocks.NewMockAutomationRepository(ctrl), authService, pkgmocks.NewMockLogger(ctrl), false)
			ctx := context.Background()
			authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "workspace-123").Return(ctx, nil, nil, errors.New("token expired"))

			err := c.call(service, ctx, "workspace-123")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to authenticate")
		})
	}
}
