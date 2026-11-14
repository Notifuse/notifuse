package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTaskServiceForResetTrigger(t *testing.T) (*TaskService, *mocks.MockTaskRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockTaskRepository(ctrl)
	return NewTaskService(repo, mocks.NewMockSettingRepository(ctrl), logger.NewLoggerWithLevel("disabled"), nil, "http://localhost:8080"), repo
}

// A failed read of the task is a failure, not a missing task: only the repository's
// not-found answer becomes ErrTaskNotFound.
func TestTaskService_ResetAndTrigger_LookupFailureIsNotNotFound(t *testing.T) {
	calls := map[string]func(s *TaskService) error{
		"reset":   func(s *TaskService) error { return s.ResetTask(context.Background(), "ws-1", "task-1") },
		"trigger": func(s *TaskService) error { return s.TriggerTask(context.Background(), "ws-1", "task-1") },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			svc, repo := newTaskServiceForResetTrigger(t)
			sentinel := errors.New("connection refused")
			repo.EXPECT().Get(gomock.Any(), "ws-1", "task-1").Return(nil, fmt.Errorf("failed to get task: %w", sentinel))

			err := call(svc)

			require.Error(t, err)
			assert.ErrorIs(t, err, sentinel)
			assert.NotErrorIs(t, err, domain.ErrTaskNotFound)
		})
	}
}

// A refusal of the task's state is the caller's to fix: it is a validation error,
// which the API answers with 400 and its text.
func TestTaskService_ResetAndTrigger_StateRefusalsAreValidationErrors(t *testing.T) {
	interval := int64(60)
	cases := []struct {
		name    string
		task    *domain.Task
		call    func(s *TaskService) error
		wantErr string
	}{
		{
			name:    "reset: not failed",
			task:    &domain.Task{ID: "task-1", Status: domain.TaskStatusPending, RecurringInterval: &interval},
			call:    func(s *TaskService) error { return s.ResetTask(context.Background(), "ws-1", "task-1") },
			wantErr: "validation error: task is not in failed state, current status: pending",
		},
		{
			name:    "reset: not recurring",
			task:    &domain.Task{ID: "task-1", Status: domain.TaskStatusFailed},
			call:    func(s *TaskService) error { return s.ResetTask(context.Background(), "ws-1", "task-1") },
			wantErr: "validation error: task is not a recurring task",
		},
		{
			name:    "trigger: not recurring",
			task:    &domain.Task{ID: "task-1", Status: domain.TaskStatusPending},
			call:    func(s *TaskService) error { return s.TriggerTask(context.Background(), "ws-1", "task-1") },
			wantErr: "validation error: task is not a recurring task",
		},
		{
			name:    "trigger: completed",
			task:    &domain.Task{ID: "task-1", Status: domain.TaskStatusCompleted, RecurringInterval: &interval},
			call:    func(s *TaskService) error { return s.TriggerTask(context.Background(), "ws-1", "task-1") },
			wantErr: "validation error: task cannot be triggered in current status: completed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newTaskServiceForResetTrigger(t)
			repo.EXPECT().Get(gomock.Any(), "ws-1", "task-1").Return(tc.task, nil)

			err := tc.call(svc)

			var validationErr domain.ValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.EqualError(t, err, tc.wantErr)
		})
	}
}

// A task deleted between the read and the write is still a missing task.
func TestTaskService_ResetAndTrigger_TaskDeletedBeforeTheWrite(t *testing.T) {
	interval := int64(60)

	t.Run("reset", func(t *testing.T) {
		svc, repo := newTaskServiceForResetTrigger(t)
		repo.EXPECT().Get(gomock.Any(), "ws-1", "task-1").
			Return(&domain.Task{ID: "task-1", Status: domain.TaskStatusFailed, RecurringInterval: &interval}, nil)
		repo.EXPECT().MarkAsPending(gomock.Any(), "ws-1", "task-1", gomock.Any(), float64(0), gomock.Any()).
			Return(domain.ErrTaskNotFound)

		assert.ErrorIs(t, svc.ResetTask(context.Background(), "ws-1", "task-1"), domain.ErrTaskNotFound)
	})

	t.Run("trigger", func(t *testing.T) {
		svc, repo := newTaskServiceForResetTrigger(t)
		repo.EXPECT().Get(gomock.Any(), "ws-1", "task-1").
			Return(&domain.Task{ID: "task-1", Status: domain.TaskStatusPending, RecurringInterval: &interval}, nil)
		repo.EXPECT().WakeTask(gomock.Any(), "ws-1", "task-1", gomock.Any()).Return(false, domain.ErrTaskNotFound)

		assert.ErrorIs(t, svc.TriggerTask(context.Background(), "ws-1", "task-1"), domain.ErrTaskNotFound)
	})
}
