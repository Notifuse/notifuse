package notifuse_mjml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const conditionMarker = "CONDITIONAL-BLOCK-MARKER"

// conditionalTextTree is a template whose one text block carries condition.
func conditionalTextTree(t *testing.T, condition *DisplayCondition) EmailBlock {
	t.Helper()
	text := &MJTextBlock{BaseBlock: NewBaseBlock("cond-text", MJMLComponentMjText)}
	text.Content = stringPtr("<p>" + conditionMarker + "</p>")
	text.DisplayCondition = condition
	column := &MJColumnBlock{BaseBlock: NewBaseBlock("col", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{
		&MJTextBlock{BaseBlock: &BaseBlock{ID: "always", Type: MJMLComponentMjText, Content: stringPtr("<p>always</p>")}},
		text,
	}
	section := &MJSectionBlock{BaseBlock: NewBaseBlock("sec", MJMLComponentMjSection)}
	section.Children = []EmailBlock{column}
	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}
	root := &MJMLBlock{BaseBlock: NewBaseBlock("root", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}
	return root
}

func compileOK(t *testing.T, req CompileTemplateRequest) *CompileTemplateResponse {
	t.Helper()
	req.WorkspaceID, req.MessageID = "ws", "msg"
	resp, err := CompileTemplate(req)
	require.NoError(t, err)
	require.True(t, resp.Success, "compile failed: %+v", resp.Error)
	return resp
}

func TestCompileTemplate_DisplayCondition(t *testing.T) {
	vip := &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{
		{Variable: "contact.custom_string_1", Operator: DisplayConditionEq, Value: stringPtr("vip")},
		{Variable: "order.items.size", Operator: DisplayConditionGte, Value: stringPtr("2")},
	}}

	t.Run("hides and shows the block in the HTML", func(t *testing.T) {
		shown := compileOK(t, CompileTemplateRequest{VisualEditorTree: conditionalTextTree(t, vip), TemplateData: MapOfAny{
			"contact": MapOfAny{"custom_string_1": "vip"},
			"order":   MapOfAny{"items": []any{MapOfAny{"sku": "a"}, MapOfAny{"sku": "b"}}},
		}})
		assert.Contains(t, *shown.HTML, conditionMarker)

		hidden := compileOK(t, CompileTemplateRequest{VisualEditorTree: conditionalTextTree(t, vip), TemplateData: MapOfAny{
			"contact": MapOfAny{"custom_string_1": "vip"},
			"order":   MapOfAny{"items": []any{MapOfAny{"sku": "a"}}},
		}})
		assert.NotContains(t, *hidden.HTML, conditionMarker)
		assert.Contains(t, *hidden.HTML, "always")
	})
	t.Run("Go int values in the data", func(t *testing.T) {
		total := &DisplayCondition{Match: "all", Rules: []DisplayConditionRule{{Variable: "order.total", Operator: DisplayConditionGt, Value: stringPtr("99")}}}
		resp := compileOK(t, CompileTemplateRequest{VisualEditorTree: conditionalTextTree(t, total), TemplateData: MapOfAny{"order": map[string]interface{}{"total": 100}}})
		assert.Contains(t, *resp.HTML, conditionMarker)
		resp = compileOK(t, CompileTemplateRequest{VisualEditorTree: conditionalTextTree(t, total), TemplateData: MapOfAny{"order": map[string]interface{}{"total": int64(99)}}})
		assert.NotContains(t, *resp.HTML, conditionMarker)
	})
	t.Run("preview without data hides a block that needs a variable", func(t *testing.T) {
		resp := compileOK(t, CompileTemplateRequest{VisualEditorTree: conditionalTextTree(t, vip)})
		assert.NotContains(t, *resp.HTML, conditionMarker)
	})
	t.Run("the export keeps the condition and still compiles", func(t *testing.T) {
		for _, value := range []string{"vip", `R&D <x> "a" 'b'`, "line\nbreak"} {
			condition := &DisplayCondition{Match: "any", Rules: []DisplayConditionRule{
				{Variable: "contact.custom_string_1", Operator: DisplayConditionEq, Value: stringPtr(value)},
				{Variable: "contact.custom_number_1", Operator: DisplayConditionLt, Value: stringPtr("10")},
			}}
			resp := compileOK(t, CompileTemplateRequest{VisualEditorTree: conditionalTextTree(t, condition), PreserveLiquid: true})
			open, close := condition.ToLiquid("cond-text")
			assert.Contains(t, *resp.MJML, open)
			assert.Contains(t, *resp.MJML, close)
		}
	})
	t.Run("a hidden block between an mj-liquid if pair", func(t *testing.T) {
		tree := conditionalTextTree(t, vip)
		column := tree.GetChildren()[0].GetChildren()[0].GetChildren()[0]
		opening := &MJLiquidBlock{BaseBlock: &BaseBlock{ID: "l1", Type: MJMLComponentMjLiquid, Content: stringPtr("{% if contact.email %}")}}
		closing := &MJLiquidBlock{BaseBlock: &BaseBlock{ID: "l2", Type: MJMLComponentMjLiquid, Content: stringPtr("{% endif %}")}}
		column.SetChildren([]EmailBlock{column.GetChildren()[0], opening, column.GetChildren()[1], closing})
		resp := compileOK(t, CompileTemplateRequest{VisualEditorTree: tree, TemplateData: MapOfAny{"contact": MapOfAny{"email": "a@b.c"}}})
		assert.NotContains(t, *resp.HTML, conditionMarker)
		assert.Contains(t, *resp.HTML, "always")
	})
}

// A template moved to code mode through the MJML export keeps the condition as Liquid
// that liquidgo evaluates on send: for every fixture case it must decide as the visual
// editor's send does.
func TestCompileTemplate_DisplayConditionExportRoundTrip(t *testing.T) {
	fixture := loadDisplayConditionFixture(t)
	for _, tc := range fixture.Evaluate {
		t.Run(tc.Name, func(t *testing.T) {
			export := compileOK(t, CompileTemplateRequest{VisualEditorTree: conditionalTextTree(t, tc.Condition), PreserveLiquid: true})
			data := MapOfAny(tc.Data)
			if len(data) == 0 {
				// code mode renders Liquid only when there is data, as a send always has
				data = MapOfAny{"workspace": MapOfAny{"name": "w"}}
			}
			code := compileOK(t, CompileTemplateRequest{MjmlSource: export.MJML, TemplateData: data})
			visual := compileOK(t, CompileTemplateRequest{VisualEditorTree: conditionalTextTree(t, tc.Condition), TemplateData: data})
			assert.Equal(t, tc.Expected, strings.Contains(*code.HTML, conditionMarker), "code mode")
			assert.Equal(t, tc.Expected, strings.Contains(*visual.HTML, conditionMarker), "visual mode")
			assert.NotContains(t, *code.HTML, "{%")
		})
	}
}

// Nested conditional blocks with the most rules, exported then compiled in code mode,
// stay under liquidgo's nesting limit and the template size cap.
func TestCompileTemplate_NestedLargestConditionsInCodeMode(t *testing.T) {
	rules := make([]DisplayConditionRule, MaxDisplayConditionRules)
	for i := range rules {
		rules[i] = DisplayConditionRule{Variable: "order.total", Operator: DisplayConditionGte, Value: stringPtr("10")}
	}
	condition := &DisplayCondition{Match: "all", Rules: rules}
	tree := conditionalTextTree(t, condition)
	body := tree.GetChildren()[0]
	section := body.GetChildren()[0].(*MJSectionBlock)
	section.DisplayCondition = condition
	wrapper := &MJWrapperBlock{BaseBlock: NewBaseBlock("wrap", MJMLComponentMjWrapper)}
	wrapper.Children = []EmailBlock{section}
	wrapper.DisplayCondition = condition
	body.SetChildren([]EmailBlock{wrapper})

	export := compileOK(t, CompileTemplateRequest{VisualEditorTree: tree, PreserveLiquid: true})
	for _, flag := range []string{"__dc_wrap", "__dc_sec", "__dc_cond_text"} {
		assert.Contains(t, *export.MJML, "{% if "+flag+" %}")
	}
	for total, shown := range map[float64]bool{10: true, 9: false} {
		code := compileOK(t, CompileTemplateRequest{MjmlSource: export.MJML, TemplateData: MapOfAny{"order": MapOfAny{"total": total}}})
		assert.Equal(t, shown, strings.Contains(*code.HTML, conditionMarker), "total %v", total)
	}
}
