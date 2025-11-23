# Notifuse Editor - Dynamic Styling Guide

This guide explains how to customize the Notifuse Editor's appearance through the `styleConfig` prop.

## Overview

The Notifuse Editor supports dynamic styling through a type-safe JSON configuration. This allows you to customize fonts, colors, sizes, and spacing for all text elements in the editor.

## Table of Contents

- [Quick Start](#quick-start)
- [Style Configuration Reference](#style-configuration-reference)
- [Integration with Notifuse Go Backend](#integration-with-notifuse-go-backend)
- [Blog Post Rendering](#blog-post-rendering)
- [Examples](#examples)
- [Validation](#validation)

## Quick Start

```typescript
import {
  NotifuseEditor,
  defaultEditorStyles,
  type EditorStyleConfig
} from '@/components/blog_editor'

// Use default styles (styleConfig is optional)
function MyEditor() {
  return <NotifuseEditor />
}

// Or explicitly pass default styles
function MyEditorWithDefaults() {
  return <NotifuseEditor styleConfig={defaultEditorStyles} />
}

// Or customize
const customStyles: EditorStyleConfig = {
  ...defaultEditorStyles,
  h1: {
    ...defaultEditorStyles.h1,
    color: '#ff0000',
    fontSize: { value: 2.5, unit: 'rem' }
  }
}

function MyCustomEditor() {
  return <NotifuseEditor styleConfig={customStyles} />
}
```

## Style Configuration Reference

### EditorStyleConfig

The complete configuration object with all customizable properties:

```typescript
interface EditorStyleConfig {
  version: string // Config version (currently "1.0")
  default: DefaultStyles // Base styles for all text
  paragraph: ParagraphStyles // Paragraph-specific styles
  headings: HeadingStyles // Shared heading styles
  h1: HeadingLevelStyles // H1-specific styles
  h2: HeadingLevelStyles // H2-specific styles
  h3: HeadingLevelStyles // H3-specific styles
  caption: CaptionStyles // Image/code block captions
  separator: SeparatorStyles // Horizontal rule (hr)
  codeBlock: CodeBlockStyles // Code block container
  blockquote: BlockquoteStyles // Blockquote styles
  inlineCode: InlineCodeStyles // Inline code styles
  list: ListStyles // List (ul/ol) styles
  link: LinkStyles // Link styles
}

// Component Props
interface NotifuseEditorProps {
  placeholder?: string
  initialContent?: string
  styleConfig?: EditorStyleConfig // Optional - defaults to defaultEditorStyles
}
```

### CSSValue Type

For type-safe CSS values with units:

```typescript
interface CSSValue {
  value: number
  unit: 'px' | 'rem' | 'em'
}

// Examples:
{ value: 16, unit: 'px' }   // "16px"
{ value: 1.5, unit: 'rem' }  // "1.5rem"
{ value: 2, unit: 'em' }     // "2em"
```

### Default Styles

Base styles applied to all text:

```typescript
interface DefaultStyles {
  fontFamily: string // System font stack recommended (see examples)
  fontSize: CSSValue // Base font size
  color: string // Text color (hex, rgb, rgba, named)
  backgroundColor: string // Editor background color
  lineHeight: number // Line height multiplier (e.g., 1.6)
}
```

### Paragraph Styles

```typescript
interface ParagraphStyles {
  marginTop: CSSValue // Space above paragraphs
  marginBottom: CSSValue // Space below paragraphs
  lineHeight: number // Line height for paragraphs
}
```

### Heading Styles

```typescript
interface HeadingStyles {
  fontFamily: string // Font family for all headings
}

interface HeadingLevelStyles {
  fontSize: CSSValue
  color: string // Can use "inherit" to use default color
  marginTop: CSSValue
  marginBottom: CSSValue
}
```

### Other Style Interfaces

```typescript
interface CaptionStyles {
  fontSize: CSSValue
  color: string
}

interface SeparatorStyles {
  color: string
  marginTop: CSSValue
  marginBottom: CSSValue
}

interface CodeBlockStyles {
  marginTop: CSSValue
  marginBottom: CSSValue
}

interface BlockquoteStyles {
  fontSize: CSSValue
  color: string
  marginTop: CSSValue
  marginBottom: CSSValue
  lineHeight: number
}

interface InlineCodeStyles {
  fontFamily: string
  fontSize: CSSValue
  color: string
  backgroundColor: string
}

interface ListStyles {
  marginTop: CSSValue
  marginBottom: CSSValue
  paddingLeft: CSSValue // Indentation amount
}

interface LinkStyles {
  color: string
  hoverColor: string
}
```

## Integration with Notifuse Go Backend

### Step 1: Store Style Config in Database

Store the `EditorStyleConfig` JSON in your database alongside blog posts or as user preferences.

### Step 2: Pass Config to Editor

```typescript
import {
  NotifuseEditor,
  defaultEditorStyles,
  validateStyleConfig,
  type EditorStyleConfig
} from '@/components/blog_editor'

interface EditorComponentProps {
  configFromBackend?: EditorStyleConfig
}

function BlogEditorComponent({ configFromBackend }: EditorComponentProps) {
  // Use backend config or fall back to defaults
  const styleConfig = configFromBackend || defaultEditorStyles

  // Validate config (throws StyleConfigValidationError on invalid config)
  const validConfig = validateStyleConfig(styleConfig)

  return <NotifuseEditor styleConfig={validConfig} />
}
```

### Step 3: Generate CSS for Blog Post

When saving a blog post, generate the CSS stylesheet:

```typescript
import { generateBlogPostCSS, type EditorStyleConfig } from '@/components/blog_editor'
import { useEditor } from '@tiptap/react'

function SaveBlogPost() {
  const editor = useEditor()
  const styleConfig: EditorStyleConfig = getStyleConfig()

  // Get HTML content
  const html = editor.getHTML()

  // Generate CSS stylesheet with .blog-post scope
  const css = generateBlogPostCSS(styleConfig, '.blog-post')

  // Send to backend
  saveToDB({
    html: html,
    css: css,
    styleConfig: JSON.stringify(styleConfig)
  })
}
```

## Blog Post Rendering

On the Go backend, render blog posts with the generated CSS:

```html
<!-- Blog post page template -->
<!DOCTYPE html>
<html>
  <head>
    <style>
      /* CSS generated from styleConfig */
       {
         {
          .CSS;
        }
      }
    </style>
  </head>
  <body>
    <article class="blog-post">
      <!-- HTML content from editor.getHTML() -->
      {{.HTML}}
    </article>
  </body>
</html>
```

## Style Presets

The editor includes four ready-to-use presets:

### 1. Times Journal

Traditional newspaper typography with classic serif fonts:

```typescript
import { NotifuseEditor, timesJournalPreset } from '@/components/blog_editor'

;<NotifuseEditor styleConfig={timesJournalPreset} />
```

- ✍️ Georgia/Times New Roman serif
- 📰 18px body text
- 📏 1.7 line height
- 🎯 Bold headlines (40px H1)

### 2. Modern Magazine

Clean, contemporary design with sans-serif:

```typescript
import { NotifuseEditor, modernMagazinePreset } from '@/components/blog_editor'

;<NotifuseEditor styleConfig={modernMagazinePreset} />
```

- 🎨 System sans-serif fonts
- 📱 17px body text
- ✨ Spacious 1.75 line height
- 🔵 Vibrant blue links

### 3. Minimal Blog

Distraction-free Medium-inspired design:

```typescript
import { NotifuseEditor, minimalBlogPreset } from '@/components/blog_editor'

;<NotifuseEditor styleConfig={minimalBlogPreset} />
```

- 📖 Clean 18px reading size
- 🎯 Balanced 1.6 line height
- ⚫ Subtle gray links
- 🖼️ Minimal separator lines

### 4. Academic Paper

Formal, structured scholarly writing:

```typescript
import { NotifuseEditor, academicPaperPreset } from '@/components/blog_editor'

;<NotifuseEditor styleConfig={academicPaperPreset} />
```

- 📚 Georgia/Times serif
- 📄 16px standard size
- 📏 Double-spaced (2.0 line height)
- 🔗 Classic blue underlined links

## Custom Examples

### Example 1: Brand Colors

```typescript
import { defaultEditorStyles } from '@/components/blog_editor'

const brandStyles = {
  ...defaultEditorStyles,
  default: {
    ...defaultEditorStyles.default,
    color: '#1a1a1a',
    backgroundColor: '#ffffff'
  },
  h1: {
    ...defaultEditorStyles.h1,
    color: '#2563eb', // Brand primary color
    fontSize: { value: 3, unit: 'rem' }
  },
  h2: {
    ...defaultEditorStyles.h2,
    color: '#3b82f6' // Brand secondary color
  },
  link: {
    color: '#2563eb',
    hoverColor: '#1d4ed8'
  }
}
```

### Example 2: Compact Layout

```typescript
const compactStyles = {
  ...defaultEditorStyles,
  paragraph: {
    marginTop: { value: 0.5, unit: 'rem' },
    marginBottom: { value: 0.5, unit: 'rem' },
    lineHeight: 1.4
  },
  h1: {
    ...defaultEditorStyles.h1,
    marginTop: { value: 1.5, unit: 'rem' },
    marginBottom: { value: 0.5, unit: 'rem' }
  },
  h2: {
    ...defaultEditorStyles.h2,
    marginTop: { value: 1, unit: 'rem' },
    marginBottom: { value: 0.25, unit: 'rem' }
  }
}
```

### Example 3: Dark Theme

```typescript
const darkTheme = {
  ...defaultEditorStyles,
  default: {
    ...defaultEditorStyles.default,
    color: '#e5e7eb',
    backgroundColor: '#1f2937'
  },
  h1: {
    ...defaultEditorStyles.h1,
    color: '#f9fafb'
  },
  h2: {
    ...defaultEditorStyles.h2,
    color: '#f3f4f6'
  },
  h3: {
    ...defaultEditorStyles.h3,
    color: '#e5e7eb'
  },
  link: {
    color: '#60a5fa',
    hoverColor: '#93c5fd'
  },
  blockquote: {
    ...defaultEditorStyles.blockquote,
    color: '#9ca3af'
  }
}
```

### Example 4: Serif Typography

```typescript
const serifStyles = {
  ...defaultEditorStyles,
  default: {
    ...defaultEditorStyles.default,
    fontFamily: 'Georgia, "Times New Roman", Times, serif',
    fontSize: { value: 1.125, unit: 'rem' },
    lineHeight: 1.8
  },
  headings: {
    fontFamily: 'Georgia, "Times New Roman", Times, serif'
  },
  paragraph: {
    ...defaultEditorStyles.paragraph,
    lineHeight: 1.8
  }
}
```

### Example 5: Custom Monospace for Code

```typescript
const customCodeStyles = {
  ...defaultEditorStyles,
  inlineCode: {
    ...defaultEditorStyles.inlineCode,
    // System monospace stack (native fonts)
    fontFamily:
      'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace',
    fontSize: { value: 0.9, unit: 'em' }
  }
}
```

## Validation

The editor automatically validates the style configuration. You can also manually validate:

```typescript
import { validateStyleConfig, StyleConfigValidationError } from '@/components/blog_editor'

try {
  const validConfig = validateStyleConfig(myConfig)
  // Config is valid
} catch (error) {
  if (error instanceof StyleConfigValidationError) {
    console.error('Invalid style config:', error.message)
    // e.g., "h1.fontSize: value must be positive"
  }
}
```

### Validation Rules

- **Font sizes**: Must be positive numbers
- **Margins**: Can be zero or positive (negative values not allowed)
- **Colors**: Must be valid CSS colors (hex, rgb, rgba, hsl, hsla, or named colors)
- **Line heights**: Must be positive numbers (max 10)
- **Font families**: Must be non-empty strings
- **Units**: Must be 'px', 'rem', or 'em'
- **Version**: Must be a non-empty string

## CSS Variable Names

The following CSS variables are injected by the editor:

```css
/* Default styles */
--editor-default-font-family
--editor-default-font-size
--editor-default-color
--editor-default-background-color
--editor-default-line-height

/* Paragraphs */
--editor-paragraph-margin-top
--editor-paragraph-margin-bottom
--editor-paragraph-line-height

/* Headings */
--editor-headings-font-family
--editor-h1-font-size
--editor-h1-color
--editor-h1-margin-top
--editor-h1-margin-bottom
/* ... similar for h2, h3 */

/* Captions */
--editor-caption-font-size
--editor-caption-color

/* Separator */
--editor-separator-color
--editor-separator-margin-top
--editor-separator-margin-bottom

/* Code blocks */
--editor-codeblock-margin-top
--editor-codeblock-margin-bottom

/* Blockquotes */
--editor-blockquote-font-size
--editor-blockquote-color
--editor-blockquote-margin-top
--editor-blockquote-margin-bottom
--editor-blockquote-line-height

/* Inline code */
--editor-inline-code-font-family
--editor-inline-code-font-size
--editor-inline-code-color
--editor-inline-code-background-color

/* Lists */
--editor-list-margin-top
--editor-list-margin-bottom
--editor-list-padding-left

/* Links */
--editor-link-color
--editor-link-hover-color
```

## Notes

- The `version` field is for future compatibility when new style properties are added
- CSS generation is memoized for performance - identical configs return cached CSS
- The `generateBlogPostCSS` function accepts an optional scope class (defaults to `.blog-post`)
- All colors support `inherit` to use parent element colors
- Font families should include fallback fonts (e.g., `"Arial, sans-serif"`)

## Best Practices

### Font Families

**Use native system fonts** for best performance and consistency:

**Sans-serif (body text & headings):**

```typescript
'-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol"'
```

**Monospace (code):**

```typescript
'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace'
```

**Serif (editorial style):**

```typescript
'Georgia, "Times New Roman", Times, serif'
```

These stacks:

- ✅ Work on all browsers without web fonts
- ✅ Provide native performance
- ✅ Use each OS's native design language
- ✅ Include proper emoji support (sans-serif stack)

### Colors

- Use hex codes for solid colors: `#ff0000`
- Use rgba for transparency: `rgba(0, 0, 0, 0.75)`
- Use `inherit` to use parent element colors
- Test contrast for accessibility (WCAG AA minimum: 4.5:1 for body text)

### Sizing

- Prefer `rem` for font sizes (scalable with browser settings)
- Use `em` for margins/padding relative to font size
- Use `px` for fixed spacing that shouldn't scale

## Support

For issues or questions about dynamic styling, refer to the Notifuse Editor documentation or contact support.
