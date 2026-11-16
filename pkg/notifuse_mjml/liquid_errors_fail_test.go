package notifuse_mjml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileWithLiquid(t *testing.T, body string, subject string, data MapOfAny) *CompileTemplateResponse {
	t.Helper()
	source := `<mjml><mj-body><mj-section><mj-column><mj-text>` + body + `</mj-text></mj-column></mj-section></mj-body></mjml>`
	resp, err := CompileTemplate(CompileTemplateRequest{
		WorkspaceID:  "ws",
		MessageID:    "m",
		MjmlSource:   &source,
		Subject:      &subject,
		TemplateData: data,
	})
	require.NoError(t, err)
	return resp
}

// A Liquid error the render records, which Liquid would write in the email as
// "Liquid error: ...", fails the compilation instead: the email is not sent, and
// the preview shows the error.
func TestLiquidErrorFailsTheEmail(t *testing.T) {
	asText := MapOfAny{"contact": map[string]interface{}{"custom_string_1": "12"}}
	asNumber := MapOfAny{"contact": map[string]interface{}{"custom_string_1": 12}}
	vip := `{% if contact.custom_string_1 >= 10 %}VIP{% endif %}`

	t.Run("in the body", func(t *testing.T) {
		resp := compileWithLiquid(t, "Hi "+vip, "Hello", asText)
		require.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Contains(t, resp.Error.Message, "comparison of String with 10 failed")
		assert.Nil(t, resp.HTML)
	})

	t.Run("in the subject", func(t *testing.T) {
		resp := compileWithLiquid(t, "Hi", "Hello "+vip, asText)
		require.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Contains(t, resp.Error.Message, "comparison of String with 10 failed")
	})

	t.Run("with the right type, the email compiles", func(t *testing.T) {
		resp := compileWithLiquid(t, "Hi "+vip, "Hello "+vip, asNumber)
		require.True(t, resp.Success, "error: %v", resp.Error)
		require.NotNil(t, resp.HTML)
		assert.Contains(t, *resp.HTML, "VIP")
	})

	t.Run("a missing field is no error", func(t *testing.T) {
		resp := compileWithLiquid(t, "Hi {{ contact.first_name }}{% if contact.missing > 10 %}x{% endif %}", "Hello", MapOfAny{})
		require.True(t, resp.Success, "error: %v", resp.Error)
	})
}

// A display condition never records a Liquid error, whatever the type of its
// value: the rule that a recorded error fails the email cannot hide a block that
// was shown, or stop an email that was sent.
func TestDisplayConditionsRecordNoLiquidError(t *testing.T) {
	ops := []string{DisplayConditionIsTrue, DisplayConditionIsBlank, DisplayConditionIsPresent, DisplayConditionIsAbsent,
		DisplayConditionEq, DisplayConditionNeq, DisplayConditionContains, DisplayConditionNotContains,
		DisplayConditionGt, DisplayConditionGte, DisplayConditionLt, DisplayConditionLte}
	values := []interface{}{nil, "", "abc", 3, 3.5, true, []interface{}{1, "a"}, map[string]interface{}{"k": 1}, "10", "-", "1e400"}
	compared := []string{"", "abc", "3", "10", "x", "1e400", "-1"}
	for _, op := range ops {
		for _, v := range values {
			for _, c := range compared {
				value := c
				cond := &DisplayCondition{Rules: []DisplayConditionRule{{Variable: "order.total", Operator: op, Value: &value}}}
				for _, data := range []map[string]interface{}{{}, {"order": map[string]interface{}{"total": v}}, {"order": "flat"}} {
					_, err := cond.Evaluate(data)
					require.NoError(t, err, "%s %v against %q on %v", op, v, c, data)
				}
			}
		}
	}
}
