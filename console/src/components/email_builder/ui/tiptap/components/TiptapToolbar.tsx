import React from 'react'
import { Space, ColorPicker, Popover, Tooltip, Select, Input, Button } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faUndo,
  faRedo,
  faSuperscript,
  faSubscript,
  faBold,
  faItalic,
  faUnderline,
  faStrikethrough,
  faHighlighter,
  faFont,
  faSmile,
  faLink
} from '@fortawesome/free-solid-svg-icons'
import data from '@emoji-mart/data'
import Picker from '@emoji-mart/react'
import { useLingui } from '@lingui/react/macro'
import type {
  TiptapToolbarProps,
  ToolbarButtonProps,
  ColorButtonProps,
  EmojiButtonProps,
  LinkButtonProps,
  LinkType
} from '../shared/types'
import {
  defaultToolbarStyle,
  defaultToolbarClasses,
  getToolbarButtonClasses,
  toolbarSeparatorClasses
} from '../shared/styles'
import { applyFormattingWithNodeSelection, applyInlineFormatting } from '../shared/utils'
import {
  handleTextColorChange as handleTextColorChangeUtil,
  handleBackgroundColorChange as handleBackgroundColorChangeUtil,
  getEffectiveTextColor,
  getEffectiveBackgroundColor,
  createLinkWithStyleMerging,
  expandSelectionToLinkRange
} from '../shared/utils'

// Toolbar Button Component
export const ToolbarButton: React.FC<ToolbarButtonProps> = ({
  onClick,
  disabled = false,
  isActive = false,
  title,
  children
}) => {
  return (
    <Tooltip title={title}>
      <button
        type="button"
        onClick={onClick}
        disabled={disabled}
        style={{ width: 28, height: 28 }}
        className={getToolbarButtonClasses(isActive, disabled)}
      >
        {children}
      </button>
    </Tooltip>
  )
}

// Color Button Component (for color pickers)
export const ColorButton: React.FC<ColorButtonProps> = ({
  icon,
  currentColor,
  onColorChange,
  title,
  isActive = false
}) => {
  const { t } = useLingui()
  const handleColorChange = (color: unknown) => {
    const hexValue = (color as { toHexString?: () => string })?.toHexString?.() || ''
    onColorChange(hexValue)
  }

  const handleClear = () => {
    onColorChange('')
  }

  return (
    <Tooltip title={title}>
      <span>
        <ColorPicker
          value={currentColor || undefined}
          onChange={handleColorChange}
          onClear={handleClear}
          allowClear={true}
          size="small"
          showText={false}
          presets={[
            {
              label: t`Recommended`,
              colors: [
                // Basic + Gray
                '#000000',
                '#ffffff',
                '#1f2937',
                '#f9fafb',
                '#f3f4f6',
                '#e5e7eb',
                '#d1d5db',
                '#9ca3af',
                '#6b7280',
                '#4b5563',
                '#374151',
                '#111827',
                // Red
                '#fdf2f2',
                '#fde8e8',
                '#fbd5d5',
                '#f8b4b4',
                '#f98080',
                '#f05252',
                '#e02424',
                '#c81e1e',
                '#9b1c1c',
                '#771d1d',
                // Yellow
                '#fdfdea',
                '#fdf6b2',
                '#fce96a',
                '#faca15',
                '#e3a008',
                '#c27803',
                '#9f580a',
                '#8e4b10',
                '#723b13',
                '#633112',
                // Green
                '#f3faf7',
                '#def7ec',
                '#bcf0da',
                '#84e1bc',
                '#31c48d',
                '#0e9f6e',
                '#057a55',
                '#046c4e',
                '#03543f',
                '#014737',
                // Blue
                '#ebf5ff',
                '#e1effe',
                '#c3ddfd',
                '#a4cafe',
                '#76a9fa',
                '#3f83f8',
                '#1c64f2',
                '#1a56db',
                '#1e429f',
                '#233876',
                // Indigo
                '#f0f5ff',
                '#e5edff',
                '#cddbfe',
                '#b4c6fc',
                '#8da2fb',
                '#6875f5',
                '#5850ec',
                '#5145cd',
                '#42389d',
                '#362f78',
                // Purple
                '#f6f5ff',
                '#edebfe',
                '#dcd7fe',
                '#cabffd',
                '#ac94fa',
                '#9061f9',
                '#7e3af2',
                '#6c2bd9',
                '#5521b5',
                '#4a1d96',
                // Pink
                '#fdf2f8',
                '#fce8f3',
                '#fad1e8',
                '#f8b4d9',
                '#f17eb8',
                '#e74694',
                '#d61f69',
                '#bf125d',
                '#99154b',
                '#751a3d'
              ]
            }
          ]}
        >
          <div style={{ position: 'relative' }}>
            <ToolbarButton title={title} isActive={isActive}>
              <FontAwesomeIcon
                icon={icon as import('@fortawesome/fontawesome-svg-core').IconProp}
                size="xs"
              />
            </ToolbarButton>
            <div
              style={{
                position: 'absolute',
                bottom: '2px',
                left: '2px',
                right: '2px',
                height: '3px',
                backgroundColor: currentColor || '#ffffff',
                borderRadius: '1px'
              }}
            />
          </div>
        </ColorPicker>
      </span>
    </Tooltip>
  )
}

// Toolbar Separator Component
export const ToolbarSeparator: React.FC = () => <div className={toolbarSeparatorClasses} />

// Emoji Button Component
export const EmojiButton: React.FC<EmojiButtonProps> = ({ onEmojiSelect, title }) => {
  const [visible, setVisible] = React.useState(false)

  const handleEmojiSelect = (emoji: { native?: string }) => {
    onEmojiSelect(emoji)
    setVisible(false)
  }

  return (
    <Popover
      content={
        <div
          style={{ border: 'none', maxHeight: '300px', overflow: 'auto' }}
          onClick={(e) => e.stopPropagation()}
        >
          <Picker
            data={data}
            onEmojiSelect={handleEmojiSelect}
            theme="light"
            set="native"
            skinTonePosition="none"
            perLine={8}
            maxFrequentRows={2}
          />
        </div>
      }
      trigger="click"
      placement="bottom"
      arrow={false}
      overlayClassName="tiptap-emoji-popover"
      title={title}
      open={visible}
      onOpenChange={setVisible}
    >
      <span>
        <ToolbarButton title={title} onClick={() => setVisible(!visible)}>
          <FontAwesomeIcon icon={faSmile} size="xs" />
        </ToolbarButton>
      </span>
    </Popover>
  )
}

// Link Button Component
export const LinkButton: React.FC<LinkButtonProps> = ({ editor, title }) => {
  const { t } = useLingui()
  const [visible, setVisible] = React.useState(false)
  const [linkType, setLinkType] = React.useState<LinkType>('url')
  const [linkValue, setLinkValue] = React.useState('')

  // Get current link if selection is inside a link
  const getCurrentLink = () => {
    const { href } = editor.getAttributes('link')
    return href || ''
  }

  // Check if current selection is a link
  const isActiveLink = editor.isActive('link')

  React.useEffect(() => {
    if (visible) {
      const currentHref = getCurrentLink()
      if (currentHref) {
        setLinkValue(currentHref)
        // Try to determine link type based on current href
        if (currentHref.startsWith('mailto:')) {
          setLinkType('email')
          setLinkValue(currentHref.replace('mailto:', ''))
        } else if (currentHref.startsWith('tel:')) {
          setLinkType('phone')
          setLinkValue(currentHref.replace('tel:', ''))
        } else if (currentHref.startsWith('#')) {
          setLinkType('anchor')
          setLinkValue(currentHref.replace('#', ''))
        } else {
          setLinkType('url')
          setLinkValue(currentHref)
        }
      } else {
        setLinkValue('')
        setLinkType('url')
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- getCurrentLink is stable
  }, [visible, isActiveLink])

  const handleInsertLink = () => {
    if (!linkValue.trim()) {
      // Remove link if value is empty
      // Expand selection to full link range first to ensure entire link is removed
      expandSelectionToLinkRange(editor)
      editor.chain().focus().unsetLink().run()
    } else {
      // If we're editing an existing link, expand selection to cover the full link
      // This ensures the entire link text gets updated, not just the cursor position
      if (isActiveLink) {
        expandSelectionToLinkRange(editor)
      }
      // Use our enhanced link creation that merges textStyle attributes
      createLinkWithStyleMerging(editor, linkValue, linkType)
    }
    setVisible(false)
    setLinkValue('')
  }

  const handleRemoveLink = () => {
    // Expand selection to full link range first to ensure entire link is removed
    expandSelectionToLinkRange(editor)
    editor.chain().focus().unsetLink().run()
    setVisible(false)
    setLinkValue('')
  }

  const linkTypeOptions = [
    { value: 'url', label: t`URL` },
    { value: 'email', label: t`Email` },
    { value: 'phone', label: t`Phone` },
    { value: 'anchor', label: t`Anchor` }
  ]

  // Placeholder strings contain Liquid template syntax - not translatable
  const getPlaceholder = () => {
    switch (linkType) {
      case 'email':
        return 'user@example.com or {{ contact.email }}'
      case 'phone':
        return '+1234567890 or {{ contact.phone }}'
      case 'anchor':
        return 'section-name'
      case 'url':
      default:
        return 'https://example.com or {{ url }}'
    }
  }

  return (
    <Popover
      content={
        <div style={{ minWidth: '300px', padding: '8px' }}>
          <div style={{ marginBottom: '12px' }}>
            <label
              style={{ display: 'block', marginBottom: '4px', fontSize: '14px', fontWeight: '500' }}
            >
              {t`Link Type`}
            </label>
            <Select
              value={linkType}
              onChange={setLinkType}
              options={linkTypeOptions}
              style={{ width: '100%' }}
              size="small"
            />
          </div>
          <div style={{ marginBottom: '12px' }}>
            <label
              style={{ display: 'block', marginBottom: '4px', fontSize: '14px', fontWeight: '500' }}
            >
              {linkType === 'url'
                ? t`URL`
                : linkType === 'email'
                  ? t`Email Address`
                  : linkType === 'phone'
                    ? t`Phone Number`
                    : t`Anchor ID`}
            </label>
            <Input
              value={linkValue}
              onChange={(e) => setLinkValue(e.target.value)}
              placeholder={getPlaceholder()}
              size="small"
              onPressEnter={handleInsertLink}
            />
          </div>
          <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
            <Button size="small" onClick={() => setVisible(false)}>
              {t`Cancel`}
            </Button>
            {isActiveLink && (
              <Button size="small" onClick={handleRemoveLink} color="danger" variant="outlined">
                {t`Remove Link`}
              </Button>
            )}
            <Button size="small" type="primary" onClick={handleInsertLink}>
              {isActiveLink ? t`Update Link` : t`Insert Link`}
            </Button>
          </div>
        </div>
      }
      trigger="click"
      arrow={false}
      overlayClassName="tiptap-link-popover"
      title={t`Insert Link`}
      open={visible}
      onOpenChange={setVisible}
    >
      <span>
        <ToolbarButton title={title} isActive={isActiveLink} onClick={() => setVisible(!visible)}>
          <FontAwesomeIcon icon={faLink} size="xs" />
        </ToolbarButton>
      </span>
    </Popover>
  )
}

// Main Toolbar Component
export const TiptapToolbar: React.FC<TiptapToolbarProps> = ({ editor, buttons, mode = 'rich' }) => {
  const { t } = useLingui()
  if (!editor) {
    return null
  }

  // Default buttons if none specified
  const defaultButtons = [
    'undo',
    'redo',
    'bold',
    'italic',
    'underline',
    'strikethrough',
    'textColor',
    'backgroundColor',
    'emoji',
    'link',
    'superscript',
    'subscript'
  ]

  const activeButtons = buttons || defaultButtons

  // Helper function to check if a button should be shown
  const shouldShowButton = (buttonName: string) => {
    return activeButtons.includes(buttonName)
  }

  // Get current text color and background color from editor (with link priority)
  const getCurrentTextColor = () => {
    return getEffectiveTextColor(editor)
  }

  const getCurrentBackgroundColor = () => {
    return getEffectiveBackgroundColor(editor)
  }

  const handleTextColorChange = (color: string) => {
    handleTextColorChangeUtil(editor, color)
  }

  const handleBackgroundColorChange = (color: string) => {
    handleBackgroundColorChangeUtil(editor, color)
  }

  const handleEmojiSelect = (emoji: { native?: string }) => {
    if (emoji.native) {
      editor.chain().focus().insertContent(emoji.native).run()
    }
  }

  const getFormattingHandler = (action: () => void) => {
    return mode === 'inline'
      ? () => applyInlineFormatting(editor, action)
      : () => applyFormattingWithNodeSelection(editor, action)
  }

  return (
    <span style={defaultToolbarStyle} className={`${defaultToolbarClasses} tiptap-toolbar`}>
      {/* Undo/Redo Group */}
      {(shouldShowButton('undo') || shouldShowButton('redo')) && (
        <>
          <Space size="small">
            {shouldShowButton('undo') && (
              <ToolbarButton
                onClick={() => editor.chain().focus().undo().run()}
                disabled={!editor.can().chain().focus().undo().run()}
                title={t`Undo`}
              >
                <FontAwesomeIcon icon={faUndo} size="xs" />
              </ToolbarButton>
            )}
            {shouldShowButton('redo') && (
              <ToolbarButton
                onClick={() => editor.chain().focus().redo().run()}
                disabled={!editor.can().chain().focus().redo().run()}
                title={t`Redo`}
              >
                <FontAwesomeIcon icon={faRedo} size="xs" />
              </ToolbarButton>
            )}
          </Space>
          <ToolbarSeparator />
        </>
      )}

      {/* Text Formatting Group */}
      {(shouldShowButton('bold') ||
        shouldShowButton('italic') ||
        shouldShowButton('underline') ||
        shouldShowButton('strikethrough')) && (
        <>
          <Space size="small">
            {shouldShowButton('bold') && (
              <ToolbarButton
                onClick={getFormattingHandler(() => editor.chain().focus().toggleBold().run())}
                disabled={!editor.can().chain().focus().toggleBold().run()}
                isActive={editor.isActive('bold')}
                title={t`Bold`}
              >
                <FontAwesomeIcon icon={faBold} size="xs" />
              </ToolbarButton>
            )}
            {shouldShowButton('italic') && (
              <ToolbarButton
                onClick={getFormattingHandler(() => editor.chain().focus().toggleItalic().run())}
                disabled={!editor.can().chain().focus().toggleItalic().run()}
                isActive={editor.isActive('italic')}
                title={t`Italic`}
              >
                <FontAwesomeIcon icon={faItalic} size="xs" />
              </ToolbarButton>
            )}
            {shouldShowButton('underline') && (
              <ToolbarButton
                onClick={getFormattingHandler(() => editor.chain().focus().toggleUnderline().run())}
                disabled={!editor.can().chain().focus().toggleUnderline().run()}
                isActive={editor.isActive('underline')}
                title={t`Underline`}
              >
                <FontAwesomeIcon icon={faUnderline} size="xs" />
              </ToolbarButton>
            )}
            {shouldShowButton('strikethrough') && (
              <ToolbarButton
                onClick={getFormattingHandler(() => editor.chain().focus().toggleStrike().run())}
                disabled={!editor.can().chain().focus().toggleStrike().run()}
                isActive={editor.isActive('strike')}
                title={t`Strikethrough`}
              >
                <FontAwesomeIcon icon={faStrikethrough} size="xs" />
              </ToolbarButton>
            )}
          </Space>
          <ToolbarSeparator />
        </>
      )}

      {/* Color Formatting Group */}
      {(shouldShowButton('textColor') || shouldShowButton('backgroundColor')) && (
        <>
          <Space size="small">
            {shouldShowButton('textColor') && (
              <ColorButton
                icon={faFont}
                currentColor={getCurrentTextColor()}
                onColorChange={handleTextColorChange}
                title={t`Text Color`}
                isActive={!!getCurrentTextColor()}
              />
            )}
            {shouldShowButton('backgroundColor') && (
              <ColorButton
                icon={faHighlighter}
                currentColor={getCurrentBackgroundColor()}
                onColorChange={handleBackgroundColorChange}
                title={t`Background Color`}
                isActive={!!getCurrentBackgroundColor()}
              />
            )}
          </Space>
          <ToolbarSeparator />
        </>
      )}

      {/* Emoji Button */}
      {shouldShowButton('emoji') && (
        <>
          <Space size="small">
            <EmojiButton onEmojiSelect={handleEmojiSelect} title={t`Insert Emoji`} />
          </Space>
          <ToolbarSeparator />
        </>
      )}

      {/* Link Button */}
      {shouldShowButton('link') && (
        <>
          <Space size="small">
            <LinkButton editor={editor} title={t`Insert Link`} />
          </Space>
          <ToolbarSeparator />
        </>
      )}

      {/* Additional Formatting */}
      {(shouldShowButton('superscript') || shouldShowButton('subscript')) && (
        <>
          <Space size="small">
            {shouldShowButton('superscript') && (
              <ToolbarButton
                onClick={getFormattingHandler(() =>
                  editor.chain().focus().toggleSuperscript().run()
                )}
                isActive={editor.isActive('superscript')}
                title={t`Superscript`}
              >
                <FontAwesomeIcon icon={faSuperscript} size="xs" />
              </ToolbarButton>
            )}
            {shouldShowButton('subscript') && (
              <ToolbarButton
                onClick={getFormattingHandler(() => editor.chain().focus().toggleSubscript().run())}
                isActive={editor.isActive('subscript')}
                title={t`Subscript`}
              >
                <FontAwesomeIcon icon={faSubscript} size="xs" />
              </ToolbarButton>
            )}
          </Space>
        </>
      )}
    </span>
  )
}
