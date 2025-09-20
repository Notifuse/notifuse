package notifuse_mjml

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMJMLComponentTypeConstants(t *testing.T) {
	tests := []struct {
		constant MJMLComponentType
		expected string
	}{
		{MJMLComponentMjml, "mjml"},
		{MJMLComponentMjBody, "mj-body"},
		{MJMLComponentMjWrapper, "mj-wrapper"},
		{MJMLComponentMjSection, "mj-section"},
		{MJMLComponentMjColumn, "mj-column"},
		{MJMLComponentMjGroup, "mj-group"},
		{MJMLComponentMjText, "mj-text"},
		{MJMLComponentMjButton, "mj-button"},
		{MJMLComponentMjImage, "mj-image"},
		{MJMLComponentMjDivider, "mj-divider"},
		{MJMLComponentMjSpacer, "mj-spacer"},
		{MJMLComponentMjSocial, "mj-social"},
		{MJMLComponentMjSocialElement, "mj-social-element"},
		{MJMLComponentMjHead, "mj-head"},
		{MJMLComponentMjAttributes, "mj-attributes"},
		{MJMLComponentMjBreakpoint, "mj-breakpoint"},
		{MJMLComponentMjFont, "mj-font"},
		{MJMLComponentMjHtmlAttributes, "mj-html-attributes"},
		{MJMLComponentMjPreview, "mj-preview"},
		{MJMLComponentMjStyle, "mj-style"},
		{MJMLComponentMjTitle, "mj-title"},
		{MJMLComponentMjRaw, "mj-raw"},
	}

	for _, test := range tests {
		if string(test.constant) != test.expected {
			t.Errorf("Expected %s to equal %s", string(test.constant), test.expected)
		}
	}
}

func TestBaseBlockInterface(t *testing.T) {
	// Create a test BaseBlock
	baseBlock := BaseBlock{
		ID:   "test-id",
		Type: MJMLComponentMjText,
		Children: []interface{}{
			&MJTextBlock{
				BaseBlock: BaseBlock{ID: "child-1", Type: MJMLComponentMjText},
			},
		},
		Attributes: map[string]interface{}{
			"fontSize": "16px",
			"color":    "#333",
		},
	}

	// Test GetID
	if baseBlock.GetID() != "test-id" {
		t.Errorf("Expected GetID() to return 'test-id', got %s", baseBlock.GetID())
	}

	// Test GetType
	if baseBlock.GetType() != MJMLComponentMjText {
		t.Errorf("Expected GetType() to return MJMLComponentMjText, got %s", baseBlock.GetType())
	}

	// Test GetAttributes
	attrs := baseBlock.GetAttributes()
	if attrs["fontSize"] != "16px" {
		t.Errorf("Expected fontSize to be '16px', got %v", attrs["fontSize"])
	}
	if attrs["color"] != "#333" {
		t.Errorf("Expected color to be '#333', got %v", attrs["color"])
	}

	// Test GetChildren
	children := baseBlock.GetChildren()
	if len(children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(children))
	}
	if children[0] != nil && children[0].GetID() != "child-1" {
		t.Errorf("Expected child ID to be 'child-1', got %s", children[0].GetID())
	}
}

func TestCanDropCheck(t *testing.T) {
	tests := []struct {
		dragType MJMLComponentType
		dropType MJMLComponentType
		expected bool
		desc     string
	}{
		{MJMLComponentMjText, MJMLComponentMjColumn, true, "text can be dropped in column"},
		{MJMLComponentMjButton, MJMLComponentMjColumn, true, "button can be dropped in column"},
		{MJMLComponentMjColumn, MJMLComponentMjSection, true, "column can be dropped in section"},
		{MJMLComponentMjSection, MJMLComponentMjBody, true, "section can be dropped in body"},
		{MJMLComponentMjHead, MJMLComponentMjml, true, "head can be dropped in mjml"},
		{MJMLComponentMjBody, MJMLComponentMjml, true, "body can be dropped in mjml"},
		{MJMLComponentMjText, MJMLComponentMjText, false, "text cannot be dropped in text (leaf)"},
		{MJMLComponentMjButton, MJMLComponentMjButton, false, "button cannot be dropped in button (leaf)"},
		{MJMLComponentMjSection, MJMLComponentMjColumn, false, "section cannot be dropped in column"},
		{MJMLComponentMjBody, MJMLComponentMjSection, false, "body cannot be dropped in section"},
	}

	for _, test := range tests {
		result := CanDropCheck(test.dragType, test.dropType)
		if result != test.expected {
			t.Errorf("%s: expected %v, got %v", test.desc, test.expected, result)
		}
	}
}

func TestIsLeafComponent(t *testing.T) {
	tests := []struct {
		componentType MJMLComponentType
		expected      bool
		desc          string
	}{
		{MJMLComponentMjText, true, "text is a leaf component"},
		{MJMLComponentMjButton, true, "button is a leaf component"},
		{MJMLComponentMjImage, true, "image is a leaf component"},
		{MJMLComponentMjDivider, true, "divider is a leaf component"},
		{MJMLComponentMjSpacer, true, "spacer is a leaf component"},
		{MJMLComponentMjSocialElement, true, "social element is a leaf component"},
		{MJMLComponentMjSection, false, "section is not a leaf component"},
		{MJMLComponentMjColumn, false, "column is not a leaf component"},
		{MJMLComponentMjBody, false, "body is not a leaf component"},
		{MJMLComponentMjSocial, false, "social is not a leaf component"},
	}

	for _, test := range tests {
		result := IsLeafComponent(test.componentType)
		if result != test.expected {
			t.Errorf("%s: expected %v, got %v", test.desc, test.expected, result)
		}
	}
}

func TestGetComponentDisplayName(t *testing.T) {
	tests := []struct {
		componentType MJMLComponentType
		expected      string
	}{
		{MJMLComponentMjml, "MJML Document"},
		{MJMLComponentMjBody, "Body"},
		{MJMLComponentMjSection, "Section"},
		{MJMLComponentMjColumn, "Column"},
		{MJMLComponentMjText, "Text"},
		{MJMLComponentMjButton, "Button"},
		{MJMLComponentMjImage, "Image"},
		{MJMLComponentMjDivider, "Divider"},
		{MJMLComponentMjSpacer, "Spacer"},
		{MJMLComponentMjSocial, "Social"},
		{MJMLComponentMjSocialElement, "Social Element"},
		{MJMLComponentMjHead, "Head"},
		{MJMLComponentMjRaw, "Raw HTML"},
	}

	for _, test := range tests {
		result := GetComponentDisplayName(test.componentType)
		if result != test.expected {
			t.Errorf("GetComponentDisplayName(%s) = %s, expected %s", test.componentType, result, test.expected)
		}
	}

	// Test default case with a custom component
	customType := MJMLComponentType("mj-custom-component")
	result := GetComponentDisplayName(customType)
	expected := "Mj Custom Component"
	if result != expected {
		t.Errorf("GetComponentDisplayName(%s) = %s, expected %s", customType, result, expected)
	}
}

func TestGetComponentCategory(t *testing.T) {
	tests := []struct {
		componentType MJMLComponentType
		expected      string
	}{
		{MJMLComponentMjml, "Document"},
		{MJMLComponentMjBody, "Document"},
		{MJMLComponentMjHead, "Document"},
		{MJMLComponentMjWrapper, "Layout"},
		{MJMLComponentMjSection, "Layout"},
		{MJMLComponentMjColumn, "Layout"},
		{MJMLComponentMjGroup, "Layout"},
		{MJMLComponentMjText, "Content"},
		{MJMLComponentMjButton, "Content"},
		{MJMLComponentMjImage, "Content"},
		{MJMLComponentMjDivider, "Spacing"},
		{MJMLComponentMjSpacer, "Spacing"},
		{MJMLComponentMjSocial, "Social"},
		{MJMLComponentMjSocialElement, "Social"},
		{MJMLComponentMjAttributes, "Head"},
		{MJMLComponentMjBreakpoint, "Head"},
		{MJMLComponentMjFont, "Head"},
		{MJMLComponentMjRaw, "Raw"},
	}

	for _, test := range tests {
		result := GetComponentCategory(test.componentType)
		if result != test.expected {
			t.Errorf("GetComponentCategory(%s) = %s, expected %s", test.componentType, result, test.expected)
		}
	}

	// Test default case
	customType := MJMLComponentType("mj-unknown")
	result := GetComponentCategory(customType)
	if result != "Other" {
		t.Errorf("GetComponentCategory(%s) = %s, expected 'Other'", customType, result)
	}
}

func TestIsContentComponent(t *testing.T) {
	contentComponents := []MJMLComponentType{
		MJMLComponentMjText,
		MJMLComponentMjButton,
		MJMLComponentMjImage,
		MJMLComponentMjDivider,
		MJMLComponentMjSpacer,
		MJMLComponentMjSocial,
		MJMLComponentMjSocialElement,
		MJMLComponentMjRaw,
	}

	nonContentComponents := []MJMLComponentType{
		MJMLComponentMjml,
		MJMLComponentMjBody,
		MJMLComponentMjSection,
		MJMLComponentMjColumn,
		MJMLComponentMjHead,
		MJMLComponentMjWrapper,
	}

	for _, comp := range contentComponents {
		if !IsContentComponent(comp) {
			t.Errorf("Expected %s to be a content component", comp)
		}
	}

	for _, comp := range nonContentComponents {
		if IsContentComponent(comp) {
			t.Errorf("Expected %s to NOT be a content component", comp)
		}
	}
}

func TestIsLayoutComponent(t *testing.T) {
	layoutComponents := []MJMLComponentType{
		MJMLComponentMjWrapper,
		MJMLComponentMjSection,
		MJMLComponentMjColumn,
		MJMLComponentMjGroup,
	}

	nonLayoutComponents := []MJMLComponentType{
		MJMLComponentMjml,
		MJMLComponentMjBody,
		MJMLComponentMjText,
		MJMLComponentMjButton,
		MJMLComponentMjHead,
	}

	for _, comp := range layoutComponents {
		if !IsLayoutComponent(comp) {
			t.Errorf("Expected %s to be a layout component", comp)
		}
	}

	for _, comp := range nonLayoutComponents {
		if IsLayoutComponent(comp) {
			t.Errorf("Expected %s to NOT be a layout component", comp)
		}
	}
}

func TestIsHeadComponent(t *testing.T) {
	headComponents := []MJMLComponentType{
		MJMLComponentMjAttributes,
		MJMLComponentMjBreakpoint,
		MJMLComponentMjFont,
		MJMLComponentMjHtmlAttributes,
		MJMLComponentMjPreview,
		MJMLComponentMjStyle,
		MJMLComponentMjTitle,
	}

	nonHeadComponents := []MJMLComponentType{
		MJMLComponentMjml,
		MJMLComponentMjBody,
		MJMLComponentMjText,
		MJMLComponentMjButton,
		MJMLComponentMjSection,
	}

	for _, comp := range headComponents {
		if !IsHeadComponent(comp) {
			t.Errorf("Expected %s to be a head component", comp)
		}
	}

	for _, comp := range nonHeadComponents {
		if IsHeadComponent(comp) {
			t.Errorf("Expected %s to NOT be a head component", comp)
		}
	}
}

func TestGetDefaultAttributes(t *testing.T) {
	tests := []struct {
		componentType   MJMLComponentType
		expectedAttr    string
		expectedValue   string
		shouldHaveAttrs bool
	}{
		{MJMLComponentMjText, "fontSize", "14px", true},
		{MJMLComponentMjText, "lineHeight", "1.5", true},
		{MJMLComponentMjText, "color", "#000000", true},
		{MJMLComponentMjButton, "backgroundColor", "#414141", true},
		{MJMLComponentMjButton, "color", "#ffffff", true},
		{MJMLComponentMjButton, "fontSize", "13px", true},
		{MJMLComponentMjImage, "align", "center", true},
		{MJMLComponentMjImage, "fluidOnMobile", "true", true},
		{MJMLComponentMjDivider, "borderColor", "#000000", true},
		{MJMLComponentMjDivider, "borderStyle", "solid", true},
		{MJMLComponentMjSpacer, "height", "20px", true},
		// {MJMLComponentMjSection, "padding", "20px 0", true},
		{MJMLComponentMjSection, "paddingTop", "20px", true},
		{MJMLComponentMjSection, "paddingRight", "0px", true},
		{MJMLComponentMjSection, "paddingBottom", "20px", true},
		{MJMLComponentMjSection, "paddingLeft", "0px", true},
		// {MJMLComponentMjColumn, "padding", "0", true},
		{MJMLComponentMjColumn, "paddingTop", "0px", true},
		{MJMLComponentMjColumn, "paddingRight", "0px", true},
		{MJMLComponentMjColumn, "paddingBottom", "0px", true},
		{MJMLComponentMjColumn, "paddingLeft", "0px", true},
		{MJMLComponentMjWrapper, "", "", false}, // No defaults for wrapper
	}

	for _, test := range tests {
		attrs := GetDefaultAttributes(test.componentType)

		if test.shouldHaveAttrs {
			if attrs[test.expectedAttr] != test.expectedValue {
				t.Errorf("GetDefaultAttributes(%s)[%s] = %v, expected %s",
					test.componentType, test.expectedAttr, attrs[test.expectedAttr], test.expectedValue)
			}
		} else {
			if len(attrs) > 0 {
				t.Errorf("GetDefaultAttributes(%s) should return empty map, got %v",
					test.componentType, attrs)
			}
		}
	}
}

func TestValidateComponentHierarchy(t *testing.T) {
	// Test valid hierarchy
	validEmail := &MJMLBlock{
		BaseBlock: BaseBlock{
			ID:   "mjml-1",
			Type: MJMLComponentMjml,
			Children: []interface{}{
				&MJBodyBlock{
					BaseBlock: BaseBlock{
						ID:   "body-1",
						Type: MJMLComponentMjBody,
						Children: []interface{}{
							&MJSectionBlock{
								BaseBlock: BaseBlock{
									ID:   "section-1",
									Type: MJMLComponentMjSection,
									Children: []interface{}{
										&MJColumnBlock{
											BaseBlock: BaseBlock{
												ID:   "column-1",
												Type: MJMLComponentMjColumn,
												Children: []interface{}{
													&MJTextBlock{
														BaseBlock: BaseBlock{
															ID:       "text-1",
															Type:     MJMLComponentMjText,
															Children: []interface{}{},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	err := ValidateComponentHierarchy(validEmail)
	if err != nil {
		t.Errorf("Valid hierarchy should not return error, got: %v", err)
	}

	// Test invalid hierarchy - text with children
	invalidEmail := &MJTextBlock{
		BaseBlock: BaseBlock{
			ID:   "text-1",
			Type: MJMLComponentMjText,
			Children: []interface{}{
				&MJTextBlock{
					BaseBlock: BaseBlock{ID: "child-text", Type: MJMLComponentMjText},
				},
			},
		},
	}

	err = ValidateComponentHierarchy(invalidEmail)
	if err == nil {
		t.Error("Invalid hierarchy (text with children) should return error")
	}
	if !strings.Contains(err.Error(), "cannot have children") {
		t.Errorf("Error should mention 'cannot have children', got: %v", err)
	}

	// Test invalid parent-child relationship
	invalidParentChild := &MJSectionBlock{
		BaseBlock: BaseBlock{
			ID:   "section-1",
			Type: MJMLComponentMjSection,
			Children: []interface{}{
				&MJTextBlock{ // Text cannot be direct child of section
					BaseBlock: BaseBlock{ID: "text-1", Type: MJMLComponentMjText},
				},
			},
		},
	}

	err = ValidateComponentHierarchy(invalidParentChild)
	if err == nil {
		t.Error("Invalid parent-child relationship should return error")
	}
	if !strings.Contains(err.Error(), "cannot be a child of") {
		t.Errorf("Error should mention 'cannot be a child of', got: %v", err)
	}
}

func TestValidateEmailStructure(t *testing.T) {
	// Test valid email structure
	validEmail := &MJMLBlock{
		BaseBlock: BaseBlock{
			ID:   "mjml-1",
			Type: MJMLComponentMjml,
			Children: []interface{}{
				&MJHeadBlock{
					BaseBlock: BaseBlock{ID: "head-1", Type: MJMLComponentMjHead, Children: []interface{}{}},
				},
				&MJBodyBlock{
					BaseBlock: BaseBlock{ID: "body-1", Type: MJMLComponentMjBody, Children: []interface{}{}},
				},
			},
		},
	}

	err := ValidateEmailStructure(validEmail)
	if err != nil {
		t.Errorf("Valid email structure should not return error, got: %v", err)
	}

	// Test invalid root type
	invalidRoot := &MJBodyBlock{
		BaseBlock: BaseBlock{ID: "body-1", Type: MJMLComponentMjBody},
	}

	err = ValidateEmailStructure(invalidRoot)
	if err == nil {
		t.Error("Invalid root type should return error")
	}
	if !strings.Contains(err.Error(), "root component must be mjml") {
		t.Errorf("Error should mention root component, got: %v", err)
	}

	// Test empty mjml
	emptyMjml := &MJMLBlock{
		BaseBlock: BaseBlock{
			ID:       "mjml-1",
			Type:     MJMLComponentMjml,
			Children: []interface{}{},
		},
	}

	err = ValidateEmailStructure(emptyMjml)
	if err == nil {
		t.Error("Empty MJML should return error")
	}
	if !strings.Contains(err.Error(), "mjml document must have children") {
		t.Errorf("Error should mention missing children, got: %v", err)
	}

	// Test mjml without body
	mjmlWithoutBody := &MJMLBlock{
		BaseBlock: BaseBlock{
			ID:   "mjml-1",
			Type: MJMLComponentMjml,
			Children: []interface{}{
				&MJHeadBlock{
					BaseBlock: BaseBlock{ID: "head-1", Type: MJMLComponentMjHead},
				},
			},
		},
	}

	err = ValidateEmailStructure(mjmlWithoutBody)
	if err == nil {
		t.Error("MJML without body should return error")
	}
	if !strings.Contains(err.Error(), "mjml document must contain an mj-body") {
		t.Errorf("Error should mention missing body, got: %v", err)
	}

	// Test mjml with invalid child
	mjmlWithInvalidChild := &MJMLBlock{
		BaseBlock: BaseBlock{
			ID:   "mjml-1",
			Type: MJMLComponentMjml,
			Children: []interface{}{
				&MJTextBlock{ // Text cannot be direct child of mjml
					BaseBlock: BaseBlock{ID: "text-1", Type: MJMLComponentMjText},
				},
				&MJBodyBlock{
					BaseBlock: BaseBlock{ID: "body-1", Type: MJMLComponentMjBody},
				},
			},
		},
	}

	err = ValidateEmailStructure(mjmlWithInvalidChild)
	if err == nil {
		t.Error("MJML with invalid child should return error")
	}
	if !strings.Contains(err.Error(), "mjml can only contain mj-head and mj-body") {
		t.Errorf("Error should mention valid children, got: %v", err)
	}
}

func TestValidChildrenMap(t *testing.T) {
	// Test that all component types are covered in ValidChildrenMap
	allComponents := []MJMLComponentType{
		MJMLComponentMjml,
		MJMLComponentMjBody,
		MJMLComponentMjWrapper,
		MJMLComponentMjSection,
		MJMLComponentMjColumn,
		MJMLComponentMjGroup,
		MJMLComponentMjText,
		MJMLComponentMjButton,
		MJMLComponentMjImage,
		MJMLComponentMjDivider,
		MJMLComponentMjSpacer,
		MJMLComponentMjSocial,
		MJMLComponentMjSocialElement,
		MJMLComponentMjHead,
		MJMLComponentMjAttributes,
		MJMLComponentMjBreakpoint,
		MJMLComponentMjFont,
		MJMLComponentMjHtmlAttributes,
		MJMLComponentMjPreview,
		MJMLComponentMjStyle,
		MJMLComponentMjTitle,
		MJMLComponentMjRaw,
	}

	for _, comp := range allComponents {
		if _, exists := ValidChildrenMap[comp]; !exists {
			t.Errorf("Component %s is missing from ValidChildrenMap", comp)
		}
	}

	// Test specific relationships
	mjmlChildren := ValidChildrenMap[MJMLComponentMjml]
	expectedMjmlChildren := []MJMLComponentType{MJMLComponentMjHead, MJMLComponentMjBody}
	if len(mjmlChildren) != len(expectedMjmlChildren) {
		t.Errorf("MJML should have %d children, got %d", len(expectedMjmlChildren), len(mjmlChildren))
	}

	// Test that leaf components have empty children lists
	leafComponents := []MJMLComponentType{
		MJMLComponentMjText,
		MJMLComponentMjButton,
		MJMLComponentMjImage,
		MJMLComponentMjDivider,
		MJMLComponentMjSpacer,
		MJMLComponentMjSocialElement,
		MJMLComponentMjRaw,
	}

	for _, leaf := range leafComponents {
		children := ValidChildrenMap[leaf]
		if len(children) != 0 {
			t.Errorf("Leaf component %s should have no children, got %v", leaf, children)
		}
	}
}

func TestFormFieldAndSavedBlock(t *testing.T) {
	// Test FormField
	field := FormField{
		Key:         "fontSize",
		Label:       "Font Size",
		Type:        "text",
		Placeholder: stringPtr("14px"),
		Description: stringPtr("The size of the font"),
		Options: []FormFieldOption{
			{Value: "12px", Label: "Small"},
			{Value: "14px", Label: "Medium"},
			{Value: "16px", Label: "Large"},
		},
	}

	if field.Key != "fontSize" {
		t.Errorf("Expected Key to be 'fontSize', got %s", field.Key)
	}
	if len(field.Options) != 3 {
		t.Errorf("Expected 3 options, got %d", len(field.Options))
	}

	// Test SavedBlock
	now := time.Now()
	textBlock := &MJTextBlock{
		BaseBlock: BaseBlock{ID: "text-1", Type: MJMLComponentMjText},
	}

	savedBlock := SavedBlock{
		ID:      "saved-1",
		Name:    "My Text Block",
		Block:   textBlock,
		Created: &now,
		Updated: &now,
	}

	if savedBlock.Name != "My Text Block" {
		t.Errorf("Expected Name to be 'My Text Block', got %s", savedBlock.Name)
	}
	if savedBlock.Block.GetID() != "text-1" {
		t.Errorf("Expected Block ID to be 'text-1', got %s", savedBlock.Block.GetID())
	}
}

func TestSaveOperation(t *testing.T) {
	if SaveOperationCreate != "create" {
		t.Errorf("Expected SaveOperationCreate to be 'create', got %s", SaveOperationCreate)
	}
	if SaveOperationUpdate != "update" {
		t.Errorf("Expected SaveOperationUpdate to be 'update', got %s", SaveOperationUpdate)
	}
}

func TestEmailBlockJSONMarshaling(t *testing.T) {
	// Create a test block with children
	textBlock := &MJTextBlock{
		BaseBlock: BaseBlock{
			ID:   "text1",
			Type: MJMLComponentMjText,
		},
		Type:    MJMLComponentMjText,
		Content: stringPtr("Hello World"),
	}

	bodyBlock := &MJBodyBlock{
		BaseBlock: BaseBlock{
			ID:       "body1",
			Type:     MJMLComponentMjBody,
			Children: []interface{}{textBlock},
		},
		Type:     MJMLComponentMjBody,
		Children: []EmailBlock{textBlock}, // Set in concrete type too
	}

	block := &MJMLBlock{
		BaseBlock: BaseBlock{
			ID:         "test",
			Type:       MJMLComponentMjml,
			Attributes: map[string]interface{}{"version": "4.0.0"},
			Children:   []interface{}{bodyBlock},
		},
		Type:       MJMLComponentMjml,
		Attributes: map[string]interface{}{"version": "4.0.0"},
		Children:   []EmailBlock{bodyBlock}, // Set in concrete type too
	}

	// Marshal it
	data, err := json.Marshal(block)
	require.NoError(t, err)
	t.Logf("Marshaled JSON: %s", string(data))

	// Unmarshal it using our custom function
	unmarshaled, err := UnmarshalEmailBlock(data)
	require.NoError(t, err)

	assert.Equal(t, "test", unmarshaled.GetID())
	assert.Equal(t, MJMLComponentMjml, unmarshaled.GetType())
	if unmarshaled.GetAttributes() != nil {
		assert.Equal(t, "4.0.0", unmarshaled.GetAttributes()["version"])
	}

	// Verify it's the correct concrete type
	mjmlBlock, ok := unmarshaled.(*MJMLBlock)
	require.True(t, ok, "Expected MJMLBlock but got %T", unmarshaled)
	assert.Equal(t, MJMLComponentMjml, mjmlBlock.Type)

	// Check children
	children := unmarshaled.GetChildren()
	require.Len(t, children, 1, "Expected 1 child")

	bodyChild, ok := children[0].(*MJBodyBlock)
	require.True(t, ok, "Expected MJBodyBlock child but got %T", children[0])
	assert.Equal(t, "body1", bodyChild.GetID())
	assert.Equal(t, MJMLComponentMjBody, bodyChild.GetType())

	// Check grandchildren
	bodyChildren := bodyChild.GetChildren()
	require.Len(t, bodyChildren, 1, "Expected 1 grandchild")

	textChild, ok := bodyChildren[0].(*MJTextBlock)
	require.True(t, ok, "Expected MJTextBlock grandchild but got %T", bodyChildren[0])
	assert.Equal(t, "text1", textChild.GetID())
	assert.Equal(t, MJMLComponentMjText, textChild.GetType())
	assert.Equal(t, "Hello World", *textChild.Content)
}

func TestAllComponentTypesUnmarshal(t *testing.T) {
	// Test that all defined component types can be unmarshaled without errors
	allComponentTypes := []MJMLComponentType{
		MJMLComponentMjml,
		MJMLComponentMjBody,
		MJMLComponentMjWrapper,
		MJMLComponentMjSection,
		MJMLComponentMjColumn,
		MJMLComponentMjGroup,
		MJMLComponentMjText,
		MJMLComponentMjButton,
		MJMLComponentMjImage,
		MJMLComponentMjDivider,
		MJMLComponentMjSpacer,
		MJMLComponentMjSocial,
		MJMLComponentMjSocialElement,
		MJMLComponentMjHead,
		MJMLComponentMjAttributes,
		MJMLComponentMjBreakpoint,
		MJMLComponentMjFont,
		MJMLComponentMjHtmlAttributes,
		MJMLComponentMjPreview,
		MJMLComponentMjStyle,
		MJMLComponentMjTitle,
		MJMLComponentMjRaw,
	}

	for _, componentType := range allComponentTypes {
		t.Run(string(componentType), func(t *testing.T) {
			// Create a basic JSON structure for each component type
			jsonData := map[string]interface{}{
				"id":         "test-" + string(componentType),
				"type":       componentType,
				"attributes": map[string]interface{}{"testAttr": "testValue"},
			}

			// Add content field to components that support it
			contentComponents := []MJMLComponentType{
				MJMLComponentMjText, MJMLComponentMjButton, MJMLComponentMjPreview,
				MJMLComponentMjStyle, MJMLComponentMjTitle, MJMLComponentMjRaw,
				MJMLComponentMjSocialElement,
			}
			for _, contentComp := range contentComponents {
				if componentType == contentComp {
					jsonData["content"] = "Test content for " + string(componentType)
					break
				}
			}

			// Marshal to JSON
			jsonBytes, err := json.Marshal(jsonData)
			if err != nil {
				t.Fatalf("Failed to marshal JSON for %s: %v", componentType, err)
			}

			// Unmarshal using our function
			block, err := UnmarshalEmailBlock(jsonBytes)
			if err != nil {
				t.Fatalf("Failed to unmarshal %s: %v", componentType, err)
			}

			// Verify the block was created correctly
			if block == nil {
				t.Fatalf("UnmarshalEmailBlock returned nil for %s", componentType)
			}

			if block.GetType() != componentType {
				t.Errorf("Expected type %s, got %s", componentType, block.GetType())
			}

			if block.GetID() != "test-"+string(componentType) {
				t.Errorf("Expected ID 'test-%s', got '%s'", componentType, block.GetID())
			}

			// Verify attributes
			attrs := block.GetAttributes()
			if attrs == nil {
				t.Errorf("Attributes should not be nil for %s", componentType)
			} else if testAttr, exists := attrs["testAttr"]; !exists || testAttr != "testValue" {
				t.Errorf("Expected testAttr=testValue for %s, got %v", componentType, testAttr)
			}

			t.Logf("Successfully unmarshaled %s component", componentType)
		})
	}
}

func TestUnmarshalEmailBlockWithChildren(t *testing.T) {
	// Test unmarshaling of complex nested structures with all component types
	complexJSON := `{
		"id": "root",
		"type": "mjml",
		"children": [
			{
				"id": "head",
				"type": "mj-head",
				"children": [
					{
						"id": "title",
						"type": "mj-title",
						"content": "Test Email"
					},
					{
						"id": "breakpoint",
						"type": "mj-breakpoint",
						"attributes": {"width": "600px"}
					},
					{
						"id": "font",
						"type": "mj-font",
						"attributes": {"name": "Arial", "href": "https://fonts.google.com/arial"}
					}
				]
			},
			{
				"id": "body",
				"type": "mj-body",
				"children": [
					{
						"id": "wrapper",
						"type": "mj-wrapper",
						"children": [
							{
								"id": "section",
								"type": "mj-section",
								"children": [
									{
										"id": "group",
										"type": "mj-group",
										"children": [
											{
												"id": "column",
												"type": "mj-column",
												"children": [
													{
														"id": "text",
														"type": "mj-text",
														"content": "Hello World"
													},
													{
														"id": "button",
														"type": "mj-button",
														"content": "Click Me"
													},
													{
														"id": "image",
														"type": "mj-image",
														"attributes": {"src": "https://example.com/image.jpg"}
													},
													{
														"id": "divider",
														"type": "mj-divider"
													},
													{
														"id": "spacer",
														"type": "mj-spacer",
														"attributes": {"height": "20px"}
													},
													{
														"id": "social",
														"type": "mj-social",
														"children": [
															{
																"id": "social-element",
																"type": "mj-social-element",
																"content": "Follow Us",
																"attributes": {"name": "facebook"}
															}
														]
													},
													{
														"id": "raw",
														"type": "mj-raw",
														"content": "<p>Raw HTML content</p>"
													}
												]
											}
										]
									}
								]
							}
						]
					}
				]
			}
		]
	}`

	block, err := UnmarshalEmailBlock([]byte(complexJSON))
	if err != nil {
		t.Fatalf("Failed to unmarshal complex JSON: %v", err)
	}

	// Verify root block
	if block.GetType() != MJMLComponentMjml {
		t.Errorf("Expected root type mjml, got %s", block.GetType())
	}

	// Verify children exist
	children := block.GetChildren()
	if len(children) != 2 {
		t.Errorf("Expected 2 root children, got %d", len(children))
	}

	// Verify head block
	if len(children) > 0 && children[0].GetType() != MJMLComponentMjHead {
		t.Errorf("Expected first child to be mj-head, got %s", children[0].GetType())
	}

	// Verify body block
	if len(children) > 1 && children[1].GetType() != MJMLComponentMjBody {
		t.Errorf("Expected second child to be mj-body, got %s", children[1].GetType())
	}

	t.Log("Successfully unmarshaled complex nested structure with all component types")
}

// Helper function for tests - using stringPtr from examples.go

func TestUnmarshalEmailBlockWithoutAttributes(t *testing.T) {
	// Test the specific case shown by the user - blocks without attributes field
	testJSON := `{
		"id": "e2f8ab42-c479-4561-8016-9eb72de7931e",
		"type": "mj-column",
		"children": [
			{
				"id": "7148af9b-7906-40d7-807e-8a111ca22be8",
				"type": "mj-spacer"
			}
		]
	}`

	block, err := UnmarshalEmailBlock([]byte(testJSON))
	require.NoError(t, err, "Failed to unmarshal block without attributes")
	require.NotNil(t, block, "Block should not be nil")

	// Verify the column block
	assert.Equal(t, "e2f8ab42-c479-4561-8016-9eb72de7931e", block.GetID())
	assert.Equal(t, MJMLComponentMjColumn, block.GetType())

	// Verify that default attributes are applied
	attrs := block.GetAttributes()
	assert.NotNil(t, attrs, "Attributes should not be nil even when not provided")
	assert.NotEmpty(t, attrs, "Attributes should have default values")

	// Verify the spacer child
	children := block.GetChildren()
	require.Len(t, children, 1, "Should have exactly one child")

	spacerChild := children[0]
	assert.Equal(t, "7148af9b-7906-40d7-807e-8a111ca22be8", spacerChild.GetID())
	assert.Equal(t, MJMLComponentMjSpacer, spacerChild.GetType())

	// Verify that the spacer also has default attributes
	spacerAttrs := spacerChild.GetAttributes()
	assert.NotNil(t, spacerAttrs, "Spacer attributes should not be nil")
	assert.NotEmpty(t, spacerAttrs, "Spacer should have default attributes")

	// Spacer should have a default height
	if height, exists := spacerAttrs["height"]; exists {
		assert.Equal(t, "20px", height, "Spacer should have default height of 20px")
	}

	// Verify typed attributes are properly set
	if columnBlock, ok := block.(*MJColumnBlock); ok {
		assert.NotNil(t, columnBlock.Attributes, "Typed attributes should be set")
	} else {
		t.Error("Block should be of type *MJColumnBlock")
	}

	if spacerBlock, ok := spacerChild.(*MJSpacerBlock); ok {
		assert.NotNil(t, spacerBlock.Attributes, "Spacer typed attributes should be set")
	} else {
		t.Error("Child should be of type *MJSpacerBlock")
	}
}

func TestUnmarshalEmailBlockWithPartialAttributes(t *testing.T) {
	// Test blocks with some attributes present
	testJSON := `{
		"id": "test-text-block",
		"type": "mj-text",
		"content": "Hello World",
		"attributes": {
			"color": "#ff0000",
			"fontSize": "18px"
		}
	}`

	block, err := UnmarshalEmailBlock([]byte(testJSON))
	require.NoError(t, err, "Failed to unmarshal block with partial attributes")
	require.NotNil(t, block, "Block should not be nil")

	// Verify the text block
	assert.Equal(t, "test-text-block", block.GetID())
	assert.Equal(t, MJMLComponentMjText, block.GetType())

	// Verify that provided attributes are preserved
	attrs := block.GetAttributes()
	assert.Equal(t, "#ff0000", attrs["color"])
	assert.Equal(t, "18px", attrs["fontSize"])

	// Verify that default attributes are still applied for missing ones
	assert.Equal(t, "1.5", attrs["lineHeight"]) // Default from GetDefaultAttributes

	// Verify content is preserved
	if textBlock, ok := block.(*MJTextBlock); ok {
		assert.NotNil(t, textBlock.Content)
		assert.Equal(t, "Hello World", *textBlock.Content)
		assert.NotNil(t, textBlock.Attributes, "Typed attributes should be set")
	} else {
		t.Error("Block should be of type *MJTextBlock")
	}
}

func TestUnmarshalEmailBlockWithEmptyAttributes(t *testing.T) {
	// Test blocks with empty attributes object
	testJSON := `{
		"id": "test-button-block",
		"type": "mj-button",
		"content": "Click Me",
		"attributes": {}
	}`

	block, err := UnmarshalEmailBlock([]byte(testJSON))
	require.NoError(t, err, "Failed to unmarshal block with empty attributes")
	require.NotNil(t, block, "Block should not be nil")

	// Verify the button block
	assert.Equal(t, "test-button-block", block.GetID())
	assert.Equal(t, MJMLComponentMjButton, block.GetType())

	// Verify that default attributes are applied
	attrs := block.GetAttributes()
	assert.NotEmpty(t, attrs, "Should have default attributes")
	assert.Equal(t, "#414141", attrs["backgroundColor"]) // Default background color
	assert.Equal(t, "#ffffff", attrs["color"])           // Default text color

	// Verify content is preserved
	if buttonBlock, ok := block.(*MJButtonBlock); ok {
		assert.NotNil(t, buttonBlock.Content)
		assert.Equal(t, "Click Me", *buttonBlock.Content)
		assert.NotNil(t, buttonBlock.Attributes, "Typed attributes should be set")
	} else {
		t.Error("Block should be of type *MJButtonBlock")
	}
}
