package notifuse_mjml

import (
	"strings"
	"testing"
)

// compileSectionForGroupWidth compiles a 600px email whose single section holds
// the given children, and returns its HTML.
func compileSectionForGroupWidth(t *testing.T, children ...EmailBlock) string {
	t.Helper()
	section := &MJSectionBlock{BaseBlock: NewBaseBlock("section-1", MJMLComponentMjSection)}
	section.Children = children

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body-1", MJMLComponentMjBody)}
	body.Attributes["width"] = "600px"
	body.Children = []EmailBlock{section}

	root := &MJMLBlock{BaseBlock: NewBaseBlock("mjml-1", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}

	resp, err := CompileTemplate(CompileTemplateRequest{
		WorkspaceID:      "test-workspace",
		MessageID:        "test-message",
		VisualEditorTree: root,
	})
	if err != nil {
		t.Fatalf("CompileTemplate failed: %v", err)
	}
	if !resp.Success || resp.HTML == nil {
		t.Fatalf("expected a successful compilation, got %+v", resp.Error)
	}
	return *resp.HTML
}

func groupWidthTestColumn(id, text string, attrs map[string]interface{}) *MJColumnBlock {
	content := text
	textBase := NewBaseBlock(id+"-text", MJMLComponentMjText)
	textBase.Content = &content
	column := &MJColumnBlock{BaseBlock: NewBaseBlock(id, MJMLComponentMjColumn)}
	for k, v := range attrs {
		column.Attributes[k] = v
	}
	column.Children = []EmailBlock{&MJTextBlock{BaseBlock: textBase}}
	return column
}

func groupWidthTestGroup(attrs map[string]interface{}) *MJGroupBlock {
	group := &MJGroupBlock{BaseBlock: NewBaseBlock("group-1", MJMLComponentMjGroup)}
	for k, v := range attrs {
		group.Attributes[k] = v
	}
	group.Children = []EmailBlock{
		groupWidthTestColumn("group-col-1", "G1", nil),
		groupWidthTestColumn("group-col-2", "G2", nil),
	}
	return group
}

// TestCompileTemplateGroupTakesItsShareOfTheSection checks that a group without a
// width shares the section with the column next to it, in the desktop CSS and
// in Outlook, as MJML renders it.
func TestCompileTemplateGroupTakesItsShareOfTheSection(t *testing.T) {
	//GIVEN: a section holding a width-less group of two columns and a column.
	//WHEN: it is compiled.
	html := compileSectionForGroupWidth(t, groupWidthTestGroup(nil), groupWidthTestColumn("column-1", "C", nil))

	//THEN: the group takes half the section, its columns a quarter each in Outlook,
	// and the group and the column sit in one Outlook row.
	if !strings.Contains(html, `class="mj-column-per-50 mj-outlook-group-fix" style="font-size:0;line-height:0;`) {
		t.Errorf("expected the group to take half the section:\n%s", html)
	}
	if !strings.Contains(html, `<td style="vertical-align:top;width:150px;" >`) {
		t.Errorf("expected 150px Outlook cells for the group's columns:\n%s", html)
	}
	if !strings.Contains(html, `<!--[if mso | IE]></td><td class="" style="vertical-align:top;width:300px;" ><![endif]-->`) {
		t.Errorf("expected the column in the group's Outlook row:\n%s", html)
	}
}

// TestCompileTemplateGroupPercentageWidthHasItsRule checks that a group's
// percentage width gets its desktop CSS rule.
func TestCompileTemplateGroupPercentageWidthHasItsRule(t *testing.T) {
	//GIVEN: a 60% group next to a 40% column.
	//WHEN: it is compiled.
	html := compileSectionForGroupWidth(t,
		groupWidthTestGroup(map[string]interface{}{"width": "60%"}),
		groupWidthTestColumn("column-1", "C", map[string]interface{}{"width": "40%"}),
	)

	//THEN: the group class has a 60% rule.
	if !strings.Contains(html, `.mj-column-per-60 { width:60% !important; max-width: 60%; }`) {
		t.Errorf("expected a 60%% rule for the group class:\n%s", html)
	}
}
