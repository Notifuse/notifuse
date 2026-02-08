package domain

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskState_Value(t *testing.T) {
	t.Run("empty state", func(t *testing.T) {
		state := TaskState{}
		value, err := state.Value()
		require.NoError(t, err)

		// Convert value back to bytes for assertion
		bytes, ok := value.([]byte)
		require.True(t, ok, "Expected []byte, got %T", value)

		// Validate JSON
		var m map[string]interface{}
		err = json.Unmarshal(bytes, &m)
		require.NoError(t, err)

		// Should be an empty object
		assert.Empty(t, m)
	})

	t.Run("with common fields", func(t *testing.T) {
		state := TaskState{
			Progress: 50.5,
			Message:  "Half done",
		}
		value, err := state.Value()
		require.NoError(t, err)

		bytes, ok := value.([]byte)
		require.True(t, ok)

		var m map[string]interface{}
		err = json.Unmarshal(bytes, &m)
		require.NoError(t, err)

		assert.Equal(t, 50.5, m["progress"])
		assert.Equal(t, "Half done", m["message"])
	})

	t.Run("with specialized fields", func(t *testing.T) {
		state := TaskState{
			Progress: 75.0,
			Message:  "Processing broadcast",
			SendBroadcast: &SendBroadcastState{
				BroadcastID:     "broadcast-123",
				TotalRecipients: 1000,
				EnqueuedCount:   750,
				FailedCount:     10,
				ChannelType:     "email",
				RecipientOffset: 750,
			},
		}
		value, err := state.Value()
		require.NoError(t, err)

		bytes, ok := value.([]byte)
		require.True(t, ok)

		var m map[string]interface{}
		err = json.Unmarshal(bytes, &m)
		require.NoError(t, err)

		assert.Equal(t, 75.0, m["progress"])
		assert.Equal(t, "Processing broadcast", m["message"])

		// Check specialized fields
		broadcastMap, ok := m["send_broadcast"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "broadcast-123", broadcastMap["broadcast_id"])
		assert.Equal(t, float64(1000), broadcastMap["total_recipients"])
		assert.Equal(t, float64(750), broadcastMap["enqueued_count"])
		assert.Equal(t, float64(10), broadcastMap["failed_count"])
		assert.Equal(t, "email", broadcastMap["channel_type"])
		assert.Equal(t, float64(750), broadcastMap["recipient_offset"])
	})
}

func TestTaskState_Scan(t *testing.T) {
	t.Run("nil value", func(t *testing.T) {
		var state TaskState
		err := state.Scan(nil)
		require.NoError(t, err)

		// Should result in empty task state
		assert.Equal(t, 0.0, state.Progress)
		assert.Equal(t, "", state.Message)
		assert.Nil(t, state.SendBroadcast)
	})

	t.Run("empty json", func(t *testing.T) {
		var state TaskState
		err := state.Scan([]byte(`{}`))
		require.NoError(t, err)

		// Should result in empty task state
		assert.Equal(t, 0.0, state.Progress)
		assert.Equal(t, "", state.Message)
		assert.Nil(t, state.SendBroadcast)
	})

	t.Run("with common fields", func(t *testing.T) {
		var state TaskState
		data := []byte(`{"progress": 42.5, "message": "Working on it"}`)

		err := state.Scan(data)
		require.NoError(t, err)

		assert.Equal(t, 42.5, state.Progress)
		assert.Equal(t, "Working on it", state.Message)
		assert.Nil(t, state.SendBroadcast)
	})

	t.Run("with specialized fields", func(t *testing.T) {
		var state TaskState
		data := []byte(`{
			"progress": 60.0,
			"message": "Sending emails",
			"send_broadcast": {
				"broadcast_id": "broadcast-456",
				"total_recipients": 500,
				"enqueued_count": 300,
				"failed_count": 5,
				"channel_type": "email",
				"recipient_offset": 300
			}
		}`)

		err := state.Scan(data)
		require.NoError(t, err)

		assert.Equal(t, 60.0, state.Progress)
		assert.Equal(t, "Sending emails", state.Message)
		assert.NotNil(t, state.SendBroadcast)
		assert.Equal(t, "broadcast-456", state.SendBroadcast.BroadcastID)
		assert.Equal(t, 500, state.SendBroadcast.TotalRecipients)
		assert.Equal(t, 300, state.SendBroadcast.EnqueuedCount)
		assert.Equal(t, 5, state.SendBroadcast.FailedCount)
		assert.Equal(t, "email", state.SendBroadcast.ChannelType)
		assert.Equal(t, int64(300), state.SendBroadcast.RecipientOffset)
	})

	t.Run("invalid type", func(t *testing.T) {
		var state TaskState
		err := state.Scan(123)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected []byte")
	})

	t.Run("invalid json", func(t *testing.T) {
		var state TaskState
		err := state.Scan([]byte(`{not valid json`))
		require.Error(t, err)
	})
}

func TestGetTaskRequest_FromURLParams(t *testing.T) {
	t.Run("valid params", func(t *testing.T) {
		values := url.Values{
			"workspace_id": []string{"ws-123"},
			"id":           []string{"task-456"},
		}

		req := &GetTaskRequest{}
		err := req.FromURLParams(values)
		require.NoError(t, err)

		assert.Equal(t, "ws-123", req.WorkspaceID)
		assert.Equal(t, "task-456", req.ID)
	})

	t.Run("missing params", func(t *testing.T) {
		values := url.Values{
			"workspace_id": []string{"ws-123"},
			// Missing ID
		}

		req := &GetTaskRequest{}
		err := req.FromURLParams(values)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestDeleteTaskRequest_FromURLParams(t *testing.T) {
	t.Run("valid params", func(t *testing.T) {
		values := url.Values{
			"workspace_id": []string{"ws-123"},
			"id":           []string{"task-456"},
		}

		req := &DeleteTaskRequest{}
		err := req.FromURLParams(values)
		require.NoError(t, err)

		assert.Equal(t, "ws-123", req.WorkspaceID)
		assert.Equal(t, "task-456", req.ID)
	})

	t.Run("missing workspace", func(t *testing.T) {
		values := url.Values{
			// Missing workspace_id
			"id": []string{"task-456"},
		}

		req := &DeleteTaskRequest{}
		err := req.FromURLParams(values)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})
}

func TestListTasksRequest_FromURLParams(t *testing.T) {
	t.Run("full params", func(t *testing.T) {
		values := url.Values{
			"workspace_id":   []string{"ws-123"},
			"status":         []string{"pending,running"},
			"type":           []string{"broadcast,import"},
			"created_after":  []string{"2023-01-01T00:00:00Z"},
			"created_before": []string{"2023-12-31T23:59:59Z"},
			"limit":          []string{"50"},
			"offset":         []string{"10"},
		}

		req := &ListTasksRequest{}
		err := req.FromURLParams(values)
		require.NoError(t, err)

		assert.Equal(t, "ws-123", req.WorkspaceID)
		assert.Equal(t, []string{"pending", "running"}, req.Status)
		assert.Equal(t, []string{"broadcast", "import"}, req.Type)
		assert.Equal(t, "2023-01-01T00:00:00Z", req.CreatedAfter)
		assert.Equal(t, "2023-12-31T23:59:59Z", req.CreatedBefore)
		assert.Equal(t, 50, req.Limit)
		assert.Equal(t, 10, req.Offset)
	})

	t.Run("minimal params", func(t *testing.T) {
		values := url.Values{
			"workspace_id": []string{"ws-123"},
			// No optional params
		}

		req := &ListTasksRequest{}
		err := req.FromURLParams(values)
		require.NoError(t, err)

		assert.Equal(t, "ws-123", req.WorkspaceID)
		assert.Empty(t, req.Status)
		assert.Empty(t, req.Type)
		assert.Empty(t, req.CreatedAfter)
		assert.Empty(t, req.CreatedBefore)
		assert.Equal(t, 0, req.Limit)  // Default value
		assert.Equal(t, 0, req.Offset) // Default value
	})

	t.Run("invalid limit/offset", func(t *testing.T) {
		values := url.Values{
			"workspace_id": []string{"ws-123"},
			"limit":        []string{"not-a-number"},
			"offset":       []string{"also-not-a-number"},
		}

		req := &ListTasksRequest{}
		err := req.FromURLParams(values)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid limit")
	})
}

func TestListTasksRequest_ToFilter(t *testing.T) {
	t.Run("convert all fields", func(t *testing.T) {
		// Create a request with all fields populated
		req := &ListTasksRequest{
			WorkspaceID:   "ws-123",
			Status:        []string{"pending", "running"},
			Type:          []string{"broadcast", "import"},
			CreatedAfter:  "2023-01-01T00:00:00Z",
			CreatedBefore: "2023-12-31T23:59:59Z",
			Limit:         50,
			Offset:        10,
		}

		filter := req.ToFilter()

		// Check that statuses were converted properly
		assert.Len(t, filter.Status, 2)
		assert.Contains(t, filter.Status, TaskStatus("pending"))
		assert.Contains(t, filter.Status, TaskStatus("running"))

		// Check other fields
		assert.Equal(t, []string{"broadcast", "import"}, filter.Type)
		assert.Equal(t, 50, filter.Limit)
		assert.Equal(t, 10, filter.Offset)

		// Check time conversions
		require.NotNil(t, filter.CreatedAfter)
		require.NotNil(t, filter.CreatedBefore)

		expectedStartTime, _ := time.Parse(time.RFC3339, "2023-01-01T00:00:00Z")
		expectedEndTime, _ := time.Parse(time.RFC3339, "2023-12-31T23:59:59Z")

		assert.Equal(t, expectedStartTime, *filter.CreatedAfter)
		assert.Equal(t, expectedEndTime, *filter.CreatedBefore)
	})

	t.Run("minimal fields", func(t *testing.T) {
		// Create a request with minimal fields
		req := &ListTasksRequest{
			WorkspaceID: "ws-123",
			// No optional params
		}

		filter := req.ToFilter()

		// Check defaults
		assert.Empty(t, filter.Status)
		assert.Empty(t, filter.Type)
		assert.Nil(t, filter.CreatedAfter)
		assert.Nil(t, filter.CreatedBefore)
		assert.Equal(t, 100, filter.Limit)
		assert.Equal(t, 0, filter.Offset)
	})

	t.Run("invalid time format", func(t *testing.T) {
		// Create a request with invalid time format
		req := &ListTasksRequest{
			WorkspaceID:  "ws-123",
			CreatedAfter: "not-a-valid-time",
		}

		filter := req.ToFilter()

		// Time parsing should fail silently, returning nil
		assert.Nil(t, filter.CreatedAfter)
	})
}

func TestSplitAndTrim(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		result := splitAndTrim("")
		assert.Empty(t, result)
	})

	t.Run("single value", func(t *testing.T) {
		result := splitAndTrim("value")
		assert.Equal(t, []string{"value"}, result)
	})

	t.Run("multiple values", func(t *testing.T) {
		result := splitAndTrim("one,two,three")
		assert.Equal(t, []string{"one", "two", "three"}, result)
	})

	t.Run("with spaces", func(t *testing.T) {
		result := splitAndTrim(" one , two , three ")
		assert.Equal(t, []string{"one", "two", "three"}, result)
	})

	t.Run("with empty segments", func(t *testing.T) {
		result := splitAndTrim("one,,three")
		assert.Equal(t, []string{"one", "three"}, result)
	})
}

func TestExecutePendingTasksRequest_FromURLParams(t *testing.T) {
	t.Run("with max_tasks", func(t *testing.T) {
		values := url.Values{
			"max_tasks": []string{"20"},
		}

		req := &ExecutePendingTasksRequest{}
		err := req.FromURLParams(values)
		require.NoError(t, err)

		assert.Equal(t, 20, req.MaxTasks)
	})

	t.Run("without max_tasks", func(t *testing.T) {
		values := url.Values{}

		req := &ExecutePendingTasksRequest{}
		err := req.FromURLParams(values)
		require.NoError(t, err)

		// The implementation uses a default value of 100
		assert.Equal(t, 100, req.MaxTasks) // Default value is 100 in the implementation
	})

	t.Run("invalid max_tasks", func(t *testing.T) {
		values := url.Values{
			"max_tasks": []string{"not-a-number"},
		}

		req := &ExecutePendingTasksRequest{}
		err := req.FromURLParams(values)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid max_tasks")
	})

	// The implementation doesn't validate for negative max_tasks values
	t.Run("negative max_tasks", func(t *testing.T) {
		values := url.Values{
			"max_tasks": []string{"-10"},
		}

		req := &ExecutePendingTasksRequest{}
		err := req.FromURLParams(values)

		// There's no validation for negative max_tasks in the implementation
		require.NoError(t, err)
		assert.Equal(t, -10, req.MaxTasks)
	})
}

func TestCreateTaskRequest_Validate(t *testing.T) {
	t.Run("valid request with minimal fields", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID: "ws-123",
			Type:        "send_broadcast",
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)

		assert.Equal(t, "ws-123", task.WorkspaceID)
		assert.Equal(t, "send_broadcast", task.Type)
		assert.Equal(t, TaskStatusPending, task.Status)
		assert.Equal(t, 0.0, task.Progress)
		assert.Nil(t, task.State)
		assert.Nil(t, task.ErrorMessage)
		assert.Equal(t, 50, task.MaxRuntime)     // Default value
		assert.Equal(t, 3, task.MaxRetries)      // Default value
		assert.Equal(t, 300, task.RetryInterval) // Default value
		assert.Equal(t, 0, task.RetryCount)
		assert.Nil(t, task.BroadcastID)
		assert.Nil(t, task.LastRunAt)
		assert.Nil(t, task.CompletedAt)
		assert.Nil(t, task.NextRunAfter)
		assert.Nil(t, task.TimeoutAfter)
		assert.False(t, task.CreatedAt.IsZero())
		assert.False(t, task.UpdatedAt.IsZero())
	})

	t.Run("valid request with all fields", func(t *testing.T) {
		nextRunAfter := time.Now().Add(time.Hour)
		state := &TaskState{
			Progress: 25.0,
			Message:  "Starting task",
			SendBroadcast: &SendBroadcastState{
				BroadcastID:     "broadcast-123",
				TotalRecipients: 1000,
				EnqueuedCount:   0,
				FailedCount:     0,
				ChannelType:     "email",
				RecipientOffset: 0,
			},
		}

		req := &CreateTaskRequest{
			WorkspaceID:   "ws-456",
			Type:          "send_broadcast",
			State:         state,
			MaxRuntime:    600,
			MaxRetries:    5,
			RetryInterval: 120,
			NextRunAfter:  &nextRunAfter,
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)

		assert.Equal(t, "ws-456", task.WorkspaceID)
		assert.Equal(t, "send_broadcast", task.Type)
		assert.Equal(t, TaskStatusPending, task.Status)
		assert.Equal(t, 0.0, task.Progress)
		assert.Equal(t, state, task.State)
		assert.Nil(t, task.ErrorMessage)
		assert.Equal(t, 600, task.MaxRuntime)
		assert.Equal(t, 5, task.MaxRetries)
		assert.Equal(t, 120, task.RetryInterval)
		assert.Equal(t, 0, task.RetryCount)
		assert.Equal(t, &nextRunAfter, task.NextRunAfter)
		assert.Nil(t, task.BroadcastID)
		assert.Nil(t, task.LastRunAt)
		assert.Nil(t, task.CompletedAt)
		assert.Nil(t, task.TimeoutAfter)
		assert.False(t, task.CreatedAt.IsZero())
		assert.False(t, task.UpdatedAt.IsZero())
	})

	t.Run("missing workspace_id", func(t *testing.T) {
		req := &CreateTaskRequest{
			Type: "send_broadcast",
		}

		task, err := req.Validate()
		require.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("empty workspace_id", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID: "",
			Type:        "send_broadcast",
		}

		task, err := req.Validate()
		require.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("missing type", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID: "ws-123",
		}

		task, err := req.Validate()
		require.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "task type is required")
	})

	t.Run("empty type", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID: "ws-123",
			Type:        "",
		}

		task, err := req.Validate()
		require.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "task type is required")
	})

	t.Run("zero max_runtime gets default", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID: "ws-123",
			Type:        "send_broadcast",
			MaxRuntime:  0,
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, 50, task.MaxRuntime) // Default value
	})

	t.Run("negative max_runtime gets default", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID: "ws-123",
			Type:        "send_broadcast",
			MaxRuntime:  -100,
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, 50, task.MaxRuntime) // Default value
	})

	t.Run("zero max_retries gets default", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID: "ws-123",
			Type:        "send_broadcast",
			MaxRetries:  0,
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, 3, task.MaxRetries) // Default value
	})

	t.Run("negative max_retries gets default", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID: "ws-123",
			Type:        "send_broadcast",
			MaxRetries:  -5,
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, 3, task.MaxRetries) // Default value
	})

	t.Run("zero retry_interval gets default", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID:   "ws-123",
			Type:          "send_broadcast",
			RetryInterval: 0,
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, 300, task.RetryInterval) // Default value
	})

	t.Run("negative retry_interval gets default", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID:   "ws-123",
			Type:          "send_broadcast",
			RetryInterval: -60,
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, 300, task.RetryInterval) // Default value
	})

	t.Run("custom values are preserved", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID:   "ws-123",
			Type:          "send_broadcast",
			MaxRuntime:    1800, // 30 minutes
			MaxRetries:    10,
			RetryInterval: 60, // 1 minute
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, 1800, task.MaxRuntime)
		assert.Equal(t, 10, task.MaxRetries)
		assert.Equal(t, 60, task.RetryInterval)
	})

	t.Run("state is preserved", func(t *testing.T) {
		state := &TaskState{
			Progress: 10.5,
			Message:  "Initializing",
			SendBroadcast: &SendBroadcastState{
				BroadcastID:     "broadcast-789",
				TotalRecipients: 500,
				EnqueuedCount:   0,
				FailedCount:     0,
				ChannelType:     "sms",
				RecipientOffset: 0,
			},
		}

		req := &CreateTaskRequest{
			WorkspaceID: "ws-123",
			Type:        "send_broadcast",
			State:       state,
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, state, task.State)
		assert.Equal(t, state.Progress, task.State.Progress)
		assert.Equal(t, state.Message, task.State.Message)
		assert.Equal(t, state.SendBroadcast, task.State.SendBroadcast)
	})

	t.Run("next_run_after is preserved", func(t *testing.T) {
		futureTime := time.Now().Add(2 * time.Hour)
		req := &CreateTaskRequest{
			WorkspaceID:  "ws-123",
			Type:         "send_broadcast",
			NextRunAfter: &futureTime,
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, &futureTime, task.NextRunAfter)
	})

	t.Run("recurring_interval and integration_id are preserved", func(t *testing.T) {
		interval := int64(300)
		integrationID := "integration-123"
		req := &CreateTaskRequest{
			WorkspaceID:       "ws-123",
			Type:              "sync_integration",
			RecurringInterval: &interval,
			IntegrationID:     &integrationID,
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)
		require.NotNil(t, task.RecurringInterval)
		assert.Equal(t, int64(300), *task.RecurringInterval)
		require.NotNil(t, task.IntegrationID)
		assert.Equal(t, "integration-123", *task.IntegrationID)
		assert.True(t, task.IsRecurring())
	})

	t.Run("nil recurring fields create non-recurring task", func(t *testing.T) {
		req := &CreateTaskRequest{
			WorkspaceID: "ws-123",
			Type:        "send_broadcast",
		}

		task, err := req.Validate()
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Nil(t, task.RecurringInterval)
		assert.Nil(t, task.IntegrationID)
		assert.False(t, task.IsRecurring())
	})
}

func TestExecuteTaskRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &ExecuteTaskRequest{
			WorkspaceID: "ws-123",
			ID:          "task-456",
		}

		err := req.Validate()
		require.NoError(t, err)
	})

	t.Run("missing workspace_id", func(t *testing.T) {
		req := &ExecuteTaskRequest{
			ID: "task-456",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("missing id", func(t *testing.T) {
		req := &ExecuteTaskRequest{
			WorkspaceID: "ws-123",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "task id is required")
	})
}

// Helper function for creating int64 pointers
func ptrInt64(v int64) *int64 {
	return &v
}

// Helper function for creating string pointers
func ptrString(v string) *string {
	return &v
}

func TestTask_IsRecurring(t *testing.T) {
	t.Run("nil interval", func(t *testing.T) {
		task := &Task{
			RecurringInterval: nil,
		}
		assert.False(t, task.IsRecurring())
	})

	t.Run("zero interval", func(t *testing.T) {
		task := &Task{
			RecurringInterval: ptrInt64(0),
		}
		assert.False(t, task.IsRecurring())
	})

	t.Run("negative interval", func(t *testing.T) {
		task := &Task{
			RecurringInterval: ptrInt64(-1),
		}
		assert.False(t, task.IsRecurring())
	})

	t.Run("positive interval", func(t *testing.T) {
		task := &Task{
			RecurringInterval: ptrInt64(60),
		}
		assert.True(t, task.IsRecurring())
	})

	t.Run("large interval", func(t *testing.T) {
		task := &Task{
			RecurringInterval: ptrInt64(3600), // 1 hour
		}
		assert.True(t, task.IsRecurring())
	})
}

func TestIntegrationSyncState_JSON(t *testing.T) {
	t.Run("marshal and unmarshal", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		errMsg := "connection timeout"
		state := &IntegrationSyncState{
			IntegrationID:   "int-123",
			IntegrationType: "staminads",
			Cursor:          "cursor-abc",
			LastSyncAt:      &now,
			LastSuccessAt:   &now,
			EventsImported:  1000,
			LastEventCount:  50,
			ConsecErrors:    3,
			LastError:       &errMsg,
			LastErrorType:   ErrorTypeTransient,
		}

		// Marshal
		data, err := json.Marshal(state)
		require.NoError(t, err)

		// Unmarshal
		var restored IntegrationSyncState
		err = json.Unmarshal(data, &restored)
		require.NoError(t, err)

		// Verify fields
		assert.Equal(t, "int-123", restored.IntegrationID)
		assert.Equal(t, "staminads", restored.IntegrationType)
		assert.Equal(t, "cursor-abc", restored.Cursor)
		assert.NotNil(t, restored.LastSyncAt)
		assert.Equal(t, now.Unix(), restored.LastSyncAt.Unix())
		assert.NotNil(t, restored.LastSuccessAt)
		assert.Equal(t, now.Unix(), restored.LastSuccessAt.Unix())
		assert.Equal(t, int64(1000), restored.EventsImported)
		assert.Equal(t, 50, restored.LastEventCount)
		assert.Equal(t, 3, restored.ConsecErrors)
		assert.NotNil(t, restored.LastError)
		assert.Equal(t, "connection timeout", *restored.LastError)
		assert.Equal(t, ErrorTypeTransient, restored.LastErrorType)
	})

	t.Run("omitempty fields", func(t *testing.T) {
		state := &IntegrationSyncState{
			IntegrationID:   "int-456",
			IntegrationType: "mixpanel",
		}

		data, err := json.Marshal(state)
		require.NoError(t, err)

		// Verify omitted fields are not in JSON
		var m map[string]interface{}
		err = json.Unmarshal(data, &m)
		require.NoError(t, err)

		assert.Equal(t, "int-456", m["integration_id"])
		assert.Equal(t, "mixpanel", m["integration_type"])
		_, hasCursor := m["cursor"]
		assert.False(t, hasCursor, "cursor should be omitted when empty")
		_, hasLastError := m["last_error"]
		assert.False(t, hasLastError, "last_error should be omitted when nil")
	})
}

func TestTaskState_WithIntegrationSync(t *testing.T) {
	t.Run("value and scan with integration sync", func(t *testing.T) {
		state := TaskState{
			Progress: 75.0,
			Message:  "Syncing integration",
			IntegrationSync: &IntegrationSyncState{
				IntegrationID:   "int-789",
				IntegrationType: "staminads",
				EventsImported:  500,
				ConsecErrors:    0,
			},
		}

		// Test Value
		value, err := state.Value()
		require.NoError(t, err)

		bytes, ok := value.([]byte)
		require.True(t, ok)

		// Verify JSON structure
		var m map[string]interface{}
		err = json.Unmarshal(bytes, &m)
		require.NoError(t, err)

		assert.Equal(t, 75.0, m["progress"])
		assert.Equal(t, "Syncing integration", m["message"])

		syncMap, ok := m["integration_sync"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "int-789", syncMap["integration_id"])
		assert.Equal(t, "staminads", syncMap["integration_type"])
		assert.Equal(t, float64(500), syncMap["events_imported"])

		// Test Scan
		var scanned TaskState
		err = scanned.Scan(bytes)
		require.NoError(t, err)

		assert.Equal(t, 75.0, scanned.Progress)
		assert.Equal(t, "Syncing integration", scanned.Message)
		assert.NotNil(t, scanned.IntegrationSync)
		assert.Equal(t, "int-789", scanned.IntegrationSync.IntegrationID)
		assert.Equal(t, "staminads", scanned.IntegrationSync.IntegrationType)
		assert.Equal(t, int64(500), scanned.IntegrationSync.EventsImported)
	})

	t.Run("only one specialized state", func(t *testing.T) {
		// TaskState should only have one specialized state at a time
		state := TaskState{
			IntegrationSync: &IntegrationSyncState{
				IntegrationID: "int-123",
			},
		}

		assert.Nil(t, state.SendBroadcast)
		assert.Nil(t, state.BuildSegment)
		assert.NotNil(t, state.IntegrationSync)
	})
}

func TestErrorTypeConstants(t *testing.T) {
	assert.Equal(t, "transient", ErrorTypeTransient)
	assert.Equal(t, "permanent", ErrorTypePermanent)
	assert.Equal(t, "unknown", ErrorTypeUnknown)
}

func TestResetTaskRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &ResetTaskRequest{
			WorkspaceID: "ws-123",
			ID:          "task-456",
		}

		err := req.Validate()
		require.NoError(t, err)
	})

	t.Run("missing workspace_id", func(t *testing.T) {
		req := &ResetTaskRequest{
			ID: "task-456",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("empty workspace_id", func(t *testing.T) {
		req := &ResetTaskRequest{
			WorkspaceID: "",
			ID:          "task-456",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("missing id", func(t *testing.T) {
		req := &ResetTaskRequest{
			WorkspaceID: "ws-123",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("empty id", func(t *testing.T) {
		req := &ResetTaskRequest{
			WorkspaceID: "ws-123",
			ID:          "",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestTriggerTaskRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &TriggerTaskRequest{
			WorkspaceID: "ws-123",
			ID:          "task-456",
		}

		err := req.Validate()
		require.NoError(t, err)
	})

	t.Run("missing workspace_id", func(t *testing.T) {
		req := &TriggerTaskRequest{
			ID: "task-456",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("empty workspace_id", func(t *testing.T) {
		req := &TriggerTaskRequest{
			WorkspaceID: "",
			ID:          "task-456",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("missing id", func(t *testing.T) {
		req := &TriggerTaskRequest{
			WorkspaceID: "ws-123",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("empty id", func(t *testing.T) {
		req := &TriggerTaskRequest{
			WorkspaceID: "ws-123",
			ID:          "",
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestTask_RecurringFields(t *testing.T) {
	t.Run("task with recurring fields", func(t *testing.T) {
		interval := int64(60)
		integrationID := "int-123"
		task := &Task{
			ID:                "task-1",
			WorkspaceID:       "ws-1",
			Type:              "sync_integration",
			RecurringInterval: &interval,
			IntegrationID:     &integrationID,
		}

		assert.Equal(t, int64(60), *task.RecurringInterval)
		assert.Equal(t, "int-123", *task.IntegrationID)
		assert.True(t, task.IsRecurring())
	})

	t.Run("task without recurring fields", func(t *testing.T) {
		task := &Task{
			ID:          "task-1",
			WorkspaceID: "ws-1",
			Type:        "send_broadcast",
		}

		assert.Nil(t, task.RecurringInterval)
		assert.Nil(t, task.IntegrationID)
		assert.False(t, task.IsRecurring())
	})
}
