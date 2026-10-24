package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Provider streaming tests run the real provider SDKs against local SSE
// fixtures (Anthropic via ANTHROPIC_BASE_URL, Gemini via
// GOOGLE_GEMINI_BASE_URL, OpenAI via its BaseURL setting). They pin the
// event contract the console consumes: text/thinking stay on separate
// channels, tool calls surface as tool_use with parsed input, truncation is
// a flag on done (never an error), and callback/transport failures abort
// with wrapped errors.

func newLLMStreamService() *LLMService {
	return NewLLMService(LLMServiceConfig{
		Logger: logger.NewLoggerWithLevel("disabled"),
	})
}

func llmReq() *domain.LLMChatRequest {
	return &domain.LLMChatRequest{
		WorkspaceID:   "ws-1",
		IntegrationID: "int-1",
		Messages:      []domain.LLMMessage{{Role: "user", Content: "Hi"}},
	}
}

func collectEvents(events *[]domain.LLMChatEvent) func(domain.LLMChatEvent) error {
	return func(e domain.LLMChatEvent) error {
		*events = append(*events, e)
		return nil
	}
}

func sseServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const anthropicTextStream = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"usage":{"input_tokens":11,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}

event: message_stop
data: {"type":"message_stop"}

`

func TestStreamChatAnthropic_Streaming(t *testing.T) {
	ctx := context.Background()

	t.Run("text deltas arrive in order and done carries usage and model", func(t *testing.T) {
		srv := sseServer(t, anthropicTextStream, http.StatusOK)
		t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
		svc := newLLMStreamService()
		var events []domain.LLMChatEvent
		err := svc.streamChatAnthropic(ctx, llmReq(),
			&domain.AnthropicSettings{APIKey: "sk-test", Model: "claude-test"}, nil, collectEvents(&events))
		require.NoError(t, err)
		require.Len(t, events, 3)
		assert.Equal(t, domain.LLMChatEvent{Type: "text", Content: "Hello "}, events[0])
		assert.Equal(t, domain.LLMChatEvent{Type: "text", Content: "world"}, events[1])
		done := events[2]
		assert.Equal(t, "done", done.Type)
		assert.False(t, done.Truncated)
		require.NotNil(t, done.InputTokens)
		require.NotNil(t, done.OutputTokens)
		assert.Equal(t, int64(11), *done.InputTokens)
		assert.Equal(t, int64(7), *done.OutputTokens)
		assert.Equal(t, "claude-test", done.Model)
	})

	t.Run("max_tokens stop marks done as truncated, not an error", func(t *testing.T) {
		body := strings.Replace(anthropicTextStream, `"stop_reason":"end_turn"`, `"stop_reason":"max_tokens"`, 1)
		srv := sseServer(t, body, http.StatusOK)
		t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
		svc := newLLMStreamService()
		var events []domain.LLMChatEvent
		err := svc.streamChatAnthropic(ctx, llmReq(),
			&domain.AnthropicSettings{APIKey: "sk-test", Model: "claude-test"}, nil, collectEvents(&events))
		require.NoError(t, err)
		done := events[len(events)-1]
		assert.Equal(t, "done", done.Type)
		assert.True(t, done.Truncated, "the frontend keeps streamed text and only flags truncation")
	})

	t.Run("provider HTTP failure is a stream error", func(t *testing.T) {
		srv := sseServer(t, `{"type":"error","error":{"type":"overloaded_error","message":"boom"}}`, http.StatusInternalServerError)
		t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
		svc := newLLMStreamService()
		err := svc.streamChatAnthropic(ctx, llmReq(),
			&domain.AnthropicSettings{APIKey: "sk-test", Model: "claude-test"}, nil, collectEvents(&[]domain.LLMChatEvent{}))
		require.ErrorContains(t, err, "stream error:")
	})

	t.Run("a failing event callback aborts the stream", func(t *testing.T) {
		srv := sseServer(t, anthropicTextStream, http.StatusOK)
		t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
		svc := newLLMStreamService()
		sentinel := errors.New("client disconnected")
		err := svc.streamChatAnthropic(ctx, llmReq(),
			&domain.AnthropicSettings{APIKey: "sk-test", Model: "claude-test"}, nil,
			func(domain.LLMChatEvent) error { return sentinel })
		require.ErrorContains(t, err, "failed to send event:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("an invalid tool schema fails before any request", func(t *testing.T) {
		svc := newLLMStreamService()
		req := llmReq()
		req.Tools = []domain.LLMTool{{Name: "lookup", Description: "d", InputSchema: json.RawMessage(`{not json`)}}
		err := svc.streamChatAnthropic(ctx, req,
			&domain.AnthropicSettings{APIKey: "sk-test", Model: "claude-test"}, nil, collectEvents(&[]domain.LLMChatEvent{}))
		require.ErrorContains(t, err, "failed to parse tool input schema:")
	})

	t.Run("client-side tool calls surface as tool_use with parsed input", func(t *testing.T) {
		body := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"usage":{"input_tokens":5,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"Paris\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9}}

event: message_stop
data: {"type":"message_stop"}

`
		srv := sseServer(t, body, http.StatusOK)
		t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
		svc := newLLMStreamService()
		var events []domain.LLMChatEvent
		err := svc.streamChatAnthropic(ctx, llmReq(),
			&domain.AnthropicSettings{APIKey: "sk-test", Model: "claude-test"}, nil, collectEvents(&events))
		require.NoError(t, err)
		require.Len(t, events, 2)
		assert.Equal(t, "tool_use", events[0].Type)
		assert.Equal(t, "get_weather", events[0].ToolName)
		assert.Equal(t, map[string]interface{}{"city": "Paris"}, events[0].ToolInput)
		assert.Equal(t, "done", events[1].Type)
	})
}

const geminiTextStream = `data: {"candidates":[{"content":{"parts":[{"text":"Hello "}],"role":"model"}}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":1}}

data: {"candidates":[{"content":{"parts":[{"text":"world"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":6}}

`

func TestStreamChatGemini_Streaming(t *testing.T) {
	ctx := context.Background()

	t.Run("text chunks stream and done carries the latest cumulative usage", func(t *testing.T) {
		srv := sseServer(t, geminiTextStream, http.StatusOK)
		t.Setenv("GOOGLE_GEMINI_BASE_URL", srv.URL)
		svc := newLLMStreamService()
		var events []domain.LLMChatEvent
		err := svc.streamChatGemini(ctx, llmReq(),
			&domain.GeminiSettings{APIKey: "gm-test", Model: "gemini-test"}, nil, collectEvents(&events))
		require.NoError(t, err)
		require.Len(t, events, 3)
		assert.Equal(t, domain.LLMChatEvent{Type: "text", Content: "Hello "}, events[0])
		assert.Equal(t, domain.LLMChatEvent{Type: "text", Content: "world"}, events[1])
		done := events[2]
		assert.Equal(t, "done", done.Type)
		assert.False(t, done.Truncated)
		require.NotNil(t, done.InputTokens)
		require.NotNil(t, done.OutputTokens)
		assert.Equal(t, int64(4), *done.InputTokens)
		assert.Equal(t, int64(6), *done.OutputTokens, "Gemini usage is cumulative: the last chunk wins")
		assert.Equal(t, "gemini-test", done.Model)
	})

	t.Run("thought parts go to the thinking channel, never the answer", func(t *testing.T) {
		body := `data: {"candidates":[{"content":{"parts":[{"text":"let me think","thought":true},{"text":"Answer"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":6}}

`
		srv := sseServer(t, body, http.StatusOK)
		t.Setenv("GOOGLE_GEMINI_BASE_URL", srv.URL)
		svc := newLLMStreamService()
		var events []domain.LLMChatEvent
		err := svc.streamChatGemini(ctx, llmReq(),
			&domain.GeminiSettings{APIKey: "gm-test", Model: "gemini-test"}, nil, collectEvents(&events))
		require.NoError(t, err)
		require.Len(t, events, 3)
		assert.Equal(t, domain.LLMChatEvent{Type: "thinking", Content: "let me think"}, events[0])
		assert.Equal(t, domain.LLMChatEvent{Type: "text", Content: "Answer"}, events[1])
		assert.Equal(t, "done", events[2].Type)
	})

	t.Run("MAX_TOKENS finish marks done as truncated", func(t *testing.T) {
		body := strings.Replace(geminiTextStream, `"finishReason":"STOP"`, `"finishReason":"MAX_TOKENS"`, 1)
		srv := sseServer(t, body, http.StatusOK)
		t.Setenv("GOOGLE_GEMINI_BASE_URL", srv.URL)
		svc := newLLMStreamService()
		var events []domain.LLMChatEvent
		err := svc.streamChatGemini(ctx, llmReq(),
			&domain.GeminiSettings{APIKey: "gm-test", Model: "gemini-test"}, nil, collectEvents(&events))
		require.NoError(t, err)
		assert.True(t, events[len(events)-1].Truncated)
	})

	t.Run("provider HTTP failure is a stream error", func(t *testing.T) {
		srv := sseServer(t, `{"error":{"code":500,"message":"boom","status":"INTERNAL"}}`, http.StatusInternalServerError)
		t.Setenv("GOOGLE_GEMINI_BASE_URL", srv.URL)
		svc := newLLMStreamService()
		err := svc.streamChatGemini(ctx, llmReq(),
			&domain.GeminiSettings{APIKey: "gm-test", Model: "gemini-test"}, nil, collectEvents(&[]domain.LLMChatEvent{}))
		require.ErrorContains(t, err, "stream error:")
	})

	t.Run("a failing event callback aborts with its error", func(t *testing.T) {
		srv := sseServer(t, geminiTextStream, http.StatusOK)
		t.Setenv("GOOGLE_GEMINI_BASE_URL", srv.URL)
		svc := newLLMStreamService()
		sentinel := errors.New("client disconnected")
		err := svc.streamChatGemini(ctx, llmReq(),
			&domain.GeminiSettings{APIKey: "gm-test", Model: "gemini-test"}, nil,
			func(domain.LLMChatEvent) error { return sentinel })
		require.ErrorContains(t, err, "failed to send event:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("an invalid tool schema names the tool", func(t *testing.T) {
		svc := newLLMStreamService()
		req := llmReq()
		req.Tools = []domain.LLMTool{{Name: "lookup", Description: "d", InputSchema: json.RawMessage(`{not json`)}}
		err := svc.streamChatGemini(ctx, req,
			&domain.GeminiSettings{APIKey: "gm-test", Model: "gemini-test"}, nil, collectEvents(&[]domain.LLMChatEvent{}))
		require.ErrorContains(t, err, `failed to parse tool input schema for "lookup":`)
	})

	t.Run("function calls surface as tool_use with parsed args", func(t *testing.T) {
		body := `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"city":"Paris"},"id":"fc_1"}}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":6}}

`
		srv := sseServer(t, body, http.StatusOK)
		t.Setenv("GOOGLE_GEMINI_BASE_URL", srv.URL)
		svc := newLLMStreamService()
		var events []domain.LLMChatEvent
		err := svc.streamChatGemini(ctx, llmReq(),
			&domain.GeminiSettings{APIKey: "gm-test", Model: "gemini-test"}, nil, collectEvents(&events))
		require.NoError(t, err)
		require.Len(t, events, 2)
		assert.Equal(t, "tool_use", events[0].Type)
		assert.Equal(t, "get_weather", events[0].ToolName)
		assert.Equal(t, map[string]interface{}{"city": "Paris"}, events[0].ToolInput)
		assert.Equal(t, "done", events[1].Type)
	})
}

func TestStreamChatOpenAI_ToolUseAndErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("function call deltas assemble into one tool_use event", func(t *testing.T) {
		body := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":9,"completion_tokens":12}}

data: [DONE]

`
		srv := sseServer(t, body, http.StatusOK)
		svc := newLLMStreamService()
		var events []domain.LLMChatEvent
		err := svc.streamChatOpenAI(ctx, llmReq(),
			&domain.OpenAISettings{APIKey: "sk-test", Model: "gpt-test", BaseURL: srv.URL}, nil, collectEvents(&events))
		require.NoError(t, err)
		require.Len(t, events, 2)
		assert.Equal(t, "tool_use", events[0].Type)
		assert.Equal(t, "get_weather", events[0].ToolName)
		assert.Equal(t, map[string]interface{}{"city": "Paris"}, events[0].ToolInput)
		done := events[1]
		assert.Equal(t, "done", done.Type)
		require.NotNil(t, done.InputTokens)
		assert.Equal(t, int64(9), *done.InputTokens)
		assert.Equal(t, int64(12), *done.OutputTokens)
	})

	t.Run("a failing event callback aborts the stream", func(t *testing.T) {
		body := fmt.Sprintf("data: %s\n\ndata: [DONE]\n\n",
			`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}`)
		srv := sseServer(t, body, http.StatusOK)
		svc := newLLMStreamService()
		sentinel := errors.New("client disconnected")
		err := svc.streamChatOpenAI(ctx, llmReq(),
			&domain.OpenAISettings{APIKey: "sk-test", Model: "gpt-test", BaseURL: srv.URL}, nil,
			func(domain.LLMChatEvent) error { return sentinel })
		require.ErrorContains(t, err, "failed to send event:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("an invalid tool schema fails before any request", func(t *testing.T) {
		svc := newLLMStreamService()
		req := llmReq()
		req.Tools = []domain.LLMTool{{Name: "lookup", Description: "d", InputSchema: json.RawMessage(`{not json`)}}
		err := svc.streamChatOpenAI(ctx, req,
			&domain.OpenAISettings{APIKey: "sk-test", Model: "gpt-test"}, nil, collectEvents(&[]domain.LLMChatEvent{}))
		require.ErrorContains(t, err, "failed to parse tool input schema:")
	})
}

func TestJsonSchemaToGenaiSchema(t *testing.T) {
	t.Run("empty schema stays nil and invalid JSON errors", func(t *testing.T) {
		schema, err := jsonSchemaToGenaiSchema(nil)
		require.NoError(t, err)
		assert.Nil(t, schema)

		_, err = jsonSchemaToGenaiSchema(json.RawMessage(`{nope`))
		require.ErrorContains(t, err, "invalid JSON schema:")
	})

	t.Run("types map to Gemini enums and the root defaults to object", func(t *testing.T) {
		schema, err := jsonSchemaToGenaiSchema(json.RawMessage(`{
			"properties": {
				"city": {"type": "string", "description": "City name"},
				"days": {"type": "integer"},
				"tags": {"type": "array"},
				"mode": {"type": "string", "enum": ["fast", "slow"]}
			},
			"required": ["city"]
		}`))
		require.NoError(t, err)
		require.NotNil(t, schema)
		assert.Equal(t, "OBJECT", string(schema.Type))
		assert.Equal(t, []string{"city"}, schema.Required)
		assert.Equal(t, "STRING", string(schema.Properties["city"].Type))
		assert.Equal(t, "City name", schema.Properties["city"].Description)
		assert.Equal(t, "INTEGER", string(schema.Properties["days"].Type))
		// An array without items gets a string item schema: Gemini rejects
		// item-less arrays, so this default is load-bearing.
		require.NotNil(t, schema.Properties["tags"].Items)
		assert.Equal(t, "STRING", string(schema.Properties["tags"].Items.Type))
		assert.Equal(t, []string{"fast", "slow"}, schema.Properties["mode"].Enum)
	})

	t.Run("unknown types become unspecified and nested objects recurse", func(t *testing.T) {
		schema, err := jsonSchemaToGenaiSchema(json.RawMessage(`{
			"type": "object",
			"properties": {
				"nested": {"type": "object", "properties": {"n": {"type": "number"}}},
				"weird": {"type": "funky"}
			}
		}`))
		require.NoError(t, err)
		assert.Equal(t, "NUMBER", string(schema.Properties["nested"].Properties["n"].Type))
		assert.Equal(t, "TYPE_UNSPECIFIED", string(schema.Properties["weird"].Type))
	})
}
