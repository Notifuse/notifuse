package notifuse_mjml

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MJMLComponentType represents the available MJML component types
type MJMLComponentType string

const (
	MJMLComponentMjml             MJMLComponentType = "mjml"
	MJMLComponentMjBody           MJMLComponentType = "mj-body"
	MJMLComponentMjWrapper        MJMLComponentType = "mj-wrapper"
	MJMLComponentMjSection        MJMLComponentType = "mj-section"
	MJMLComponentMjColumn         MJMLComponentType = "mj-column"
	MJMLComponentMjGroup          MJMLComponentType = "mj-group"
	MJMLComponentMjText           MJMLComponentType = "mj-text"
	MJMLComponentMjButton         MJMLComponentType = "mj-button"
	MJMLComponentMjImage          MJMLComponentType = "mj-image"
	MJMLComponentMjDivider        MJMLComponentType = "mj-divider"
	MJMLComponentMjSpacer         MJMLComponentType = "mj-spacer"
	MJMLComponentMjSocial         MJMLComponentType = "mj-social"
	MJMLComponentMjSocialElement  MJMLComponentType = "mj-social-element"
	MJMLComponentMjHead           MJMLComponentType = "mj-head"
	MJMLComponentMjAttributes     MJMLComponentType = "mj-attributes"
	MJMLComponentMjBreakpoint     MJMLComponentType = "mj-breakpoint"
	MJMLComponentMjFont           MJMLComponentType = "mj-font"
	MJMLComponentMjHtmlAttributes MJMLComponentType = "mj-html-attributes"
	MJMLComponentMjPreview        MJMLComponentType = "mj-preview"
	MJMLComponentMjStyle          MJMLComponentType = "mj-style"
	MJMLComponentMjTitle          MJMLComponentType = "mj-title"
	MJMLComponentMjRaw            MJMLComponentType = "mj-raw"
)

// Common attribute interfaces
type PaddingAttributes struct {
	PaddingBottom *string `json:"paddingBottom,omitempty"`
	PaddingLeft   *string `json:"paddingLeft,omitempty"`
	PaddingRight  *string `json:"paddingRight,omitempty"`
	PaddingTop    *string `json:"paddingTop,omitempty"`
}

type BorderAttributes struct {
	BorderBottom *string `json:"borderBottom,omitempty"`
	BorderLeft   *string `json:"borderLeft,omitempty"`
	BorderRadius *string `json:"borderRadius,omitempty"`
	BorderRight  *string `json:"borderRight,omitempty"`
	BorderTop    *string `json:"borderTop,omitempty"`
}

type BackgroundAttributes struct {
	BackgroundColor     *string `json:"backgroundColor,omitempty"`
	BackgroundURL       *string `json:"backgroundUrl,omitempty"`
	BackgroundRepeat    *string `json:"backgroundRepeat,omitempty"`
	BackgroundSize      *string `json:"backgroundSize,omitempty"`
	BackgroundPosition  *string `json:"backgroundPosition,omitempty"`
	BackgroundPositionX *string `json:"backgroundPositionX,omitempty"`
	BackgroundPositionY *string `json:"backgroundPositionY,omitempty"`
}

type TextAttributes struct {
	Align          *string `json:"align,omitempty"` // left, right, center, justify
	Color          *string `json:"color,omitempty"`
	FontFamily     *string `json:"fontFamily,omitempty"`
	FontSize       *string `json:"fontSize,omitempty"`
	FontStyle      *string `json:"fontStyle,omitempty"`
	FontWeight     *string `json:"fontWeight,omitempty"`
	LetterSpacing  *string `json:"letterSpacing,omitempty"`
	LineHeight     *string `json:"lineHeight,omitempty"`
	TextAlign      *string `json:"textAlign,omitempty"` // left, right, center, justify
	TextDecoration *string `json:"textDecoration,omitempty"`
	TextTransform  *string `json:"textTransform,omitempty"`
}

type LayoutAttributes struct {
	Height        *string `json:"height,omitempty"`
	Width         *string `json:"width,omitempty"`
	VerticalAlign *string `json:"verticalAlign,omitempty"` // top, bottom, middle
}

type CommonAttributes struct {
	CSSClass *string `json:"cssClass,omitempty"`
}

type LinkAttributes struct {
	Href   *string `json:"href,omitempty"`
	Rel    *string `json:"rel,omitempty"`
	Target *string `json:"target,omitempty"` // _blank, _self, _parent, _top
}

type ContainerAttributes struct {
	ContainerBackgroundColor *string `json:"containerBackgroundColor,omitempty"`
}

// Base interface for all MJML blocks
type BaseBlock struct {
	ID         string                 `json:"id"`
	Type       MJMLComponentType      `json:"type"`
	Children   []EmailBlock           `json:"children,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Content    *string                `json:"content,omitempty"`
}

// Specific attribute types for complex blocks
type MJBodyAttributes struct {
	BackgroundAttributes
	CommonAttributes
	Width           *string `json:"width,omitempty"`
	BackgroundColor *string `json:"backgroundColor,omitempty"`
}

type MJWrapperAttributes struct {
	BackgroundAttributes
	BorderAttributes
	PaddingAttributes
	CommonAttributes
	FullWidthBackgroundColor *string `json:"fullWidthBackgroundColor,omitempty"`
	FullWidth                *string `json:"fullWidth,omitempty"` // full-width
	TextAlign                *string `json:"textAlign,omitempty"` // left, right, center, justify
}

type MJSectionAttributes struct {
	BackgroundAttributes
	BorderAttributes
	PaddingAttributes
	CommonAttributes
	Direction *string `json:"direction,omitempty"` // ltr, rtl
	FullWidth *string `json:"fullWidth,omitempty"` // full-width
	TextAlign *string `json:"textAlign,omitempty"` // left, right, center, justify
}

type MJColumnAttributes struct {
	BackgroundAttributes
	BorderAttributes
	PaddingAttributes
	LayoutAttributes
	CommonAttributes
	InnerBackgroundColor *string `json:"innerBackgroundColor,omitempty"`
	InnerBorderTop       *string `json:"innerBorderTop,omitempty"`
	InnerBorderRight     *string `json:"innerBorderRight,omitempty"`
	InnerBorderBottom    *string `json:"innerBorderBottom,omitempty"`
	InnerBorderLeft      *string `json:"innerBorderLeft,omitempty"`
	InnerBorderRadius    *string `json:"innerBorderRadius,omitempty"`
}

type MJGroupAttributes struct {
	BackgroundAttributes
	LayoutAttributes
	CommonAttributes
	Direction *string `json:"direction,omitempty"` // ltr, rtl
}

type MJTextAttributes struct {
	TextAttributes
	PaddingAttributes
	LayoutAttributes
	ContainerAttributes
	CommonAttributes
}

type MJButtonAttributes struct {
	TextAttributes
	BackgroundAttributes
	BorderAttributes
	PaddingAttributes
	LayoutAttributes
	LinkAttributes
	ContainerAttributes
	CommonAttributes
}

type MJImageAttributes struct {
	BorderAttributes
	PaddingAttributes
	LayoutAttributes
	LinkAttributes
	ContainerAttributes
	CommonAttributes
	Align         *string `json:"align,omitempty"` // left, right, center
	Alt           *string `json:"alt,omitempty"`
	FluidOnMobile *string `json:"fluidOnMobile,omitempty"` // true, false
	Name          *string `json:"name,omitempty"`
	Sizes         *string `json:"sizes,omitempty"`
	Src           *string `json:"src,omitempty"`
	Srcset        *string `json:"srcset,omitempty"`
	Title         *string `json:"title,omitempty"`
	Usemap        *string `json:"usemap,omitempty"`
}

type MJDividerAttributes struct {
	BorderAttributes
	PaddingAttributes
	ContainerAttributes
	CommonAttributes
	Align       *string `json:"align,omitempty"` // left, right, center
	BorderColor *string `json:"borderColor,omitempty"`
	BorderStyle *string `json:"borderStyle,omitempty"` // solid, dashed, dotted
	BorderWidth *string `json:"borderWidth,omitempty"`
	Width       *string `json:"width,omitempty"`
}

type MJSpacerAttributes struct {
	PaddingAttributes
	ContainerAttributes
	CommonAttributes
	Height *string `json:"height,omitempty"`
}

type MJSocialAttributes struct {
	PaddingAttributes
	ContainerAttributes
	CommonAttributes
	Align        *string `json:"align,omitempty"` // left, right, center
	BorderRadius *string `json:"borderRadius,omitempty"`
	IconHeight   *string `json:"iconHeight,omitempty"`
	IconSize     *string `json:"iconSize,omitempty"`
	InnerPadding *string `json:"innerPadding,omitempty"`
	LineHeight   *string `json:"lineHeight,omitempty"`
	Mode         *string `json:"mode,omitempty"`        // horizontal, vertical
	TableLayout  *string `json:"tableLayout,omitempty"` // auto, fixed
	TextPadding  *string `json:"textPadding,omitempty"`
}

type MJSocialElementAttributes struct {
	// Layout and positioning
	Align *string `json:"align,omitempty"` // left, center, right

	// Icon properties
	Alt             *string `json:"alt,omitempty"`
	BackgroundColor *string `json:"backgroundColor,omitempty"`
	BorderRadius    *string `json:"borderRadius,omitempty"`
	IconHeight      *string `json:"iconHeight,omitempty"`
	IconSize        *string `json:"iconSize,omitempty"`
	IconPadding     *string `json:"iconPadding,omitempty"`
	IconPosition    *string `json:"iconPosition,omitempty"` // left, right
	Name            *string `json:"name,omitempty"`
	Src             *string `json:"src,omitempty"`
	Sizes           *string `json:"sizes,omitempty"`
	Srcset          *string `json:"srcset,omitempty"`

	// Text properties
	Color          *string `json:"color,omitempty"`
	FontFamily     *string `json:"fontFamily,omitempty"`
	FontSize       *string `json:"fontSize,omitempty"`
	FontStyle      *string `json:"fontStyle,omitempty"`
	FontWeight     *string `json:"fontWeight,omitempty"`
	LineHeight     *string `json:"lineHeight,omitempty"`
	TextDecoration *string `json:"textDecoration,omitempty"`
	TextPadding    *string `json:"textPadding,omitempty"`
	VerticalAlign  *string `json:"verticalAlign,omitempty"`

	// Link properties
	Href   *string `json:"href,omitempty"`
	Rel    *string `json:"rel,omitempty"`
	Target *string `json:"target,omitempty"`
	Title  *string `json:"title,omitempty"`

	// Layout properties
	Padding       *string `json:"padding,omitempty"`
	PaddingTop    *string `json:"paddingTop,omitempty"`
	PaddingRight  *string `json:"paddingRight,omitempty"`
	PaddingBottom *string `json:"paddingBottom,omitempty"`
	PaddingLeft   *string `json:"paddingLeft,omitempty"`

	// Advanced properties
	CSSClass *string `json:"cssClass,omitempty"`
}

type MJRawAttributes struct {
	CommonAttributes
}

type MJBreakpointAttributes struct {
	Width *string `json:"width,omitempty"`
}

type MJFontAttributes struct {
	Name *string `json:"name,omitempty"`
	Href *string `json:"href,omitempty"`
}

type MJStyleAttributes struct {
	Inline *string `json:"inline,omitempty"` // inline
}

// Block interfaces
type MJMLBlock struct {
	*BaseBlock
}

type MJHeadBlock struct {
	*BaseBlock
}

type MJAttributesBlock struct {
	*BaseBlock
}

type MJAttributeElementBlock struct {
	*BaseBlock
}

type MJBreakpointBlock struct {
	*BaseBlock
}

type MJFontBlock struct {
	*BaseBlock
}

type MJHtmlAttributesBlock struct {
	*BaseBlock
}

type MJPreviewBlock struct {
	*BaseBlock
}

type MJStyleBlock struct {
	*BaseBlock
}

type MJTitleBlock struct {
	*BaseBlock
}

type MJBodyBlock struct {
	*BaseBlock
}

type MJWrapperBlock struct {
	*BaseBlock
}

type MJSectionBlock struct {
	*BaseBlock
}

type MJColumnBlock struct {
	*BaseBlock
}

type MJGroupBlock struct {
	*BaseBlock
}

type MJTextBlock struct {
	*BaseBlock
}

type MJButtonBlock struct {
	*BaseBlock
}

type MJImageBlock struct {
	*BaseBlock
}

type MJDividerBlock struct {
	*BaseBlock
}

type MJSpacerBlock struct {
	*BaseBlock
}

type MJSocialBlock struct {
	*BaseBlock
}

type MJSocialElementBlock struct {
	*BaseBlock
}

type MJRawBlock struct {
	*BaseBlock
}

// Email builder state types
type EmailBuilderState struct {
	SelectedBlockID *string      `json:"selectedBlockId,omitempty"`
	History         []EmailBlock `json:"history"`
	HistoryIndex    int          `json:"historyIndex"`
	ViewportMode    *string      `json:"viewportMode,omitempty"` // mobile, desktop
}

// Tree node for UI components
type TreeNode struct {
	Key        string            `json:"key"`
	Disabled   *bool             `json:"disabled,omitempty"`
	Title      string            `json:"title"`
	Children   []TreeNode        `json:"children,omitempty"`
	Icon       interface{}       `json:"icon,omitempty"`
	IsLeaf     *bool             `json:"isLeaf,omitempty"`
	Selectable *bool             `json:"selectable,omitempty"`
	Draggable  *bool             `json:"draggable,omitempty"`
	BlockType  MJMLComponentType `json:"blockType"`
}

// Drag and drop types
type DragInfo struct {
	Node          TreeNode `json:"node"`
	DragNode      TreeNode `json:"dragNode"`
	DragNodesKeys []string `json:"dragNodesKeys"`
	DropPosition  int      `json:"dropPosition"`
	DropToGap     bool     `json:"dropToGap"`
}

// Email builder actions (interface for service methods)
type EmailBuilderActions interface {
	SelectBlock(blockID *string) error
	AddBlock(parentID string, blockType MJMLComponentType, position *int) error
	UpdateBlock(blockID string, updates map[string]interface{}) error
	DeleteBlock(blockID string) error
	MoveBlock(blockID string, newParentID string, position int) error
	Undo() error
	Redo() error
}

// Settings panel configuration
type SettingsConfig map[string][]FormField

// Form field types for the settings panel
type FormField struct {
	Key          string            `json:"key"`
	Label        string            `json:"label"`
	Type         string            `json:"type"` // text, number, color, select, textarea, url, switch
	Options      []FormFieldOption `json:"options,omitempty"`
	Placeholder  *string           `json:"placeholder,omitempty"`
	Description  *string           `json:"description,omitempty"`
	DefaultValue interface{}       `json:"defaultValue,omitempty"`
}

type FormFieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SaveOperation represents the type of save operation
type SaveOperation string

const (
	SaveOperationCreate SaveOperation = "create"
	SaveOperationUpdate SaveOperation = "update"
)

// SavedBlock interface for storing custom blocks
type SavedBlock struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Block   EmailBlock `json:"block"`
	Created *time.Time `json:"created,omitempty"`
	Updated *time.Time `json:"updated,omitempty"`
}

// EmailBlock represents any MJML block type
type EmailBlock interface {
	GetID() string
	SetID(string)
	GetType() MJMLComponentType
	GetChildren() []EmailBlock
	SetChildren([]EmailBlock)
	GetAttributes() map[string]interface{}
	SetAttributes(map[string]interface{})
	GetContent() *string
	SetContent(*string)
}

// Helper methods for EmailBlock interface implementation
func (b *BaseBlock) GetID() string {
	if b == nil {
		return ""
	}
	return b.ID
}

func (b *BaseBlock) SetID(id string) {
	if b != nil {
		b.ID = id
	}
}

func (b *BaseBlock) GetType() MJMLComponentType {
	if b == nil {
		return ""
	}
	return b.Type
}

func (b *BaseBlock) GetChildren() []EmailBlock {
	if b == nil {
		return nil
	}
	return b.Children
}

func (b *BaseBlock) SetChildren(children []EmailBlock) {
	if b != nil {
		b.Children = children
	}
}

func (b *BaseBlock) GetAttributes() map[string]interface{} {
	if b == nil {
		return nil
	}
	return b.Attributes
}

func (b *BaseBlock) SetAttributes(attrs map[string]interface{}) {
	if b != nil {
		b.Attributes = attrs
	}
}

func (b *BaseBlock) GetContent() *string {
	if b == nil {
		return nil
	}
	return b.Content
}

func (b *BaseBlock) SetContent(content *string) {
	if b != nil {
		b.Content = content
	}
}

// EmailBlockJSON is used for JSON marshaling/unmarshaling with type information
type EmailBlockJSON struct {
	ID         string                 `json:"id"`
	Type       MJMLComponentType      `json:"type"`
	Children   []json.RawMessage      `json:"children,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Content    *string                `json:"content,omitempty"`
}

// MarshalJSON implements custom JSON marshaling for EmailBlock interface
func MarshalEmailBlock(block EmailBlock) ([]byte, error) {
	if block == nil {
		return []byte("null"), nil
	}

	return json.Marshal(block)
}

// UnmarshalEmailBlock implements custom JSON unmarshaling for EmailBlock interface
func UnmarshalEmailBlock(data []byte) (EmailBlock, error) {
	var blockJSON EmailBlockJSON
	if err := json.Unmarshal(data, &blockJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal EmailBlock JSON: %w", err)
	}

	// Recursively unmarshal children
	children := make([]EmailBlock, len(blockJSON.Children))
	for i, childData := range blockJSON.Children {
		child, err := UnmarshalEmailBlock(childData)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal child at index %d: %w", i, err)
		}
		children[i] = child
	}

	// Merge with defaults
	mergedAttrs := mergeAttributesWithDefaults(blockJSON.Type, blockJSON.Attributes)

	// Create base block
	base := &BaseBlock{
		ID:         blockJSON.ID,
		Type:       blockJSON.Type,
		Children:   children,
		Attributes: mergedAttrs,
		Content:    blockJSON.Content,
	}

	// Return typed wrapper based on component type
	return createTypedBlock(base), nil
}

// mergeAttributesWithDefaults merges default attributes with provided attributes
func mergeAttributesWithDefaults(componentType MJMLComponentType, attrs map[string]interface{}) map[string]interface{} {
	defaults := GetDefaultAttributes(componentType)
	if attrs == nil {
		return defaults
	}
	merged := make(map[string]interface{})
	// Copy defaults first
	for k, v := range defaults {
		merged[k] = v
	}
	// Override with provided attributes
	for k, v := range attrs {
		merged[k] = v
	}
	return merged
}

// createTypedBlock creates a typed block wrapper for a BaseBlock
func createTypedBlock(base *BaseBlock) EmailBlock {
	switch base.Type {
	case MJMLComponentMjml:
		return &MJMLBlock{BaseBlock: base}
	case MJMLComponentMjHead:
		return &MJHeadBlock{BaseBlock: base}
	case MJMLComponentMjBody:
		return &MJBodyBlock{BaseBlock: base}
	case MJMLComponentMjWrapper:
		return &MJWrapperBlock{BaseBlock: base}
	case MJMLComponentMjSection:
		return &MJSectionBlock{BaseBlock: base}
	case MJMLComponentMjColumn:
		return &MJColumnBlock{BaseBlock: base}
	case MJMLComponentMjGroup:
		return &MJGroupBlock{BaseBlock: base}
	case MJMLComponentMjText:
		return &MJTextBlock{BaseBlock: base}
	case MJMLComponentMjButton:
		return &MJButtonBlock{BaseBlock: base}
	case MJMLComponentMjImage:
		return &MJImageBlock{BaseBlock: base}
	case MJMLComponentMjDivider:
		return &MJDividerBlock{BaseBlock: base}
	case MJMLComponentMjSpacer:
		return &MJSpacerBlock{BaseBlock: base}
	case MJMLComponentMjSocial:
		return &MJSocialBlock{BaseBlock: base}
	case MJMLComponentMjSocialElement:
		return &MJSocialElementBlock{BaseBlock: base}
	case MJMLComponentMjRaw:
		return &MJRawBlock{BaseBlock: base}
	case MJMLComponentMjAttributes:
		return &MJAttributesBlock{BaseBlock: base}
	case MJMLComponentMjBreakpoint:
		return &MJBreakpointBlock{BaseBlock: base}
	case MJMLComponentMjFont:
		return &MJFontBlock{BaseBlock: base}
	case MJMLComponentMjHtmlAttributes:
		return &MJHtmlAttributesBlock{BaseBlock: base}
	case MJMLComponentMjPreview:
		return &MJPreviewBlock{BaseBlock: base}
	case MJMLComponentMjStyle:
		return &MJStyleBlock{BaseBlock: base}
	case MJMLComponentMjTitle:
		return &MJTitleBlock{BaseBlock: base}
	default:
		// For unknown types, return the base block itself
		return base
	}
}

// Helper function to unmarshal a slice of EmailBlocks
func UnmarshalEmailBlocks(data []byte) ([]EmailBlock, error) {
	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(data, &rawBlocks); err != nil {
		return nil, fmt.Errorf("failed to unmarshal EmailBlocks array: %w", err)
	}

	blocks := make([]EmailBlock, len(rawBlocks))
	for i, rawBlock := range rawBlocks {
		block, err := UnmarshalEmailBlock(rawBlock)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal block at index %d: %w", i, err)
		}
		blocks[i] = block
	}

	return blocks, nil
}

// ValidChildrenMap defines valid parent-child relationships for MJML components
var ValidChildrenMap = map[MJMLComponentType][]MJMLComponentType{
	MJMLComponentMjml: {
		MJMLComponentMjHead,
		MJMLComponentMjBody,
	},
	MJMLComponentMjBody: {
		MJMLComponentMjWrapper,
		MJMLComponentMjSection,
		MJMLComponentMjRaw,
	},
	MJMLComponentMjWrapper: {
		MJMLComponentMjSection,
		MJMLComponentMjRaw,
	},
	MJMLComponentMjSection: {
		MJMLComponentMjColumn,
		MJMLComponentMjGroup,
		MJMLComponentMjRaw,
	},
	MJMLComponentMjColumn: {
		MJMLComponentMjText,
		MJMLComponentMjButton,
		MJMLComponentMjImage,
		MJMLComponentMjDivider,
		MJMLComponentMjSpacer,
		MJMLComponentMjSocial,
		MJMLComponentMjRaw,
	},
	MJMLComponentMjGroup: {
		MJMLComponentMjColumn,
	},
	MJMLComponentMjSocial: {
		MJMLComponentMjSocialElement,
	},
	MJMLComponentMjHead: {
		MJMLComponentMjAttributes,
		MJMLComponentMjBreakpoint,
		MJMLComponentMjFont,
		MJMLComponentMjHtmlAttributes,
		MJMLComponentMjPreview,
		MJMLComponentMjStyle,
		MJMLComponentMjTitle,
		MJMLComponentMjRaw,
	},
	// Leaf components (no children allowed)
	MJMLComponentMjText:           {},
	MJMLComponentMjButton:         {},
	MJMLComponentMjImage:          {},
	MJMLComponentMjDivider:        {},
	MJMLComponentMjSpacer:         {},
	MJMLComponentMjSocialElement:  {},
	MJMLComponentMjRaw:            {},
	MJMLComponentMjAttributes:     {},
	MJMLComponentMjBreakpoint:     {},
	MJMLComponentMjFont:           {},
	MJMLComponentMjHtmlAttributes: {},
	MJMLComponentMjPreview:        {},
	MJMLComponentMjStyle:          {},
	MJMLComponentMjTitle:          {},
}

// CanDropCheck validates if a drag type can be dropped into a drop type
func CanDropCheck(dragType, dropType MJMLComponentType) bool {
	validChildren, exists := ValidChildrenMap[dropType]
	if !exists {
		return false
	}

	for _, validChild := range validChildren {
		if validChild == dragType {
			return true
		}
	}
	return false
}

// IsLeafComponent checks if a component type is a leaf (cannot have children)
func IsLeafComponent(componentType MJMLComponentType) bool {
	validChildren, exists := ValidChildrenMap[componentType]
	return exists && len(validChildren) == 0
}

// ValidateComponentHierarchy validates that a component hierarchy is valid
func ValidateComponentHierarchy(block EmailBlock) error {
	children := block.GetChildren()
	blockType := block.GetType()

	// Check if this component should have children
	if IsLeafComponent(blockType) && len(children) > 0 {
		return fmt.Errorf("component %s cannot have children", blockType)
	}

	// Validate each child
	for _, child := range children {
		if child == nil {
			continue
		}

		childType := child.GetType()
		if !CanDropCheck(childType, blockType) {
			return fmt.Errorf("component %s cannot be a child of %s", childType, blockType)
		}

		// Recursively validate children
		if err := ValidateComponentHierarchy(child); err != nil {
			return err
		}
	}

	return nil
}

// GetComponentDisplayName returns a human-readable name for a component type
func GetComponentDisplayName(componentType MJMLComponentType) string {
	switch componentType {
	case MJMLComponentMjml:
		return "MJML Document"
	case MJMLComponentMjBody:
		return "Body"
	case MJMLComponentMjWrapper:
		return "Wrapper"
	case MJMLComponentMjSection:
		return "Section"
	case MJMLComponentMjColumn:
		return "Column"
	case MJMLComponentMjGroup:
		return "Group"
	case MJMLComponentMjText:
		return "Text"
	case MJMLComponentMjButton:
		return "Button"
	case MJMLComponentMjImage:
		return "Image"
	case MJMLComponentMjDivider:
		return "Divider"
	case MJMLComponentMjSpacer:
		return "Spacer"
	case MJMLComponentMjSocial:
		return "Social"
	case MJMLComponentMjSocialElement:
		return "Social Element"
	case MJMLComponentMjHead:
		return "Head"
	case MJMLComponentMjAttributes:
		return "Attributes"
	case MJMLComponentMjBreakpoint:
		return "Breakpoint"
	case MJMLComponentMjFont:
		return "Font"
	case MJMLComponentMjHtmlAttributes:
		return "HTML Attributes"
	case MJMLComponentMjPreview:
		return "Preview"
	case MJMLComponentMjStyle:
		return "Style"
	case MJMLComponentMjTitle:
		return "Title"
	case MJMLComponentMjRaw:
		return "Raw HTML"
	default:
		// Convert kebab-case to Title Case
		parts := strings.Split(string(componentType), "-")
		for i, part := range parts {
			if len(part) > 0 {
				parts[i] = strings.ToUpper(part[:1]) + part[1:]
			}
		}
		return strings.Join(parts, " ")
	}
}

// GetComponentCategory returns the category of a component for UI organization
func GetComponentCategory(componentType MJMLComponentType) string {
	switch componentType {
	case MJMLComponentMjml, MJMLComponentMjBody, MJMLComponentMjHead:
		return "Document"
	case MJMLComponentMjWrapper, MJMLComponentMjSection, MJMLComponentMjColumn, MJMLComponentMjGroup:
		return "Layout"
	case MJMLComponentMjText, MJMLComponentMjButton, MJMLComponentMjImage:
		return "Content"
	case MJMLComponentMjDivider, MJMLComponentMjSpacer:
		return "Spacing"
	case MJMLComponentMjSocial, MJMLComponentMjSocialElement:
		return "Social"
	case MJMLComponentMjAttributes, MJMLComponentMjBreakpoint, MJMLComponentMjFont,
		MJMLComponentMjHtmlAttributes, MJMLComponentMjPreview, MJMLComponentMjStyle, MJMLComponentMjTitle:
		return "Head"
	case MJMLComponentMjRaw:
		return "Raw"
	default:
		return "Other"
	}
}

// IsContentComponent checks if a component is a content component (text, button, image, etc.)
func IsContentComponent(componentType MJMLComponentType) bool {
	contentTypes := []MJMLComponentType{
		MJMLComponentMjText,
		MJMLComponentMjButton,
		MJMLComponentMjImage,
		MJMLComponentMjDivider,
		MJMLComponentMjSpacer,
		MJMLComponentMjSocial,
		MJMLComponentMjSocialElement,
		MJMLComponentMjRaw,
	}

	for _, contentType := range contentTypes {
		if contentType == componentType {
			return true
		}
	}
	return false
}

// IsLayoutComponent checks if a component is a layout component (section, column, etc.)
func IsLayoutComponent(componentType MJMLComponentType) bool {
	layoutTypes := []MJMLComponentType{
		MJMLComponentMjWrapper,
		MJMLComponentMjSection,
		MJMLComponentMjColumn,
		MJMLComponentMjGroup,
	}

	for _, layoutType := range layoutTypes {
		if layoutType == componentType {
			return true
		}
	}
	return false
}

// IsHeadComponent checks if a component belongs in the head section
func IsHeadComponent(componentType MJMLComponentType) bool {
	headTypes := []MJMLComponentType{
		MJMLComponentMjAttributes,
		MJMLComponentMjBreakpoint,
		MJMLComponentMjFont,
		MJMLComponentMjHtmlAttributes,
		MJMLComponentMjPreview,
		MJMLComponentMjStyle,
		MJMLComponentMjTitle,
	}

	for _, headType := range headTypes {
		if headType == componentType {
			return true
		}
	}
	return false
}

// ValidateEmailStructure validates the overall structure of an email
func ValidateEmailStructure(email EmailBlock) error {
	if email.GetType() != MJMLComponentMjml {
		return fmt.Errorf("root component must be mjml, got %s", email.GetType())
	}

	children := email.GetChildren()
	if len(children) == 0 {
		return fmt.Errorf("mjml document must have children")
	}

	hasBody := false
	for _, child := range children {
		if child == nil {
			continue
		}

		childType := child.GetType()
		if childType == MJMLComponentMjBody {
			hasBody = true
		} else if childType != MJMLComponentMjHead {
			return fmt.Errorf("mjml can only contain mj-head and mj-body, found %s", childType)
		}
	}

	if !hasBody {
		return fmt.Errorf("mjml document must contain an mj-body")
	}

	return ValidateComponentHierarchy(email)
}

// GetDefaultAttributes returns default attributes for a given component type
func GetDefaultAttributes(componentType MJMLComponentType) map[string]interface{} {
	defaults := make(map[string]interface{})

	switch componentType {
	case MJMLComponentMjText:
		defaults["fontSize"] = "14px"
		defaults["lineHeight"] = "1.5"
		defaults["color"] = "#000000"
	case MJMLComponentMjButton:
		defaults["backgroundColor"] = "#414141"
		defaults["color"] = "#ffffff"
		defaults["fontSize"] = "13px"
		defaults["fontWeight"] = "bold"
		defaults["borderRadius"] = "3px"
		// defaults["padding"] = "10px 25px"
		defaults["paddingTop"] = "10px"
		defaults["paddingRight"] = "25px"
		defaults["paddingBottom"] = "10px"
		defaults["paddingLeft"] = "25px"
	case MJMLComponentMjImage:
		defaults["align"] = "center"
		defaults["fluidOnMobile"] = "true"
	case MJMLComponentMjDivider:
		defaults["borderColor"] = "#000000"
		defaults["borderStyle"] = "solid"
		defaults["borderWidth"] = "4px"
		defaults["width"] = "100%"
	case MJMLComponentMjSpacer:
		defaults["height"] = "20px"
	case MJMLComponentMjSection:
		// defaults["padding"] = "20px 0"
		defaults["paddingTop"] = "20px"
		defaults["paddingRight"] = "0px"
		defaults["paddingBottom"] = "20px"
		defaults["paddingLeft"] = "0px"
	case MJMLComponentMjColumn:
		// defaults["padding"] = "0"
		defaults["paddingTop"] = "0px"
		defaults["paddingRight"] = "0px"
		defaults["paddingBottom"] = "0px"
		defaults["paddingLeft"] = "0px"
	}

	return defaults
}

// ConvertMapToTypedAttributes converts a map[string]interface{} to a typed attribute struct
func ConvertMapToTypedAttributes(attrs map[string]interface{}, target interface{}) error {
	if attrs == nil {
		return nil
	}
	attrBytes, err := json.Marshal(attrs)
	if err != nil {
		return fmt.Errorf("failed to marshal attributes: %w", err)
	}
	if err := json.Unmarshal(attrBytes, target); err != nil {
		return fmt.Errorf("failed to unmarshal typed attributes: %w", err)
	}
	return nil
}

// structToMap converts a typed attribute struct to a map[string]interface{}
func structToMap(v interface{}) (map[string]interface{}, error) {
	if v == nil {
		return make(map[string]interface{}), nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal struct: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}
	return result, nil
}

// CreateBlockWithDefaults creates a block with default attributes merged with provided attributes
func CreateBlockWithDefaults(blockType MJMLComponentType, providedAttrs map[string]interface{}) map[string]interface{} {
	defaults := GetDefaultAttributes(blockType)
	if providedAttrs == nil {
		return defaults
	}

	merged := make(map[string]interface{})
	// Copy defaults first
	for k, v := range defaults {
		merged[k] = v
	}
	// Override with provided attributes
	for k, v := range providedAttrs {
		merged[k] = v
	}
	return merged
}

// ValidateAndFixAttributes ensures attributes are valid for the given block type
func ValidateAndFixAttributes(blockType MJMLComponentType, attrs map[string]interface{}) map[string]interface{} {
	if attrs == nil {
		return GetDefaultAttributes(blockType)
	}

	// Ensure required attributes exist
	defaults := GetDefaultAttributes(blockType)
	validated := make(map[string]interface{})

	// Copy all provided attributes
	for k, v := range attrs {
		validated[k] = v
	}

	// Add missing defaults
	for k, v := range defaults {
		if _, exists := validated[k]; !exists {
			validated[k] = v
		}
	}

	return validated
}
