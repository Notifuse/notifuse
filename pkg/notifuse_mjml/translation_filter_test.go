package notifuse_mjml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslationFilter_SimpleKey(t *testing.T) {
	engine := NewSecureLiquidEngine()
	translations := map[string]interface{}{
		"welcome": map[string]interface{}{
			"heading": "Welcome!",
		},
	}
	engine.RegisterTranslations(translations)

	result, err := engine.Render(`{{ "welcome.heading" | t }}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "Welcome!", result)
}

func TestTranslationFilter_MissingKey(t *testing.T) {
	engine := NewSecureLiquidEngine()
	translations := map[string]interface{}{}
	engine.RegisterTranslations(translations)

	result, err := engine.Render(`{{ "missing.key" | t }}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "[Missing translation: missing.key]", result)
}

func TestTranslationFilter_WithPlaceholders(t *testing.T) {
	engine := NewSecureLiquidEngine()
	translations := map[string]interface{}{
		"welcome": map[string]interface{}{
			"greeting": "Hello {{ name }}, welcome to {{ site }}!",
		},
	}
	engine.RegisterTranslations(translations)

	// The liquidgo filter receives named keyword args as a map
	result, err := engine.Render(
		`{{ "welcome.greeting" | t: name: "John", site: "Notifuse" }}`,
		map[string]interface{}{},
	)
	require.NoError(t, err)
	assert.Equal(t, "Hello John, welcome to Notifuse!", result)
}

func TestTranslationFilter_WithContactVariable(t *testing.T) {
	engine := NewSecureLiquidEngine()
	translations := map[string]interface{}{
		"welcome": map[string]interface{}{
			"greeting": "Hello {{ name }}!",
		},
	}
	engine.RegisterTranslations(translations)

	result, err := engine.Render(
		`{{ "welcome.greeting" | t: name: contact.first_name }}`,
		map[string]interface{}{
			"contact": map[string]interface{}{
				"first_name": "Alice",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "Hello Alice!", result)
}

func TestTranslationFilter_FlatKey(t *testing.T) {
	engine := NewSecureLiquidEngine()
	translations := map[string]interface{}{
		"flat_key": "Flat value",
	}
	engine.RegisterTranslations(translations)

	result, err := engine.Render(`{{ "flat_key" | t }}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "Flat value", result)
}

func TestTranslationFilter_NoRegistration(t *testing.T) {
	// When no translations registered, t filter should return missing translation marker
	engine := NewSecureLiquidEngine()

	result, err := engine.Render(`{{ "some.key" | t }}`, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, "[Missing translation: some.key]", result)
}
