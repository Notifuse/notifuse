# Editor Style Presets

Pre-configured style presets for common use cases.

## Available Presets

### 1. Times Journal (`timesJournalPreset`)

**Traditional newspaper typography**

- Classic serif fonts (Georgia/Times New Roman)
- 18px body text for readability
- 1.7 line height (generous spacing)
- Bold headlines (40px H1, 30px H2, 24px H3)
- Traditional blue links (#0055aa)
- Perfect for: News articles, editorials, long-form journalism

### 2. Modern Magazine (`modernMagazinePreset`)

**Contemporary design**

- System sans-serif fonts
- 17px body text
- 1.75 line height (spacious)
- Large, bold headlines (48px H1, 32px H2, 24px H3)
- Vibrant blue links (#2563eb)
- Perfect for: Lifestyle blogs, tech articles, modern publications

### 3. Minimal Blog (`minimalBlogPreset`)

**Medium-inspired clean design**

- System sans-serif fonts
- 18px body text
- 1.6 line height (balanced)
- Moderate headlines (36px H1, 28px H2, 22px H3)
- Subtle gray links (barely visible)
- Perfect for: Personal blogs, minimal design, distraction-free reading

### 4. Academic Paper (`academicPaperPreset`)

**Formal scholarly writing**

- Traditional serif fonts (Georgia/Times New Roman)
- 16px body text (standard academic size)
- 2.0 line height (double-spaced)
- Conservative headlines (24px H1, 20px H2, 18px H3)
- Classic blue underlined links
- Perfect for: Research papers, academic writing, formal documentation

## Usage

### Import a single preset:

```typescript
import { NotifuseEditor, timesJournalPreset } from '@/components/blog_editor'

;<NotifuseEditor styleConfig={timesJournalPreset} />
```

### Import multiple presets:

```typescript
import {
  NotifuseEditor,
  timesJournalPreset,
  modernMagazinePreset,
  minimalBlogPreset,
  academicPaperPreset
} from '@/components/blog_editor'

// Let user choose
const presets = {
  journal: timesJournalPreset,
  magazine: modernMagazinePreset,
  blog: minimalBlogPreset,
  academic: academicPaperPreset
}

<NotifuseEditor styleConfig={presets[selectedPreset]} />
```

### Customize a preset:

```typescript
import { timesJournalPreset } from '@/components/blog_editor'

const customJournal = {
  ...timesJournalPreset,
  h1: {
    ...timesJournalPreset.h1,
    color: '#8b0000' // Change to dark red
  },
  link: {
    color: '#8b0000',
    hoverColor: '#660000'
  }
}

<NotifuseEditor styleConfig={customJournal} />
```

## Creating Your Own Preset

See the source files in this directory for examples. Each preset is a complete `EditorStyleConfig` object with all required properties.

```typescript
import type { EditorStyleConfig } from '../types/EditorStyleConfig'

export const myCustomPreset: EditorStyleConfig = {
  version: '1.0',
  default: {
    /* ... */
  },
  paragraph: {
    /* ... */
  },
  headings: {
    /* ... */
  },
  h1: {
    /* ... */
  },
  h2: {
    /* ... */
  },
  h3: {
    /* ... */
  },
  caption: {
    /* ... */
  },
  separator: {
    /* ... */
  },
  codeBlock: {
    /* ... */
  },
  blockquote: {
    /* ... */
  },
  inlineCode: {
    /* ... */
  },
  list: {
    /* ... */
  },
  link: {
    /* ... */
  }
}
```

## Comparison

| Preset          | Font  | Size | Line Height | H1 Size | Style        |
| --------------- | ----- | ---- | ----------- | ------- | ------------ |
| Times Journal   | Serif | 18px | 1.7         | 40px    | Traditional  |
| Modern Magazine | Sans  | 17px | 1.75        | 48px    | Contemporary |
| Minimal Blog    | Sans  | 18px | 1.6         | 36px    | Clean        |
| Academic Paper  | Serif | 16px | 2.0         | 24px    | Formal       |
