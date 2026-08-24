package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskService_ExecuteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	// Use nil for auth service since it's not used in our tests
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger to return itself for chaining
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	// Setup transaction mocking for all tests
	mockRepo.EXPECT().
		WithTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error {
			return fn(nil)
		}).AnyTimes()

	t.Run("Task not found error", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		taskID := "task123"

		// Configure mock repository to return a "not found" error
		notFoundErr := fmt.Errorf("task not found")
		mockRepo.EXPECT().
			GetTx(gomock.Any(), gomock.Any(), workspaceID, taskID).
			Return(nil, notFoundErr)

		// Call the method under test
		timeoutAt := time.Now().Add(60 * time.Second) // 60 seconds timeout for test
		err := taskService.ExecuteTask(ctx, workspaceID, taskID, timeoutAt)

		// Verify returned error is of type ErrNotFound
		assert.Error(t, err)
		var notFoundError *domain.ErrNotFound
		assert.True(t, errors.As(err, &notFoundError))
		assert.Equal(t, "task", notFoundError.Entity)
		assert.Equal(t, taskID, notFoundError.ID)
	})

	t.Run("Processor not found error", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		taskID := "task123"

		// Create a task with an unsupported type
		task := &domain.Task{
			ID:          taskID,
			WorkspaceID: workspaceID,
			Type:        "unsupported_task_type",
			Status:      domain.TaskStatusPending,
		}

		// Configure mock repository
		mockRepo.EXPECT().
			GetTx(gomock.Any(), gomock.Any(), workspaceID, taskID).
			Return(task, nil)

		// Call the method under test
		timeoutAt := time.Now().Add(60 * time.Second) // 60 seconds timeout for test
		err := taskService.ExecuteTask(ctx, workspaceID, taskID, timeoutAt)

		// Verify returned error is of type ErrTaskExecution
		assert.Error(t, err)
		var taskExecError *domain.ErrTaskExecution
		assert.True(t, errors.As(err, &taskExecError))
		assert.Equal(t, taskID, taskExecError.TaskID)
		assert.Equal(t, "no processor registered for task type", taskExecError.Reason)
		assert.Contains(t, taskExecError.Error(), "unsupported_task_type")
	})

	t.Run("Mark as running error", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		taskID := "task123"

		// Create a task with a supported type
		task := &domain.Task{
			ID:          taskID,
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPending,
			MaxRuntime:  60,
		}

		// Register a processor for the task type
		mockProcessor := mocks.NewMockTaskProcessor(ctrl)
		// Configure CanProcess to be called for all supported task types
		for _, supportedType := range getTaskTypes() {
			mockProcessor.EXPECT().
				CanProcess(supportedType).
				Return(supportedType == "send_broadcast").
				AnyTimes()
		}
		taskService.RegisterProcessor(mockProcessor)

		// Configure mock repository
		mockRepo.EXPECT().
			GetTx(gomock.Any(), gomock.Any(), workspaceID, taskID).
			Return(task, nil)

		// MarkAsRunningTx should return an error
		markingError := fmt.Errorf("database connection error")
		mockRepo.EXPECT().
			MarkAsRunningTx(gomock.Any(), gomock.Any(), workspaceID, taskID, gomock.Any()).
			Return(markingError)

		// Call the method under test
		timeoutAt := time.Now().Add(60 * time.Second) // 60 seconds timeout for test
		err := taskService.ExecuteTask(ctx, workspaceID, taskID, timeoutAt)

		// Verify returned error is of type ErrTaskExecution with the correct reason
		assert.Error(t, err)
		var taskExecError *domain.ErrTaskExecution
		assert.True(t, errors.As(err, &taskExecError))
		assert.Equal(t, taskID, taskExecError.TaskID)
		assert.Equal(t, "failed to mark task as running", taskExecError.Reason)
		assert.Equal(t, markingError, taskExecError.Err)
	})

	t.Run("Processing error returns ErrTaskExecution", func(t *testing.T) {
		// Setup - create a new controller for this test to avoid interference
		procCtrl := gomock.NewController(t)
		defer procCtrl.Finish()

		ctx := context.Background()
		workspaceID := "workspace1"
		taskID := "task456"

		// Create a task with a supported type
		task := &domain.Task{
			ID:          taskID,
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPending,
			MaxRuntime:  60,
		}

		// Create a new task service instance for this test
		procTaskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
		procTaskService.SetAutoExecuteImmediate(false) // Disable for testing

		// Register a processor for the task type
		mockProcessor := mocks.NewMockTaskProcessor(procCtrl)
		// Configure CanProcess to be called for all supported task types
		for _, supportedType := range getTaskTypes() {
			mockProcessor.EXPECT().
				CanProcess(supportedType).
				Return(supportedType == "send_broadcast").
				AnyTimes()
		}
		procTaskService.RegisterProcessor(mockProcessor)

		// Configure mock repository
		mockRepo.EXPECT().
			GetTx(gomock.Any(), gomock.Any(), workspaceID, taskID).
			Return(task, nil)

		// MarkAsRunningTx should succeed
		mockRepo.EXPECT().
			MarkAsRunningTx(gomock.Any(), gomock.Any(), workspaceID, taskID, gomock.Any()).
			Return(nil)

		// Configure processor to return an error
		processingError := fmt.Errorf("processing failed")
		mockProcessor.EXPECT().
			Process(gomock.Any(), task, gomock.Any()).
			Return(false, processingError)

		// Mark as failed should succeed
		mockRepo.EXPECT().
			MarkAsFailed(gomock.Any(), workspaceID, taskID, gomock.Any()).
			Return(nil)

		// Call the method under test
		timeoutAt := time.Now().Add(60 * time.Second) // 60 seconds timeout for test
		err := procTaskService.ExecuteTask(ctx, workspaceID, taskID, timeoutAt)

		// Verify returned error is of type ErrTaskExecution
		assert.Error(t, err)
		var taskExecError *domain.ErrTaskExecution
		assert.True(t, errors.As(err, &taskExecError))
		assert.Equal(t, taskID, taskExecError.TaskID)
		assert.Equal(t, "processing failed", taskExecError.Reason)
		assert.Equal(t, processingError, taskExecError.Err)
	})

	t.Run("Timeout error returns ErrTaskTimeout", func(t *testing.T) {
		t.Skip("Skipping timeout test because it depends on context timing which is flaky in tests")
		// Note: This test is more integration-style and might be flaky due to timing issues

		// Setup - create a context that's already timed out
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		time.Sleep(2 * time.Millisecond) // Ensure the context times out
		defer cancel()

		workspaceID := "workspace1"
		taskID := "task123"

		// Create a task with a supported type
		task := &domain.Task{
			ID:          taskID,
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPending,
			MaxRuntime:  60,
			MaxRetries:  1,
			RetryCount:  1, // Max retries reached
		}

		// Register a processor for the task type
		mockProcessor := mocks.NewMockTaskProcessor(ctrl)
		mockProcessor.EXPECT().CanProcess("send_broadcast").Return(true)
		taskService.RegisterProcessor(mockProcessor)

		// Configure mock repository
		mockRepo.EXPECT().
			GetTx(gomock.Any(), gomock.Any(), workspaceID, taskID).
			Return(task, nil)

		// MarkAsRunningTx should succeed
		mockRepo.EXPECT().
			MarkAsRunningTx(gomock.Any(), gomock.Any(), workspaceID, taskID, gomock.Any()).
			Return(nil)

		// Mark as failed should succeed for a timeout
		mockRepo.EXPECT().
			MarkAsFailed(gomock.Any(), workspaceID, taskID, gomock.Any()).
			Return(nil)

		// Call the method with the timed out context
		timeoutAt := time.Now().Add(60 * time.Second) // 60 seconds timeout for test
		err := taskService.ExecuteTask(timeoutCtx, workspaceID, taskID, timeoutAt)

		// Verify returned error is of type ErrTaskTimeout
		assert.Error(t, err)
		var timeoutError *domain.ErrTaskTimeout
		assert.True(t, errors.As(err, &timeoutError))
		assert.Equal(t, taskID, timeoutError.TaskID)
		assert.Equal(t, 60, timeoutError.MaxRuntime)
	})

	t.Run("Task execution successful completion", func(t *testing.T) {
		// Setup - create a new controller for this test to avoid interference
		procCtrl := gomock.NewController(t)
		defer procCtrl.Finish()

		ctx := context.Background()
		workspaceID := "workspace1"
		taskID := "task789"

		// Create a task with a supported type
		task := &domain.Task{
			ID:          taskID,
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPending,
			MaxRuntime:  60,
		}

		// Create a new task service instance for this test
		procTaskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
		procTaskService.SetAutoExecuteImmediate(false) // Disable for testing

		// Register a processor for the task type
		mockProcessor := mocks.NewMockTaskProcessor(procCtrl)
		// Configure CanProcess to be called for all supported task types
		for _, supportedType := range getTaskTypes() {
			mockProcessor.EXPECT().
				CanProcess(supportedType).
				Return(supportedType == "send_broadcast").
				AnyTimes()
		}
		procTaskService.RegisterProcessor(mockProcessor)

		// Configure mock repository
		mockRepo.EXPECT().
			GetTx(gomock.Any(), gomock.Any(), workspaceID, taskID).
			Return(task, nil)

		// MarkAsRunningTx should succeed
		mockRepo.EXPECT().
			MarkAsRunningTx(gomock.Any(), gomock.Any(), workspaceID, taskID, gomock.Any()).
			Return(nil)

		// Configure processor to successfully complete the task
		mockProcessor.EXPECT().
			Process(gomock.Any(), task, gomock.Any()).
			Return(true, nil)

		// Mark as completed should succeed
		mockRepo.EXPECT().
			MarkAsCompleted(gomock.Any(), workspaceID, taskID, gomock.Any()).
			Return(nil)

		// Call the method under test
		timeoutAt := time.Now().Add(60 * time.Second) // 60 seconds timeout for test
		err := procTaskService.ExecuteTask(ctx, workspaceID, taskID, timeoutAt)

		// Verify no error returned
		assert.NoError(t, err)
	})

	t.Run("Task execution with partial completion", func(t *testing.T) {
		// Setup - create a new controller for this test
		procCtrl := gomock.NewController(t)
		defer procCtrl.Finish()

		ctx := context.Background()
		workspaceID := "workspace1"
		taskID := "task555"

		// Create a task with a supported type
		task := &domain.Task{
			ID:          taskID,
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusRunning,
			MaxRuntime:  60,
			Progress:    50.0,
			State:       &domain.TaskState{Progress: 50.0, Message: "Halfway done"},
		}

		// Create a new task service instance for this test
		procTaskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
		procTaskService.SetAutoExecuteImmediate(false) // Disable for testing

		// Register a processor for the task type
		mockProcessor := mocks.NewMockTaskProcessor(procCtrl)
		// Configure CanProcess
		for _, supportedType := range getTaskTypes() {
			mockProcessor.EXPECT().
				CanProcess(supportedType).
				Return(supportedType == "send_broadcast").
				AnyTimes()
		}
		procTaskService.RegisterProcessor(mockProcessor)

		// Configure mock repository
		mockRepo.EXPECT().
			GetTx(gomock.Any(), gomock.Any(), workspaceID, taskID).
			Return(task, nil)

		// MarkAsRunningTx should succeed
		mockRepo.EXPECT().
			MarkAsRunningTx(gomock.Any(), gomock.Any(), workspaceID, taskID, gomock.Any()).
			Return(nil)

		// Configure processor to return partial completion
		mockProcessor.EXPECT().
			Process(gomock.Any(), task, gomock.Any()).
			Return(false, nil)

		// Mark as pending should be called for a partial completion
		nextRun := time.Now().Add(1 * time.Minute)
		mockRepo.EXPECT().
			MarkAsPending(gomock.Any(), workspaceID, taskID, gomock.Any(), task.Progress, task.State).
			DoAndReturn(func(_ context.Context, _, _ string, actualNextRun time.Time, progress float64, state *domain.TaskState) error {
				// Verify the next run is set approximately 1 minute in the future
				assert.WithinDuration(t, nextRun, actualNextRun, 60*time.Second)
				assert.Equal(t, task.Progress, progress)
				assert.Equal(t, task.State, state)
				return nil
			})

		// Call the method under test
		timeoutAt := time.Now().Add(60 * time.Second) // 60 seconds timeout for test
		err := procTaskService.ExecuteTask(ctx, workspaceID, taskID, timeoutAt)

		// Verify no error returned
		assert.NoError(t, err)
	})
}

func TestTaskService_CreateTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Sets default values when not provided", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"

		task := &domain.Task{
			ID:          "task123",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPending,
			// No MaxRuntime, MaxRetries, or RetryInterval set
		}

		// Expect the repository to be called with default values
		mockRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, taskArg *domain.Task) error {
				// Verify default values were set
				assert.Equal(t, defaultMaxTaskRuntime, taskArg.MaxRuntime)
				assert.Equal(t, 3, taskArg.MaxRetries)
				assert.Equal(t, 60, taskArg.RetryInterval)
				return nil
			})

		// Call the method
		err := taskService.CreateTask(ctx, workspaceID, task)

		// Assert no error was returned
		assert.NoError(t, err)
	})

	t.Run("Uses provided values when specified", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"

		task := &domain.Task{
			ID:            "task123",
			WorkspaceID:   workspaceID,
			Type:          "send_broadcast",
			Status:        domain.TaskStatusPending,
			MaxRuntime:    120, // Custom value
			MaxRetries:    5,   // Custom value
			RetryInterval: 300, // Custom value
		}

		// Expect the repository to be called with the provided values
		mockRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, taskArg *domain.Task) error {
				// Verify custom values were preserved
				assert.Equal(t, 120, taskArg.MaxRuntime)
				assert.Equal(t, 5, taskArg.MaxRetries)
				assert.Equal(t, 300, taskArg.RetryInterval)
				return nil
			})

		// Call the method
		err := taskService.CreateTask(ctx, workspaceID, task)

		// Assert no error was returned
		assert.NoError(t, err)
	})

	t.Run("Returns repository error", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		expectedErr := errors.New("database error")

		task := &domain.Task{
			ID:          "task123",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
		}

		// Configure mock to return an error
		mockRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any()).
			Return(expectedErr)

		// Call the method
		err := taskService.CreateTask(ctx, workspaceID, task)

		// Assert the error was passed through
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestTaskService_ListTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Returns tasks with pagination info", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		filter := domain.TaskFilter{
			Limit:  10,
			Offset: 0,
		}

		// Mock data
		tasks := []*domain.Task{
			{ID: "task1", WorkspaceID: workspaceID, Type: "send_broadcast"},
			{ID: "task2", WorkspaceID: workspaceID, Type: "import_contacts"},
		}
		totalCount := 25 // More tasks than returned in this page

		// Configure repository mock
		mockRepo.EXPECT().
			List(gomock.Any(), workspaceID, filter).
			Return(tasks, totalCount, nil)

		// Call the method
		result, err := taskService.ListTasks(ctx, workspaceID, filter)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, tasks, result.Tasks)
		assert.Equal(t, totalCount, result.TotalCount)
		assert.Equal(t, filter.Limit, result.Limit)
		assert.Equal(t, filter.Offset, result.Offset)
		assert.True(t, result.HasMore) // Should be true since total is more than returned
	})

	t.Run("Handles no more results correctly", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		filter := domain.TaskFilter{
			Limit:  10,
			Offset: 20,
		}

		// Mock data for last page
		tasks := []*domain.Task{
			{ID: "task21", WorkspaceID: workspaceID, Type: "send_broadcast"},
			{ID: "task22", WorkspaceID: workspaceID, Type: "import_contacts"},
		}
		totalCount := 22 // No more tasks after this page

		// Configure repository mock
		mockRepo.EXPECT().
			List(gomock.Any(), workspaceID, filter).
			Return(tasks, totalCount, nil)

		// Call the method
		result, err := taskService.ListTasks(ctx, workspaceID, filter)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, tasks, result.Tasks)
		assert.Equal(t, totalCount, result.TotalCount)
		assert.Equal(t, filter.Limit, result.Limit)
		assert.Equal(t, filter.Offset, result.Offset)
		assert.False(t, result.HasMore) // Should be false since we're at the end
	})

	t.Run("Returns repository error", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		filter := domain.TaskFilter{
			Limit:  10,
			Offset: 0,
		}
		expectedErr := errors.New("database error")

		// Configure repository mock to return an error
		mockRepo.EXPECT().
			List(gomock.Any(), workspaceID, filter).
			Return(nil, 0, expectedErr)

		// Call the method
		result, err := taskService.ListTasks(ctx, workspaceID, filter)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, expectedErr, err)
	})
}

func TestTaskService_GetTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Returns task when found", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		taskID := "task123"

		task := &domain.Task{
			ID:          taskID,
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPending,
		}

		// Configure mock
		mockRepo.EXPECT().
			Get(gomock.Any(), workspaceID, taskID).
			Return(task, nil)

		// Call the method
		result, err := taskService.GetTask(ctx, workspaceID, taskID)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, task, result)
	})

	t.Run("Returns error when task not found", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		taskID := "nonexistent"
		expectedErr := errors.New("task not found")

		// Configure mock
		mockRepo.EXPECT().
			Get(gomock.Any(), workspaceID, taskID).
			Return(nil, expectedErr)

		// Call the method
		result, err := taskService.GetTask(ctx, workspaceID, taskID)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, expectedErr, err)
	})
}

func TestTaskService_DeleteTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Deletes task successfully", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		taskID := "task123"

		// Configure mock
		mockRepo.EXPECT().
			Delete(gomock.Any(), workspaceID, taskID).
			Return(nil)

		// Call the method
		err := taskService.DeleteTask(ctx, workspaceID, taskID)

		// Assert
		assert.NoError(t, err)
	})

	t.Run("Returns error when delete fails", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		taskID := "task123"
		expectedErr := errors.New("delete failed")

		// Configure mock
		mockRepo.EXPECT().
			Delete(gomock.Any(), workspaceID, taskID).
			Return(expectedErr)

		// Call the method
		err := taskService.DeleteTask(ctx, workspaceID, taskID)

		// Assert
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestTaskService_RegisterProcessor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Registers processor for supported task types", func(t *testing.T) {
		// Create a processor that only supports certain task types
		mockProcessor := mocks.NewMockTaskProcessor(ctrl)

		// Configure CanProcess to return true only for specific types
		mockProcessor.EXPECT().
			CanProcess("import_contacts").
			Return(true).
			Times(1)

		mockProcessor.EXPECT().
			CanProcess("export_contacts").
			Return(false).
			Times(1)

		mockProcessor.EXPECT().
			CanProcess("send_broadcast").
			Return(true).
			Times(1)

		mockProcessor.EXPECT().
			CanProcess("generate_report").
			Return(false).
			Times(1)

		mockProcessor.EXPECT().
			CanProcess("build_segment").
			Return(false).
			Times(1)

		mockProcessor.EXPECT().
			CanProcess("process_contact_segment_queue").
			Return(false).
			Times(1)

		mockProcessor.EXPECT().
			CanProcess("check_segment_recompute").
			Return(false).
			Times(1)

		mockProcessor.EXPECT().
			CanProcess("sync_integration").
			Return(false).
			Times(1)

		mockProcessor.EXPECT().
			CanProcess(domain.WebAnalyticsBackfillTaskType).
			Return(false).
			Times(1)

		// Register the processor
		taskService.RegisterProcessor(mockProcessor)

		// Now test that GetProcessor returns the processor for supported types
		// and returns an error for unsupported types

		// Should return processor for import_contacts
		proc1, err1 := taskService.GetProcessor("import_contacts")
		assert.NoError(t, err1)
		assert.Equal(t, mockProcessor, proc1)

		// Should return processor for send_broadcast
		proc2, err2 := taskService.GetProcessor("send_broadcast")
		assert.NoError(t, err2)
		assert.Equal(t, mockProcessor, proc2)

		// Should return error for export_contacts
		proc3, err3 := taskService.GetProcessor("export_contacts")
		assert.Error(t, err3)
		assert.Nil(t, proc3)

		// Should return error for generate_report
		proc4, err4 := taskService.GetProcessor("generate_report")
		assert.Error(t, err4)
		assert.Nil(t, proc4)
	})
}

func TestTaskService_BroadcastEventHandlers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockEventBus := mocks.NewMockEventBus(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)

	// Setup subscription to events
	mockEventBus.EXPECT().Subscribe(domain.EventBroadcastScheduled, gomock.Any()).Times(1)
	mockEventBus.EXPECT().Subscribe(domain.EventBroadcastPaused, gomock.Any()).Times(1)
	mockEventBus.EXPECT().Subscribe(domain.EventBroadcastResumed, gomock.Any()).Times(1)
	mockEventBus.EXPECT().Subscribe(domain.EventBroadcastSent, gomock.Any()).Times(1)
	mockEventBus.EXPECT().Subscribe(domain.EventBroadcastFailed, gomock.Any()).Times(1)
	mockEventBus.EXPECT().Subscribe(domain.EventBroadcastCancelled, gomock.Any()).Times(1)

	// Disable auto-execution for testing to avoid goroutine issues
	taskService.SetAutoExecuteImmediate(false)

	// Subscribe to events
	taskService.SubscribeToBroadcastEvents(mockEventBus)

	t.Run("handleBroadcastScheduled creates a new task", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastScheduled,
			WorkspaceID: workspaceID,
			EntityID:    broadcastID,
			Data: map[string]interface{}{
				"send_now": true,
				"status":   string(domain.BroadcastStatusProcessing),
			},
		}

		// Configure mock repository to return no existing task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(nil, errors.New("not found"))

		// Configure the transaction
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error {
				return fn(nil)
			})

		// Expect task creation
		mockRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, task *domain.Task) error {
				// Verify task properties
				assert.Equal(t, workspaceID, task.WorkspaceID)
				assert.Equal(t, "send_broadcast", task.Type)
				assert.Equal(t, domain.TaskStatusPending, task.Status)
				assert.Equal(t, broadcastID, *task.BroadcastID)
				assert.Equal(t, 50, task.MaxRuntime) // 10 minutes
				assert.Equal(t, 3, task.MaxRetries)
				assert.Equal(t, 300, task.RetryInterval) // 5 minutes
				return nil
			})

		// Call the event handler
		taskService.handleBroadcastScheduled(ctx, payload)
	})

	t.Run("handleBroadcastPaused pauses the related task", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastPaused,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Create task to be returned by mock
		task := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusRunning,
			BroadcastID: &broadcastID,
		}

		// Configure mock repository to return the task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)

		// Expect task to be paused
		mockRepo.EXPECT().
			MarkAsPaused(gomock.Any(), workspaceID, task.ID, gomock.Any(), task.Progress, task.State).
			DoAndReturn(func(_ context.Context, _, _ string, nextRun time.Time, progress float64, state *domain.TaskState) error {
				// Just verify next run is in the future, with more lenient timing check
				future := time.Now().Add(23 * time.Hour) // Just under 24 hours
				assert.True(t, nextRun.After(future), "Next run time should be at least 23 hours in the future")
				return nil
			})

		// Call the event handler
		taskService.handleBroadcastPaused(ctx, payload)
	})

	t.Run("handleBroadcastPaused skips when task already completed (Phase 2)", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "phase2-paused"

		payload := domain.EventPayload{
			Type:        domain.EventBroadcastPaused,
			WorkspaceID: workspaceID,
			Data:        map[string]interface{}{"broadcast_id": broadcastID},
		}

		task := &domain.Task{
			ID:          "task-done",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusCompleted,
			BroadcastID: &broadcastID,
		}

		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)
		// CRITICAL: MarkAsPaused must NOT be called for a completed task.

		taskService.handleBroadcastPaused(ctx, payload)
	})

	t.Run("handleBroadcastResumed skips when task already completed (Phase 2)", func(t *testing.T) {
		// This is the critical re-run protection: without the guard, the handler
		// would flip the task to Pending + NextRunAfter=now, and the next
		// ExecutePendingTasks tick would re-run the orchestrator.
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "phase2-resumed"

		payload := domain.EventPayload{
			Type:        domain.EventBroadcastResumed,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
				"start_now":    false,
			},
		}

		task := &domain.Task{
			ID:          "task-done",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusCompleted,
			BroadcastID: &broadcastID,
		}

		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)
		// CRITICAL: Update must NOT be called, otherwise task.Status would be
		// reset to Pending and orchestrator would re-enqueue every recipient.

		taskService.handleBroadcastResumed(ctx, payload)
	})

	t.Run("handleBroadcastCancelled skips when task already completed (Phase 2)", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "phase2-cancelled"

		payload := domain.EventPayload{
			Type:        domain.EventBroadcastCancelled,
			WorkspaceID: workspaceID,
			Data:        map[string]interface{}{"broadcast_id": broadcastID},
		}

		task := &domain.Task{
			ID:          "task-done",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusCompleted,
			BroadcastID: &broadcastID,
		}

		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)
		// The enqueue task genuinely succeeded — don't falsify it to Failed.
		// MarkAsFailed must NOT be called.

		taskService.handleBroadcastCancelled(ctx, payload)
	})
}

func TestTaskService_ExecutePendingTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	t.Run("Uses HTTP execution when API endpoint is configured", func(t *testing.T) {
		// Create TaskService with API endpoint
		taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
		taskService.SetAutoExecuteImmediate(false) // Disable for testing

		// Setup
		ctx := context.Background()
		maxTasks := 5

		// Tasks to be returned
		tasks := []*domain.Task{
			{
				ID:          "task1",
				WorkspaceID: "workspace1",
				Type:        "send_broadcast",
				Status:      domain.TaskStatusPending,
			},
			{
				ID:          "task2",
				WorkspaceID: "workspace2",
				Type:        "import_contacts",
				Status:      domain.TaskStatusPending,
			},
		}

		// Configure setting repo mock to expect SetLastCronRun call
		mockSettingRepo.EXPECT().
			SetLastCronRun(gomock.Any()).
			Return(nil)

		// Configure mock
		mockRepo.EXPECT().
			GetNextBatch(gomock.Any(), maxTasks).
			Return(tasks, nil)

		// Configure the logger to handle any error messages that might occur during HTTP requests
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		// Call the method - we don't expect any direct mockRepo calls for
		// execution because tasks should be dispatched via HTTP
		err := taskService.ExecutePendingTasks(ctx, maxTasks)

		// Assert
		assert.NoError(t, err)

		// Wait a tiny bit to allow goroutines to start
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("Falls back to direct execution when no API endpoint", func(t *testing.T) {
		// Create a new controller just for this test to avoid interference
		localCtrl := gomock.NewController(t)
		defer localCtrl.Finish()

		localRepo := mocks.NewMockTaskRepository(localCtrl)
		localLogger := pkgmocks.NewMockLogger(localCtrl)

		// Configure logger expectations
		localLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(localLogger).AnyTimes()
		localLogger.EXPECT().WithFields(gomock.Any()).Return(localLogger).AnyTimes()
		localLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		localLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
		localLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		localLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Create TaskService without API endpoint
		localSettingRepo := mocks.NewMockSettingRepository(localCtrl)
		taskService := NewTaskService(localRepo, localSettingRepo, localLogger, mockAuthService, "")
		taskService.SetAutoExecuteImmediate(false) // Disable for testing

		// Setup
		ctx := context.Background()
		maxTasks := 5

		// Tasks to be returned
		tasks := []*domain.Task{
			{
				ID:          "task1",
				WorkspaceID: "workspace1",
				Type:        "send_broadcast",
				Status:      domain.TaskStatusPending,
				MaxRuntime:  60,
			},
		}

		// Configure setting repo mock to expect SetLastCronRun call
		localSettingRepo.EXPECT().
			SetLastCronRun(gomock.Any()).
			Return(nil)

		// Configure mocks for everything that might happen during execution
		localRepo.EXPECT().
			GetNextBatch(gomock.Any(), maxTasks).
			Return(tasks, nil)

		// For direct execution, expect transaction and task retrieval
		localRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error {
				return fn(nil)
			}).AnyTimes()

		// The task might be retrieved during execution
		localRepo.EXPECT().
			GetTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(tasks[0], nil).AnyTimes()

		// It might try to mark the task as running
		localRepo.EXPECT().
			MarkAsRunningTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).AnyTimes()

		// Since we don't have a registered processor, it should just fail with no processor
		// But we'll let the test complete rather than waiting for all execution steps

		// Call the method
		err := taskService.ExecutePendingTasks(ctx, maxTasks)

		// Assert
		assert.NoError(t, err)

		// Wait a tiny bit to allow goroutines to start
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("Handles GetNextBatch error", func(t *testing.T) {
		// Create TaskService
		taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
		taskService.SetAutoExecuteImmediate(false) // Disable for testing

		// Setup
		ctx := context.Background()
		maxTasks := 5
		expectedErr := errors.New("database error")

		// Configure setting repo mock to expect SetLastCronRun call
		mockSettingRepo.EXPECT().
			SetLastCronRun(gomock.Any()).
			Return(nil)

		// Configure mock to return error
		mockRepo.EXPECT().
			GetNextBatch(gomock.Any(), maxTasks).
			Return(nil, expectedErr)

		// Call the method
		err := taskService.ExecutePendingTasks(ctx, maxTasks)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get next batch of tasks")
	})

	t.Run("Uses default maxTasks when 0 is provided", func(t *testing.T) {
		// Create TaskService
		taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
		taskService.SetAutoExecuteImmediate(false) // Disable for testing

		// Setup
		ctx := context.Background()
		maxTasks := 0 // Should default to 100

		// Configure setting repo mock to expect SetLastCronRun call
		mockSettingRepo.EXPECT().
			SetLastCronRun(gomock.Any()).
			Return(nil)

		// Configure mock - expect 100 as the default
		mockRepo.EXPECT().
			GetNextBatch(gomock.Any(), 100).
			Return([]*domain.Task{}, nil)

		// Call the method
		err := taskService.ExecutePendingTasks(ctx, maxTasks)

		// Assert
		assert.NoError(t, err)
	})
}

// TestTaskService_ExecutePendingTasks_ExecutionMode verifies the execution-mode
// branch in ExecutePendingTasks: direct (in-process) vs HTTP dispatch. The
// decisive signal is an httptest server with a request counter — direct mode
// must never touch it, even when an API endpoint is configured.
func TestTaskService_ExecutePendingTasks_ExecutionMode(t *testing.T) {
	makeTask := func() *domain.Task {
		return &domain.Task{
			ID:          "task1",
			WorkspaceID: "workspace1",
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPending,
			MaxRuntime:  60,
		}
	}

	newLogger := func(ctrl *gomock.Controller) *pkgmocks.MockLogger {
		l := pkgmocks.NewMockLogger(ctrl)
		l.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().WithFields(gomock.Any()).Return(l).AnyTimes()
		l.EXPECT().Info(gomock.Any()).AnyTimes()
		l.EXPECT().Warn(gomock.Any()).AnyTimes()
		l.EXPECT().Debug(gomock.Any()).AnyTimes()
		l.EXPECT().Error(gomock.Any()).AnyTimes()
		return l
	}

	t.Run("direct execution bypasses HTTP even when API endpoint is set", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		var httpHits int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&httpHits, 1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		repo := mocks.NewMockTaskRepository(ctrl)
		settingRepo := mocks.NewMockSettingRepository(ctrl)

		// API endpoint IS set, but direct execution must take precedence.
		svc := NewTaskService(repo, settingRepo, newLogger(ctrl), nil, server.URL)
		svc.SetAutoExecuteImmediate(false)
		svc.SetDirectExecution(true)
		assert.True(t, svc.IsDirectExecution())

		settingRepo.EXPECT().SetLastCronRun(gomock.Any()).Return(nil)
		repo.EXPECT().GetNextBatch(gomock.Any(), 5).Return([]*domain.Task{makeTask()}, nil)
		// Direct path runs ExecuteTask in-process (no processor registered → it fails
		// gracefully, but crucially never makes an HTTP call).
		repo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error { return fn(nil) }).AnyTimes()
		repo.EXPECT().GetTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(makeTask(), nil).AnyTimes()
		repo.EXPECT().MarkAsRunningTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		repo.EXPECT().MarkAsFailed(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		err := svc.ExecutePendingTasks(context.Background(), 5)
		assert.NoError(t, err)

		time.Sleep(100 * time.Millisecond) // allow any (erroneous) dispatch goroutine to fire
		assert.Equal(t, int32(0), atomic.LoadInt32(&httpHits),
			"direct mode must not call the HTTP /api/tasks.execute endpoint")
	})

	t.Run("http execution dispatches to the API endpoint when direct is off", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		hit := make(chan struct{}, 4)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case hit <- struct{}{}:
			default:
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		repo := mocks.NewMockTaskRepository(ctrl)
		settingRepo := mocks.NewMockSettingRepository(ctrl)

		// directExecution defaults to false; API endpoint set → HTTP dispatch.
		svc := NewTaskService(repo, settingRepo, newLogger(ctrl), nil, server.URL)
		svc.SetAutoExecuteImmediate(false)
		assert.False(t, svc.IsDirectExecution())

		settingRepo.EXPECT().SetLastCronRun(gomock.Any()).Return(nil)
		repo.EXPECT().GetNextBatch(gomock.Any(), 5).Return([]*domain.Task{makeTask()}, nil)
		// No direct-execution mocks: if it wrongly took the direct branch it would
		// call WithTransaction and gomock would fail on the unexpected call.

		err := svc.ExecutePendingTasks(context.Background(), 5)
		assert.NoError(t, err)

		select {
		case <-hit:
			// success: the dispatch reached the HTTP endpoint
		case <-time.After(2 * time.Second):
			t.Fatal("expected HTTP dispatch to /api/tasks.execute, but the endpoint was never hit")
		}
	})

	t.Run("falls back to direct execution when API endpoint is empty", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mocks.NewMockTaskRepository(ctrl)
		settingRepo := mocks.NewMockSettingRepository(ctrl)

		svc := NewTaskService(repo, settingRepo, newLogger(ctrl), nil, "") // empty endpoint
		svc.SetAutoExecuteImmediate(false)
		assert.False(t, svc.IsDirectExecution(), "flag itself is false; fallback is driven by empty endpoint")

		settingRepo.EXPECT().SetLastCronRun(gomock.Any()).Return(nil)
		repo.EXPECT().GetNextBatch(gomock.Any(), 5).Return([]*domain.Task{makeTask()}, nil)
		repo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error { return fn(nil) }).AnyTimes()
		repo.EXPECT().GetTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(makeTask(), nil).AnyTimes()
		repo.EXPECT().MarkAsRunningTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		repo.EXPECT().MarkAsFailed(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		err := svc.ExecutePendingTasks(context.Background(), 5)
		assert.NoError(t, err)
	})
}

func TestTaskService_HandleBroadcastResumed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Successfully resumes a task for resumed broadcast", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastResumed,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Create task to be returned by mock
		task := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPaused,
			BroadcastID: &broadcastID,
		}

		// Configure mock repository to return the task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)

		// Expect task to be updated with pending status and next run time
		mockRepo.EXPECT().
			Update(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, updatedTask *domain.Task) error {
				// Verify task updates
				assert.Equal(t, domain.TaskStatusPending, updatedTask.Status)
				assert.NotNil(t, updatedTask.NextRunAfter)
				// The next run time should be now or in the past
				assert.True(t, updatedTask.NextRunAfter.Before(time.Now().Add(1*time.Second)))
				return nil
			})

		// Call the event handler
		taskService.handleBroadcastResumed(ctx, payload)
	})

	t.Run("Handles missing broadcast ID", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"

		// Create event payload with missing broadcast ID
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastResumed,
			WorkspaceID: workspaceID,
			Data:        map[string]interface{}{},
		}

		// No repository calls expected - should just log an error

		// Call the event handler
		taskService.handleBroadcastResumed(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})

	t.Run("Handles task not found", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastResumed,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Configure mock repository to return no task (error)
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(nil, errors.New("task not found"))

		// No other repository calls expected

		// Call the event handler
		taskService.handleBroadcastResumed(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})

	t.Run("Handles update error", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastResumed,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Create task to be returned by mock
		task := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPaused,
			BroadcastID: &broadcastID,
		}

		// Configure mock repository to return the task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)

		// Expect task update to fail
		mockRepo.EXPECT().
			Update(gomock.Any(), workspaceID, gomock.Any()).
			Return(errors.New("update failed"))

		// Call the event handler
		taskService.handleBroadcastResumed(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})
}

func TestTaskService_HandleBroadcastSent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Successfully completes a task for sent broadcast", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastSent,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Create task to be returned by mock
		task := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusRunning,
			BroadcastID: &broadcastID,
		}

		// Configure mock repository to return the task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)

		// Expect task to be marked as completed
		mockRepo.EXPECT().
			MarkAsCompleted(gomock.Any(), workspaceID, task.ID, gomock.Any()).
			Return(nil)

		// Call the event handler
		taskService.handleBroadcastSent(ctx, payload)
	})

	t.Run("Handles missing broadcast ID", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"

		// Create event payload with missing broadcast ID
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastSent,
			WorkspaceID: workspaceID,
			Data:        map[string]interface{}{},
		}

		// No repository calls expected - should just log an error

		// Call the event handler
		taskService.handleBroadcastSent(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})

	t.Run("Handles task not found", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastSent,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Configure mock repository to return no task (error)
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(nil, errors.New("task not found"))

		// No other repository calls expected

		// Call the event handler
		taskService.handleBroadcastSent(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})

	t.Run("Handles mark as completed error", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastSent,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Create task to be returned by mock
		task := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusRunning,
			BroadcastID: &broadcastID,
		}

		// Configure mock repository to return the task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)

		// Expect mark as completed to fail
		mockRepo.EXPECT().
			MarkAsCompleted(gomock.Any(), workspaceID, task.ID, gomock.Any()).
			Return(errors.New("operation failed"))

		// Call the event handler
		taskService.handleBroadcastSent(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})
}

func TestTaskService_HandleBroadcastFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Successfully marks task as failed for failed broadcast", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"
		failureReason := "API error"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastFailed,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
				"reason":       failureReason,
			},
		}

		// Create task to be returned by mock
		task := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusRunning,
			BroadcastID: &broadcastID,
		}

		// Configure mock repository to return the task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)

		// Expect task to be marked as failed with the reason
		mockRepo.EXPECT().
			MarkAsFailed(gomock.Any(), workspaceID, task.ID, failureReason).
			Return(nil)

		// Call the event handler
		taskService.handleBroadcastFailed(ctx, payload)
	})

	t.Run("Uses default reason when reason is missing", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"
		defaultReason := "Broadcast failed" // Default reason in the code

		// Create event payload without a reason
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastFailed,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Create task to be returned by mock
		task := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusRunning,
			BroadcastID: &broadcastID,
		}

		// Configure mock repository to return the task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)

		// Expect task to be marked as failed with the default reason
		mockRepo.EXPECT().
			MarkAsFailed(gomock.Any(), workspaceID, task.ID, defaultReason).
			Return(nil)

		// Call the event handler
		taskService.handleBroadcastFailed(ctx, payload)
	})

	t.Run("Handles missing broadcast ID", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"

		// Create event payload with missing broadcast ID
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastFailed,
			WorkspaceID: workspaceID,
			Data:        map[string]interface{}{},
		}

		// No repository calls expected - should just log an error

		// Call the event handler
		taskService.handleBroadcastFailed(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})

	t.Run("Handles task not found", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastFailed,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Configure mock repository to return no task (error)
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(nil, errors.New("task not found"))

		// No other repository calls expected

		// Call the event handler
		taskService.handleBroadcastFailed(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})

	t.Run("Handles mark as failed error", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastFailed,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Create task to be returned by mock
		task := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusRunning,
			BroadcastID: &broadcastID,
		}

		// Configure mock repository to return the task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)

		// Expect mark as failed to error
		mockRepo.EXPECT().
			MarkAsFailed(gomock.Any(), workspaceID, task.ID, gomock.Any()).
			Return(errors.New("operation failed"))

		// Call the event handler
		taskService.handleBroadcastFailed(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})
}

func TestTaskService_HandleBroadcastCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Successfully marks task as failed for cancelled broadcast", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"
		cancelReason := "Broadcast was cancelled" // The expected reason in the code

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastCancelled,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Create task to be returned by mock
		task := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusRunning,
			BroadcastID: &broadcastID,
		}

		// Configure mock repository to return the task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)

		// Expect task to be marked as failed with the cancellation reason
		mockRepo.EXPECT().
			MarkAsFailed(gomock.Any(), workspaceID, task.ID, cancelReason).
			Return(nil)

		// Call the event handler
		taskService.handleBroadcastCancelled(ctx, payload)
	})

	t.Run("Handles missing broadcast ID", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"

		// Create event payload with missing broadcast ID
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastCancelled,
			WorkspaceID: workspaceID,
			Data:        map[string]interface{}{},
		}

		// No repository calls expected - should just log an error

		// Call the event handler
		taskService.handleBroadcastCancelled(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})

	t.Run("Handles task not found", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastCancelled,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Configure mock repository to return no task (error)
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(nil, errors.New("task not found"))

		// No other repository calls expected

		// Call the event handler
		taskService.handleBroadcastCancelled(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})

	t.Run("Handles mark as failed error", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastCancelled,
			WorkspaceID: workspaceID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
			},
		}

		// Create task to be returned by mock
		task := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusRunning,
			BroadcastID: &broadcastID,
		}

		// Configure mock repository to return the task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(task, nil)

		// Expect mark as failed to error
		mockRepo.EXPECT().
			MarkAsFailed(gomock.Any(), workspaceID, task.ID, gomock.Any()).
			Return(errors.New("operation failed"))

		// Call the event handler
		taskService.handleBroadcastCancelled(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})
}

func TestTaskService_HandleBroadcastScheduledExtended(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Updates existing task when found for immediate sending", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload for immediate sending
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastScheduled,
			WorkspaceID: workspaceID,
			EntityID:    broadcastID,
			Data: map[string]interface{}{
				"send_now": true,
				"status":   string(domain.BroadcastStatusProcessing),
			},
		}

		// Create existing task to be returned by mock
		existingTask := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPaused,
			// BroadcastID not set initially
		}

		// Configure mock repository transaction
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error {
				return fn(nil)
			})

		// Configure mock repository to return the existing task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(existingTask, nil)

		// Expect task to be updated
		mockRepo.EXPECT().
			Update(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, updatedTask *domain.Task) error {
				// Verify task updates
				assert.Equal(t, domain.TaskStatusPending, updatedTask.Status)
				assert.NotNil(t, updatedTask.NextRunAfter)
				assert.NotNil(t, updatedTask.BroadcastID)
				assert.Equal(t, broadcastID, *updatedTask.BroadcastID)
				// The next run time should be now or in the past
				assert.True(t, updatedTask.NextRunAfter.Before(time.Now().Add(1*time.Second)))
				return nil
			})

		// Call the event handler
		taskService.handleBroadcastScheduled(ctx, payload)
	})

	t.Run("Creates a new task for future scheduled broadcast", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast789"

		// Create event payload for scheduled (not immediate) sending
		scheduledTime := time.Now().Add(1 * time.Hour)
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastScheduled,
			WorkspaceID: workspaceID,
			EntityID:    broadcastID,
			Data: map[string]interface{}{
				"send_now":       false,
				"status":         string(domain.BroadcastStatusScheduled),
				"scheduled_time": scheduledTime.Format(time.RFC3339),
			},
		}

		// Configure mock repository transaction
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error {
				return fn(nil)
			})

		// Configure mock repository to return no existing task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(nil, errors.New("not found"))

		// Expect task creation
		mockRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, task *domain.Task) error {
				// Verify task properties
				assert.Equal(t, workspaceID, task.WorkspaceID)
				assert.Equal(t, "send_broadcast", task.Type)
				assert.Equal(t, domain.TaskStatusPending, task.Status)
				assert.Equal(t, broadcastID, *task.BroadcastID)
				assert.Equal(t, 50, task.MaxRuntime) // 50 seconds
				assert.Equal(t, 3, task.MaxRetries)
				assert.Equal(t, 300, task.RetryInterval) // 5 minutes
				assert.NotNil(t, task.NextRunAfter)      // Should have a future execution time
				// Verify the scheduled time is used correctly (within 1 second tolerance)
				assert.True(t, task.NextRunAfter.Sub(scheduledTime) < time.Second)
				assert.True(t, task.NextRunAfter.Sub(scheduledTime) > -time.Second)
				return nil
			})

		// Call the event handler
		taskService.handleBroadcastScheduled(ctx, payload)
	})

	t.Run("Handles task creation error", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast999"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastScheduled,
			WorkspaceID: workspaceID,
			EntityID:    broadcastID,
			Data: map[string]interface{}{
				"send_now": true,
				"status":   string(domain.BroadcastStatusProcessing),
			},
		}

		// Configure mock repository transaction
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error {
				return fn(nil)
			})

		// Configure mock repository to return no existing task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(nil, errors.New("not found"))

		// Expect task creation to fail
		mockRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any()).
			Return(errors.New("database error"))

		// Call the event handler
		taskService.handleBroadcastScheduled(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})

	t.Run("Handles update error for existing task", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastScheduled,
			WorkspaceID: workspaceID,
			EntityID:    broadcastID,
			Data: map[string]interface{}{
				"send_now": true,
				"status":   string(domain.BroadcastStatusProcessing),
			},
		}

		// Create existing task
		existingTask := &domain.Task{
			ID:          "task456",
			WorkspaceID: workspaceID,
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPaused,
			BroadcastID: &broadcastID,
		}

		// Configure mock repository transaction
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error {
				return fn(nil)
			})

		// Configure mock repository to return the existing task
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(existingTask, nil)

		// Expect update to fail
		mockRepo.EXPECT().
			Update(gomock.Any(), workspaceID, gomock.Any()).
			Return(errors.New("update failed"))

		// Call the event handler
		taskService.handleBroadcastScheduled(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})

	t.Run("Handles transaction error", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"

		// Create event payload
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastScheduled,
			WorkspaceID: workspaceID,
			EntityID:    broadcastID,
			Data: map[string]interface{}{
				"send_now": true,
				"status":   string(domain.BroadcastStatusProcessing),
			},
		}

		// Configure mock repository transaction to fail
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			Return(errors.New("transaction failed"))

		// Call the event handler
		taskService.handleBroadcastScheduled(ctx, payload)
		// No assertions needed - if no panic, the test passes
	})
}

// TestTaskService_HandleBroadcastScheduled_ScheduledTime tests that scheduled broadcasts
// use the actual scheduled time instead of hardcoded 1 minute delay
func TestTaskService_HandleBroadcastScheduled_ScheduledTime(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Uses scheduled_time from payload when provided", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast123"
		scheduledTime := time.Now().Add(2 * time.Hour) // 2 hours in the future

		// Create event payload with scheduled time
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastScheduled,
			WorkspaceID: workspaceID,
			EntityID:    broadcastID,
			Data: map[string]interface{}{
				"send_now":       false,
				"status":         string(domain.BroadcastStatusScheduled),
				"scheduled_time": scheduledTime.Format(time.RFC3339),
			},
		}

		// Configure mock repository transaction
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error {
				return fn(nil)
			})

		// Configure mock repository to return nil (no existing task)
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(nil, errors.New("not found"))

		// Expect new task to be created
		mockRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, newTask *domain.Task) error {
				// Verify task uses the scheduled time from payload
				assert.NotNil(t, newTask.NextRunAfter)
				// Should be exactly the scheduled time (within 1 second tolerance)
				assert.True(t, newTask.NextRunAfter.Sub(scheduledTime) < time.Second)
				assert.True(t, newTask.NextRunAfter.Sub(scheduledTime) > -time.Second)
				return nil
			})

		// Call the event handler
		taskService.handleBroadcastScheduled(ctx, payload)
	})

	t.Run("Creates task without NextRunAfter when scheduled_time is missing", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		workspaceID := "workspace1"
		broadcastID := "broadcast456"

		// Create event payload WITHOUT scheduled time
		payload := domain.EventPayload{
			Type:        domain.EventBroadcastScheduled,
			WorkspaceID: workspaceID,
			EntityID:    broadcastID,
			Data: map[string]interface{}{
				"send_now": false,
				"status":   string(domain.BroadcastStatusScheduled),
				// No scheduled_time in payload - NextRunAfter will be nil
			},
		}

		// Configure mock repository transaction
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error {
				return fn(nil)
			})

		// Configure mock repository to return nil (no existing task)
		mockRepo.EXPECT().
			GetTaskByBroadcastID(gomock.Any(), workspaceID, broadcastID).
			Return(nil, errors.New("not found"))

		// Expect new task to be created without NextRunAfter when no scheduled_time
		mockRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, newTask *domain.Task) error {
				// Verify task doesn't have NextRunAfter set when no scheduled_time in payload
				assert.Nil(t, newTask.NextRunAfter, "NextRunAfter should be nil when no scheduled_time is provided")
				return nil
			})

		// Call the event handler
		taskService.handleBroadcastScheduled(ctx, payload)
	})
}

func TestTaskService_GetLastCronRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	service := NewTaskService(mockTaskRepo, mockSettingRepo, mockLogger, mockAuthService, "")

	t.Run("Returns last cron run when available", func(t *testing.T) {
		expectedTime := time.Now().Add(-1 * time.Hour).UTC()
		mockSettingRepo.EXPECT().
			GetLastCronRun(gomock.Any()).
			Return(&expectedTime, nil)

		result, err := service.GetLastCronRun(context.Background())

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, expectedTime, *result)
	})

	t.Run("Returns nil when no cron run recorded", func(t *testing.T) {
		mockSettingRepo.EXPECT().
			GetLastCronRun(gomock.Any()).
			Return(nil, nil)

		result, err := service.GetLastCronRun(context.Background())

		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("Handles repository error", func(t *testing.T) {
		mockSettingRepo.EXPECT().
			GetLastCronRun(gomock.Any()).
			Return(nil, assert.AnError)

		result, err := service.GetLastCronRun(context.Background())

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestTaskService_ExecutePendingTasks_SetLastCronRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false) // Disable for testing

	t.Run("Successfully sets last cron run before processing tasks", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		maxTasks := 5

		// Configure setting repo mock to expect SetLastCronRun call
		mockSettingRepo.EXPECT().
			SetLastCronRun(gomock.Any()).
			Return(nil).
			Times(1)

		// Configure mock to return empty task list
		mockRepo.EXPECT().
			GetNextBatch(gomock.Any(), maxTasks).
			Return([]*domain.Task{}, nil)

		// Call the method
		err := taskService.ExecutePendingTasks(ctx, maxTasks)

		// Assert no error
		assert.NoError(t, err)
	})

	t.Run("Continues processing even if SetLastCronRun fails", func(t *testing.T) {
		// Setup
		ctx := context.Background()
		maxTasks := 5
		settingErr := errors.New("setting update failed")

		// Configure setting repo mock to fail
		mockSettingRepo.EXPECT().
			SetLastCronRun(gomock.Any()).
			Return(settingErr).
			Times(1)

		// Configure mock to return empty task list - should still be called
		mockRepo.EXPECT().
			GetNextBatch(gomock.Any(), maxTasks).
			Return([]*domain.Task{}, nil)

		// Call the method
		err := taskService.ExecutePendingTasks(ctx, maxTasks)

		// Assert no error - should continue processing even if setting update fails
		assert.NoError(t, err)
	})
}

func TestTaskService_IsAutoExecuteEnabled(t *testing.T) {
	// Test TaskService.IsAutoExecuteEnabled - this was at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)

	t.Run("Default is enabled", func(t *testing.T) {
		// NewTaskService creates service with autoExecuteImmediate = true by default
		assert.True(t, taskService.IsAutoExecuteEnabled())
	})

	t.Run("Returns false when disabled", func(t *testing.T) {
		taskService.SetAutoExecuteImmediate(false)
		assert.False(t, taskService.IsAutoExecuteEnabled())
	})

	t.Run("Returns true when enabled", func(t *testing.T) {
		taskService.SetAutoExecuteImmediate(true)
		assert.True(t, taskService.IsAutoExecuteEnabled())
	})
}

func TestTaskService_ResetTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger to return itself for chaining
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)

	t.Run("Success - resets failed recurring task", func(t *testing.T) {
		ctx := context.Background()
		workspace := "ws-1"
		taskID := "task-1"
		interval := int64(60)
		integrationID := "int-123"
		lastError := "some error"

		task := &domain.Task{
			ID:                taskID,
			WorkspaceID:       workspace,
			Type:              "sync_integration",
			Status:            domain.TaskStatusFailed,
			RecurringInterval: &interval,
			IntegrationID:     &integrationID,
			State: &domain.TaskState{
				IntegrationSync: &domain.IntegrationSyncState{
					IntegrationID: integrationID,
					ConsecErrors:  5,
					LastError:     &lastError,
					LastErrorType: domain.ErrorTypeTransient,
				},
			},
		}

		mockRepo.EXPECT().Get(gomock.Any(), workspace, taskID).Return(task, nil)
		mockRepo.EXPECT().MarkAsPending(gomock.Any(), workspace, taskID, gomock.Any(), float64(0), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, nextRun time.Time, progress float64, state *domain.TaskState) error {
				// Verify error state was cleared
				assert.Equal(t, 0, state.IntegrationSync.ConsecErrors)
				assert.Nil(t, state.IntegrationSync.LastError)
				assert.Empty(t, state.IntegrationSync.LastErrorType)
				return nil
			})

		err := taskService.ResetTask(ctx, workspace, taskID)
		assert.NoError(t, err)
	})

	t.Run("Error - task not found", func(t *testing.T) {
		ctx := context.Background()
		workspace := "ws-1"
		taskID := "task-not-found"

		mockRepo.EXPECT().Get(gomock.Any(), workspace, taskID).Return(nil, fmt.Errorf("not found"))

		err := taskService.ResetTask(ctx, workspace, taskID)
		assert.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrTaskNotFound)
	})

	t.Run("Error - task not in failed state", func(t *testing.T) {
		ctx := context.Background()
		workspace := "ws-1"
		taskID := "task-1"
		interval := int64(60)

		task := &domain.Task{
			ID:                taskID,
			WorkspaceID:       workspace,
			Status:            domain.TaskStatusRunning, // Not failed
			RecurringInterval: &interval,
		}

		mockRepo.EXPECT().Get(gomock.Any(), workspace, taskID).Return(task, nil)

		err := taskService.ResetTask(ctx, workspace, taskID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task is not in failed state")
	})

	t.Run("Error - task is not recurring", func(t *testing.T) {
		ctx := context.Background()
		workspace := "ws-1"
		taskID := "task-1"

		task := &domain.Task{
			ID:          taskID,
			WorkspaceID: workspace,
			Status:      domain.TaskStatusFailed,
			// No RecurringInterval - not a recurring task
		}

		mockRepo.EXPECT().Get(gomock.Any(), workspace, taskID).Return(task, nil)

		err := taskService.ResetTask(ctx, workspace, taskID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task is not a recurring task")
	})
}

func TestTaskService_TriggerTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger to return itself for chaining
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)

	t.Run("Success - triggers recurring task", func(t *testing.T) {
		ctx := context.Background()
		workspace := "ws-1"
		taskID := "task-1"
		interval := int64(60)

		task := &domain.Task{
			ID:                taskID,
			WorkspaceID:       workspace,
			Status:            domain.TaskStatusPending,
			RecurringInterval: &interval,
		}

		mockRepo.EXPECT().Get(gomock.Any(), workspace, taskID).Return(task, nil)
		mockRepo.EXPECT().MarkAsPending(gomock.Any(), workspace, taskID, gomock.Any(), task.Progress, task.State).Return(nil)

		err := taskService.TriggerTask(ctx, workspace, taskID)
		assert.NoError(t, err)
	})

	t.Run("Error - task not found", func(t *testing.T) {
		ctx := context.Background()
		workspace := "ws-1"
		taskID := "task-not-found"

		mockRepo.EXPECT().Get(gomock.Any(), workspace, taskID).Return(nil, fmt.Errorf("not found"))

		err := taskService.TriggerTask(ctx, workspace, taskID)
		assert.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrTaskNotFound)
	})

	t.Run("Error - task is not recurring", func(t *testing.T) {
		ctx := context.Background()
		workspace := "ws-1"
		taskID := "task-1"

		task := &domain.Task{
			ID:          taskID,
			WorkspaceID: workspace,
			Status:      domain.TaskStatusPending,
			// No RecurringInterval
		}

		mockRepo.EXPECT().Get(gomock.Any(), workspace, taskID).Return(task, nil)

		err := taskService.TriggerTask(ctx, workspace, taskID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task is not a recurring task")
	})

	t.Run("Error - task already running", func(t *testing.T) {
		ctx := context.Background()
		workspace := "ws-1"
		taskID := "task-1"
		interval := int64(60)

		task := &domain.Task{
			ID:                taskID,
			WorkspaceID:       workspace,
			Status:            domain.TaskStatusRunning, // Already running
			RecurringInterval: &interval,
		}

		mockRepo.EXPECT().Get(gomock.Any(), workspace, taskID).Return(task, nil)

		err := taskService.TriggerTask(ctx, workspace, taskID)
		assert.Error(t, err)
		var alreadyRunningErr *domain.ErrTaskAlreadyRunning
		assert.True(t, errors.As(err, &alreadyRunningErr))
		assert.Equal(t, taskID, alreadyRunningErr.TaskID)
	})

	t.Run("Error - task in failed state cannot be triggered", func(t *testing.T) {
		ctx := context.Background()
		workspace := "ws-1"
		taskID := "task-1"
		interval := int64(60)

		task := &domain.Task{
			ID:                taskID,
			WorkspaceID:       workspace,
			Status:            domain.TaskStatusFailed, // Failed tasks need reset first
			RecurringInterval: &interval,
		}

		mockRepo.EXPECT().Get(gomock.Any(), workspace, taskID).Return(task, nil)

		err := taskService.TriggerTask(ctx, workspace, taskID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task cannot be triggered in current status")
	})
}

func TestTaskService_ExecuteTask_RecurringReschedules(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil
	apiEndpoint := "http://localhost:8080"

	// Configure logger to return itself for chaining
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, apiEndpoint)
	taskService.SetAutoExecuteImmediate(false)

	// Create a mock processor that handles sync_integration tasks
	mockProcessor := mocks.NewMockTaskProcessor(ctrl)
	// RegisterProcessor calls CanProcess for all task types
	mockProcessor.EXPECT().CanProcess(gomock.Any()).DoAndReturn(func(taskType string) bool {
		return taskType == "sync_integration"
	}).AnyTimes()
	taskService.RegisterProcessor(mockProcessor)

	// Setup transaction mocking
	mockRepo.EXPECT().
		WithTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error {
			return fn(nil)
		}).AnyTimes()

	t.Run("Recurring task reschedules after completion", func(t *testing.T) {
		ctx := context.Background()
		workspace := "ws-1"
		taskID := "task-1"
		interval := int64(60)
		integrationID := "int-123"

		task := &domain.Task{
			ID:                taskID,
			WorkspaceID:       workspace,
			Type:              "sync_integration",
			Status:            domain.TaskStatusPending,
			RecurringInterval: &interval,
			IntegrationID:     &integrationID,
			MaxRuntime:        300,
			State: &domain.TaskState{
				IntegrationSync: &domain.IntegrationSyncState{
					IntegrationID:   integrationID,
					IntegrationType: "test",
				},
			},
		}

		// Mock GetTx
		mockRepo.EXPECT().GetTx(gomock.Any(), gomock.Any(), workspace, taskID).Return(task, nil)

		// Mock MarkAsRunningTx
		mockRepo.EXPECT().MarkAsRunningTx(gomock.Any(), gomock.Any(), workspace, taskID, gomock.Any()).Return(nil)

		// Mock processor to complete successfully
		mockProcessor.EXPECT().Process(gomock.Any(), task, gomock.Any()).Return(true, nil)

		// Expect MarkAsPending (not MarkAsCompleted) because it's recurring
		mockRepo.EXPECT().MarkAsPending(gomock.Any(), workspace, taskID, gomock.Any(), float64(0), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, nextRun time.Time, progress float64, state *domain.TaskState) error {
				// Verify next run is approximately interval + jitter seconds in the future
				// Allow 1 second tolerance for timing differences between time.Now() calls
				expectedMinNext := time.Now().Add(time.Duration(interval-1) * time.Second)
				expectedMaxNext := time.Now().Add(time.Duration(interval+interval/10+5) * time.Second)
				assert.True(t, nextRun.After(expectedMinNext) || nextRun.Equal(expectedMinNext),
					"nextRun %v should be after expectedMinNext %v", nextRun, expectedMinNext)
				assert.True(t, nextRun.Before(expectedMaxNext),
					"nextRun %v should be before expectedMaxNext %v", nextRun, expectedMaxNext)
				return nil
			})

		timeoutAt := time.Now().Add(60 * time.Second)
		err := taskService.ExecuteTask(ctx, workspace, taskID, timeoutAt)
		assert.NoError(t, err)
	})
}

// Regression test for #320: an auth proxy (Cloudflare Access, oauth2-proxy,
// etc.) sitting in front of /api/tasks.execute returns a 302 to its login
// page. The Go default http.Client would follow as a GET to a 200 OK HTML
// page, and the dispatcher would silently log "dispatched successfully"
// while the task never ran. The dispatch client configures
// CheckRedirect = ErrUseLastResponse so the 302 surfaces in the non-200
// branch — and, critically, the redirect target is never fetched.
func TestTaskService_ExecutePendingTasks_DoesNotFollowRedirect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	var mockAuthService *AuthService = nil

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	var loginHits int32
	loginServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&loginHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>Please log in</html>"))
	}))
	defer loginServer.Close()

	var dispatchHits int32
	dispatchDone := make(chan struct{}, 1)
	dispatchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&dispatchHits, 1)
		w.Header().Set("Location", loginServer.URL+"/login")
		w.WriteHeader(http.StatusFound)
		select {
		case dispatchDone <- struct{}{}:
		default:
		}
	}))
	defer dispatchServer.Close()

	taskService := NewTaskService(mockRepo, mockSettingRepo, mockLogger, mockAuthService, dispatchServer.URL)
	taskService.SetAutoExecuteImmediate(false)

	mockSettingRepo.EXPECT().SetLastCronRun(gomock.Any()).Return(nil)
	mockRepo.EXPECT().GetNextBatch(gomock.Any(), gomock.Any()).Return([]*domain.Task{
		{ID: "t1", WorkspaceID: "ws1", Type: "send_broadcast", Status: domain.TaskStatusPending},
	}, nil)

	err := taskService.ExecutePendingTasks(context.Background(), 1)
	assert.NoError(t, err)

	select {
	case <-dispatchDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch goroutine to hit test server")
	}
	// Give the goroutine a moment to finish processing the response after the
	// server sent it, so a redirect-follow (if any) would have already fired.
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&dispatchHits), "dispatch endpoint should have been called exactly once")
	assert.Equal(t, int32(0), atomic.LoadInt32(&loginHits), "login endpoint must not be reached — redirect must not be followed")
}

// TestTaskService_ExecuteTask_GraceDeadline covers the execution backstop that
// lives inside ExecuteTask: the processor context carries a deadline of
// timeoutAt+taskExecutionGrace regardless of entry path, a processor stuck in a
// call that ignores that context is released at the deadline (task marked
// failed, function returns), and a panicking processor fails its task instead
// of crashing the process.
func TestTaskService_ExecuteTask_GraceDeadline(t *testing.T) {
	newService := func(t *testing.T) (*TaskService, *mocks.MockTaskRepository, *gomock.Controller) {
		ctrl := gomock.NewController(t)
		mockRepo := mocks.NewMockTaskRepository(ctrl)
		mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		svc := NewTaskService(mockRepo, mockSettingRepo, mockLogger, nil, "http://localhost:8080")
		svc.SetAutoExecuteImmediate(false)

		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error {
				return fn(nil)
			}).AnyTimes()

		return svc, mockRepo, ctrl
	}

	registerProcessor := func(ctrl *gomock.Controller, svc *TaskService) *mocks.MockTaskProcessor {
		mockProcessor := mocks.NewMockTaskProcessor(ctrl)
		for _, supportedType := range getTaskTypes() {
			mockProcessor.EXPECT().
				CanProcess(supportedType).
				Return(supportedType == "send_broadcast").
				AnyTimes()
		}
		svc.RegisterProcessor(mockProcessor)
		return mockProcessor
	}

	task := func() *domain.Task {
		return &domain.Task{
			ID:          "task123",
			WorkspaceID: "workspace1",
			Type:        "send_broadcast",
			Status:      domain.TaskStatusPending,
			MaxRuntime:  60,
		}
	}

	t.Run("processor context carries the grace deadline", func(t *testing.T) {
		svc, mockRepo, ctrl := newService(t)
		defer ctrl.Finish()
		mockProcessor := registerProcessor(ctrl, svc)

		ctx := context.Background()
		timeoutAt := time.Now().Add(60 * time.Second)
		wantDeadline := timeoutAt.Add(taskExecutionGrace)

		mockRepo.EXPECT().
			GetTx(gomock.Any(), gomock.Any(), "workspace1", "task123").
			Return(task(), nil)
		// The stored lease must also carry the grace, so a re-dispatcher cannot
		// reap this execution while it is still inside its grace window.
		mockRepo.EXPECT().
			MarkAsRunningTx(gomock.Any(), gomock.Any(), "workspace1", "task123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, _, _ string, lease time.Time) error {
				assert.WithinDuration(t, wantDeadline, lease, time.Second,
					"lease must extend past the timeout by the grace")
				return nil
			})

		var gotDeadline time.Time
		var hadDeadline bool
		mockProcessor.EXPECT().
			Process(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(procCtx context.Context, _ *domain.Task, procTimeout time.Time) (bool, error) {
				gotDeadline, hadDeadline = procCtx.Deadline()
				// The processor's own budget parameter stays the raw timeout.
				assert.WithinDuration(t, timeoutAt, procTimeout, time.Second)
				return true, nil
			})
		mockRepo.EXPECT().
			MarkAsCompleted(gomock.Any(), "workspace1", "task123", gomock.Any()).
			Return(nil)

		err := svc.ExecuteTask(ctx, "workspace1", "task123", timeoutAt)
		require.NoError(t, err)
		require.True(t, hadDeadline, "processor context must carry a deadline on every entry path")
		assert.WithinDuration(t, wantDeadline, gotDeadline, time.Second)
	})

	t.Run("stuck processor is released at the deadline", func(t *testing.T) {
		svc, mockRepo, ctrl := newService(t)
		defer ctrl.Finish()
		mockProcessor := registerProcessor(ctrl, svc)

		ctx := context.Background()
		// Deadline fires ~700ms after start: timeoutAt sits in the past so that
		// timeoutAt+grace lands just ahead of now. This simulates a processor
		// stuck in a call that ignores context cancellation entirely.
		timeoutAt := time.Now().Add(-taskExecutionGrace).Add(700 * time.Millisecond)

		block := make(chan struct{})
		t.Cleanup(func() { close(block) }) // release the zombie goroutine

		mockRepo.EXPECT().
			GetTx(gomock.Any(), gomock.Any(), "workspace1", "task123").
			Return(task(), nil)
		mockRepo.EXPECT().
			MarkAsRunningTx(gomock.Any(), gomock.Any(), "workspace1", "task123", gomock.Any()).
			Return(nil)
		mockProcessor.EXPECT().
			Process(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(context.Context, *domain.Task, time.Time) (bool, error) {
				<-block // ignores ctx: only the select's ctx.Done case can save the caller
				return false, nil
			})
		mockRepo.EXPECT().
			MarkAsFailed(gomock.Any(), "workspace1", "task123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, msg string) error {
				assert.Contains(t, msg, "exceeded timeout and grace")
				return nil
			})

		start := time.Now()
		err := svc.ExecuteTask(ctx, "workspace1", "task123", timeoutAt)
		elapsed := time.Since(start)

		require.Error(t, err)
		var taskExecError *domain.ErrTaskExecution
		require.True(t, errors.As(err, &taskExecError))
		assert.Equal(t, "execution exceeded timeout and grace", taskExecError.Reason)
		assert.Less(t, elapsed, 5*time.Second,
			"a stuck processor must not hold ExecuteTask past the deadline")
	})

	t.Run("panicking processor fails the task instead of crashing", func(t *testing.T) {
		svc, mockRepo, ctrl := newService(t)
		defer ctrl.Finish()
		mockProcessor := registerProcessor(ctrl, svc)

		ctx := context.Background()
		timeoutAt := time.Now().Add(60 * time.Second)

		mockRepo.EXPECT().
			GetTx(gomock.Any(), gomock.Any(), "workspace1", "task123").
			Return(task(), nil)
		mockRepo.EXPECT().
			MarkAsRunningTx(gomock.Any(), gomock.Any(), "workspace1", "task123", gomock.Any()).
			Return(nil)
		mockProcessor.EXPECT().
			Process(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(context.Context, *domain.Task, time.Time) (bool, error) {
				panic("boom: nil map write in a processor")
			})
		mockRepo.EXPECT().
			MarkAsFailed(gomock.Any(), "workspace1", "task123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, msg string) error {
				assert.Contains(t, msg, "processor panicked")
				assert.Contains(t, msg, "boom")
				return nil
			})

		var err error
		require.NotPanics(t, func() {
			err = svc.ExecuteTask(ctx, "workspace1", "task123", timeoutAt)
		})
		require.Error(t, err)
		var taskExecError *domain.ErrTaskExecution
		require.True(t, errors.As(err, &taskExecError))
		assert.Equal(t, "processor panicked", taskExecError.Reason)
	})
}

// TestDispatchTimeout_ScalesWithMaxRuntime pins the dispatch wait to the task's
// own budget. The fixed 53s it replaced left only a 3s margin over a broadcast
// slice — the gap the enqueue transaction was cancelled in — and silently
// truncated every longer-running task type.
func TestDispatchTimeout_ScalesWithMaxRuntime(t *testing.T) {
	tests := []struct {
		name       string
		maxRuntime int
		want       time.Duration
	}{
		{"broadcast slice", 50, 90 * time.Second},
		{"segment recompute", 300, 340 * time.Second},
		{"unset falls back to the default", 0, 90 * time.Second},
		{"negative falls back to the default", -1, 90 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dispatchTimeout(&domain.Task{MaxRuntime: tt.maxRuntime})
			assert.Equal(t, tt.want, got)
			assert.Greater(t, got, time.Duration(tt.maxRuntime)*time.Second,
				"the dispatcher must outlast the task it started")
		})
	}
}

// interruptingProcessor blocks until its context is cancelled, then reports the
// cancellation the way a real processor does — wrapped, not bare.
type interruptingProcessor struct {
	taskType string
	started  chan struct{}
	wrap     func(error) error
}

func (p *interruptingProcessor) CanProcess(taskType string) bool { return taskType == p.taskType }

func (p *interruptingProcessor) Process(ctx context.Context, _ *domain.Task, _ time.Time) (bool, error) {
	select {
	case p.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	if p.wrap != nil {
		return false, p.wrap(ctx.Err())
	}
	return false, ctx.Err()
}

// TestTaskService_ExecuteTask_InterruptionDoesNotConsumeRetryBudget covers the
// defect that turned one cancelled context into a dead broadcast: an execution
// cut short by its parent going away was recorded as a failed attempt, so three
// deploys (or three proxy timeouts) exhausted max_retries.
func TestTaskService_ExecuteTask_InterruptionDoesNotConsumeRetryBudget(t *testing.T) {
	setup := func(t *testing.T, wrap func(error) error) (*TaskService, *mocks.MockTaskRepository, *interruptingProcessor) {
		t.Helper()
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockRepo := mocks.NewMockTaskRepository(ctrl)
		mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		svc := NewTaskService(mockRepo, mockSettingRepo, mockLogger, nil, "http://localhost:8080")
		svc.SetAutoExecuteImmediate(false)

		proc := &interruptingProcessor{taskType: "generate_report", started: make(chan struct{}, 1), wrap: wrap}
		svc.RegisterProcessor(proc)

		task := &domain.Task{
			ID:          "task-1",
			WorkspaceID: "ws-1",
			Type:        "generate_report",
			Status:      domain.TaskStatusPending,
			MaxRuntime:  50,
			MaxRetries:  3,
		}

		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error { return fn(nil) }).
			AnyTimes()
		mockRepo.EXPECT().GetTx(gomock.Any(), gomock.Any(), "ws-1", "task-1").Return(task, nil).AnyTimes()
		mockRepo.EXPECT().MarkAsRunningTx(gomock.Any(), gomock.Any(), "ws-1", "task-1", gomock.Any()).Return(nil).AnyTimes()

		return svc, mockRepo, proc
	}

	t.Run("releases instead of failing when the parent context is cancelled", func(t *testing.T) {
		svc, mockRepo, proc := setup(t, nil)

		released := make(chan time.Time, 1)
		mockRepo.EXPECT().
			ReleaseTask(gomock.Any(), "ws-1", "task-1", gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, reason string, nextRunAfter time.Time) error {
				assert.Contains(t, reason, "interrupted")
				released <- nextRunAfter
				return nil
			}).
			Times(1)
		// The whole point: no attempt is recorded.
		mockRepo.EXPECT().MarkAsFailed(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-proc.started
			cancel()
		}()

		err := svc.ExecuteTask(ctx, "ws-1", "task-1", time.Now().Add(50*time.Second))
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)

		select {
		case nextRunAfter := <-released:
			assert.False(t, nextRunAfter.IsZero(), "the task must be re-queued")
		case <-time.After(2 * time.Second):
			t.Fatal("ReleaseTask was never called")
		}
	})

	t.Run("still fails on a genuine processor error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mocks.NewMockTaskRepository(ctrl)
		mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		svc := NewTaskService(mockRepo, mockSettingRepo, mockLogger, nil, "http://localhost:8080")
		svc.SetAutoExecuteImmediate(false)
		svc.RegisterProcessor(&failingProcessor{})

		task := &domain.Task{
			ID: "task-2", WorkspaceID: "ws-1", Type: "export_contacts",
			Status: domain.TaskStatusPending, MaxRuntime: 50, MaxRetries: 3,
		}
		mockRepo.EXPECT().
			WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error { return fn(nil) }).
			AnyTimes()
		mockRepo.EXPECT().GetTx(gomock.Any(), gomock.Any(), "ws-1", "task-2").Return(task, nil).AnyTimes()
		mockRepo.EXPECT().MarkAsRunningTx(gomock.Any(), gomock.Any(), "ws-1", "task-2", gomock.Any()).Return(nil).AnyTimes()
		mockRepo.EXPECT().MarkAsFailed(gomock.Any(), "ws-1", "task-2", gomock.Any()).Return(nil).Times(1)
		mockRepo.EXPECT().ReleaseTask(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		err := svc.ExecuteTask(context.Background(), "ws-1", "task-2", time.Now().Add(50*time.Second))
		require.Error(t, err)
	})
}

// failingProcessor returns an ordinary error, which must still consume a retry.
type failingProcessor struct{}

func (p *failingProcessor) CanProcess(taskType string) bool { return taskType == "export_contacts" }
func (p *failingProcessor) Process(context.Context, *domain.Task, time.Time) (bool, error) {
	return false, errors.New("processor blew up")
}

// TestNewDispatchRequest_CarriesTheTaskBudget pins the dispatch wiring, not just
// the arithmetic: a request built without a deadline (the plain http.NewRequest
// this replaced) leaves the shared client's timeout in charge, which is how a
// 50s broadcast slice ended up cancelled by a 53s client.
func TestNewDispatchRequest_CarriesTheTaskBudget(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svc := NewTaskService(
		mocks.NewMockTaskRepository(ctrl),
		mocks.NewMockSettingRepository(ctrl),
		pkgmocks.NewMockLogger(ctrl),
		nil,
		"https://api.example.com",
	)

	for _, tc := range []struct {
		name       string
		maxRuntime int
	}{
		{"broadcast slice", 50},
		{"segment recompute", 300},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &domain.Task{ID: "task-1", WorkspaceID: "ws-1", MaxRuntime: tc.maxRuntime}

			before := time.Now()
			req, cancel, err := svc.newDispatchRequest(context.Background(), task, []byte(`{}`))
			require.NoError(t, err)
			defer cancel()

			deadline, ok := req.Context().Deadline()
			require.True(t, ok, "the dispatch must carry its own deadline")

			budget := deadline.Sub(before)
			assert.InDelta(t, dispatchTimeout(task).Seconds(), budget.Seconds(), 1.0)
			assert.Greater(t, budget, time.Duration(tc.maxRuntime)*time.Second,
				"the dispatcher must outlast the task it started")

			assert.Equal(t, "https://api.example.com/api/tasks.execute", req.URL.String())
			assert.Equal(t, "task-1", req.Header.Get("X-Task-ID"))
		})
	}
}

// TestNewDispatchHTTPClient_TimeoutNeverBinds guards the other half: the shared
// client's timeout is a backstop, so it must stay above the longest budget any
// task can ask for. Lowering it back to a fixed value silently truncates runs.
func TestNewDispatchHTTPClient_TimeoutNeverBinds(t *testing.T) {
	client := newDispatchHTTPClient()
	longest := dispatchTimeout(&domain.Task{MaxRuntime: 300}) // segment recompute

	assert.Greater(t, client.Timeout, longest,
		"the shared client must never become the binding constraint")
}

// TestTaskService_ExecuteTask_InterruptionMaskedByTxDone covers the case the
// error-only check missed: a cancellation that lands inside a transaction is
// reported by database/sql as "transaction has already been committed or rolled
// back", an error whose chain contains no context at all. The task must still be
// re-queued rather than charged a failed attempt.
func TestTaskService_ExecuteTask_InterruptionMaskedByTxDone(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockTaskRepository(ctrl)
	mockSettingRepo := mocks.NewMockSettingRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	svc := NewTaskService(mockRepo, mockSettingRepo, mockLogger, nil, "http://localhost:8080")
	svc.SetAutoExecuteImmediate(false)

	// The processor waits for the cancellation, then reports it the way a rolled
	// back transaction does — with the cancellation nowhere in the chain.
	proc := &interruptingProcessor{
		taskType: "generate_report",
		started:  make(chan struct{}, 1),
		wrap: func(error) error {
			return errors.New("failed to commit transaction: sql: transaction has already been committed or rolled back")
		},
	}
	svc.RegisterProcessor(proc)

	task := &domain.Task{
		ID: "task-1", WorkspaceID: "ws-1", Type: "generate_report",
		Status: domain.TaskStatusPending, MaxRuntime: 50, MaxRetries: 3,
	}
	mockRepo.EXPECT().
		WithTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error { return fn(nil) }).
		AnyTimes()
	mockRepo.EXPECT().GetTx(gomock.Any(), gomock.Any(), "ws-1", "task-1").Return(task, nil).AnyTimes()
	mockRepo.EXPECT().MarkAsRunningTx(gomock.Any(), gomock.Any(), "ws-1", "task-1", gomock.Any()).Return(nil).AnyTimes()

	released := make(chan struct{}, 1)
	mockRepo.EXPECT().
		ReleaseTask(gomock.Any(), "ws-1", "task-1", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ string, _ time.Time) error {
			released <- struct{}{}
			return nil
		}).
		Times(1)
	mockRepo.EXPECT().MarkAsFailed(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-proc.started
		cancel()
	}()

	err := svc.ExecuteTask(ctx, "ws-1", "task-1", time.Now().Add(50*time.Second))
	require.Error(t, err)

	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("the task was never re-queued")
	}
}

// TestNewDispatchRequest_IsSigned pins the credential the dispatch carries.
// /api/tasks.execute has no session to authenticate — the caller is this process
// talking to itself through the ingress — so an unsigned dispatch is
// indistinguishable from anyone else's and the handler rejects it.
func TestNewDispatchRequest_IsSigned(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	newService := func() *TaskService {
		return NewTaskService(
			mocks.NewMockTaskRepository(ctrl),
			mocks.NewMockSettingRepository(ctrl),
			pkgmocks.NewMockLogger(ctrl),
			nil,
			"https://api.example.com",
		)
	}

	task := &domain.Task{ID: "task-1", WorkspaceID: "ws-1", MaxRuntime: 50}
	body := []byte(`{"workspace_id":"ws-1","id":"task-1"}`)

	t.Run("the signature verifies against the path and the body", func(t *testing.T) {
		svc := newService()
		svc.SetSecretKey("installation-secret")

		req, cancel, err := svc.newDispatchRequest(context.Background(), task, body)
		require.NoError(t, err)
		defer cancel()

		timestamp, err := domain.ParseTaskExecuteTimestamp(req.Header.Get(domain.TaskExecuteTimestampHeader))
		require.NoError(t, err)

		key := domain.TaskExecuteSigningKey("installation-secret")
		assert.NoError(t, domain.VerifyTaskExecuteSignature(key, timestamp, req.URL.Path, body,
			req.Header.Get(domain.TaskExecuteSignatureHeader), time.Now()))

		// The body is inside the signed content, so the same headers do not
		// authenticate a dispatch for another task.
		other := []byte(`{"workspace_id":"ws-1","id":"task-2"}`)
		assert.Error(t, domain.VerifyTaskExecuteSignature(key, timestamp, req.URL.Path, other,
			req.Header.Get(domain.TaskExecuteSignatureHeader), time.Now()))
	})

	t.Run("no secret key means no signature headers", func(t *testing.T) {
		svc := newService()

		req, cancel, err := svc.newDispatchRequest(context.Background(), task, body)
		require.NoError(t, err)
		defer cancel()

		assert.Empty(t, req.Header.Get(domain.TaskExecuteTimestampHeader))
		assert.Empty(t, req.Header.Get(domain.TaskExecuteSignatureHeader))
	})
}
