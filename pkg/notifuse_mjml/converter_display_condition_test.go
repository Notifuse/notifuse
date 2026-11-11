package notifuse_mjml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// conditionTree builds a body with a section > column holding the given blocks. Golden
// strings use blocks with at most one attribute, since attribute order follows map order.
func conditionTree(t *testing.T, blocks ...string) EmailBlock {
	t.Helper()
	raw := `{"id":"root","type":"mjml","children":[{"id":"body","type":"mj-body","children":[
		{"id":"sec","type":"mj-section","children":[{"id":"col","type":"mj-column","children":[` +
		strings.Join(blocks, ",") + `]}]}]}]}`
	tree, err := UnmarshalEmailBlock([]byte(raw))
	require.NoError(t, err)
	return tree
}

const (
	goldenText   = `{"id":"t1","type":"mj-text","content":"Hello {{ contact.first_name }}"}`
	goldenImage  = `{"id":"i1","type":"mj-image","attributes":{"src":"https://example.com/a.png"}}`
	goldenSpacer = `{"id":"s1","type":"mj-spacer"}`
)

// Trees without conditions convert exactly as before display conditions existed.
func TestDisplayConditionConverter_TreesWithoutConditionsUnchanged(t *testing.T) {
	tree := conditionTree(t, goldenText, goldenImage, goldenSpacer)

	wantRaw := `<mjml>
  <mj-body>
    <mj-section>
      <mj-column>
        <mj-text>Hello {{ contact.first_name }}</mj-text>
        <mj-image src="https://example.com/a.png" />
        <mj-spacer />
      </mj-column>
    </mj-section>
  </mj-body>
</mjml>`
	assert.Equal(t, wantRaw, ConvertJSONToMJMLRaw(tree))
	assert.Equal(t, wantRaw, ConvertJSONToMJML(tree))

	withData, err := ConvertJSONToMJMLWithData(tree, `{"contact":{"first_name":"Ada"}}`)
	require.NoError(t, err)
	assert.Equal(t, strings.Replace(wantRaw, "{{ contact.first_name }}", "Ada", 1), withData)
}

func withCondition(block, condition string) string {
	return strings.TrimSuffix(block, "}") + `,"display_condition":` + condition + `}`
}

const hasImage = `{"match":"all","rules":[{"variable":"order.has_image","operator":"is_true"}]}`

func TestDisplayConditionConverter_DataPath(t *testing.T) {
	image := withCondition(goldenImage, hasImage)

	t.Run("shown when the condition holds", func(t *testing.T) {
		out, err := ConvertJSONToMJMLWithData(conditionTree(t, goldenSpacer, image), `{"order":{"has_image":true}}`)
		require.NoError(t, err)
		assert.Contains(t, out, `<mj-image src="https://example.com/a.png" />`)
	})
	for name, data := range map[string]string{"false": `{"order":{"has_image":false}}`, "absent": `{}`, "no data": ``} {
		t.Run("hidden when "+name+", without a blank line", func(t *testing.T) {
			out, err := ConvertJSONToMJMLWithData(conditionTree(t, goldenSpacer, image, goldenSpacer), data)
			require.NoError(t, err)
			assert.NotContains(t, out, "mj-image")
			assert.Contains(t, out, "        <mj-spacer />\n        <mj-spacer />\n")
		})
	}
	t.Run("a hidden section takes its children with it", func(t *testing.T) {
		raw := `{"id":"root","type":"mjml","children":[{"id":"body","type":"mj-body","children":[
			{"id":"sec","type":"mj-section","display_condition":` + hasImage + `,"children":[
				{"id":"col","type":"mj-column","children":[` + goldenText + `]}]}]}]}`
		tree, err := UnmarshalEmailBlock([]byte(raw))
		require.NoError(t, err)
		out, err := ConvertJSONToMJMLWithData(tree, `{"order":{"has_image":false}}`)
		require.NoError(t, err)
		assert.Equal(t, "<mjml>\n  <mj-body>\n\n  </mj-body>\n</mjml>", out)
	})
	t.Run("broken Liquid in a hidden block does not fail the send", func(t *testing.T) {
		broken := withCondition(`{"id":"bad","type":"mj-text","content":"{% if x %}never closed"}`, hasImage)
		_, err := ConvertJSONToMJMLWithData(conditionTree(t, broken), `{"order":{"has_image":false}}`)
		assert.NoError(t, err)
		_, err = ConvertJSONToMJMLWithData(conditionTree(t, broken), `{"order":{"has_image":true}}`)
		assert.Error(t, err)
	})
	t.Run("any with one true rule shows, all with one false rule hides", func(t *testing.T) {
		rules := `"rules":[{"variable":"order.has_image","operator":"is_true"},{"variable":"order.total","operator":"gt","value":"100"}]`
		data := `{"order":{"has_image":true,"total":50}}`
		out, err := ConvertJSONToMJMLWithData(conditionTree(t, withCondition(goldenImage, `{"match":"any",`+rules+`}`)), data)
		require.NoError(t, err)
		assert.Contains(t, out, "mj-image")
		out, err = ConvertJSONToMJMLWithData(conditionTree(t, withCondition(goldenImage, `{"match":"all",`+rules+`}`)), data)
		require.NoError(t, err)
		assert.NotContains(t, out, "mj-image")
	})
	t.Run("typed nil and untyped children are skipped", func(t *testing.T) {
		tree := conditionTree(t, goldenSpacer)
		column := tree.GetChildren()[0].GetChildren()[0].GetChildren()[0]
		var typedNil *MJTextBlock
		column.SetChildren(append(column.GetChildren(), typedNil))
		out, err := ConvertJSONToMJMLWithData(tree, `{"a":1}`)
		require.NoError(t, err)
		assert.Contains(t, out, "<mj-spacer />")
		assert.Equal(t, "", ConvertJSONToMJMLRaw(typedNil))
	})
}

// Hiding every child of a column, a wrapper or the body leaves valid MJML.
func TestDisplayConditionConverter_EveryChildHidden(t *testing.T) {
	hidden := func(id string) string {
		return withCondition(`{"id":"`+id+`","type":"mj-text","content":"x"}`, hasImage)
	}
	trees := map[string]string{
		"column": `{"id":"root","type":"mjml","children":[{"id":"body","type":"mj-body","children":[
			{"id":"sec","type":"mj-section","children":[{"id":"col","type":"mj-column","children":[` + hidden("a") + `,` + hidden("b") + `]}]}]}]}`,
		"wrapper": `{"id":"root","type":"mjml","children":[{"id":"body","type":"mj-body","children":[
			{"id":"w","type":"mj-wrapper","children":[` +
			withCondition(`{"id":"s","type":"mj-section","children":[{"id":"c","type":"mj-column","children":[`+hidden("a")+`]}]}`, hasImage) + `]}]}]}`,
		"body": `{"id":"root","type":"mjml","children":[{"id":"body","type":"mj-body","children":[` +
			withCondition(`{"id":"s","type":"mj-section","children":[{"id":"c","type":"mj-column","children":[`+hidden("a")+`]}]}`, hasImage) + `]}]}`,
	}
	for name, raw := range trees {
		t.Run(name, func(t *testing.T) {
			tree, err := UnmarshalEmailBlock([]byte(raw))
			require.NoError(t, err)
			resp, err := CompileTemplate(CompileTemplateRequest{
				WorkspaceID: "ws", MessageID: "msg", VisualEditorTree: tree,
				TemplateData: MapOfAny{"order": MapOfAny{"has_image": false}},
			})
			require.NoError(t, err)
			require.True(t, resp.Success, "%+v", resp.Error)
			assert.NotContains(t, *resp.MJML, "mj-text")
		})
	}
}

func TestDisplayConditionConverter_NoDataPath(t *testing.T) {
	present := withCondition(goldenImage, `{"match":"all","rules":[{"variable":"contact.first_name","operator":"is_present"}]}`)
	absent := withCondition(goldenSpacer, `{"match":"all","rules":[{"variable":"contact.first_name","operator":"is_absent"}]}`)
	out := ConvertJSONToMJML(conditionTree(t, present, absent))
	assert.NotContains(t, out, "mj-image")
	assert.Contains(t, out, "<mj-spacer />")
}

func TestDisplayConditionConverter_RawPathWrapsTheBlock(t *testing.T) {
	image := withCondition(`{"id":"img-1","type":"mj-image","attributes":{"src":"https://example.com/a.png"}}`,
		`{"match":"all","rules":[{"variable":"order.total","operator":"eq","value":"R&D"}]}`)
	out := ConvertJSONToMJMLRaw(conditionTree(t, goldenSpacer, image))
	open, close := (&DisplayCondition{Match: "all", Rules: []DisplayConditionRule{
		{Variable: "order.total", Operator: DisplayConditionEq, Value: stringPtr("R&D")},
	}}).ToLiquid("img-1")
	assert.Equal(t, `<mjml>
  <mj-body>
    <mj-section>
      <mj-column>
        <mj-spacer />
        `+open+`
        <mj-image src="https://example.com/a.png" />
        `+close+`
      </mj-column>
    </mj-section>
  </mj-body>
</mjml>`, out)
	assert.Contains(t, open, "__dc_img_1")
	assert.NotContains(t, out, "R&D", "values are encoded so the MJML stays well formed")
}
