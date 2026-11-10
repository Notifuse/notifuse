package notifuse_mjml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func customText(s string) *string { return &s }

func TestCompileTemplate_CustomText(t *testing.T) {
	data := MapOfAny{"name": "Ada", "unsubscribe_url": "https://e.com/u?t=1&l=2"}

	t.Run("custom text is rendered with the data", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{
			MjmlSource:   plainTextSource(),
			TemplateData: data,
			Text:         customText("Hi {{ name }} & co\nStop: {{ unsubscribe_url }}"),
			IncludeText:  true,
		})
		require.True(t, resp.Success)
		require.NotNil(t, resp.Text)
		assert.Equal(t, "Hi Ada & co\nStop: https://e.com/u?t=1&l=2", *resp.Text)
		assert.Equal(t, TextSourceCustom, resp.TextSource)
		assert.Empty(t, resp.TextError)
		assert.Equal(t, *resp.Text, resp.CustomText())
	})

	t.Run("custom text is rendered without IncludeText, as sends need it", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{
			MjmlSource:   plainTextSource(),
			TemplateData: data,
			Text:         customText("Hi {{ name }}"),
		})
		require.True(t, resp.Success)
		assert.Equal(t, "Hi Ada", resp.CustomText())
		assert.Equal(t, TextSourceCustom, resp.TextSource)
	})

	t.Run("line endings of a stored text are normalised at render", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{MjmlSource: plainTextSource(), TemplateData: data, Text: customText("a\r\nb\rc")})
		assert.Equal(t, "a\nb\nc", resp.CustomText())
	})

	t.Run("a parse error keeps the email and falls back to the automatic text", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{
			MjmlSource:   plainTextSource(),
			TemplateData: data,
			Text:         customText("{% if name %}Hello"),
			IncludeText:  true,
		})
		require.True(t, resp.Success)
		require.NotNil(t, resp.HTML)
		assert.Contains(t, *resp.HTML, "Hello Ada")
		require.NotNil(t, resp.Text)
		assert.Contains(t, *resp.Text, "Shop now (https://shop.example.com/offers)")
		assert.Equal(t, TextSourceAuto, resp.TextSource)
		assert.NotEmpty(t, resp.TextError)
		assert.Empty(t, resp.CustomText())
	})

	t.Run("a parse error at send leaves the text to SendEmail", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{MjmlSource: plainTextSource(), TemplateData: data, Text: customText("{% if name %}Hello")})
		require.True(t, resp.Success)
		assert.Nil(t, resp.Text)
		assert.Empty(t, resp.TextSource)
		assert.NotEmpty(t, resp.TextError)
		assert.Empty(t, resp.CustomText())
	})

	t.Run("a text that renders blank falls back to the automatic text without an error", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{
			MjmlSource:   plainTextSource(),
			TemplateData: data,
			Text:         customText("{% if vip %}VIP offer{% endif %}"),
			IncludeText:  true,
		})
		require.True(t, resp.Success)
		assert.Equal(t, TextSourceAuto, resp.TextSource)
		assert.Empty(t, resp.TextError)
		assert.Contains(t, *resp.Text, "Hello Ada")
		assert.Empty(t, resp.CustomText())
	})

	t.Run("a text rendered over the cap falls back to the automatic text", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{
			MjmlSource:   plainTextSource(),
			TemplateData: MapOfAny{"items": make([]interface{}, MaxRenderedCustomTextSize/4+1)},
			Text:         customText("{% for i in items %}abcd{% endfor %}"),
			IncludeText:  true,
		})
		require.True(t, resp.Success)
		assert.Equal(t, TextSourceAuto, resp.TextSource)
		assert.NotEmpty(t, resp.TextError)
		assert.Empty(t, resp.CustomText())
	})

	t.Run("blank custom text is the automatic text", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{MjmlSource: plainTextSource(), TemplateData: data, Text: customText(" \n "), IncludeText: true})
		assert.Equal(t, TextSourceAuto, resp.TextSource)
		assert.Empty(t, resp.TextError)
		assert.Contains(t, *resp.Text, "Hello Ada")
	})

	t.Run("no custom text: automatic, marked as such", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{MjmlSource: plainTextSource(), TemplateData: data, IncludeText: true})
		assert.Equal(t, TextSourceAuto, resp.TextSource)
		assert.Empty(t, resp.CustomText())
	})

	t.Run("no custom text and no IncludeText: nothing, as in PR 1 sends", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{MjmlSource: plainTextSource(), TemplateData: data})
		assert.Nil(t, resp.Text)
		assert.Empty(t, resp.TextSource)
	})

	t.Run("PreserveLiquid keeps a custom text raw", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{MjmlSource: plainTextSource(), TemplateData: data, Text: customText("Hi {{ name }}"), PreserveLiquid: true, IncludeText: true})
		assert.Equal(t, "Hi {{ name }}", resp.CustomText())
	})

	t.Run("custom text links are not tracked", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{
			MjmlSource:   plainTextSource(),
			TemplateData: data,
			Text:         customText("Shop: https://shop.example.com/offers"),
			IncludeText:  true,
			TrackingSettings: TrackingSettings{
				EnableTracking: true, Endpoint: "https://track.example.com", UTMSource: "email", WorkspaceID: "ws", MessageID: "msg",
			},
		})
		assert.Equal(t, "Shop: https://shop.example.com/offers", resp.CustomText())
		assert.Contains(t, *resp.HTML, "https://track.example.com", "the html stays tracked")
	})

	t.Run("web channel has no text, custom or not", func(t *testing.T) {
		resp := compilePlainText(t, CompileTemplateRequest{MjmlSource: plainTextSource(), Channel: "web", Text: customText("Hi"), IncludeText: true})
		assert.Nil(t, resp.Text)
		assert.Empty(t, resp.CustomText())
	})

	t.Run("html and subject are the same with and without a custom text (tracking off)", func(t *testing.T) {
		subject := "Hi {{ name }}"
		base := CompileTemplateRequest{MjmlSource: plainTextSource(), TemplateData: data, Subject: &subject, IncludeText: true}
		withText := base
		withText.Text = customText("Custom")
		a := compilePlainText(t, base)
		b := compilePlainText(t, withText)
		assert.Equal(t, *a.HTML, *b.HTML)
		assert.Equal(t, *a.Subject, *b.Subject)
	})
}

// The starting point of a custom text is the automatic text of a compile that kept
// Liquid, with no data and tracking off.
func TestCompileTemplate_CustomTextSeed(t *testing.T) {
	seed := func(t *testing.T, req CompileTemplateRequest) *CompileTemplateResponse {
		t.Helper()
		req.PreserveLiquid = true
		req.IncludeText = true
		return compilePlainText(t, req)
	}
	wrap := func(body string) *string {
		s := `<mjml><mj-body>` + body + `</mj-body></mjml>`
		return &s
	}

	t.Run("liquid links are kept", func(t *testing.T) {
		resp := seed(t, CompileTemplateRequest{MjmlSource: wrap(`<mj-section><mj-column><mj-text>Hi {{ contact.first_name }} <a href="{{ unsubscribe_url }}">Unsubscribe</a></mj-text><mj-button href="{{ confirm_subscription_url }}">Confirm</mj-button></mj-column></mj-section>`)})
		require.True(t, resp.Success)
		require.NotNil(t, resp.Text)
		assert.Contains(t, *resp.Text, "Hi {{ contact.first_name }}")
		assert.Contains(t, *resp.Text, "Unsubscribe ({{ unsubscribe_url }})")
		assert.Contains(t, *resp.Text, "Confirm ({{ confirm_subscription_url }})")
		assert.Equal(t, TextSourceAuto, resp.TextSource)
		assert.Empty(t, resp.TextError)
	})

	t.Run("liquid inside a text block is kept", func(t *testing.T) {
		resp := seed(t, CompileTemplateRequest{MjmlSource: wrap(`<mj-section><mj-column><mj-text>{% if vip %}VIP{% endif %}</mj-text></mj-column></mj-section>`)})
		assert.Contains(t, *resp.Text, "{% if vip %}VIP{% endif %}")
		assert.Empty(t, resp.TextError)
	})

	// Documented limit: Liquid tags written between blocks are text between MJML
	// elements, which the MJML compiler drops when Liquid is not rendered.
	t.Run("a loop written around sections is not carried over", func(t *testing.T) {
		resp := seed(t, CompileTemplateRequest{MjmlSource: wrap(`{% for item in order.items %}<mj-section><mj-column><mj-text>- {{ item.name }}</mj-text></mj-column></mj-section>{% endfor %}`)})
		require.True(t, resp.Success)
		assert.Contains(t, *resp.Text, "- {{ item.name }}")
		assert.NotContains(t, *resp.Text, "{% for")
		require.NotNil(t, resp.MJML)
		assert.Contains(t, *resp.MJML, "{% for", "the MJML still shows the loop, which the console checks")
	})

	t.Run("a condition written around sections is not carried over", func(t *testing.T) {
		resp := seed(t, CompileTemplateRequest{MjmlSource: wrap(`{% if vip %}<mj-section><mj-column><mj-text>VIP offer</mj-text></mj-column></mj-section>{% else %}<mj-section><mj-column><mj-text>Standard offer</mj-text></mj-column></mj-section>{% endif %}`)})
		require.True(t, resp.Success)
		assert.Contains(t, *resp.Text, "VIP offer")
		assert.Contains(t, *resp.Text, "Standard offer")
		assert.NotContains(t, *resp.Text, "{% if")
	})

	t.Run("a loop in mj-raw tags wrapping sections is kept", func(t *testing.T) {
		resp := seed(t, CompileTemplateRequest{MjmlSource: wrap(`<mj-raw>{% for item in order.items %}</mj-raw><mj-section><mj-column><mj-text>- {{ item.name }}</mj-text></mj-column></mj-section><mj-raw>{% endfor %}</mj-raw>`)})
		require.True(t, resp.Success)
		require.NotNil(t, resp.Text)
		assert.Contains(t, *resp.Text, "{% for item in order.items %}")
		assert.Contains(t, *resp.Text, "{% endfor %}")
	})

	t.Run("an mj-raw tag placed between two sections is not carried over", func(t *testing.T) {
		resp := seed(t, CompileTemplateRequest{MjmlSource: wrap(`<mj-raw>{% if vip %}</mj-raw><mj-section><mj-column><mj-text>VIP offer</mj-text></mj-column></mj-section><mj-raw>{% else %}</mj-raw><mj-section><mj-column><mj-text>Standard offer</mj-text></mj-column></mj-section><mj-raw>{% endif %}</mj-raw>`)})
		require.True(t, resp.Success)
		require.NotNil(t, resp.Text)
		assert.NotContains(t, *resp.Text, "{% else %}")
	})

	t.Run("a comparison written between blocks fails the seed", func(t *testing.T) {
		resp := seed(t, CompileTemplateRequest{MjmlSource: wrap(`{% if cart.total < 50 %}<mj-section><mj-column><mj-text>Add more</mj-text></mj-column></mj-section>{% endif %}`)})
		assert.False(t, resp.Success)
		assert.Nil(t, resp.Text)
	})

	t.Run("a visual liquid block loop is not carried over", func(t *testing.T) {
		liquidBase := NewBaseBlock("liquid-1", MJMLComponentMjLiquid)
		liquidBase.Content = stringPtr(`{% for p in products %}<mj-section><mj-column><mj-text>{{ p.name }}</mj-text></mj-column></mj-section>{% endfor %}`)
		body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
		body.Children = []EmailBlock{&MJLiquidBlock{BaseBlock: liquidBase}}
		tree := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
		tree.Children = []EmailBlock{body}

		resp := seed(t, CompileTemplateRequest{VisualEditorTree: tree})
		require.True(t, resp.Success)
		require.NotNil(t, resp.Text)
		assert.Contains(t, *resp.Text, "{{ p.name }}")
		assert.NotContains(t, *resp.Text, "{% for")
	})

	t.Run("a pair split across blocks is flagged", func(t *testing.T) {
		resp := seed(t, CompileTemplateRequest{MjmlSource: wrap(`<mj-section><mj-column><mj-text>{% if vip %}VIP</mj-text></mj-column></mj-section>{% endif %}`)})
		require.True(t, resp.Success)
		require.NotNil(t, resp.Text)
		assert.Contains(t, *resp.Text, "{% if vip %}VIP")
		assert.NotEmpty(t, resp.TextError)
	})

	t.Run("tracking off keeps static links direct", func(t *testing.T) {
		resp := seed(t, CompileTemplateRequest{MjmlSource: plainTextSource()})
		assert.Contains(t, *resp.Text, "Shop now (https://shop.example.com/offers)")
		assert.False(t, strings.Contains(*resp.Text, "/r/"))
	})
}

func TestCompileTemplateResponse_CustomText(t *testing.T) {
	var nilResp *CompileTemplateResponse
	assert.Empty(t, nilResp.CustomText())
	assert.Empty(t, (&CompileTemplateResponse{TextSource: TextSourceAuto, Text: customText("auto")}).CustomText())
	assert.Equal(t, "mine", (&CompileTemplateResponse{TextSource: TextSourceCustom, Text: customText("mine")}).CustomText())
	assert.Empty(t, (&CompileTemplateResponse{TextSource: TextSourceCustom}).CustomText())
}
