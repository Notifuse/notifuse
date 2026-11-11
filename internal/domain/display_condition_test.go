package domain

import (
	"encoding/json"
	"testing"

	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	validDisplayCondition   = `{"match":"all","rules":[{"variable":"order.has_image","operator":"is_true"}]}`
	invalidDisplayCondition = `{"match":"all","rules":[{"variable":"order.has_image","operator":"between","value":"1"}]}`
)

// conditionalTemplateTree is a template tree with an image under a display condition
// and, when columnCondition is set, a condition on its column too.
func conditionalTemplateTree(t *testing.T, imageCondition, columnCondition string) notifuse_mjml.EmailBlock {
	t.Helper()
	column := `{"id":"col","type":"mj-column"`
	if columnCondition != "" {
		column += `,"display_condition":` + columnCondition
	}
	raw := `{"id":"root","type":"mjml","children":[{"id":"body","type":"mj-body","children":[
		{"id":"sec","type":"mj-section","children":[` + column + `,"children":[
			{"id":"txt","type":"mj-text","content":"always"},
			{"id":"img","type":"mj-image","attributes":{"src":"https://example.com/hidden.png"},"display_condition":` + imageCondition + `}]}]}]}]}`
	tree, err := notifuse_mjml.UnmarshalEmailBlock([]byte(raw))
	require.NoError(t, err)
	return tree
}

func TestEmailTemplate_Validate_DisplayConditions(t *testing.T) {
	t.Run("a valid condition is accepted", func(t *testing.T) {
		email := &EmailTemplate{Subject: "s", VisualEditorTree: conditionalTemplateTree(t, validDisplayCondition, "")}
		assert.NoError(t, email.Validate(nil))
	})
	t.Run("an unknown operator is refused", func(t *testing.T) {
		email := &EmailTemplate{Subject: "s", VisualEditorTree: conditionalTemplateTree(t, invalidDisplayCondition, "")}
		err := email.Validate(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email template")
		assert.Contains(t, err.Error(), "img")
	})
	t.Run("a condition on a column is refused", func(t *testing.T) {
		email := &EmailTemplate{Subject: "s", VisualEditorTree: conditionalTemplateTree(t, validDisplayCondition, validDisplayCondition)}
		err := email.Validate(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "col")
	})
	t.Run("a condition under mj-head is refused", func(t *testing.T) {
		raw := `{"id":"root","type":"mjml","children":[
			{"id":"head","type":"mj-head","children":[{"id":"raw-1","type":"mj-raw","content":"x","display_condition":` + validDisplayCondition + `}]},
			{"id":"body","type":"mj-body","children":[]}]}`
		tree, err := notifuse_mjml.UnmarshalEmailBlock([]byte(raw))
		require.NoError(t, err)
		err = (&EmailTemplate{Subject: "s", VisualEditorTree: tree}).Validate(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "raw-1")
	})
	t.Run("the compiled preview omits the hidden block", func(t *testing.T) {
		email := &EmailTemplate{Subject: "s", VisualEditorTree: conditionalTemplateTree(t, validDisplayCondition, "")}
		require.NoError(t, email.Validate(MapOfAny{"order": map[string]interface{}{"has_image": false}}))
		assert.NotContains(t, email.CompiledPreview, "hidden.png")
		assert.Contains(t, email.CompiledPreview, "always")

		email = &EmailTemplate{Subject: "s", VisualEditorTree: conditionalTemplateTree(t, validDisplayCondition, "")}
		require.NoError(t, email.Validate(MapOfAny{"order": map[string]interface{}{"has_image": true}}))
		assert.Contains(t, email.CompiledPreview, "hidden.png")
	})
	t.Run("the condition survives JSON and the database round trip", func(t *testing.T) {
		email := &EmailTemplate{Subject: "s", CompiledPreview: "x", VisualEditorTree: conditionalTemplateTree(t, validDisplayCondition, "")}
		value, err := email.Value()
		require.NoError(t, err)
		var scanned EmailTemplate
		require.NoError(t, scanned.Scan(value))
		img := scanned.VisualEditorTree.GetChildren()[0].GetChildren()[0].GetChildren()[0].GetChildren()[1]
		require.NotNil(t, img.GetDisplayCondition())
		assert.Equal(t, "order.has_image", img.GetDisplayCondition().Rules[0].Variable)
	})
}

func TestTemplate_Validate_TranslationDisplayConditions(t *testing.T) {
	template := func(translationCondition string) *Template {
		return &Template{
			ID: "tpl", Name: "T", Version: 1, Channel: ChannelEmail, Category: "marketing",
			Email: &EmailTemplate{Subject: "s", CompiledPreview: "x", VisualEditorTree: conditionalTemplateTree(t, validDisplayCondition, "")},
			Translations: map[string]TemplateTranslation{
				"fr": {Email: &EmailTemplate{Subject: "s", CompiledPreview: "x", VisualEditorTree: conditionalTemplateTree(t, translationCondition, "")}},
			},
		}
	}
	t.Run("a translation keeps its own condition", func(t *testing.T) {
		tpl := template(`{"match":"any","rules":[{"variable":"contact.language","operator":"eq","value":"fr"}]}`)
		require.NoError(t, tpl.Validate())
		data, err := json.Marshal(tpl)
		require.NoError(t, err)
		var back Template
		require.NoError(t, json.Unmarshal(data, &back))
		img := back.Translations["fr"].Email.VisualEditorTree.GetChildren()[0].GetChildren()[0].GetChildren()[0].GetChildren()[1]
		require.NotNil(t, img.GetDisplayCondition())
		assert.Equal(t, "contact.language", img.GetDisplayCondition().Rules[0].Variable)
	})
	t.Run("an invalid condition in a translation is refused", func(t *testing.T) {
		err := template(invalidDisplayCondition).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "translation 'fr'")
	})
}

func TestTemplateBlockRequests_DisplayConditions(t *testing.T) {
	block := func(condition string) notifuse_mjml.EmailBlock {
		b, err := notifuse_mjml.UnmarshalEmailBlock([]byte(`{"id":"s1","type":"mj-section","display_condition":` + condition + `,"children":[]}`))
		require.NoError(t, err)
		return b
	}
	t.Run("create", func(t *testing.T) {
		_, _, err := (&CreateTemplateBlockRequest{WorkspaceID: "ws", Name: "Promo", Block: block(validDisplayCondition)}).Validate()
		assert.NoError(t, err)
		_, _, err = (&CreateTemplateBlockRequest{WorkspaceID: "ws", Name: "Promo", Block: block(invalidDisplayCondition)}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid create template block request")
	})
	t.Run("update", func(t *testing.T) {
		_, _, err := (&UpdateTemplateBlockRequest{WorkspaceID: "ws", ID: "b1", Name: "Promo", Block: block(validDisplayCondition)}).Validate()
		assert.NoError(t, err)
		_, _, err = (&UpdateTemplateBlockRequest{WorkspaceID: "ws", ID: "b1", Name: "Promo", Block: block(invalidDisplayCondition)}).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid update template block request")
	})
	t.Run("a column saved as a block cannot carry a condition", func(t *testing.T) {
		column, err := notifuse_mjml.UnmarshalEmailBlock([]byte(`{"id":"c1","type":"mj-column","display_condition":` + validDisplayCondition + `,"children":[]}`))
		require.NoError(t, err)
		_, _, err = (&CreateTemplateBlockRequest{WorkspaceID: "ws", Name: "Col", Block: column}).Validate()
		assert.Error(t, err)
	})
	t.Run("TemplateBlock JSON round trip keeps the condition", func(t *testing.T) {
		data, err := json.Marshal(TemplateBlock{ID: "b1", Name: "Promo", Block: block(validDisplayCondition)})
		require.NoError(t, err)
		var back TemplateBlock
		require.NoError(t, json.Unmarshal(data, &back))
		require.NotNil(t, back.Block.GetDisplayCondition())
		assert.Equal(t, "all", back.Block.GetDisplayCondition().Match)
	})
}

func TestWorkspaceSettings_Validate_TemplateBlockDisplayConditions(t *testing.T) {
	block := func(condition string) notifuse_mjml.EmailBlock {
		b, err := notifuse_mjml.UnmarshalEmailBlock([]byte(`{"id":"t1","type":"mj-text","content":"x","display_condition":` + condition + `}`))
		require.NoError(t, err)
		return b
	}
	settings := func(condition string) WorkspaceSettings {
		return WorkspaceSettings{Timezone: "UTC", DefaultLanguage: "en", Languages: []string{"en"},
			TemplateBlocks: []TemplateBlock{{ID: "b1", Name: "Block", Block: block(condition)}}}
	}
	valid := settings(validDisplayCondition)
	require.NoError(t, valid.Validate("passphrase"))
	data, err := json.Marshal(valid)
	require.NoError(t, err)
	var back WorkspaceSettings
	require.NoError(t, json.Unmarshal(data, &back))
	require.NotNil(t, back.TemplateBlocks[0].Block.GetDisplayCondition())

	invalid := settings(invalidDisplayCondition)
	err = invalid.Validate("passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template block at index 0")
}
