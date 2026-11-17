package notifuse_mjml

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A caller that shows only the subject (the console's message detail, on each message opened)
// gets it rendered without compiling the email.
func TestCompileTemplate_SubjectOnly(t *testing.T) {
	var req CompileTemplateRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"workspace_id": "ws1",
		"message_id": "preview",
		"subject": "Hello {{ name | upcase }}",
		"subject_preview": "For {{ name }}",
		"test_data": {"name": "Ann"},
		"subject_only": true
	}`), &req))
	require.True(t, req.SubjectOnly)

	t.Run("needs no body", func(t *testing.T) {
		require.NoError(t, req.Validate())
	})

	t.Run("renders the subject and the preview text, and nothing else", func(t *testing.T) {
		// A body that would not compile proves the body is not looked at.
		broken := "<mjml><mj-body>{% if %}</mj-body></mjml>"
		withBody := req
		withBody.MjmlSource = &broken
		withBody.IncludeText = true

		resp, err := CompileTemplate(withBody)
		require.NoError(t, err)
		require.True(t, resp.Success)
		require.NotNil(t, resp.Subject)
		require.NotNil(t, resp.SubjectPreview)
		assert.Equal(t, "Hello ANN", *resp.Subject)
		assert.Equal(t, "For Ann", *resp.SubjectPreview)
		assert.Nil(t, resp.HTML)
		assert.Nil(t, resp.MJML)
		assert.Nil(t, resp.Text)
		assert.Nil(t, resp.Error)
	})

	t.Run("a Liquid error in the subject is still reported", func(t *testing.T) {
		bad := req
		subject := "Hello {{ name "
		bad.Subject = &subject
		resp, err := CompileTemplate(bad)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
	})

	t.Run("without the flag a body is still required", func(t *testing.T) {
		full := req
		full.SubjectOnly = false
		assert.Error(t, full.Validate())
	})
}
