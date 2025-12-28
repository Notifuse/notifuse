package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLLMChatRequest_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		request     LLMChatRequest
		expectErr   bool
		expectedMsg string
	}{
		{
			name: "valid request",
			request: LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "integration456",
				Messages: []LLMMessage{
					{Role: "user", Content: "Hello"},
				},
			},
			expectErr: false,
		},
		{
			name: "valid request with system prompt",
			request: LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "integration456",
				Messages: []LLMMessage{
					{Role: "user", Content: "Hello"},
				},
				SystemPrompt: "You are a helpful assistant.",
				MaxTokens:    1024,
			},
			expectErr: false,
		},
		{
			name: "valid multi-turn conversation",
			request: LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "integration456",
				Messages: []LLMMessage{
					{Role: "user", Content: "Hello"},
					{Role: "assistant", Content: "Hi there!"},
					{Role: "user", Content: "How are you?"},
				},
			},
			expectErr: false,
		},
		{
			name: "missing workspace_id",
			request: LLMChatRequest{
				IntegrationID: "integration456",
				Messages: []LLMMessage{
					{Role: "user", Content: "Hello"},
				},
			},
			expectErr:   true,
			expectedMsg: "workspace_id is required",
		},
		{
			name: "missing integration_id",
			request: LLMChatRequest{
				WorkspaceID: "workspace123",
				Messages: []LLMMessage{
					{Role: "user", Content: "Hello"},
				},
			},
			expectErr:   true,
			expectedMsg: "integration_id is required",
		},
		{
			name: "empty messages",
			request: LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "integration456",
				Messages:      []LLMMessage{},
			},
			expectErr:   true,
			expectedMsg: "at least one message is required",
		},
		{
			name: "nil messages",
			request: LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "integration456",
			},
			expectErr:   true,
			expectedMsg: "at least one message is required",
		},
		{
			name: "invalid role",
			request: LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "integration456",
				Messages: []LLMMessage{
					{Role: "system", Content: "Hello"},
				},
			},
			expectErr:   true,
			expectedMsg: "message 0: role must be 'user' or 'assistant'",
		},
		{
			name: "empty content",
			request: LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "integration456",
				Messages: []LLMMessage{
					{Role: "user", Content: ""},
				},
			},
			expectErr:   true,
			expectedMsg: "message 0: content is required",
		},
		{
			name: "negative max_tokens",
			request: LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "integration456",
				Messages: []LLMMessage{
					{Role: "user", Content: "Hello"},
				},
				MaxTokens: -1,
			},
			expectErr:   true,
			expectedMsg: "max_tokens must be between 0 and 8192",
		},
		{
			name: "max_tokens too large",
			request: LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "integration456",
				Messages: []LLMMessage{
					{Role: "user", Content: "Hello"},
				},
				MaxTokens: 10000,
			},
			expectErr:   true,
			expectedMsg: "max_tokens must be between 0 and 8192",
		},
		{
			name: "max_tokens at boundary",
			request: LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "integration456",
				Messages: []LLMMessage{
					{Role: "user", Content: "Hello"},
				},
				MaxTokens: 8192,
			},
			expectErr: false,
		},
		{
			name: "second message has invalid role",
			request: LLMChatRequest{
				WorkspaceID:   "workspace123",
				IntegrationID: "integration456",
				Messages: []LLMMessage{
					{Role: "user", Content: "Hello"},
					{Role: "bot", Content: "Hi"},
				},
			},
			expectErr:   true,
			expectedMsg: "message 1: role must be 'user' or 'assistant'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.Validate()
			if tc.expectErr {
				assert.Error(t, err)
				if tc.expectedMsg != "" {
					assert.Contains(t, err.Error(), tc.expectedMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLLMChatEvent(t *testing.T) {
	t.Run("text event", func(t *testing.T) {
		event := LLMChatEvent{
			Type:    "text",
			Content: "Hello, world!",
		}
		assert.Equal(t, "text", event.Type)
		assert.Equal(t, "Hello, world!", event.Content)
		assert.Empty(t, event.Error)
	})

	t.Run("done event", func(t *testing.T) {
		event := LLMChatEvent{
			Type: "done",
		}
		assert.Equal(t, "done", event.Type)
		assert.Empty(t, event.Content)
		assert.Empty(t, event.Error)
	})

	t.Run("done event with usage stats", func(t *testing.T) {
		inputTokens := int64(150)
		outputTokens := int64(89)
		inputCost := 0.00045
		outputCost := 0.001335
		totalCost := 0.001785

		event := LLMChatEvent{
			Type:         "done",
			InputTokens:  &inputTokens,
			OutputTokens: &outputTokens,
			InputCost:    &inputCost,
			OutputCost:   &outputCost,
			TotalCost:    &totalCost,
			Model:        "claude-sonnet-4-5-20251101",
		}

		assert.Equal(t, "done", event.Type)
		assert.NotNil(t, event.InputTokens)
		assert.Equal(t, int64(150), *event.InputTokens)
		assert.NotNil(t, event.OutputTokens)
		assert.Equal(t, int64(89), *event.OutputTokens)
		assert.NotNil(t, event.InputCost)
		assert.Equal(t, 0.00045, *event.InputCost)
		assert.NotNil(t, event.OutputCost)
		assert.Equal(t, 0.001335, *event.OutputCost)
		assert.NotNil(t, event.TotalCost)
		assert.Equal(t, 0.001785, *event.TotalCost)
		assert.Equal(t, "claude-sonnet-4-5-20251101", event.Model)
	})

	t.Run("error event", func(t *testing.T) {
		event := LLMChatEvent{
			Type:  "error",
			Error: "Something went wrong",
		}
		assert.Equal(t, "error", event.Type)
		assert.Empty(t, event.Content)
		assert.Equal(t, "Something went wrong", event.Error)
	})
}

func TestLLMMessage(t *testing.T) {
	t.Run("user message", func(t *testing.T) {
		msg := LLMMessage{
			Role:    "user",
			Content: "Hello!",
		}
		assert.Equal(t, "user", msg.Role)
		assert.Equal(t, "Hello!", msg.Content)
	})

	t.Run("assistant message", func(t *testing.T) {
		msg := LLMMessage{
			Role:    "assistant",
			Content: "Hi, how can I help?",
		}
		assert.Equal(t, "assistant", msg.Role)
		assert.Equal(t, "Hi, how can I help?", msg.Content)
	})
}

func TestLLMTool(t *testing.T) {
	t.Run("valid tool", func(t *testing.T) {
		tool := LLMTool{
			Name:        "update_blog_content",
			Description: "Update blog content",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"content":{"type":"object"}}}`),
		}
		// Verify marshaling works
		data, err := json.Marshal(tool)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "update_blog_content")
		assert.Contains(t, string(data), "input_schema")
	})

	t.Run("tool JSON round-trip", func(t *testing.T) {
		original := LLMTool{
			Name:        "test_tool",
			Description: "A test tool",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"arg":{"type":"string"}},"required":["arg"]}`),
		}

		data, err := json.Marshal(original)
		assert.NoError(t, err)

		var decoded LLMTool
		err = json.Unmarshal(data, &decoded)
		assert.NoError(t, err)
		assert.Equal(t, original.Name, decoded.Name)
		assert.Equal(t, original.Description, decoded.Description)
	})
}

func TestLLMChatEvent_WithToolUse(t *testing.T) {
	t.Run("tool_use event serialization", func(t *testing.T) {
		toolInput := map[string]interface{}{
			"content": map[string]interface{}{"type": "doc"},
			"message": "Updated content",
		}
		event := LLMChatEvent{
			Type:      "tool_use",
			ToolName:  "update_blog_content",
			ToolInput: toolInput,
		}
		data, err := json.Marshal(event)
		assert.NoError(t, err)
		assert.Contains(t, string(data), `"type":"tool_use"`)
		assert.Contains(t, string(data), `"tool_name":"update_blog_content"`)
		assert.Contains(t, string(data), `"tool_input"`)
	})

	t.Run("tool_use event with complex input", func(t *testing.T) {
		toolInput := map[string]interface{}{
			"content": map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "heading",
						"attrs": map[string]interface{}{
							"level": 1,
						},
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "Hello World",
							},
						},
					},
				},
			},
			"message": "Added heading",
		}
		event := LLMChatEvent{
			Type:      "tool_use",
			ToolName:  "update_blog_content",
			ToolInput: toolInput,
		}

		data, err := json.Marshal(event)
		assert.NoError(t, err)

		// Verify we can unmarshal back
		var decoded LLMChatEvent
		err = json.Unmarshal(data, &decoded)
		assert.NoError(t, err)
		assert.Equal(t, "tool_use", decoded.Type)
		assert.Equal(t, "update_blog_content", decoded.ToolName)
		assert.NotNil(t, decoded.ToolInput)
	})
}

func TestLLMChatRequest_WithTools(t *testing.T) {
	t.Run("request with tools", func(t *testing.T) {
		tools := []LLMTool{
			{
				Name:        "test_tool",
				Description: "A test tool",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"arg":{"type":"string"}},"required":["arg"]}`),
			},
		}
		req := LLMChatRequest{
			WorkspaceID:   "ws-123",
			IntegrationID: "int-456",
			Messages:      []LLMMessage{{Role: "user", Content: "Test"}},
			Tools:         tools,
		}

		// Verify validation passes with tools
		err := req.Validate()
		assert.NoError(t, err)

		// Verify tools are included in request
		assert.Equal(t, 1, len(req.Tools))
		assert.Equal(t, "test_tool", req.Tools[0].Name)
	})

	t.Run("request with multiple tools", func(t *testing.T) {
		tools := []LLMTool{
			{
				Name:        "tool_one",
				Description: "First tool",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
			{
				Name:        "tool_two",
				Description: "Second tool",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"number"}}}`),
			},
		}
		req := LLMChatRequest{
			WorkspaceID:   "ws-123",
			IntegrationID: "int-456",
			Messages:      []LLMMessage{{Role: "user", Content: "Test"}},
			Tools:         tools,
		}

		err := req.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 2, len(req.Tools))
	})
}
