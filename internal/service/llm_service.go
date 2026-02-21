package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Model pricing per million tokens (USD)
// Pricing from: https://claude.com/pricing
var modelPricing = map[string]struct {
	InputPerMTok  float64
	OutputPerMTok float64
}{
	"claude-opus-4-6":            {5.0, 25.0},
	"claude-sonnet-4-6":          {3.0, 15.0},
	"claude-haiku-4-5-20251001":  {1.0, 5.0},
}

// calculateCost calculates the cost in USD for a given model and token counts
func calculateCost(model string, inputTokens, outputTokens int64) (inputCost, outputCost, totalCost float64) {
	pricing, ok := modelPricing[model]
	if !ok {
		return 0, 0, 0 // Unknown model, no cost info
	}
	inputCost = float64(inputTokens) / 1_000_000 * pricing.InputPerMTok
	outputCost = float64(outputTokens) / 1_000_000 * pricing.OutputPerMTok
	totalCost = inputCost + outputCost
	return
}

// LLMServiceConfig contains configuration for the LLM service
type LLMServiceConfig struct {
	AuthService   domain.AuthService
	WorkspaceRepo domain.WorkspaceRepository
	Logger        logger.Logger
	ToolRegistry  *ServerSideToolRegistry
}

// LLMService implements the LLM chat functionality
type LLMService struct {
	authService   domain.AuthService
	workspaceRepo domain.WorkspaceRepository
	logger        logger.Logger
	toolRegistry  *ServerSideToolRegistry
}

// NewLLMService creates a new LLM service
func NewLLMService(config LLMServiceConfig) *LLMService {
	return &LLMService{
		authService:   config.AuthService,
		workspaceRepo: config.WorkspaceRepo,
		logger:        config.Logger,
		toolRegistry:  config.ToolRegistry,
	}
}

// StreamChat implements streaming chat with Anthropic
func (s *LLMService) StreamChat(ctx context.Context, req *domain.LLMChatRequest, onEvent func(domain.LLMChatEvent) error) error {
	// 1. Authenticate user for workspace
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// 2. Check LLM write permission
	if !userWorkspace.HasPermission(domain.PermissionResourceLLM, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceLLM,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to LLM required",
		)
	}

	// 3. Get workspace
	workspace, err := s.workspaceRepo.GetByID(ctx, req.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	// 3.5 Find Firecrawl integration if configured
	var firecrawlSettings *domain.FirecrawlSettings
	for _, integ := range workspace.Integrations {
		if integ.Type == domain.IntegrationTypeFirecrawl && integ.FirecrawlSettings != nil {
			firecrawlSettings = integ.FirecrawlSettings
			break
		}
	}

	// 3.6 Inject server-side tools if Firecrawl is available
	if firecrawlSettings != nil && s.toolRegistry != nil {
		serverTools := s.toolRegistry.GetAvailableTools()
		req.Tools = append(req.Tools, serverTools...)
		s.logger.Debug("Injected Firecrawl tools into LLM request")
	}

	// 4. Find the LLM integration
	integration := workspace.GetIntegrationByID(req.IntegrationID)
	if integration == nil {
		return fmt.Errorf("integration not found: %s", req.IntegrationID)
	}
	if integration.Type != domain.IntegrationTypeLLM {
		return fmt.Errorf("integration is not an LLM integration: %s", req.IntegrationID)
	}
	if integration.LLMProvider == nil || integration.LLMProvider.Anthropic == nil {
		return fmt.Errorf("LLM provider configuration is missing")
	}

	// 5. Get decrypted API key (already decrypted by AfterLoad in repository)
	apiKey := integration.LLMProvider.Anthropic.APIKey
	if apiKey == "" {
		return fmt.Errorf("API key is not configured for LLM integration")
	}
	model := integration.LLMProvider.Anthropic.Model
	if model == "" {
		model = "claude-sonnet-4-6" // Default model
	}

	// 6. Create Anthropic client
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	// 7. Convert messages to Anthropic format
	messages := make([]anthropic.MessageParam, len(req.Messages))
	for i, msg := range req.Messages {
		if msg.Role == "user" {
			messages[i] = anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content))
		} else {
			messages[i] = anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content))
		}
	}

	// 8. Set default max tokens
	maxTokens := int64(req.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 2048
	}

	// 9. Build streaming request parameters
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	// Add system prompt if provided
	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	// 10. Add tools if provided
	if len(req.Tools) > 0 {
		tools := make([]anthropic.ToolUnionParam, len(req.Tools))
		for i, t := range req.Tools {
			// Parse the input schema from json.RawMessage
			var schemaProps map[string]interface{}
			if err := json.Unmarshal(t.InputSchema, &schemaProps); err != nil {
				return fmt.Errorf("failed to parse tool input schema: %w", err)
			}

			// Convert required from interface{} to []string
			var required []string
			if reqList, ok := schemaProps["required"].([]interface{}); ok {
				for _, r := range reqList {
					if s, ok := r.(string); ok {
						required = append(required, s)
					}
				}
			}

			tools[i] = anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					Name:        t.Name,
					Description: anthropic.String(t.Description),
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: schemaProps["properties"],
						Required:   required,
					},
				},
			}
		}
		params.Tools = tools
	}

	// 11. Create streaming request
	stream := client.Messages.NewStreaming(ctx, params)

	// 12. Create message accumulator to capture usage stats and tool use blocks
	message := anthropic.Message{}

	// 13. Process stream events
	for stream.Next() {
		event := stream.Current()

		// Accumulate events to build final message with usage
		if err := message.Accumulate(event); err != nil {
			s.logger.WithField("error", err.Error()).Error("Failed to accumulate event")
		}

		switch eventVariant := event.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			switch deltaVariant := eventVariant.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				if err := onEvent(domain.LLMChatEvent{
					Type:    "text",
					Content: deltaVariant.Text,
				}); err != nil {
					return fmt.Errorf("failed to send event: %w", err)
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		s.logger.WithField("error", err.Error()).Error("Stream error from Anthropic")
		return fmt.Errorf("stream error: %w", err)
	}

	// 14. Process accumulated tool use blocks - handle server-side vs client-side
	var serverToolCalls []struct {
		ID    string
		Name  string
		Input map[string]interface{}
	}

	for _, block := range message.Content {
		switch toolBlock := block.AsAny().(type) {
		case anthropic.ToolUseBlock:
			// Parse the tool input from raw JSON
			var toolInput map[string]interface{}
			if err := json.Unmarshal(toolBlock.Input, &toolInput); err != nil {
				s.logger.WithField("error", err.Error()).Error("Failed to parse tool input")
				continue
			}

			// Check if this is a server-side tool
			if firecrawlSettings != nil && s.toolRegistry != nil && s.toolRegistry.IsServerSideTool(toolBlock.Name) {
				serverToolCalls = append(serverToolCalls, struct {
					ID    string
					Name  string
					Input map[string]interface{}
				}{
					ID:    toolBlock.ID,
					Name:  toolBlock.Name,
					Input: toolInput,
				})
			} else {
				// Forward client-side tool to frontend
				if err := onEvent(domain.LLMChatEvent{
					Type:      "tool_use",
					ToolName:  toolBlock.Name,
					ToolInput: toolInput,
				}); err != nil {
					return fmt.Errorf("failed to send tool_use event: %w", err)
				}
			}
		}
	}

	// 15. Execute server-side tools and continue conversation if needed
	totalInputTokens := message.Usage.InputTokens
	totalOutputTokens := message.Usage.OutputTokens

	// Agentic loop for server-side tool execution
	for iteration := 0; iteration < 10 && len(serverToolCalls) > 0; iteration++ {
		s.logger.WithFields(map[string]interface{}{
			"iteration":  iteration,
			"tool_count": len(serverToolCalls),
		}).Debug("Executing server-side tools")

		// Execute all server-side tools and build tool result blocks
		var toolResults []anthropic.ContentBlockParamUnion
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

			result, err := s.toolRegistry.ExecuteTool(ctx, firecrawlSettings, tool.Name, tool.Input)
			isError := false
			if err != nil {
				s.logger.WithFields(map[string]interface{}{
					"tool_name": tool.Name,
					"error":     err.Error(),
				}).Warn("Server-side tool execution failed")
				result = fmt.Sprintf("Error: %s", err.Error())
				isError = true
			}

			// Emit server_tool_result event for frontend visibility
			// Truncate result for the event to avoid huge payloads
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
					} else {
						return ""
					}
				}(),
			}); err != nil {
				s.logger.WithField("error", err.Error()).Warn("Failed to send server_tool_result event")
			}

			toolResults = append(toolResults, anthropic.NewToolResultBlock(tool.ID, result, false))
		}

		// Convert message.Content (ContentBlockUnion) to ContentBlockParamUnion for assistant message
		var assistantContent []anthropic.ContentBlockParamUnion
		for _, block := range message.Content {
			switch block.Type {
			case "text":
				assistantContent = append(assistantContent, anthropic.NewTextBlock(block.Text))
			case "tool_use":
				// Parse input back for the tool use block
				var input interface{}
				if err := json.Unmarshal(block.Input, &input); err != nil {
					input = map[string]interface{}{}
				}
				assistantContent = append(assistantContent, anthropic.NewToolUseBlock(block.ID, input, block.Name))
			}
		}

		// Add assistant message with tool use and user message with tool results
		messages = append(messages, anthropic.MessageParam{
			Role:    "assistant",
			Content: assistantContent,
		})
		messages = append(messages, anthropic.NewUserMessage(toolResults...))

		// Make another API call with tool results
		params.Messages = messages
		stream = client.Messages.NewStreaming(ctx, params)
		message = anthropic.Message{}

		// Process stream events
		for stream.Next() {
			event := stream.Current()
			if err := message.Accumulate(event); err != nil {
				s.logger.WithField("error", err.Error()).Error("Failed to accumulate event")
			}

			switch eventVariant := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				switch deltaVariant := eventVariant.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					if err := onEvent(domain.LLMChatEvent{
						Type:    "text",
						Content: deltaVariant.Text,
					}); err != nil {
						return fmt.Errorf("failed to send event: %w", err)
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			s.logger.WithField("error", err.Error()).Error("Stream error from Anthropic")
			return fmt.Errorf("stream error: %w", err)
		}

		// Accumulate token counts
		totalInputTokens += message.Usage.InputTokens
		totalOutputTokens += message.Usage.OutputTokens

		// Check for more tool calls
		serverToolCalls = nil
		for _, block := range message.Content {
			switch toolBlock := block.AsAny().(type) {
			case anthropic.ToolUseBlock:
				var toolInput map[string]interface{}
				if err := json.Unmarshal(toolBlock.Input, &toolInput); err != nil {
					continue
				}

				if s.toolRegistry.IsServerSideTool(toolBlock.Name) {
					serverToolCalls = append(serverToolCalls, struct {
						ID    string
						Name  string
						Input map[string]interface{}
					}{
						ID:    toolBlock.ID,
						Name:  toolBlock.Name,
						Input: toolInput,
					})
				} else {
					// Forward client-side tool to frontend
					if err := onEvent(domain.LLMChatEvent{
						Type:      "tool_use",
						ToolName:  toolBlock.Name,
						ToolInput: toolInput,
					}); err != nil {
						return fmt.Errorf("failed to send tool_use event: %w", err)
					}
				}
			}
		}
	}

	// 16. Calculate costs
	inputCost, outputCost, totalCost := calculateCost(model, totalInputTokens, totalOutputTokens)

	// 17. Send done event with usage stats
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
