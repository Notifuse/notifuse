# Blog Editor Debug Page

## Access the Debug Page

The blog editor debug page is available at:

```
/console/workspace/{workspaceId}/debug-editor
```

For example:

```
http://localhost:5173/console/workspace/your-workspace-id/debug-editor
```

## Features

The debug page provides:

1. **Live Editor Testing** - Full-featured blog editor with sample content
2. **Control Buttons**:

   - Save (Simulated) - Test the save functionality
   - Reset to Initial Content - Restore sample content
   - Clear Editor - Start with empty editor
   - Copy JSON to Clipboard - Get the current content as JSON

3. **JSON Preview** - See the live JSON structure as you edit

4. **Sample Content** - Pre-loaded with various block types:
   - Headings (H1, H2)
   - Paragraphs with formatting
   - Bullet lists
   - Code blocks
   - Blockquotes
   - Bold, italic, underline, code

## What to Test

### Drag Handle

- Hover over any block to see the drag handle (grip icon)
- Click and drag to reorder blocks

### Floating Menu (+ Button)

- Click anywhere in a block to see the + button
- Click it to open the block insertion menu
- Insert various types of blocks

### Slash Commands

- Type `/` to open the slash command menu
- Search and select commands

### Text Formatting

- Select text to see the bubble menu
- Apply bold, italic, underline, code, etc.

### Block Types

- Try creating different block types via the + button or / commands:
  - Headings (1-6)
  - Paragraphs
  - Lists (bullet, ordered, task)
  - Code blocks
  - Blockquotes
  - Horizontal rules

## Quick Navigation

You can bookmark this URL for quick access during development:

```
http://localhost:5173/console/workspace/YOUR_WORKSPACE_ID/debug-editor
```

Replace `YOUR_WORKSPACE_ID` with your actual workspace ID.
