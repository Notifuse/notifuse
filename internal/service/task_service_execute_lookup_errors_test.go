package service

import (
	"context"
	"database/sql"
	"errors"
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
// the caller gets the database error, and the task is not marked as running.
func TestTaskService_ExecuteTask_LookupFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

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
