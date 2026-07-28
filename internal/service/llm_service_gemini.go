package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
	"google.golang.org/genai"
)

// geminiToolCall holds a server-side tool call collected from a Gemini response.
type geminiToolCall struct {
	ID    string
	Name  string
	Input map[string]interface{}
}

// streamChatGemini implements streaming chat with Google Gemini (Gemini Developer API).
func (s *LLMService) streamChatGemini(
	ctx context.Context,
	req *domain.LLMChatRequest,
	settings *domain.GeminiSettings,
	firecrawlSettings *domain.FirecrawlSettings,
	onEvent func(domain.LLMChatEvent) error,
) error {
	// Get decrypted API key (already decrypted by AfterLoad in repository)
	apiKey := settings.APIKey
	if apiKey == "" {
		return fmt.Errorf("API key is not configured for LLM integration")
	}
	model := settings.Model
	if model == "" {
		model = "gemini-3.1-pro-preview" // Default model
	}

	// Create Gemini client (Gemini Developer API backend)
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return fmt.Errorf("failed to create Gemini client: %w", err)
	}

	// Convert messages to genai contents (assistant role maps to "model")
	contents := make([]*genai.Content, 0, len(req.Messages))
	for _, msg := range req.Messages {
		var role genai.Role = genai.RoleUser
		if msg.Role != "user" {
			role = genai.RoleModel
		}
		contents = append(contents, genai.NewContentFromText(msg.Content, role))
	}

	// Set default max tokens
	maxTokens := int32(req.MaxTokens)
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	config := &genai.GenerateContentConfig{
		MaxOutputTokens: maxTokens,
		// Return the model's reasoning so it can be streamed on the "thinking" channel.
		// The model reasons regardless; this only controls whether thoughts are returned.
		ThinkingConfig: &genai.ThinkingConfig{IncludeThoughts: true},
	}

	// Add system prompt if provided
	if req.SystemPrompt != "" {
		config.SystemInstruction = genai.NewContentFromText(req.SystemPrompt, genai.RoleUser)
	}

	// Add tools if provided
	if len(req.Tools) > 0 {
		decls := make([]*genai.FunctionDeclaration, 0, len(req.Tools))
		for _, t := range req.Tools {
			schema, err := jsonSchemaToGenaiSchema(t.InputSchema)
			if err != nil {
				return fmt.Errorf("failed to parse tool input schema for %q: %w", t.Name, err)
			}
			decls = append(decls, &genai.FunctionDeclaration{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schema,
			})
		}
		config.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
	}

	// Set when any request finishes with MAX_TOKENS; reported on the final "done".
	truncated := false

	// streamOnce runs a single streaming request against the current `contents`,
	// emits text + client-side tool_use events, and returns the model's parts
	// (to append as the assistant turn) plus any server-side tool calls and the
	// usage for this request. The closure captures `contents`/`config` by
	// reference so the agentic loop below can extend the conversation between calls.
	streamOnce := func() (modelParts []*genai.Part, serverToolCalls []geminiToolCall, promptTokens, outputTokens int64, retErr error) {
		var textBuilder strings.Builder
		var fcParts []*genai.Part
		var finishReason genai.FinishReason

		for resp, streamErr := range client.Models.GenerateContentStream(ctx, model, contents, config) {
			if streamErr != nil {
				return nil, nil, promptTokens, outputTokens, fmt.Errorf("stream error: %w", streamErr)
			}

			// Gemini reports cumulative usage; keep the latest values for this request.
			if resp.UsageMetadata != nil {
				promptTokens = int64(resp.UsageMetadata.PromptTokenCount)
				outputTokens = int64(resp.UsageMetadata.CandidatesTokenCount)
			}

			// Capture the finish reason before the content nil-check below: the final
			// chunk can carry FinishReason with empty content (e.g. budget spent on
			// reasoning), which would otherwise be skipped by the `continue`.
			if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != "" {
				finishReason = resp.Candidates[0].FinishReason
			}

			if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
				continue
			}

			for _, part := range resp.Candidates[0].Content.Parts {
				if part == nil {
					continue
				}

				// Stream the model's internal reasoning ("thought") parts on a
				// separate channel; stream the visible answer as text.
				if part.Text != "" && part.Thought {
					if sendErr := onEvent(domain.LLMChatEvent{
						Type:    "thinking",
						Content: part.Text,
					}); sendErr != nil {
						return nil, nil, promptTokens, outputTokens, fmt.Errorf("failed to send thinking event: %w", sendErr)
					}
				} else if part.Text != "" {
					if sendErr := onEvent(domain.LLMChatEvent{
						Type:    "text",
						Content: part.Text,
					}); sendErr != nil {
						return nil, nil, promptTokens, outputTokens, fmt.Errorf("failed to send event: %w", sendErr)
					}
					textBuilder.WriteString(part.Text)
				}

				// Collect function calls (Gemini Developer API returns them complete).
				if part.FunctionCall != nil {
					fc := part.FunctionCall
					// Preserve the original part so any thought signature is retained
					// when the assistant turn is replayed in the agentic loop.
					fcParts = append(fcParts, part)

					if firecrawlSettings != nil && s.toolRegistry != nil && s.toolRegistry.IsServerSideTool(fc.Name) {
						serverToolCalls = append(serverToolCalls, geminiToolCall{
							ID:    fc.ID,
							Name:  fc.Name,
							Input: fc.Args,
						})
					} else {
						// Forward client-side tool to frontend
						if sendErr := onEvent(domain.LLMChatEvent{
							Type:      "tool_use",
							ToolName:  fc.Name,
							ToolInput: fc.Args,
						}); sendErr != nil {
							return nil, nil, promptTokens, outputTokens, fmt.Errorf("failed to send tool_use event: %w", sendErr)
						}
					}
				}
			}
		}

		// Truncation: when the model spends the whole budget on reasoning it finishes
		// with MAX_TOKENS (often with empty/partial parts). Report it as a
		// non-destructive flag on the "done" event rather than a terminal error.
		if finishReason == genai.FinishReasonMaxTokens {
			truncated = true
		}

		// Assemble the assistant turn: text first (if any), then function calls.
		if textBuilder.Len() > 0 {
			modelParts = append(modelParts, genai.NewPartFromText(textBuilder.String()))
		}
		modelParts = append(modelParts, fcParts...)

		return modelParts, serverToolCalls, promptTokens, outputTokens, nil
	}

	// Initial stream
	modelParts, serverToolCalls, promptTokens, outputTokens, err := streamOnce()
	if err != nil {
		s.logger.WithField("error", err.Error()).Error("Stream error from Gemini")
		return err
	}
	totalInputTokens := promptTokens
	totalOutputTokens := outputTokens

	// Agentic loop for server-side tool execution
	for iteration := 0; iteration < 10 && len(serverToolCalls) > 0; iteration++ {
		s.logger.WithFields(map[string]interface{}{
			"iteration":  iteration,
			"tool_count": len(serverToolCalls),
		}).Debug("Executing server-side tools")

		// Append the assistant turn that contains the function call(s). If the model
		// produced no parts (e.g. it spent the whole budget on reasoning), stop
		// rather than send an empty turn, which Gemini rejects as invalid history.
		if len(modelParts) == 0 {
			break
		}
		contents = append(contents, genai.NewContentFromParts(modelParts, genai.RoleModel))

		// Execute all server-side tools and build function-response parts
		var responseParts []*genai.Part
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

			// Emit server_tool_result event for frontend visibility (truncated)
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

			// Build the function response. Echo the call ID (Gemini 3 requires it)
			// and use the "output"/"error" keys per the Gemini API convention.
			responseKey := "output"
			if isError {
				responseKey = "error"
			}
			responseParts = append(responseParts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:       tool.ID,
					Name:     tool.Name,
					Response: map[string]any{responseKey: result},
				},
			})
		}

		// Append the function-response turn and continue the conversation
		contents = append(contents, genai.NewContentFromParts(responseParts, genai.RoleUser))

		modelParts, serverToolCalls, promptTokens, outputTokens, err = streamOnce()
		if err != nil {
			s.logger.WithField("error", err.Error()).Error("Stream error from Gemini")
			return err
		}
		totalInputTokens += promptTokens
		totalOutputTokens += outputTokens
	}

	// Calculate costs (returns 0 for unknown models)
	inputCost, outputCost, totalCost := calculateCost(model, totalInputTokens, totalOutputTokens)

	// Send done event with usage stats
	return onEvent(domain.LLMChatEvent{
		Type:         "done",
		Truncated:    truncated,
		InputTokens:  &totalInputTokens,
		OutputTokens: &totalOutputTokens,
		InputCost:    &inputCost,
		OutputCost:   &outputCost,
		TotalCost:    &totalCost,
		Model:        model,
	})
}

// jsonSchemaToGenaiSchema converts a JSON Schema document (as stored in
// LLMTool.InputSchema) into the typed *genai.Schema the Gemini SDK expects.
// Unlike the Anthropic/OpenAI SDKs, Gemini does not accept raw JSON Schema, so
// the structure must be rebuilt with its uppercase OpenAPI-style type enums.
func jsonSchemaToGenaiSchema(raw json.RawMessage) (*genai.Schema, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %w", err)
	}
	schema := mapToGenaiSchema(m)
	// Gemini requires function parameters to be a typed schema; default the root
	// to OBJECT when the source JSON Schema omits "type" (zero value "").
	if schema != nil && (schema.Type == "" || schema.Type == genai.TypeUnspecified) {
		schema.Type = genai.TypeObject
	}
	return schema, nil
}

// mapToGenaiSchema recursively converts a decoded JSON Schema object.
func mapToGenaiSchema(m map[string]interface{}) *genai.Schema {
	if m == nil {
		return nil
	}

	schema := &genai.Schema{}

	if t, ok := m["type"].(string); ok {
		schema.Type = jsonTypeToGenaiType(t)
	}
	if desc, ok := m["description"].(string); ok {
		schema.Description = desc
	}
	if props, ok := m["properties"].(map[string]interface{}); ok {
		schema.Properties = make(map[string]*genai.Schema, len(props))
		for name, p := range props {
			if pm, ok := p.(map[string]interface{}); ok {
				schema.Properties[name] = mapToGenaiSchema(pm)
			}
		}
	}
	if req, ok := m["required"].([]interface{}); ok {
		for _, r := range req {
			if rs, ok := r.(string); ok {
				schema.Required = append(schema.Required, rs)
			}
		}
	}
	if items, ok := m["items"].(map[string]interface{}); ok {
		schema.Items = mapToGenaiSchema(items)
	}
	if enum, ok := m["enum"].([]interface{}); ok {
		for _, e := range enum {
			if es, ok := e.(string); ok {
				schema.Enum = append(schema.Enum, es)
			}
		}
	}

	// Gemini rejects under-specified schemas: an object carrying properties must
	// be typed OBJECT, and an ARRAY must declare an item schema.
	// (A missing "type" leaves Type at its zero value "", not TypeUnspecified.)
	if (schema.Type == "" || schema.Type == genai.TypeUnspecified) && len(schema.Properties) > 0 {
		schema.Type = genai.TypeObject
	}
	if schema.Type == genai.TypeArray && schema.Items == nil {
		schema.Items = &genai.Schema{Type: genai.TypeString}
	}

	return schema
}

// jsonTypeToGenaiType maps a lowercase JSON Schema type to the Gemini enum.
func jsonTypeToGenaiType(t string) genai.Type {
	switch strings.ToLower(t) {
	case "object":
		return genai.TypeObject
	case "string":
		return genai.TypeString
	case "number":
		return genai.TypeNumber
	case "integer":
		return genai.TypeInteger
	case "boolean":
		return genai.TypeBoolean
	case "array":
		return genai.TypeArray
	default:
		return genai.TypeUnspecified
	}
}
