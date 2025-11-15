'use client'

import { useContext, forwardRef, useImperativeHandle } from 'react'
import { EditorContent, EditorContext, useEditor } from '@tiptap/react'
import type { Editor } from '@tiptap/core'

// --- Tiptap Core Extensions ---
import { StarterKit } from '@tiptap/starter-kit'
import { Color } from '@tiptap/extension-color'
import { TextStyle } from '@tiptap/extension-text-style'
import { Placeholder } from '@tiptap/extension-placeholder'
import { Typography } from '@tiptap/extension-typography'
import { Highlight } from '@tiptap/extension-highlight'
import { TextAlign } from '@tiptap/extension-text-align'
import { Emoji, gitHubEmojis } from '@tiptap/extension-emoji'
import { Subscript } from '@tiptap/extension-subscript'
import { Superscript } from '@tiptap/extension-superscript'
import TableOfContents from '@tiptap/extension-table-of-contents'
import { common, createLowlight } from 'lowlight'
import javascript from 'highlight.js/lib/languages/javascript'
import typescript from 'highlight.js/lib/languages/typescript'
import css from 'highlight.js/lib/languages/css'
import xml from 'highlight.js/lib/languages/xml' // for HTML
import json from 'highlight.js/lib/languages/json'
import python from 'highlight.js/lib/languages/python'
import go from 'highlight.js/lib/languages/go'
import sql from 'highlight.js/lib/languages/sql'
import bash from 'highlight.js/lib/languages/bash'
import yaml from 'highlight.js/lib/languages/yaml'
import markdown from 'highlight.js/lib/languages/markdown'

// --- Hooks ---
import { useControls } from './core/state/useControls'
import { useEditorStyles } from './hooks/useEditorStyles'

// --- Types ---
import type { EditorStyleConfig } from './types/EditorStyleConfig'

// --- Config ---
import { defaultEditorStyles } from './config/defaultEditorStyles'

// --- Utils ---
import { generateBlogPostCSS } from './utils/styleUtils'

// --- Custom Extensions ---
import { ControlsExtension } from './core/state/EditorControls'
import { BackgroundExtension } from './extensions/BackgroundExtension'
import { AlignmentExtension } from './extensions/AlignmentExtension'
import { HorizontalRuleExtension } from './extensions/HorizontalRuleExtension'
import { ImageExtension } from './extensions/ImageExtension'
import { YoutubeExtension } from './extensions/YoutubeExtension'
import { CodeBlockLowlightExtension } from './extensions/CodeBlockLowlightExtension'

// --- Action Registry (Import to register all actions) ---
import './core/registry/action-specs'

// --- Styles ---
import './styles/nodes.css'
import './styles/editor.css'
import './components/image/image-node.css'
import './components/youtube/youtube-node.css'

// --- UI ---
import { BlockActionsMenu } from './menus/block-actions'
import { EmojiMenu, SlashMenu } from './menus/suggestion'
import { SelectionToolbar } from './toolbars'
import { CodeBlockToolbar } from './toolbars/components'

// --- Components ---
import { EditorHeader } from './components/EditorHeader'

// --- Lowlight Setup ---
const lowlight = createLowlight(common)
lowlight.register('javascript', javascript)
lowlight.register('typescript', typescript)
lowlight.register('css', css)
lowlight.register('html', xml)
lowlight.register('json', json)
lowlight.register('python', python)
lowlight.register('go', go)
lowlight.register('sql', sql)
lowlight.register('bash', bash)
lowlight.register('yaml', yaml)
lowlight.register('markdown', markdown)

/**
 * Ref API for NotifuseEditor - allows parent components to retrieve content on-demand
 */
export interface NotifuseEditorRef {
  getJSON: () => any
  getHTML: () => string
  getCSS: () => string
  undo: () => void
  redo: () => void
  canUndo: () => boolean
  canRedo: () => boolean
  editor: Editor | null
}

/**
 * Type definition for Table of Contents anchor
 */
export interface TOCAnchor {
  dom: HTMLElement
  id: string
  isActive: boolean
  isScrolledOver: boolean
  level: number
  node: any
  pos: number
  textContent: string
}

/**
 * Default initial content for the editor
 */
const DEFAULT_INITIAL_CONTENT = `
<h1>Welcome to the Notifuse Editor! 👋</h1>
<p>This is a <strong>powerful</strong> and <em>flexible</em> rich text editor built with Tiptap. Try out these features:</p>

<h2>Text Formatting</h2>
<p>You can make text <strong>bold</strong>, <em>italic</em>, <u>underlined</u>, <s>strikethrough</s>, <code>code</code>, or combine them like <strong><em>bold and italic</em></strong>.</p>

<h2>Lists</h2>
<p>Create bullet lists:</p>
<ul>
  <li>First item</li>
  <li>Second item with <strong>bold text</strong></li>
  <li>Third item</li>
</ul>

<p>Or numbered lists:</p>
<ol>
  <li>Step one</li>
  <li>Step two</li>
  <li>Step three</li>
</ol>

<h2>Block Actions</h2>
<p>Hover over any block and click the <strong>::</strong> handle to:</p>
<ul>
  <li>Transform blocks (turn paragraphs into headings, lists, etc.)</li>
  <li>Change text and background colors</li>
  <li>Duplicate or delete blocks</li>
  <li>Reset formatting</li>
</ul>

<blockquote>
  <p>This is a quote block. You can use it for important callouts or citations.</p>
</blockquote>

<h2>Code Blocks with Syntax Highlighting</h2>
<p>Click inside any code block to access the floating toolbar with language selection and height controls:</p>

<pre><code class="language-json">{
  "name": "Tiptap Editor",
  "version": "1.0.0",
  "features": [
    "Syntax highlighting",
    "Multiple languages",
    "Adjustable height"
  ],
  "supported": true
}</code></pre>

<h2>Try These Features</h2>
<p>Select text to see the formatting toolbar, or type <code>/</code> to open the command menu with more options!</p>

<h2>Media Support</h2>
<p>Add images with alignment and resize controls. Select the image to see the toolbar:</p>

<img src="https://images.unsplash.com/photo-1506905925346-21bda4d32df4?w=800&auto=format&fit=crop" alt="Mountain landscape" data-align="center" data-width="600" data-show-caption="true" data-caption="Beautiful mountain landscape at sunset" />

<p>Embed YouTube videos with custom playback options. Select the video to adjust settings:</p>

<div data-youtube-video data-align="center" data-width="640">
  <iframe src="https://www.youtube.com/watch?v=jNQXAC9IVRw"></iframe>
</div>

<p>Start editing and make this document your own! ✨</p>
`

export interface NotifuseEditorProps {
  placeholder?: string
  initialContent?: string
  styleConfig?: EditorStyleConfig
  disableH1?: boolean
  showHeader?: boolean
  onChange?: (json: any) => void
  onTableOfContentsUpdate?: (anchors: TOCAnchor[], isCreate?: boolean) => void
}

export interface EditorProviderProps {
  placeholder?: string
  initialContent?: string
  styleConfig?: EditorStyleConfig
  disableH1?: boolean
  showHeader?: boolean
  onChange?: (json: any) => void
  onTableOfContentsUpdate?: (anchors: TOCAnchor[], isCreate?: boolean) => void
}

/**
 * EditorContent component that renders the actual editor
 */
export function EditorContentArea() {
  const { editor } = useContext(EditorContext)!
  const { isDragging } = useControls(editor)

  if (!editor) {
    return null
  }

  return (
    <EditorContent
      editor={editor}
      role="presentation"
      className="notifuse-editor-content"
      style={{
        cursor: isDragging ? 'grabbing' : 'auto'
      }}
    >
      <BlockActionsMenu />
      <EmojiMenu editor={editor} />
      <SlashMenu editor={editor} />
      <SelectionToolbar />
      <CodeBlockToolbar />
    </EditorContent>
  )
}

/**
 * Component that creates and provides the editor instance
 */
export const EditorProvider = forwardRef<NotifuseEditorRef, EditorProviderProps>((props, ref) => {
  const {
    placeholder = 'Start writing...',
    initialContent = DEFAULT_INITIAL_CONTENT,
    styleConfig = defaultEditorStyles,
    disableH1 = false,
    showHeader = true,
    onChange,
    onTableOfContentsUpdate
  } = props
  const editorStyles = useEditorStyles(styleConfig)

  const editor = useEditor({
    immediatelyRender: false,
    content: initialContent,
    onUpdate: ({ editor }) => {
      if (onChange) {
        onChange(editor.getJSON())
      }
    },
    editorProps: {
      attributes: {
        class: 'notifuse-editor'
      }
    },
    extensions: [
      StarterKit.configure({
        heading: {
          levels: disableH1 ? [2, 3, 4, 5, 6] : [1, 2, 3, 4, 5, 6]
        },
        undoRedo: {
          depth: 100,
          newGroupDelay: 500
        },
        horizontalRule: false,
        codeBlock: false,
        dropcursor: {
          width: 2
        }
      }),
      CodeBlockLowlightExtension.configure({
        lowlight,
        enableTabIndentation: true,
        tabSize: 2,
        defaultLanguage: 'plaintext'
      }),
      HorizontalRuleExtension,
      ImageExtension.configure({
        inline: false,
        allowBase64: true // Allow base64 images for paste support
      }),
      YoutubeExtension.configure({
        width: 560,
        height: 315,
        controls: true,
        allowFullscreen: true,
        nocookie: true, // Better privacy - use youtube-nocookie.com
        modestBranding: true, // Hide YouTube logo for cleaner look
        ccLoadPolicy: true // Show captions by default for accessibility
      }),
      TextAlign.configure({ types: ['heading', 'paragraph'] }),
      Placeholder.configure({
        placeholder,
        emptyNodeClass: 'is-empty with-slash'
      }),
      Emoji.configure({
        emojis: gitHubEmojis.filter((emoji: any) => !emoji.name.includes('regional')),
        forceFallbackImages: true
      }),
      TextStyle,
      Color,
      Highlight.configure({ multicolor: true }),
      Subscript,
      Superscript,
      Typography,
      ControlsExtension.configure({
        disableH1
      }),
      BackgroundExtension,
      AlignmentExtension,
      TableOfContents.configure({
        anchorTypes: ['heading'],
        onUpdate: onTableOfContentsUpdate
      })
    ]
  })

  // Expose ref API to parent component
  useImperativeHandle(
    ref,
    () => ({
      getJSON: () => editor?.getJSON() ?? null,
      getHTML: () => editor?.getHTML() ?? '',
      getCSS: () => generateBlogPostCSS(styleConfig),
      undo: () => editor?.chain().focus().undo().run(),
      redo: () => editor?.chain().focus().redo().run(),
      canUndo: () => editor?.can().undo() ?? false,
      canRedo: () => editor?.can().redo() ?? false,
      editor: editor
    }),
    [editor, styleConfig]
  )

  if (!editor) {
    return 'Loading...'
  }

  return (
    <div className="notifuse-editor-wrapper" style={editorStyles}>
      <EditorContext.Provider value={{ editor }}>
        {showHeader && <EditorHeader />}
        <EditorContentArea />
      </EditorContext.Provider>
    </div>
  )
})

EditorProvider.displayName = 'EditorProvider'

/**
 * Full editor with all necessary providers, ready to use
 */
export const NotifuseEditor = forwardRef<NotifuseEditorRef, NotifuseEditorProps>(
  (
    {
      placeholder = 'Start writing...',
      initialContent,
      styleConfig = defaultEditorStyles,
      disableH1 = false,
      showHeader = true,
      onChange,
      onTableOfContentsUpdate
    },
    ref
  ) => {
    return (
      <EditorProvider
        ref={ref}
        placeholder={placeholder}
        initialContent={initialContent}
        styleConfig={styleConfig}
        disableH1={disableH1}
        showHeader={showHeader}
        onChange={onChange}
        onTableOfContentsUpdate={onTableOfContentsUpdate}
      />
    )
  }
)

NotifuseEditor.displayName = 'NotifuseEditor'

// Export default styles and utility functions for external use
export { defaultEditorStyles } from './config/defaultEditorStyles'
export { generateBlogPostCSS } from './utils/styleUtils'
export { validateStyleConfig } from './utils/validateStyleConfig'
export type { EditorStyleConfig } from './types/EditorStyleConfig'
