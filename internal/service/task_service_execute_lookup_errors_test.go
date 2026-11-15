package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A task that cannot be read because the database fails is not reported as missing:
// the caller gets the database error, and the task is not marked as running. The service
// logs nothing: its caller logs the error once (the strict logger mock refuses Error).
func TestTaskService_ExecuteTask_LookupFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()

	svc := NewTaskService(mockRepo, mocks.NewMockSettingRepository(ctrl), mockLogger, nil, "http://localhost:8080")
	svc.SetAutoExecuteImmediate(false)

	dbErr := errors.New("pq: connection refused")
	mockRepo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error { return fn(nil) })
	mockRepo.EXPECT().GetTx(gomock.Any(), gomock.Any(), "workspace1", "task123").Return(nil, dbErr)

	err := svc.ExecuteTask(context.Background(), "workspace1", "task123", time.Now().Add(time.Minute))

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
	var notFound *domain.ErrNotFound
	assert.False(t, errors.As(err, &notFound), "a failed read is not a missing task: %v", err)
}

// A missing task is returned as *domain.ErrNotFound without a log of the service's own.
func TestTaskService_ExecuteTask_MissingTaskIsNotLogged(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()

	svc := NewTaskService(mockRepo, mocks.NewMockSettingRepository(ctrl), mockLogger, nil, "http://localhost:8080")
	svc.SetAutoExecuteImmediate(false)
	mockRepo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error { return fn(nil) })
	mockRepo.EXPECT().GetTx(gomock.Any(), gomock.Any(), "workspace1", "task123").Return(nil, domain.ErrTaskNotFound)

	err := svc.ExecuteTask(context.Background(), "workspace1", "task123", time.Now().Add(time.Minute))
	var notFound *domain.ErrNotFound
	require.True(t, errors.As(err, &notFound), "got %v", err)
}

// The callers of ExecuteTask log a missing task at Warn and any other failure at Error,
// each with the error's text.
func TestLogExecutionFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		warn bool
	}{
		{"a missing task", &domain.ErrNotFound{Entity: "task", ID: "task123"}, true},
		{"a missing task, wrapped", fmt.Errorf("run: %w", &domain.ErrNotFound{Entity: "task", ID: "task123"}), true},
		{"a missing task, as the repository says it", fmt.Errorf("read: %w", domain.ErrTaskNotFound), true},
		{"a database failure", errors.New("pq: connection refused"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			log := pkgmocks.NewMockLogger(ctrl)
			log.EXPECT().WithField("error", tc.err.Error()).Return(log)
			if tc.warn {
				log.EXPECT().Warn("Failed to execute task: the task no longer exists")
			} else {
				log.EXPECT().Error("Failed to execute task")
			}
			logExecutionFailure(log, tc.err, "Failed to execute task")
		})
	}
}
