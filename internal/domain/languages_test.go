package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidLanguage(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{"valid English", "en", true},
		{"valid French", "fr", true},
		{"valid Portuguese Brazil", "pt-BR", true},
		{"valid Chinese Traditional", "zh-TW", true},
		{"valid Arabic", "ar", true},
		{"valid Japanese", "ja", true},
		{"invalid empty", "", false},
		{"invalid unknown", "xx", false},
		{"invalid case sensitive", "EN", false},
		{"invalid with space", "en ", false},
		{"invalid full name", "English", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsValidLanguage(tc.code)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestDefaultLanguageCode(t *testing.T) {
	assert.Equal(t, "en", DefaultLanguageCode)
	assert.True(t, IsValidLanguage(DefaultLanguageCode), "DefaultLanguageCode must be a valid language")
}

func TestSupportedLanguages(t *testing.T) {
	// Verify we have a reasonable number of languages
	assert.True(t, len(SupportedLanguages) >= 30, "should have at least 30 supported languages")

	// Verify key languages are present
	expectedLanguages := []string{"en", "fr", "de", "es", "pt", "pt-BR", "zh", "zh-TW", "ja", "ko", "ar"}
	for _, code := range expectedLanguages {
		_, ok := SupportedLanguages[code]
		assert.True(t, ok, "expected language %s to be in SupportedLanguages", code)
	}
}
