import React from 'react'

// Base types for MJML components
export type MJMLComponentType =
  | 'mjml'
  | 'mj-body'
  | 'mj-wrapper'
  | 'mj-section'
  | 'mj-column'
  | 'mj-group'
  | 'mj-text'
  | 'mj-button'
  | 'mj-image'
  | 'mj-divider'
  | 'mj-spacer'
  | 'mj-social'
  | 'mj-social-element'
  | 'mj-head'
  | 'mj-attributes'
  | 'mj-breakpoint'
  | 'mj-font'
  | 'mj-html-attributes'
  | 'mj-preview'
  | 'mj-style'
  | 'mj-title'
  | 'mj-raw'

// Common attribute interfaces
export interface PaddingAttributes {
  //   padding?: string // we prefer to use the individual padding attributes
  paddingBottom?: string
  paddingLeft?: string
  paddingRight?: string
  paddingTop?: string
}

export interface BorderAttributes {
  //   border?: string
  borderBottom?: string
  borderLeft?: string
  borderRadius?: string
  borderRight?: string
  borderTop?: string
}

export interface BackgroundAttributes {
  backgroundColor?: string
  backgroundUrl?: string
  backgroundRepeat?: string
  backgroundSize?: string
  backgroundPosition?: string
  backgroundPositionX?: string
  backgroundPositionY?: string
}

export interface TextAttributes {
  align?: 'left' | 'right' | 'center' | 'justify'
  color?: string
  fontFamily?: string
  fontSize?: string
  fontStyle?: string
  fontWeight?: string
  letterSpacing?: string
  lineHeight?: string
  textAlign?: 'left' | 'right' | 'center' | 'justify'
  textDecoration?: string
  textTransform?: string
}

export interface LayoutAttributes {
  height?: string
  width?: string
  verticalAlign?: 'top' | 'bottom' | 'middle'
}

export interface CommonAttributes {
  cssClass?: string
}

export interface LinkAttributes {
  href?: string
  rel?: string
  target?: '_blank' | '_self' | '_parent' | '_top'
}

export interface ContainerAttributes {
  containerBackgroundColor?: string
}

// Base interface for all MJML blocks
export interface BaseBlock {
  id: string
  type: MJMLComponentType
  children?: BaseBlock[]
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  attributes?: Record<string, any>
  content?: string
}

// MJML Head - Contains head components
export interface MJHeadBlock extends BaseBlock {
  type: 'mj-head'
  children?: (
    | MJAttributesBlock
    | MJBreakpointBlock
    | MJFontBlock
    | MJHtmlAttributesBlock
    | MJPreviewBlock
    | MJStyleBlock
    | MJTitleBlock
    | MJRawBlock
  )[]
  attributes?: Record<string, never> // Head doesn't have attributes
}

// MJML Head Components
export interface MJAttributesBlock extends BaseBlock {
  type: 'mj-attributes'
  children?: MJAttributeElementBlock[]
  attributes?: Record<string, never> // mj-attributes doesn't have its own attributes
}

// MJML Root - Contains mj-head and mj-body
export interface MJMLBlock extends BaseBlock {
  type: 'mjml'
  children?: (MJHeadBlock | MJBodyBlock)[]
  attributes?: Record<string, never> // mjml doesn't have attributes
}

// Helper type to extract attributes from each block type
type ComponentAttributesMap = {
  mjml: MJMLBlock['attributes']
  'mj-body': MJBodyBlock['attributes']
  'mj-wrapper': MJWrapperBlock['attributes']
  'mj-section': MJSectionBlock['attributes']
  'mj-column': MJColumnBlock['attributes']
  'mj-group': MJGroupBlock['attributes']
  'mj-text': MJTextBlock['attributes']
  'mj-button': MJButtonBlock['attributes']
  'mj-image': MJImageBlock['attributes']
  'mj-divider': MJDividerBlock['attributes']
  'mj-spacer': MJSpacerBlock['attributes']
  'mj-social': MJSocialBlock['attributes']
  'mj-social-element': MJSocialElementBlock['attributes']
  'mj-head': MJHeadBlock['attributes']
  'mj-attributes': MJAttributesBlock['attributes']
  'mj-breakpoint': MJBreakpointBlock['attributes']
  'mj-font': MJFontBlock['attributes']
  'mj-html-attributes': MJHtmlAttributesBlock['attributes']
  'mj-preview': MJPreviewBlock['attributes']
  'mj-style': MJStyleBlock['attributes']
  'mj-title': MJTitleBlock['attributes']
  'mj-raw': MJRawBlock['attributes']
}

// Individual attribute element within mj-attributes
export interface MJAttributeElementBlock<T extends MJMLComponentType = MJMLComponentType>
  extends BaseBlock {
  type: T
  children?: never
  attributes?: ComponentAttributesMap[T]
}

export interface MJBreakpointBlock extends BaseBlock {
  type: 'mj-breakpoint'
  children?: never
  attributes?: {
    width?: string
  }
}

export interface MJFontBlock extends BaseBlock {
  type: 'mj-font'
  children?: never
  attributes?: {
    name?: string
    href?: string
  }
}

export interface MJHtmlAttributesBlock extends BaseBlock {
  type: 'mj-html-attributes'
  children?: never
  attributes?: Record<string, unknown> // Dynamic HTML attributes
}

export interface MJPreviewBlock extends BaseBlock {
  type: 'mj-preview'
  children?: never
  content?: string
  attributes?: Record<string, never>
}

export interface MJStyleBlock extends BaseBlock {
  type: 'mj-style'
  children?: never
  content?: string
  attributes?: {
    inline?: 'inline'
  }
}

export interface MJTitleBlock extends BaseBlock {
  type: 'mj-title'
  children?: never
  content?: string
  attributes?: Record<string, never>
}

// MJML Body - Root block (can contain mj-wrapper and mj-section blocks)
export interface MJBodyBlock extends BaseBlock {
  type: 'mj-body'
  children?: (MJWrapperBlock | MJSectionBlock | MJRawBlock)[]
  attributes?: MJBodyAttributes
}

// MJML Wrapper - Contains mj-section blocks and provides styling context
export interface MJWrapperBlock extends BaseBlock {
  type: 'mj-wrapper'
  children?: (MJSectionBlock | MJRawBlock)[]
  attributes?: MJWrapperAttributes
}

// MJML Section - Contains only mj-column blocks
export interface MJSectionBlock extends BaseBlock {
  type: 'mj-section'
  children?: (MJColumnBlock | MJGroupBlock | MJRawBlock)[]
  attributes?: MJSectionAttributes
}

// MJML Column - Contains mj-text, mj-button, mj-image, mj-divider, mj-spacer, mj-social blocks
export interface MJColumnBlock extends BaseBlock {
  type: 'mj-column'
  children?: (
    | MJTextBlock
    | MJButtonBlock
    | MJImageBlock
    | MJDividerBlock
    | MJSpacerBlock
    | MJSocialBlock
    | MJRawBlock
  )[]
  attributes?: MJColumnAttributes
}

// MJML Group - Contains mj-column blocks and prevents stacking on mobile
export interface MJGroupBlock extends BaseBlock {
  type: 'mj-group'
  children?: MJColumnBlock[]
  attributes?: MJGroupAttributes
}

// MJML Text - Leaf component (no children)
export interface MJTextBlock extends BaseBlock {
  type: 'mj-text'
  children?: never
  content?: string
  attributes?: MJTextAttributes
}

// MJML Button - Leaf component (no children)
export interface MJButtonBlock extends BaseBlock {
  type: 'mj-button'
  children?: never
  content?: string
  attributes?: MJButtonAttributes
}

// MJML Image - Leaf component (no children)
export interface MJImageBlock extends BaseBlock {
  type: 'mj-image'
  children?: never
  attributes?: MJImageAttributes
}

// MJML Divider - Leaf component (no children)
export interface MJDividerBlock extends BaseBlock {
  type: 'mj-divider'
  children?: never
  attributes?: MJDividerAttributes
}

// MJML Spacer - Leaf component (no children)
export interface MJSpacerBlock extends BaseBlock {
  type: 'mj-spacer'
  children?: never
  attributes?: MJSpacerAttributes
}

// MJML Social - Can contain social element children
export interface MJSocialBlock extends BaseBlock {
  type: 'mj-social'
  children?: MJSocialElementBlock[]
  attributes?: MJSocialAttributes
}

// MJML Raw - Leaf component (no children) - allows raw HTML content
export interface MJRawBlock extends BaseBlock {
  type: 'mj-raw'
  children?: never
  content?: string
  attributes?: MJRawAttributes
}

// Union type for all block types
export type EmailBlock =
  | MJMLBlock
  | MJBodyBlock
  | MJSectionBlock
  | MJColumnBlock
  | MJTextBlock
  | MJButtonBlock
  | MJImageBlock
  | MJDividerBlock
  | MJSpacerBlock
  | MJSocialBlock
  | MJSocialElementBlock
  | MJHeadBlock
  | MJAttributesBlock
  | MJAttributeElementBlock
  | MJBreakpointBlock
  | MJFontBlock
  | MJHtmlAttributesBlock
  | MJPreviewBlock
  | MJStyleBlock
  | MJTitleBlock
  | MJWrapperBlock
  | MJGroupBlock
  | MJRawBlock

// Email builder state types
export interface EmailBuilderState {
  selectedBlockId: string | null
  history: EmailBlock[]
  historyIndex: number
  viewportMode?: 'mobile' | 'desktop'
}

// Tree node for Ant Design Tree component
export interface TreeNode {
  key: string
  disabled?: boolean
  title: string | React.ReactNode
  children?: TreeNode[]
  icon?: React.ReactNode
  isLeaf?: boolean
  selectable?: boolean
  draggable?: boolean
  blockType: MJMLComponentType
}

// Drag and drop types
export interface DragInfo {
  node: TreeNode
  dragNode: TreeNode
  dragNodesKeys: string[]
  dropPosition: number
  dropToGap: boolean
}

// Valid parent-child relationships
export type ValidChildren = {
  'mj-body': 'mj-wrapper' | 'mj-section' | 'mj-raw'
  'mj-wrapper': 'mj-section' | 'mj-raw'
  'mj-section': 'mj-column' | 'mj-group' | 'mj-raw'
  'mj-column':
    | 'mj-text'
    | 'mj-button'
    | 'mj-image'
    | 'mj-divider'
    | 'mj-spacer'
    | 'mj-social'
    | 'mj-raw'
  'mj-group': 'mj-column'
  'mj-text': never
  'mj-button': never
  'mj-image': never
  'mj-divider': never
  'mj-spacer': never
  'mj-social': 'mj-social-element'
  'mj-social-element': never
  'mj-raw': never
  'mj-head':
    | 'mj-attributes'
    | 'mj-breakpoint'
    | 'mj-font'
    | 'mj-html-attributes'
    | 'mj-preview'
    | 'mj-style'
    | 'mj-title'
    | 'mj-raw'
  'mj-attributes': never
  'mj-breakpoint': never
  'mj-font': never
  'mj-html-attributes': never
  'mj-preview': never
  'mj-style': never
  'mj-title': never
}

// Helper type to check valid drop targets
export type CanDropCheck = (dragType: MJMLComponentType, dropType: MJMLComponentType) => boolean

// Email builder actions
export interface EmailBuilderActions {
  selectBlock: (blockId: string | null) => void
  addBlock: (parentId: string, blockType: MJMLComponentType, position?: number) => void
  updateBlock: (blockId: string, updates: Partial<EmailBlock>) => void
  deleteBlock: (blockId: string) => void
  moveBlock: (blockId: string, newParentId: string, position: number) => void
  undo: () => void
  redo: () => void
}

// Settings panel configuration
export interface SettingsConfig {
  [blockType: string]: FormField[]
}

// Form field types for the settings panel
export interface FormField {
  key: string
  label: string
  type: 'text' | 'number' | 'color' | 'select' | 'textarea' | 'url' | 'switch'
  options?: { value: string; label: string }[]
  placeholder?: string
  description?: string
  defaultValue?: unknown
}

// Specific attribute types for complex blocks
export type MJBodyAttributes = BackgroundAttributes &
  CommonAttributes & {
    width?: string
    backgroundColor?: string
  }

export type MJWrapperAttributes = BackgroundAttributes &
  BorderAttributes &
  PaddingAttributes &
  CommonAttributes & {
    fullWidthBackgroundColor?: string
    fullWidth?: 'full-width'
    textAlign?: 'left' | 'right' | 'center' | 'justify'
  }

export type MJSectionAttributes = BackgroundAttributes &
  BorderAttributes &
  PaddingAttributes &
  CommonAttributes & {
    direction?: 'ltr' | 'rtl'
    fullWidth?: 'full-width'
    textAlign?: 'left' | 'right' | 'center' | 'justify'
  }

export type MJColumnAttributes = BackgroundAttributes &
  BorderAttributes &
  PaddingAttributes &
  LayoutAttributes &
  CommonAttributes & {
    innerBackgroundColor?: string
    innerBorderTop?: string
    innerBorderRight?: string
    innerBorderBottom?: string
    innerBorderLeft?: string
    innerBorderRadius?: string
  }

export type MJGroupAttributes = BackgroundAttributes &
  LayoutAttributes &
  CommonAttributes & {
    direction?: 'ltr' | 'rtl'
  }

export type MJTextAttributes = TextAttributes &
  PaddingAttributes &
  LayoutAttributes &
  ContainerAttributes &
  CommonAttributes

export type MJButtonAttributes = TextAttributes &
  BackgroundAttributes &
  BorderAttributes &
  PaddingAttributes &
  LayoutAttributes &
  LinkAttributes &
  ContainerAttributes &
  CommonAttributes & {
    innerPadding?: string
  }

export type MJImageAttributes = BorderAttributes &
  PaddingAttributes &
  LayoutAttributes &
  LinkAttributes &
  ContainerAttributes &
  CommonAttributes & {
    align?: 'left' | 'right' | 'center'
    alt?: string
    fluidOnMobile?: 'true' | 'false'
    name?: string
    sizes?: string
    src?: string
    srcset?: string
    title?: string
    usemap?: string
  }

export type MJRawAttributes = CommonAttributes

export type MJBreakpointAttributes = {
  width?: string
}

export type MJFontAttributes = {
  name?: string
  href?: string
}

export type MJStyleAttributes = {
  inline?: 'inline'
}

export type MJHeadAttributes = Record<string, never>

export type MJMLAttributes = Record<string, never>

export type MJDividerAttributes = BorderAttributes &
  PaddingAttributes &
  ContainerAttributes &
  CommonAttributes & {
    align?: 'left' | 'right' | 'center'
    borderColor?: string
    borderStyle?: 'solid' | 'dashed' | 'dotted'
    borderWidth?: string
    width?: string
  }

export type MJSpacerAttributes = PaddingAttributes &
  ContainerAttributes &
  CommonAttributes & {
    height?: string
  }

export type MJSocialAttributes = PaddingAttributes &
  ContainerAttributes &
  CommonAttributes & {
    align?: 'left' | 'right' | 'center'
    borderRadius?: string
    iconHeight?: string
    iconSize?: string
    innerPadding?: string
    lineHeight?: string
    mode?: 'horizontal' | 'vertical'
    tableLayout?: 'auto' | 'fixed'
    textPadding?: string
  }

// mj-social-element attributes interface
export interface MJSocialElementAttributes {
  // Layout and positioning
  align?: 'left' | 'center' | 'right'

  // Icon properties
  alt?: string
  backgroundColor?: string
  borderRadius?: string
  iconHeight?: string
  iconSize?: string
  iconPadding?: string
  // iconPosition?: 'left' | 'right' // Not supported by MJML
  name?: string
  src?: string
  sizes?: string
  srcset?: string

  // Text properties
  color?: string
  fontFamily?: string
  fontSize?: string
  fontStyle?: string
  fontWeight?: string
  lineHeight?: string
  textDecoration?: string
  textPadding?: string
  verticalAlign?: string

  // Link properties
  href?: string
  rel?: string
  target?: string
  title?: string

  // Layout properties
  padding?: string
  paddingTop?: string
  paddingRight?: string
  paddingBottom?: string
  paddingLeft?: string

  // Advanced properties
  cssClass?: string
}

// mj-social-element block interface
export interface MJSocialElementBlock extends BaseBlock {
  type: 'mj-social-element'
  attributes: MJSocialElementAttributes
  children: []
  content?: string
}

export type SaveOperation = 'create' | 'update'

// Helper type for merged attributes that provides string access with proper typing
export interface MergedBlockAttributes {
  // Common attributes
  cssClass?: string
  // Padding attributes
  paddingTop?: string
  paddingRight?: string
  paddingBottom?: string
  paddingLeft?: string
  padding?: string
  innerPadding?: string
  // Border attributes
  border?: string
  borderTop?: string
  borderRight?: string
  borderBottom?: string
  borderLeft?: string
  borderRadius?: string
  borderColor?: string
  borderStyle?: string
  borderWidth?: string
  // Inner border attributes
  innerBorderTop?: string
  innerBorderRight?: string
  innerBorderBottom?: string
  innerBorderLeft?: string
  innerBorderRadius?: string
  // Background attributes
  backgroundColor?: string
  backgroundUrl?: string
  backgroundRepeat?: string
  backgroundSize?: string
  backgroundPosition?: string
  backgroundPositionX?: string
  backgroundPositionY?: string
  containerBackgroundColor?: string
  fullWidthBackgroundColor?: string
  innerBackgroundColor?: string
  // Text attributes
  align?: string
  color?: string
  fontFamily?: string
  fontSize?: string
  fontStyle?: string
  fontWeight?: string
  letterSpacing?: string
  lineHeight?: string
  textAlign?: string
  textDecoration?: string
  textTransform?: string
  verticalAlign?: string
  // Layout attributes
  height?: string
  width?: string
  direction?: string
  fullWidth?: string
  // Link attributes
  href?: string
  rel?: string
  target?: string
  title?: string
  // Image attributes
  alt?: string
  src?: string
  srcset?: string
  sizes?: string
  fluidOnMobile?: string
  name?: string
  usemap?: string
  // Social attributes
  iconHeight?: string
  iconSize?: string
  iconPadding?: string
  textPadding?: string
  mode?: string
  tableLayout?: string
  // Other
  inline?: string
  content?: string
  [key: string]: string | undefined
}

// Saved block interface for storing custom blocks in localStorage
export interface SavedBlock {
  id: string
  name: string
  block: EmailBlock
  created?: string
  updated?: string
}
