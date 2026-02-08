package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestIntegrationSyncProcessor_CanProcess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	processor := NewIntegrationSyncProcessor(mockLogger)

	t.Run("Returns true for sync_integration", func(t *testing.T) {
		assert.True(t, processor.CanProcess("sync_integration"))
	})

	t.Run("Returns false for other types", func(t *testing.T) {
		assert.False(t, processor.CanProcess("send_broadcast"))
		assert.False(t, processor.CanProcess("import_contacts"))
		assert.False(t, processor.CanProcess(""))
	})
}

func TestIntegrationSyncProcessor_Process(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	processor := NewIntegrationSyncProcessor(mockLogger)

	t.Run("Missing IntegrationSync state returns error", func(t *testing.T) {
		ctx := context.Background()
		task := &domain.Task{
			ID:          "task-1",
			WorkspaceID: "ws-1",
			Type:        "sync_integration",
			State:       &domain.TaskState{},
		}
		timeoutAt := time.Now().Add(60 * time.Second)

		completed, err := processor.Process(ctx, task, timeoutAt)
		assert.False(t, completed)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing IntegrationSync state")
	})

	t.Run("Missing IntegrationType returns error", func(t *testing.T) {
		ctx := context.Background()
		integrationID := "int-123"
		task := &domain.Task{
			ID:            "task-1",
			WorkspaceID:   "ws-1",
			Type:          "sync_integration",
			IntegrationID: &integrationID,
			State: &domain.TaskState{
				IntegrationSync: &domain.IntegrationSyncState{
					IntegrationID: integrationID,
					// Missing IntegrationType
				},
			},
		}
		timeoutAt := time.Now().Add(60 * time.Second)

		completed, err := processor.Process(ctx, task, timeoutAt)
		assert.False(t, completed)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing IntegrationType")
	})

	t.Run("No handler registered returns placeholder success", func(t *testing.T) {
		ctx := context.Background()
		integrationID := "int-123"
		task := &domain.Task{
			ID:            "task-1",
			WorkspaceID:   "ws-1",
			Type:          "sync_integration",
			IntegrationID: &integrationID,
			State: &domain.TaskState{
				IntegrationSync: &domain.IntegrationSyncState{
					IntegrationID:   integrationID,
					IntegrationType: "unknown_type",
				},
			},
		}
		timeoutAt := time.Now().Add(60 * time.Second)

		// With no handler registered, the processor returns completed=true
		// This allows the recurring task to reschedule
		completed, err := processor.Process(ctx, task, timeoutAt)
		assert.True(t, completed)
		assert.NoError(t, err)
	})
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		// Transient errors
		{"connection timeout", errors.New("connection timeout"), domain.ErrorTypeTransient},
		{"timeout error", errors.New("request timeout exceeded"), domain.ErrorTypeTransient},
		{"rate limit exceeded", errors.New("rate limit exceeded"), domain.ErrorTypeTransient},
		{"rate limited", errors.New("rate limited: try again later"), domain.ErrorTypeTransient},
		{"429 too many requests", errors.New("HTTP 429: too many requests"), domain.ErrorTypeTransient},
		{"503 service unavailable", errors.New("HTTP 503 service unavailable"), domain.ErrorTypeTransient},
		{"502 bad gateway", errors.New("502 bad gateway"), domain.ErrorTypeTransient},
		{"temporary failure", errors.New("temporary failure in name resolution"), domain.ErrorTypeTransient},
		{"connection refused", errors.New("connection refused"), domain.ErrorTypeTransient},
		{"network unreachable", errors.New("network unreachable"), domain.ErrorTypeTransient},
		{"EOF error", errors.New("unexpected EOF"), domain.ErrorTypeTransient},

		// Permanent errors
		{"invalid api key", errors.New("invalid api key"), domain.ErrorTypePermanent},
		{"401 unauthorized", errors.New("HTTP 401 unauthorized"), domain.ErrorTypePermanent},
		{"403 forbidden", errors.New("HTTP 403 forbidden"), domain.ErrorTypePermanent},
		{"invalid credentials", errors.New("invalid credentials"), domain.ErrorTypePermanent},
		{"authentication failed", errors.New("authentication failed"), domain.ErrorTypePermanent},
		{"access denied", errors.New("access denied"), domain.ErrorTypePermanent},
		{"permission denied", errors.New("permission denied"), domain.ErrorTypePermanent},
		{"integration disabled", errors.New("integration is disabled"), domain.ErrorTypePermanent},
		{"account suspended", errors.New("account suspended"), domain.ErrorTypePermanent},

		// Unknown errors
		{"random error", errors.New("something went wrong"), domain.ErrorTypeUnknown},
		{"nil error", nil, domain.ErrorTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIntegrationSyncProcessor_UpdateSyncState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	processor := NewIntegrationSyncProcessor(mockLogger)

	t.Run("Success updates state correctly", func(t *testing.T) {
		state := &domain.IntegrationSyncState{
			IntegrationID:   "int-123",
			IntegrationType: "test",
			ConsecErrors:    3, // Had previous errors
		}

		processor.updateSyncStateSuccess(state, 10)

		assert.Equal(t, 0, state.ConsecErrors)
		assert.NotNil(t, state.LastSuccessAt)
		assert.Equal(t, 10, state.LastEventCount)
		assert.Nil(t, state.LastError)
		assert.Empty(t, state.LastErrorType)
	})

	t.Run("Error updates state correctly", func(t *testing.T) {
		state := &domain.IntegrationSyncState{
			IntegrationID:   "int-123",
			IntegrationType: "test",
			ConsecErrors:    2,
		}

		testErr := errors.New("connection timeout")
		processor.updateSyncStateError(state, testErr)

		assert.Equal(t, 3, state.ConsecErrors)
		assert.NotNil(t, state.LastError)
		assert.Equal(t, "connection timeout", *state.LastError)
		assert.Equal(t, domain.ErrorTypeTransient, state.LastErrorType)
	})
}
