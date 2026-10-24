package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Server-side tool loop: when the model calls scrape_url/search_web, the
// service must execute it against Firecrawl, feed the result back, and keep
// streaming — emitting server_tool_start/result for the UI and summing
// usage across both provider calls. A loop that dropped the second call,
// mislabeled the tool as client-side, or lost tokens would pass a naive
// "no error" check, so the full event sequence is asserted.

func firecrawlFake(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := FirecrawlScrapeResponse{Success: true}
		resp.Data.Markdown = "# Test Content\n\nSome text here."
		resp.Data.Title = "Test Page"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newAgenticLLMService(t *testing.T) (*LLMService, *domain.FirecrawlSettings) {
	t.Helper()
	log := logger.NewLoggerWithLevel("disabled")
	registry := NewServerSideToolRegistry(NewFirecrawlService(log), log)
	svc := NewLLMService(LLMServiceConfig{Logger: log, ToolRegistry: registry})
	fcSrv := firecrawlFake(t)
	return svc, &domain.FirecrawlSettings{APIKey: "fc-test", BaseURL: fcSrv.URL}
}

// sequenceServer replies with bodies[i] to the i-th request (repeating the
// last one) and counts calls.
func sequenceServer(t *testing.T, calls *int32, bodies ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(calls, 1)) - 1
		if n >= len(bodies) {
			n = len(bodies) - 1
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(bodies[n]))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func eventTypes(events []domain.LLMChatEvent) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.Type
	}
	return types
}

const anthropicToolCallStream = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"usage":{"input_tokens":5,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"scrape_url","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"url\":\"https://example.com\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9}}

event: message_stop
data: {"type":"message_stop"}

`

const anthropicFollowUpStream = `event: message_start
data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"usage":{"input_tokens":20,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Summary done"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}

event: message_stop
data: {"type":"message_stop"}

`

func TestStreamChatAnthropic_ServerSideToolLoop(t *testing.T) {
	svc, firecrawlSettings := newAgenticLLMService(t)
	var calls int32
	srv := sequenceServer(t, &calls, anthropicToolCallStream, anthropicFollowUpStream)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)

	var events []domain.LLMChatEvent
	err := svc.streamChatAnthropic(context.Background(), llmReq(),
		&domain.AnthropicSettings{APIKey: "sk-test", Model: "claude-test"},
		firecrawlSettings, collectEvents(&events))
	require.NoError(t, err)

	assert.Equal(t, int32(2), atomic.LoadInt32(&calls),
		"the tool result must be fed back in a second provider call")
	require.Equal(t, []string{"server_tool_start", "server_tool_result", "text", "done"}, eventTypes(events))

	assert.Equal(t, "scrape_url", events[0].ToolName)
	assert.Equal(t, map[string]interface{}{"url": "https://example.com"}, events[0].ToolInput)
	assert.Equal(t, "scrape_url", events[1].ToolName)
	assert.Contains(t, events[1].Content, "Test Page")
	assert.Equal(t, "", events[1].Error)
	assert.Equal(t, "Summary done", events[2].Content)

	done := events[3]
	require.NotNil(t, done.InputTokens)
	require.NotNil(t, done.OutputTokens)
	assert.Equal(t, int64(25), *done.InputTokens, "usage sums across both calls (5+20)")
	assert.Equal(t, int64(13), *done.OutputTokens, "usage sums across both calls (9+4)")
}

func TestStreamChatAnthropic_ServerToolFailureFeedsErrorBack(t *testing.T) {
	// Firecrawl is unreachable: the model must receive an "Error: ..." tool
	// result (not a hard failure), and the UI event carries the error text.
	log := logger.NewLoggerWithLevel("disabled")
	registry := NewServerSideToolRegistry(NewFirecrawlService(log), log)
	svc := NewLLMService(LLMServiceConfig{Logger: log, ToolRegistry: registry})
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close() // connections now refused

	var calls int32
	srv := sequenceServer(t, &calls, anthropicToolCallStream, anthropicFollowUpStream)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)

	var events []domain.LLMChatEvent
	err := svc.streamChatAnthropic(context.Background(), llmReq(),
		&domain.AnthropicSettings{APIKey: "sk-test", Model: "claude-test"},
		&domain.FirecrawlSettings{APIKey: "fc-test", BaseURL: deadURL}, collectEvents(&events))
	require.NoError(t, err, "a failed tool is reported to the model, not fatal to the chat")
	require.Equal(t, []string{"server_tool_start", "server_tool_result", "text", "done"}, eventTypes(events))
	// The failure reaches the model as tool-result content describing the
	// scrape failure, so it can apologize/retry instead of dying mid-chat.
	assert.Contains(t, events[1].Content, "Error scraping URL")
}

const openAIToolCallStream = `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"scrape_url","arguments":"{\"url\":\"https://example.com\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":8,"completion_tokens":6}}

data: [DONE]

`

const openAIFollowUpStream = `data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"Summary done"},"finish_reason":null}]}

data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":5}}

data: [DONE]

`

func TestStreamChatOpenAI_ServerSideToolLoop(t *testing.T) {
	svc, firecrawlSettings := newAgenticLLMService(t)
	var calls int32
	srv := sequenceServer(t, &calls, openAIToolCallStream, openAIFollowUpStream)

	var events []domain.LLMChatEvent
	err := svc.streamChatOpenAI(context.Background(), llmReq(),
		&domain.OpenAISettings{APIKey: "sk-test", Model: "gpt-test", BaseURL: srv.URL},
		firecrawlSettings, collectEvents(&events))
	require.NoError(t, err)

	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
	require.Equal(t, []string{"server_tool_start", "server_tool_result", "text", "done"}, eventTypes(events))
	assert.Contains(t, events[1].Content, "Test Page")
	done := events[3]
	require.NotNil(t, done.InputTokens)
	assert.Equal(t, int64(38), *done.InputTokens, "usage sums across both calls (8+30)")
	assert.Equal(t, int64(11), *done.OutputTokens)
}

const geminiToolCallStream = `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"scrape_url","args":{"url":"https://example.com"},"id":"fc_1"}}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":5}}

`

const geminiFollowUpStream = `data: {"candidates":[{"content":{"parts":[{"text":"Summary done"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":25,"candidatesTokenCount":3}}

`

func TestStreamChatGemini_ServerSideToolLoop(t *testing.T) {
	svc, firecrawlSettings := newAgenticLLMService(t)
	var calls int32
	srv := sequenceServer(t, &calls, geminiToolCallStream, geminiFollowUpStream)
	t.Setenv("GOOGLE_GEMINI_BASE_URL", srv.URL)

	var events []domain.LLMChatEvent
	err := svc.streamChatGemini(context.Background(), llmReq(),
		&domain.GeminiSettings{APIKey: "gm-test", Model: "gemini-test"},
		firecrawlSettings, collectEvents(&events))
	require.NoError(t, err)

	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
	require.Equal(t, []string{"server_tool_start", "server_tool_result", "text", "done"}, eventTypes(events))
	assert.Contains(t, events[1].Content, "Test Page")
	done := events[3]
	require.NotNil(t, done.InputTokens)
	assert.Equal(t, int64(32), *done.InputTokens, "usage sums across both calls (7+25)")
	assert.Equal(t, int64(8), *done.OutputTokens)
}
