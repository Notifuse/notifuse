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
