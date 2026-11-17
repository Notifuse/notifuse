package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestJSONTypeToGenaiType(t *testing.T) {
	cases := map[string]genai.Type{
		"object":  genai.TypeObject,
		"OBJECT":  genai.TypeObject, // case-insensitive
		"string":  genai.TypeString,
		"number":  genai.TypeNumber,
		"integer": genai.TypeInteger,
		"boolean": genai.TypeBoolean,
		"array":   genai.TypeArray,
		"unknown": genai.TypeUnspecified,
		"":        genai.TypeUnspecified,
	}
	for in, want := range cases {
		assert.Equalf(t, want, jsonTypeToGenaiType(in), "type %q", in)
	}
}

func TestJSONSchemaToGenaiSchema(t *testing.T) {
	t.Run("empty raw returns nil", func(t *testing.T) {
		s, err := jsonSchemaToGenaiSchema(nil)
		require.NoError(t, err)
		assert.Nil(t, s)
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		_, err := jsonSchemaToGenaiSchema(json.RawMessage(`{not valid json`))
		assert.Error(t, err)
	})

	t.Run("object with nested properties, required, array items and enum", func(t *testing.T) {
		raw := json.RawMessage(`{
			"type": "object",
			"description": "search parameters",
			"properties": {
				"query": {"type": "string", "description": "the search query"},
				"limit": {"type": "integer"},
				"tags": {"type": "array", "items": {"type": "string"}},
				"mode": {"type": "string", "enum": ["fast", "slow"]}
			},
			"required": ["query"]
		}`)

		s, err := jsonSchemaToGenaiSchema(raw)
		require.NoError(t, err)
		require.NotNil(t, s)

		assert.Equal(t, genai.TypeObject, s.Type)
		assert.Equal(t, "search parameters", s.Description)
		assert.Equal(t, []string{"query"}, s.Required)

		require.Contains(t, s.Properties, "query")
		assert.Equal(t, genai.TypeString, s.Properties["query"].Type)
		assert.Equal(t, "the search query", s.Properties["query"].Description)

		require.Contains(t, s.Properties, "limit")
		assert.Equal(t, genai.TypeInteger, s.Properties["limit"].Type)

		require.Contains(t, s.Properties, "tags")
		assert.Equal(t, genai.TypeArray, s.Properties["tags"].Type)
		require.NotNil(t, s.Properties["tags"].Items)
		assert.Equal(t, genai.TypeString, s.Properties["tags"].Items.Type)

		require.Contains(t, s.Properties, "mode")
		assert.Equal(t, []string{"fast", "slow"}, s.Properties["mode"].Enum)
	})

	t.Run("root without type defaults to OBJECT", func(t *testing.T) {
		// Gemini requires function parameters to be a typed schema.
		raw := json.RawMessage(`{"properties": {"q": {"type": "string"}}}`)
		s, err := jsonSchemaToGenaiSchema(raw)
		require.NoError(t, err)
		require.NotNil(t, s)
		assert.Equal(t, genai.TypeObject, s.Type)
	})

	t.Run("nested object with properties but no type defaults to OBJECT", func(t *testing.T) {
		raw := json.RawMessage(`{"type":"object","properties":{"nested":{"properties":{"x":{"type":"string"}}}}}`)
		s, err := jsonSchemaToGenaiSchema(raw)
		require.NoError(t, err)
		require.NotNil(t, s.Properties["nested"])
		assert.Equal(t, genai.TypeObject, s.Properties["nested"].Type)
	})

	t.Run("array without items gets a synthesized item schema", func(t *testing.T) {
		// Gemini rejects an ARRAY property that does not declare its items.
		raw := json.RawMessage(`{"type":"object","properties":{"tags":{"type":"array"}}}`)
		s, err := jsonSchemaToGenaiSchema(raw)
		require.NoError(t, err)
		require.NotNil(t, s.Properties["tags"])
		assert.Equal(t, genai.TypeArray, s.Properties["tags"].Type)
		require.NotNil(t, s.Properties["tags"].Items, "array items must be synthesized")
		assert.Equal(t, genai.TypeString, s.Properties["tags"].Items.Type)
	})
}

func TestCalculateCost_Gemini(t *testing.T) {
	// gemini-2.5-flash: $0.30 input / $2.50 output per MTok
	inputCost, outputCost, totalCost := calculateCost("gemini-2.5-flash", 1_000_000, 1_000_000)
	assert.InDelta(t, 0.30, inputCost, 0.0001)
	assert.InDelta(t, 2.50, outputCost, 0.0001)
	assert.InDelta(t, 2.80, totalCost, 0.0001)

	// The Gemini 4 models suggested in the console
	for model, want := range map[string][2]float64{
		"gemini-4-carbon":   {4.0, 20.0},
		"gemini-4-argon":    {4.0, 20.0},
		"gemini-4-hydrogen": {1.50, 7.50},
	} {
		in, out, _ := calculateCost(model, 1_000_000, 1_000_000)
		assert.InDelta(t, want[0], in, 0.0001, model)
		assert.InDelta(t, want[1], out, 0.0001, model)
	}

	// Unknown gemini model falls back to zero cost
	in, out, total := calculateCost("gemini-experimental-xyz", 1000, 1000)
	assert.Zero(t, in)
	assert.Zero(t, out)
	assert.Zero(t, total)
}

func TestLLMService_StreamChat_MissingGeminiConfig(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "llm-integration",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:   "llm-integration",
				Name: "LLM Provider",
				Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{
					Kind:   domain.LLMProviderKindGemini,
					Gemini: nil, // Missing Gemini config
				},
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Gemini configuration is missing")
}

func TestLLMService_StreamChat_EmptyAPIKey_Gemini(t *testing.T) {
	service, mockAuthService, mockWorkspaceRepo := setupLLMServiceTest(t)

	req := &domain.LLMChatRequest{
		WorkspaceID:   "workspace123",
		IntegrationID: "llm-integration",
		Messages: []domain.LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := setupLLMContextWithAuth(mockAuthService, "workspace123", true, true)

	workspace := &domain.Workspace{
		ID:   "workspace123",
		Name: "Test Workspace",
		Integrations: []domain.Integration{
			{
				ID:   "llm-integration",
				Name: "LLM Provider",
				Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{
					Kind: domain.LLMProviderKindGemini,
					Gemini: &domain.GeminiSettings{
						APIKey: "", // Empty API key
						Model:  "gemini-2.5-flash",
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	mockWorkspaceRepo.EXPECT().
		GetByID(gomock.Any(), "workspace123").
		Return(workspace, nil).
		Times(1)

	err := service.StreamChat(ctx, req, func(event domain.LLMChatEvent) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key is not configured")
}
