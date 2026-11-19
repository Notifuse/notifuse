package notifuse_mjml

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liquidBlockSection is a section whose one child is an mj-liquid block holding content.
func liquidBlockSection(content string) EmailBlock {
	base := NewBaseBlock("liq", MJMLComponentMjLiquid)
	base.Content = stringPtr(content)
	section := &MJSectionBlock{BaseBlock: NewBaseBlock("sec", MJMLComponentMjSection)}
	section.Children = []EmailBlock{&MJLiquidBlock{BaseBlock: base}}
	return section
}

// conditionMarkerJSON returns the JSON a condition marker of out carries.
func conditionMarkerJSON(t *testing.T, out string) string {
	t.Helper()
	start := strings.Index(out, conditionMarkerPrefix)
	require.GreaterOrEqual(t, start, 0, "no condition marker in:\n%s", out)
	rest := out[start+len(conditionMarkerPrefix):]
	end := strings.Index(rest, liquidCommentEnd)
	require.GreaterOrEqual(t, end, 0)
	return rest[:end]
}

func TestLiquidMarkers_LiquidBlockKeepsItsExactContent(t *testing.T) {
	content := "{% for item in items %}\n<mj-column><mj-text>{{ item.name }}</mj-text></mj-column>\n{% endfor %}"
	out := ConvertJSONToMJMLRawWithMarkers(liquidBlockSection(content))
	assert.Contains(t, out, LiquidBlockMarkerOpen+content+LiquidBlockMarkerClose)
}

func TestLiquidMarkers_ConditionWrapsTheBlockAndItsLiquid(t *testing.T) {
	condition := &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{
		{Variable: "order.total", Operator: DisplayConditionGte, Value: stringPtr("10")},
	}}
	image := withCondition(`{"id":"img-1","type":"mj-image","attributes":{"src":"https://example.com/a.png"}}`,
		`{"match":"all","rules":[{"variable":"order.total","operator":"gte","value":"10"}]}`)
	out := ConvertJSONToMJMLRawWithMarkers(conditionTree(t, goldenSpacer, image))
	open, close := condition.ToLiquid("img-1")
	assert.Equal(t, `<mjml>
  <mj-body>
    <mj-section>
      <mj-column>
        <mj-spacer />
        `+conditionMarkerPrefix+`{"match":"all","rules":[{"variable":"order.total","operator":"gte","value":"10"}]}`+liquidCommentEnd+open+`
        <mj-image src="https://example.com/a.png" />
        `+close+ConditionMarkerClose+`
      </mj-column>
    </mj-section>
  </mj-body>
</mjml>`, out)
}

// The JSON can neither end the Liquid comment (%}) nor open a tag for the import's HTML parser.
func TestLiquidMarkers_ConditionJSONIsEscaped(t *testing.T) {
	value := `50% {%} </mj-text> <b> R&D "q"`
	condition := &DisplayCondition{Match: "any", Rules: []DisplayConditionRule{
		{Variable: "contact.custom_string_1", Operator: DisplayConditionEq, Value: stringPtr(value)},
	}}
	out := ConvertJSONToMJMLRawWithMarkers(conditionalTextTree(t, condition))
	raw := conditionMarkerJSON(t, out)
	for _, c := range []string{"%", "<", ">", "&"} {
		assert.NotContains(t, raw, c)
	}
	var decoded DisplayCondition
	require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
	assert.Equal(t, *condition, decoded)
}

func TestLiquidMarkers_PlainRawExportHasNone(t *testing.T) {
	condition := &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{
		{Variable: "contact.first_name", Operator: DisplayConditionIsPresent},
	}}
	tree := conditionalTextTree(t, condition)
	column := tree.GetChildren()[0].GetChildren()[0].GetChildren()[0]
	liquid := &MJLiquidBlock{BaseBlock: &BaseBlock{ID: "l1", Type: MJMLComponentMjLiquid, Content: stringPtr("{% if a %}<mj-text>a</mj-text>{% endif %}")}}
	column.SetChildren(append(column.GetChildren(), liquid))
	assert.NotContains(t, ConvertJSONToMJMLRaw(tree), "notifuse:")
}

func TestCompileTemplate_LiquidMarkers(t *testing.T) {
	condition := &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{
		{Variable: "contact.first_name", Operator: DisplayConditionIsPresent},
	}}
	tree := func() EmailBlock {
		tree := conditionalTextTree(t, condition)
		column := tree.GetChildren()[0].GetChildren()[0].GetChildren()[0]
		liquid := &MJLiquidBlock{BaseBlock: &BaseBlock{ID: "l1", Type: MJMLComponentMjLiquid, Content: stringPtr("{% if contact.first_name %}<mj-text>Hi {{ contact.first_name }}</mj-text>{% endif %}")}}
		column.SetChildren(append(column.GetChildren(), liquid))
		return tree
	}

	t.Run("needs preserve_liquid", func(t *testing.T) {
		resp := compileOK(t, CompileTemplateRequest{VisualEditorTree: tree(), LiquidMarkers: true,
			TemplateData: MapOfAny{"contact": MapOfAny{"first_name": "Ada"}}})
		assert.NotContains(t, *resp.MJML, "notifuse:")
	})
	t.Run("marks the export", func(t *testing.T) {
		resp := compileOK(t, CompileTemplateRequest{VisualEditorTree: tree(), PreserveLiquid: true, LiquidMarkers: true})
		assert.Contains(t, *resp.MJML, LiquidBlockMarkerOpen)
		assert.Contains(t, *resp.MJML, conditionMarkerPrefix)
	})
	t.Run("without the field the export is unchanged", func(t *testing.T) {
		resp := compileOK(t, CompileTemplateRequest{VisualEditorTree: tree(), PreserveLiquid: true})
		assert.NotContains(t, *resp.MJML, "notifuse:")
	})
	t.Run("the markers leave no trace once sent", func(t *testing.T) {
		marked := compileOK(t, CompileTemplateRequest{VisualEditorTree: tree(), PreserveLiquid: true, LiquidMarkers: true})
		plain := compileOK(t, CompileTemplateRequest{VisualEditorTree: tree(), PreserveLiquid: true})
		data := MapOfAny{"contact": MapOfAny{"first_name": "Ada"}}
		markedHTML := compileOK(t, CompileTemplateRequest{MjmlSource: marked.MJML, TemplateData: data})
		plainHTML := compileOK(t, CompileTemplateRequest{MjmlSource: plain.MJML, TemplateData: data})
		assert.Equal(t, *plainHTML.HTML, *markedHTML.HTML)
		assert.Contains(t, *markedHTML.HTML, "Hi Ada")
		assert.Contains(t, *markedHTML.HTML, conditionMarker)
	})
}
