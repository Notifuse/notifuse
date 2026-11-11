package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCustomText(t *testing.T) {
	assert.Nil(t, NormalizeCustomText(nil))
	assert.Nil(t, NormalizeCustomText(strPtr("")))
	assert.Nil(t, NormalizeCustomText(strPtr(" \n\t\r\n ")))
	assert.Equal(t, "a\nb\nc", *NormalizeCustomText(strPtr("a\r\nb\rc")))
	assert.Equal(t, "  Hello\n", *NormalizeCustomText(strPtr("  Hello\r\n")), "only line endings change")
}

func TestEmailTemplate_Validate_CustomText(t *testing.T) {
	src := "<mjml><mj-body><mj-section><mj-column><mj-text>Hello</mj-text></mj-column></mj-section></mj-body></mjml>"
	newEmail := func(text *string) *EmailTemplate {
		return &EmailTemplate{EditorMode: EditorModeCode, MjmlSource: &src, Subject: "Subject", Text: text}
	}

	t.Run("nil text is accepted", func(t *testing.T) {
		require.NoError(t, newEmail(nil).Validate(nil))
	})

	t.Run("text at the size limit is accepted", func(t *testing.T) {
		e := newEmail(strPtr(strings.Repeat("a", MaxCustomTextSize)))
		require.NoError(t, e.Validate(nil))
	})

	t.Run("text over the size limit is rejected", func(t *testing.T) {
		err := newEmail(strPtr(strings.Repeat("a", MaxCustomTextSize+1))).Validate(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "text version")
	})

	t.Run("invalid liquid is rejected and names the text version", func(t *testing.T) {
		err := newEmail(strPtr("{% if x %}Hello")).Validate(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email template: text version")
	})

	t.Run("valid liquid is accepted", func(t *testing.T) {
		require.NoError(t, newEmail(strPtr("Hi {{ contact.first_name }}\n{{ unsubscribe_url }}")).Validate(nil))
	})

	t.Run("line endings are normalised", func(t *testing.T) {
		e := newEmail(strPtr("a\r\nb\rc"))
		require.NoError(t, e.Validate(nil))
		require.NotNil(t, e.Text)
		assert.Equal(t, "a\nb\nc", *e.Text)
	})

	t.Run("whitespace only becomes automatic", func(t *testing.T) {
		e := newEmail(strPtr("  \r\n "))
		require.NoError(t, e.Validate(nil))
		assert.Nil(t, e.Text)
	})

	t.Run("a translation's invalid text is rejected with its language", func(t *testing.T) {
		err := validateTranslations(map[string]TemplateTranslation{
			"fr": {Email: newEmail(strPtr("{% for x in y %}"))},
		}, ChannelEmail, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "translation 'fr'")
		assert.Contains(t, err.Error(), "text version")
	})
}

func TestEmailTemplate_ApplyToCompileRequest_Text(t *testing.T) {
	var req CompileTemplateRequest
	(&EmailTemplate{Text: strPtr("Hi {{ name }}")}).ApplyToCompileRequest(&req, nil)
	require.NotNil(t, req.Text)
	assert.Equal(t, "Hi {{ name }}", *req.Text)

	req = CompileTemplateRequest{Text: strPtr("stale")}
	(&EmailTemplate{}).ApplyToCompileRequest(&req, nil)
	assert.Nil(t, req.Text, "an automatic variant clears the text")
}

func TestTranslationsWiden_CustomText(t *testing.T) {
	withText := func(text *string) map[string]TemplateTranslation {
		return map[string]TemplateTranslation{"fr": {Email: &EmailTemplate{Subject: "Bonjour", Text: text}}}
	}

	cases := []struct {
		name        string
		prior, next *string
		want        bool
	}{
		{"changing only the text widens", strPtr("Bonjour"), strPtr("Salut"), true},
		{"adding a text widens", nil, strPtr("Bonjour"), true},
		{"nil and empty are the same", nil, strPtr(""), false},
		{"nil and whitespace are the same", nil, strPtr(" \n "), false},
		{"crlf and lf are the same", strPtr("a\r\nb"), strPtr("a\nb"), false},
		{"cr and lf are the same", strPtr("a\rb"), strPtr("a\nb"), false},
		{"same text does not widen", strPtr("Bonjour"), strPtr("Bonjour"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, TranslationsWiden(withText(tc.prior), withText(tc.next)))
		})
	}
}
