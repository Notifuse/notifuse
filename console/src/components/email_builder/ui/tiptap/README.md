# Tiptap editors of the email builder

Two editors edit the HTML that MJML blocks hold: `TiptapRichEditor` for Text blocks (paragraphs, lists, quotes) and `TiptapInlineEditor` for button text (inline content only, no line breaks).

```
tiptap/
├── TiptapRichEditor.tsx      # Text block editor
├── TiptapInlineEditor.tsx    # Button text editor
├── TiptapSchema.ts           # textStyle mark and its setInlineStyle command
├── components/
│   └── TiptapToolbar.tsx     # Toolbar, colour pickers, link popover
└── shared/
    ├── extensions.ts         # Extension sets, link mark, block styles, images, Liquid-safe input rules
    ├── inlineStyle.ts        # Reads and edits style attributes as written
    ├── utils.ts              # Content preparation, getEditorHTML, colours, links
    ├── styles.ts             # Editor CSS
    └── types.ts
```

## Rules the code holds

- **Styles are kept as written.** Email HTML carries Liquid in styles (`color: {{ brand_color }}`) and hex colours. The browser's CSS parser drops the first and rewrites the second, so styles are never read or written through it: `inlineStyle.ts` edits the attribute text, one property at a time.
- **Save with `getEditorHTML(editor)`, never `editor.getHTML()`.** ProseMirror's serializer writes every style through the CSSOM. Each style is also rendered as `data-raw-style`, which `getEditorHTML` puts back and removes.
- **Loading keeps what it cannot edit.** Paragraphs, lists and quotes keep their style; a styled `<strong>` stays bold; images in text are kept. Headings and `<div>`s are loaded as paragraphs whose text keeps their text style.
- **Link colours go on the link.** `setInlineStyle('color', …)` on linked text styles the `<a>`; removing a link moves its colours to the text.
- **Liquid markup is never rewritten by input rules** (smart quotes, Markdown shortcuts); see `isInsideLiquidMarkup`.

## Tests

`richContent.test.ts` (loading and saving, colours), `links.test.ts`, `components/TiptapToolbar.test.tsx` (toolbar on the real editor), `TiptapEditors.test.tsx` (components), `shared/*.test.ts`.
