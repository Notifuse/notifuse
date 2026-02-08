package domain

import (
	"context"
	"encoding/json"
	"fmt"
)

// LLMTool represents a tool the AI can use
type LLMTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// LLMChatRequest represents a request to the LLM chat endpoint
type LLMChatRequest struct {
	WorkspaceID   string       `json:"workspace_id"`
	IntegrationID string       `json:"integration_id"`
	Messages      []LLMMessage `json:"messages"`
	MaxTokens     int          `json:"max_tokens,omitempty"`
	SystemPrompt  string       `json:"system_prompt,omitempty"`
	Tools         []LLMTool    `json:"tools,omitempty"`
}

// LLMMessage represents a chat message
type LLMMessage struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// LLMChatEvent represents an SSE event sent during streaming
type LLMChatEvent struct {
	Type         string                 `json:"type"`                    // "text", "tool_use", "server_tool_start", "server_tool_result", "done", "error"
	Content      string                 `json:"content,omitempty"`       // Text content for "text" events
	Error        string                 `json:"error,omitempty"`         // Error message for "error" events
	ToolName     string                 `json:"tool_name,omitempty"`     // Tool name for "tool_use" events
	ToolInput    map[string]interface{} `json:"tool_input,omitempty"`    // Tool input for "tool_use" events
	InputTokens  *int64                 `json:"input_tokens,omitempty"`  // Token count (done event only)
	OutputTokens *int64                 `json:"output_tokens,omitempty"` // Token count (done event only)
	InputCost    *float64               `json:"input_cost,omitempty"`    // Cost in USD (done event only)
	OutputCost   *float64               `json:"output_cost,omitempty"`   // Cost in USD (done event only)
	TotalCost    *float64               `json:"total_cost,omitempty"`    // Total cost in USD (done event only)
	Model        string                 `json:"model,omitempty"`         // Model used (done event only)
}

// Validate validates the LLM chat request
func (r *LLMChatRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.IntegrationID == "" {
		return fmt.Errorf("integration_id is required")
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("at least one message is required")
	}
	for i, msg := range r.Messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			return fmt.Errorf("message %d: role must be 'user' or 'assistant'", i)
		}
		if msg.Content == "" {
			return fmt.Errorf("message %d: content is required", i)
		}
	}
	if r.MaxTokens < 0 || r.MaxTokens > 8192 {
		return fmt.Errorf("max_tokens must be between 0 and 8192")
	}
	return nil
}

//go:generate mockgen -destination mocks/mock_llm_service.go -package mocks github.com/Notifuse/notifuse/internal/domain LLMService

// LLMService defines the interface for LLM operations
type LLMService interface {
	// StreamChat sends a chat request and streams the response
	StreamChat(ctx context.Context, req *LLMChatRequest, onEvent func(LLMChatEvent) error) error
}
