package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkspaceTranslation_Validate(t *testing.T) {
	tests := []struct {
		name      string
		wt        WorkspaceTranslation
		expectErr bool
	}{
		{"valid", WorkspaceTranslation{Locale: "en", Content: MapOfAny{"key": "value"}}, false},
		{"empty locale", WorkspaceTranslation{Locale: "", Content: MapOfAny{"key": "value"}}, true},
		{"locale too long", WorkspaceTranslation{Locale: "12345678901", Content: MapOfAny{"key": "value"}}, true},
		{"nil content", WorkspaceTranslation{Locale: "en", Content: nil}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.wt.Validate()
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWorkspaceSettings_GetDefaultLanguage(t *testing.T) {
	ws := &WorkspaceSettings{}
	assert.Equal(t, "en", ws.GetDefaultLanguage())

	ws.DefaultLanguage = "fr"
	assert.Equal(t, "fr", ws.GetDefaultLanguage())
}

func TestWorkspaceSettings_GetSupportedLanguages(t *testing.T) {
	ws := &WorkspaceSettings{}
	assert.Equal(t, []string{"en"}, ws.GetSupportedLanguages())

	ws.SupportedLanguages = []string{"en", "fr", "de"}
	assert.Equal(t, []string{"en", "fr", "de"}, ws.GetSupportedLanguages())
}

func TestUpsertWorkspaceTranslationRequest_Validate(t *testing.T) {
	tests := []struct {
		name      string
		req       UpsertWorkspaceTranslationRequest
		expectErr bool
	}{
		{"valid", UpsertWorkspaceTranslationRequest{WorkspaceID: "ws1", Locale: "en", Content: MapOfAny{"key": "value"}}, false},
		{"empty workspace_id", UpsertWorkspaceTranslationRequest{WorkspaceID: "", Locale: "en", Content: MapOfAny{"key": "value"}}, true},
		{"empty locale", UpsertWorkspaceTranslationRequest{WorkspaceID: "ws1", Locale: "", Content: MapOfAny{"key": "value"}}, true},
		{"locale too long", UpsertWorkspaceTranslationRequest{WorkspaceID: "ws1", Locale: "12345678901", Content: MapOfAny{"key": "value"}}, true},
		{"nil content", UpsertWorkspaceTranslationRequest{WorkspaceID: "ws1", Locale: "en", Content: nil}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
