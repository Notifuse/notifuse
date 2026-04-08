package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"

	openai "github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// streamChatOpenAI implements streaming chat with OpenAI or OpenAI-compatible APIs
func (s *LLMService) streamChatOpenAI(
	ctx context.Context,
	req *domain.LLMChatRequest,
	settings *domain.OpenAISettings,
	firecrawlSettings *domain.FirecrawlSettings,
	onEvent func(domain.LLMChatEvent) error,
) error {
	// Get decrypted API key
	apiKey := settings.APIKey
	if apiKey == "" {
		return fmt.Errorf("API key is not configured for LLM integration")
	}
	model := settings.Model
	if model == "" {
		model = "gpt-4.1" // Default model
	}

	s.logger.WithFields(map[string]interface{}{
		"model":          model,
		"base_url":       settings.BaseURL,
		"api_key_len":    len(apiKey),
		"api_key_prefix": apiKey[:min(10, len(apiKey))],
		"encrypted_key_len": len(settings.EncryptedAPIKey),
	}).Debug("OpenAI streaming chat config")

	// Create OpenAI client with optional custom base URL
	opts := []openaiopt.RequestOption{openaiopt.WithAPIKey(apiKey)}
	if settings.BaseURL != "" {
		opts = append(opts, openaiopt.WithBaseURL(settings.BaseURL))
	}
	client := openai.NewClient(opts...)

	// Convert messages to OpenAI format
	var messages []openai.ChatCompletionMessageParamUnion

	// System prompt goes as a system message
	if req.SystemPrompt != "" {
		messages = append(messages, openai.SystemMessage(req.SystemPrompt))
	}

	for _, msg := range req.Messages {
		if msg.Role == "user" {
			messages = append(messages, openai.UserMessage(msg.Content))
		} else {
			messages = append(messages, openai.AssistantMessage(msg.Content))
		}
	}

	// Set default max tokens
	maxTokens := int64(req.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 2048
	}

	// Build streaming request parameters
	params := openai.ChatCompletionNewParams{
		Model:               openai.ChatModel(model),
		Messages:            messages,
		MaxCompletionTokens: openai.Int(maxTokens),
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		},
	}

	// Add tools if provided
	if len(req.Tools) > 0 {
		tools := make([]openai.ChatCompletionToolUnionParam, len(req.Tools))
		for i, t := range req.Tools {
			var funcParams shared.FunctionParameters
			if err := json.Unmarshal(t.InputSchema, &funcParams); err != nil {
				return fmt.Errorf("failed to parse tool input schema: %w", err)
			}

			tools[i] = openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  funcParams,
			})
		}
		params.Tools = tools
	}

	// Create streaming request
	stream := client.Chat.Completions.NewStreaming(ctx, params)

	// Use accumulator to assemble tool calls from streaming chunks
	acc := openai.ChatCompletionAccumulator{}

	// Process stream events
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		// Stream text deltas
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			if err := onEvent(domain.LLMChatEvent{
				Type:    "text",
				Content: chunk.Choices[0].Delta.Content,
			}); err != nil {
				return fmt.Errorf("failed to send event: %w", err)
			}
		}
	}

	if err := stream.Err(); err != nil {
		s.logger.WithField("error", err.Error()).Error("Stream error from OpenAI")
		return fmt.Errorf("stream error: %w", err)
	}

	// Process accumulated tool calls - handle server-side vs client-side
	var serverToolCalls []struct {
		ID    string
		Name  string
		Input map[string]interface{}
	}

	if len(acc.Choices) > 0 {
		for _, toolCall := range acc.Choices[0].Message.ToolCalls {
			var toolInput map[string]interface{}
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &toolInput); err != nil {
				s.logger.WithField("error", err.Error()).Error("Failed to parse tool input")
				continue
			}

			if firecrawlSettings != nil && s.toolRegistry != nil && s.toolRegistry.IsServerSideTool(toolCall.Function.Name) {
				serverToolCalls = append(serverToolCalls, struct {
					ID    string
					Name  string
					Input map[string]interface{}
				}{
					ID:    toolCall.ID,
					Name:  toolCall.Function.Name,
					Input: toolInput,
				})
			} else {
				// Forward client-side tool to frontend
				if err := onEvent(domain.LLMChatEvent{
					Type:      "tool_use",
					ToolName:  toolCall.Function.Name,
					ToolInput: toolInput,
				}); err != nil {
					return fmt.Errorf("failed to send tool_use event: %w", err)
				}
			}
		}
	}

	// Track token usage
	totalInputTokens := acc.Usage.PromptTokens
	totalOutputTokens := acc.Usage.CompletionTokens

	// Agentic loop for server-side tool execution
	for iteration := 0; iteration < 10 && len(serverToolCalls) > 0; iteration++ {
		s.logger.WithFields(map[string]interface{}{
			"iteration":  iteration,
			"tool_count": len(serverToolCalls),
		}).Debug("Executing server-side tools")

		// Add assistant message with tool calls to conversation
		messages = append(messages, acc.Choices[0].Message.ToParam())

		// Execute all server-side tools and add tool result messages
		for _, tool := range serverToolCalls {
			s.logger.WithFields(map[string]interface{}{
				"tool_name": tool.Name,
				"tool_id":   tool.ID,
			}).Debug("Executing server-side tool")

			// Emit server_tool_start event for frontend visibility
			if err := onEvent(domain.LLMChatEvent{
				Type:      "server_tool_start",
				ToolName:  tool.Name,
				ToolInput: tool.Input,
			}); err != nil {
				s.logger.WithField("error", err.Error()).Warn("Failed to send server_tool_start event")
			}

			result, execErr := s.toolRegistry.ExecuteTool(ctx, firecrawlSettings, tool.Name, tool.Input)
			isError := false
			if execErr != nil {
				s.logger.WithFields(map[string]interface{}{
					"tool_name": tool.Name,
					"error":     execErr.Error(),
				}).Warn("Server-side tool execution failed")
				result = fmt.Sprintf("Error: %s", execErr.Error())
				isError = true
			}

			// Emit server_tool_result event for frontend visibility
			resultSummary := result
			if len(resultSummary) > 500 {
				resultSummary = resultSummary[:500] + "..."
			}
			if err := onEvent(domain.LLMChatEvent{
				Type:     "server_tool_result",
				ToolName: tool.Name,
				Content:  resultSummary,
				Error: func() string {
					if isError {
						return result
					}
					return ""
				}(),
			}); err != nil {
				s.logger.WithField("error", err.Error()).Warn("Failed to send server_tool_result event")
			}

			// Add tool result message
			messages = append(messages, openai.ToolMessage(result, tool.ID))
		}

		// Make another API call with tool results
		params.Messages = messages
		stream = client.Chat.Completions.NewStreaming(ctx, params)
		acc = openai.ChatCompletionAccumulator{}

		// Process stream events
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				if err := onEvent(domain.LLMChatEvent{
					Type:    "text",
					Content: chunk.Choices[0].Delta.Content,
				}); err != nil {
					return fmt.Errorf("failed to send event: %w", err)
				}
			}
		}

		if err := stream.Err(); err != nil {
			s.logger.WithField("error", err.Error()).Error("Stream error from OpenAI")
			return fmt.Errorf("stream error: %w", err)
		}

		// Accumulate token counts
		totalInputTokens += acc.Usage.PromptTokens
		totalOutputTokens += acc.Usage.CompletionTokens

		// Check for more tool calls
		serverToolCalls = nil
		if len(acc.Choices) > 0 {
			for _, toolCall := range acc.Choices[0].Message.ToolCalls {
				var toolInput map[string]interface{}
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &toolInput); err != nil {
					continue
				}

				if s.toolRegistry.IsServerSideTool(toolCall.Function.Name) {
					serverToolCalls = append(serverToolCalls, struct {
						ID    string
						Name  string
						Input map[string]interface{}
					}{
						ID:    toolCall.ID,
						Name:  toolCall.Function.Name,
						Input: toolInput,
					})
				} else {
					if err := onEvent(domain.LLMChatEvent{
						Type:      "tool_use",
						ToolName:  toolCall.Function.Name,
						ToolInput: toolInput,
					}); err != nil {
						return fmt.Errorf("failed to send tool_use event: %w", err)
					}
				}
			}
		}
	}

	// Calculate costs (returns 0 for unknown/custom models)
	inputCost, outputCost, totalCost := calculateCost(model, totalInputTokens, totalOutputTokens)

	// Send done event with usage stats
	return onEvent(domain.LLMChatEvent{
		Type:         "done",
		InputTokens:  &totalInputTokens,
		OutputTokens: &totalOutputTokens,
		InputCost:    &inputCost,
		OutputCost:   &outputCost,
		TotalCost:    &totalCost,
		Model:        model,
	})
}
