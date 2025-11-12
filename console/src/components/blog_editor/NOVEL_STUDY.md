# Novel Editor Implementation Study

## Overview

This document analyzes Novel's implementation of Notion-like editor features, particularly focusing on drag handlers, block insertion UI, and focus behavior. Novel uses Tiptap 2.x with several custom extensions and a global drag handle package.

## Key Findings

### 1. Drag Handle Implementation

Novel uses the `tiptap-extension-global-drag-handle` package (version 0.1.16) instead of implementing their own drag handle from scratch. This is a **critical difference** from our current custom implementation.

#### How GlobalDragHandle Works

**Package:** `tiptap-extension-global-drag-handle`

**Installation:**

```bash
npm install tiptap-extension-global-drag-handle
```

**Key Features:**

1. **Single Global Handle Element:**

   - Creates ONE drag handle element that repositions on mousemove
   - Uses `position: fixed` instead of decorations
   - Much more performant than creating multiple handles

2. **Positioning Strategy:**

   - Listens to `mousemove` events on the editor
   - Uses `document.elementsFromPoint()` to find the node under cursor
   - Calculates position based on node's bounding rect
   - Dynamically updates handle position with `style.left` and `style.top`

3. **Show/Hide Logic:**

   ```javascript
   // Shows on mousemove when hovering over eligible nodes
   function showDragHandle() {
     if (dragHandleElement) {
       dragHandleElement.classList.remove('hide')
     }
   }

   // Hides on keydown, mousewheel, or when leaving editor
   function hideDragHandle() {
     if (dragHandleElement) {
       dragHandleElement.classList.add('hide')
     }
   }
   ```

4. **Node Selection:**

   - Selects nodes by their position, not by decoration widgets
   - Targets specific selectors: `li`, `p:not(:first-child)`, `pre`, `blockquote`, `h1-h6`
   - Supports custom nodes via `data-type` attributes

5. **Drag and Drop:**
   - Uses native HTML5 drag and drop API
   - Serializes selection content for clipboard
   - Properly handles list items (preserves OL vs UL type)
   - Includes auto-scroll when dragging near screen edges

#### CSS Styling

Novel's drag handle is **headless** (no default styling), styled via `.drag-handle` class:

```css
.drag-handle {
  position: fixed; /* NOT absolute - fixed relative to viewport */
  opacity: 1;
  transition: opacity ease-in 0.2s;
  border-radius: 0.25rem;

  /* Uses SVG as background image for the grip dots icon */
  background-image: url('data:image/svg+xml...');
  background-size: calc(0.5em + 0.375rem) calc(0.5em + 0.375rem);
  background-repeat: no-repeat;
  background-position: center;

  width: 1.2rem;
  height: 1.5rem;
  z-index: 50;
  cursor: grab;
}

.drag-handle:hover {
  background-color: var(--novel-stone-100);
  transition: background-color 0.2s;
}

.drag-handle:active {
  background-color: var(--novel-stone-200);
  cursor: grabbing;
}

.drag-handle.hide {
  opacity: 0;
  pointer-events: none; /* Important! Prevents clicks on hidden handle */
}
```

**Key CSS Insights:**

- Uses `position: fixed` not `absolute` for better performance
- Single element repositioned via JS, not CSS positioning
- `pointer-events: none` when hidden prevents interaction issues
- SVG background for the icon instead of React components

### 2. Block Insertion ("+" Button) Implementation

**Important Discovery:** Novel does NOT implement a traditional "+" button that appears on hover. Instead, they use:

#### Slash Commands (`/`)

Novel's primary block insertion method is typing `/` which triggers a command palette:

```typescript
// Configured via @tiptap/suggestion
const Command = Extension.create({
  name: 'slash-command',
  addOptions() {
    return {
      suggestion: {
        char: '/',
        command: ({ editor, range, props }) => {
          props.command({ editor, range })
        }
      }
    }
  }
})
```

**Key Implementation Details:**

1. **Suggestion Extension:**

   - Uses `@tiptap/suggestion` for autocomplete functionality
   - Renders menu using `tippy.js` for positioning
   - Uses `cmdk` (Command Menu) component for the UI

2. **Menu Positioning:**

   - Attaches to document body
   - Uses `getReferenceClientRect` for positioning relative to cursor
   - Placement: `bottom-start`

3. **Keyboard Navigation:**

   - ArrowUp/Down to navigate
   - Enter to select
   - Escape to close
   - Handles events at document level, dispatches to menu

4. **State Management:**
   - Uses Jotai for global state
   - Two atoms: `queryAtom` (search text) and `rangeAtom` (insertion position)
   - Tunnel-rat for rendering menu in portal while keeping React context

#### Bubble Menu (Text Selection Menu)

Novel's "block type switcher" appears in the bubble menu when text is selected:

```typescript
// EditorBubble component wraps Tiptap's BubbleMenu
const shouldShow = ({ editor, state }) => {
  const { selection } = state
  const { empty } = selection

  // Don't show if:
  // - editor not editable
  // - image selected
  // - selection empty
  // - node selection (for drag handles)
  if (!editor.isEditable || editor.isActive('image') || empty || isNodeSelection(selection)) {
    return false
  }
  return true
}
```

**Key Points:**

- Uses Tiptap's `BubbleMenu` component
- Appears only on text selection (not on node selection)
- Includes node type selector (Text, H1, H2, etc.)
- Uses Radix UI Popover for dropdowns

### 3. Focus Behavior and State Management

Novel handles focus and state differently than our current implementation:

#### Editor Context

```typescript
// Uses EditorProvider from @tiptap/react
<EditorProvider {...props} content={initialContent}>
  {children}
</EditorProvider>
```

- `EditorProvider` creates a React context for the editor
- `useCurrentEditor()` hook accesses editor anywhere in tree
- No need to pass editor as props

#### State Management

**Global State (Jotai):**

```typescript
// Simple atoms for slash command state
export const queryAtom = atom('')
export const rangeAtom = atom<Range | null>(null)
```

**Tunnel-rat for Portals:**

- Used to render slash command menu in document body
- Maintains React context despite portal rendering
- Solves the "context crossing portal boundaries" problem

#### Focus Preservation

**Key Insight:** Novel uses `view.focus()` before drag operations:

```typescript
function handleDragStart(event, view) {
  view.focus() // Crucial! Maintains editor focus
  // ... rest of drag logic
}
```

**Bubble Menu Focus Handling:**

```typescript
onCreate: (val) => {
  instanceRef.current = val

  // Prevent blur when interacting with bubble menu
  instanceRef.current.popper.firstChild?.addEventListener('blur', (event) => {
    event.preventDefault()
    event.stopImmediatePropagation()
  })
}
```

#### Custom Keyboard Shortcuts

Novel includes a custom Cmd+A behavior:

- First Cmd+A: selects text within current node
- Second Cmd+A: selects entire document
- Provides better UX for node-based selection

## Comparison with Current Implementation

### Our Current DragHandle.tsx vs Novel's GlobalDragHandle

| Feature         | Our Implementation         | Novel's Implementation                    |
| --------------- | -------------------------- | ----------------------------------------- |
| **Approach**    | Decorations with widgets   | Single global element                     |
| **Positioning** | Inline styles per node     | Fixed position, repositioned on mousemove |
| **Performance** | Multiple DOM nodes created | Single DOM node reused                    |
| **Show/Hide**   | CSS transitions            | CSS class toggle (`hide`)                 |
| **Icon**        | React component rendered   | SVG background image                      |
| **Selection**   | Manual node tracking       | ProseMirror position resolution           |

### Our Current FloatingMenu.tsx vs Novel's Approach

| Feature         | Our Implementation                | Novel's Approach                 |
| --------------- | --------------------------------- | -------------------------------- |
| **Trigger**     | Empty lines (Tiptap FloatingMenu) | Slash command (`/`)              |
| **UI Pattern**  | "+" button with dropdown          | Command palette                  |
| **State**       | Local React state                 | Global Jotai atoms               |
| **Positioning** | Tiptap's floating menu logic      | Tippy.js with custom positioning |
| **Search**      | No search                         | Fuzzy search with cmdk           |

## Issues with Current Implementation

Based on Novel's approach, here are the likely issues in our current implementation:

### 1. Drag Handle Issues

**Problem:** Creating decorations for every block is expensive and can cause:

- Performance issues with large documents
- Cursor jumping
- React rendering issues (createRoot for each decoration)
- Complex hover state management

**Solution:** Use `tiptap-extension-global-drag-handle` package or implement a single global handle element.

### 2. Floating Menu Focus Issues

**Problem:** Our current FloatingMenu likely loses focus when clicking the "+" button because:

- FloatingMenu shows on empty lines using `shouldShow` logic
- Clicking button may not properly preserve editor focus
- No explicit `view.focus()` calls

**Solution:**

- Add `view.focus()` before executing commands
- Use `mousedown` instead of `click` to prevent blur
- Add blur prevention logic like Novel's bubble menu

### 3. Position Calculation

**Problem:** Using `left: '-64px'` with inline styles is brittle:

- Doesn't account for modal dialogs
- Doesn't handle transforms
- Hardcoded values don't adapt to layout changes

**Solution:** Use Novel's approach:

- Calculate position using `getBoundingClientRect()`
- Account for modal transforms
- Use fixed positioning

## Actionable Improvements

### Priority 1: Replace Custom Drag Handle

**Action:** Replace our custom `DragHandle.tsx` with `tiptap-extension-global-drag-handle`

**Steps:**

1. Already installed: `tiptap-extension-global-drag-handle@^0.1.16`
2. Import and add to extensions:

   ```typescript
   import GlobalDragHandle from 'tiptap-extension-global-drag-handle'

   extensions: [
     // ... other extensions
     GlobalDragHandle.configure({
       dragHandleWidth: 20,
       scrollTreshold: 100
     })
   ]
   ```

3. Add CSS from Novel's prosemirror.css (lines 131-170)
4. Remove old `DragHandle.tsx` extension
5. Test drag and drop functionality

**Benefits:**

- Proven implementation used in production
- Better performance
- Cleaner code (remove ~180 lines)
- Proper list item handling
- Auto-scroll support

### Priority 2: Improve Focus Handling

**Action:** Add explicit focus preservation in menus

**FloatingMenu Changes:**

```typescript
const handleCommand = (item: CommandItem) => {
  const { view } = editor
  const { from } = view.state.selection

  // Add this line:
  view.focus()

  item.command({
    editor,
    range: { from, to: from }
  })

  setShowMenu(false)
}
```

**BubbleMenu Changes:**

```typescript
// In tippyOptions
onCreate: (instance) => {
  // Prevent blur when clicking bubble menu items
  instance.popper.firstChild?.addEventListener('blur', (event) => {
    event.preventDefault()
    event.stopImmediatePropagation()
  })
}
```

### Priority 3: Consider Slash Command Approach

**Action:** Evaluate if slash commands work better than floating menu

**Pros:**

- More discoverable (users know to type `/`)
- Keyboard-driven workflow
- Searchable commands
- Industry standard (Notion, Linear, etc.)

**Cons:**

- Our current implementation already has slash commands
- Floating menu provides additional discovery method
- Both can coexist

**Recommendation:** Keep both, but ensure they work well together. Test that:

- Slash commands work smoothly
- Floating menu doesn't interfere
- Focus is preserved in both cases

### Priority 4: Use EditorProvider Pattern

**Action:** Consider migrating to `EditorProvider` pattern

**Current:**

```typescript
const editor = useEditor({ ... })
return <EditorContent editor={editor} />
```

**Novel's Pattern:**

```typescript
<EditorProvider {...props}>
  <EditorContent />
  <BubbleMenu />
  <SlashCommand />
</EditorProvider>
```

**Benefits:**

- Cleaner component hierarchy
- Easier to access editor in child components
- No prop drilling
- Follows React best practices

**Consideration:** This is a larger refactor, assess if worth the effort.

## Code Snippets for Implementation

### 1. Add GlobalDragHandle Extension

**File:** `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/blog_editor/extensions/index.ts`

```typescript
// Remove old DragHandle import
// import { DragHandle } from './DragHandle'

// Add new import
import GlobalDragHandle from 'tiptap-extension-global-drag-handle'

export const createBlogExtensions = (placeholder?: string) => [
  // ... other extensions

  // Replace old DragHandle with GlobalDragHandle
  GlobalDragHandle.configure({
    dragHandleWidth: 20,
    scrollTreshold: 100
  })

  // ... rest of extensions
]
```

### 2. Add Drag Handle CSS

**File:** `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/blog_editor/editor.css`

Add this CSS (adapted from Novel):

```css
/* Global Drag Handle Styling */
.drag-handle {
  position: fixed;
  opacity: 1;
  transition: opacity ease-in 0.2s;
  border-radius: 0.25rem;

  /* Grip dots icon */
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 10 10' style='fill: rgba(0, 0, 0, 0.5)'%3E%3Cpath d='M3,2 C2.44771525,2 2,1.55228475 2,1 C2,0.44771525 2.44771525,0 3,0 C3.55228475,0 4,0.44771525 4,1 C4,1.55228475 3.55228475,2 3,2 Z M3,6 C2.44771525,6 2,5.55228475 2,5 C2,4.44771525 2.44771525,4 3,4 C3.55228475,4 4,4.44771525 4,5 C4,5.55228475 3.55228475,6 3,6 Z M3,10 C2.44771525,10 2,9.55228475 2,9 C2,8.44771525 2.44771525,8 3,8 C3.55228475,8 4,8.44771525 4,9 C4,9.55228475 3.55228475,10 3,10 Z M7,2 C6.44771525,2 6,1.55228475 6,1 C6,0.44771525 6.44771525,0 7,0 C7.55228475,0 8,0.44771525 8,1 C8,1.55228475 7.55228475,2 7,2 Z M7,6 C6.44771525,6 6,5.55228475 6,5 C6,4.44771525 6.44771525,4 7,4 C7.55228475,4 8,4.44771525 8,5 C8,5.55228475 7.55228475,6 7,6 Z M7,10 C6.44771525,10 6,9.55228475 6,9 C6,8.44771525 6.44771525,8 7,8 C7.55228475,8 8,8.44771525 8,9 C8,9.55228475 7.55228475,10 7,10 Z'%3E%3C/path%3E%3C/svg%3E");
  background-size: calc(0.5em + 0.375rem) calc(0.5em + 0.375rem);
  background-repeat: no-repeat;
  background-position: center;

  width: 1.2rem;
  height: 1.5rem;
  z-index: 50;
  cursor: grab;
}

.drag-handle:hover {
  background-color: #f3f4f6;
  transition: background-color 0.2s;
}

.drag-handle:active {
  background-color: #e5e7eb;
  cursor: grabbing;
}

.drag-handle.hide {
  opacity: 0;
  pointer-events: none;
}

@media screen and (max-width: 600px) {
  .drag-handle {
    display: none;
    pointer-events: none;
  }
}

/* Selected node styling during drag */
.ProseMirror:not(.dragging) .ProseMirror-selectednode {
  outline: none !important;
  background-color: #dbeafe;
  transition: background-color 0.2s;
  box-shadow: none;
}
```

### 3. Fix Focus in FloatingMenu

**File:** `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/blog_editor/toolbar/FloatingMenu.tsx`

```typescript
const handleCommand = (item: CommandItem) => {
  const { view } = editor
  const { from } = view.state.selection

  // Preserve focus before executing command
  view.focus()

  item.command({
    editor,
    range: { from, to: from }
  })

  setShowMenu(false)
}
```

### 4. Fix Focus in BubbleMenu

**File:** `/Users/pierre/Sites/notifuse3/code/notifuse/console/src/components/blog_editor/toolbar/BubbleMenu.tsx`

Add to tippyOptions:

```typescript
<TiptapBubbleMenu
  editor={editor}
  tippyOptions={{
    duration: 100,
    placement: 'top',
    // Add this:
    onCreate: (instance) => {
      // Prevent blur when interacting with menu
      const menuElement = instance.popper.firstChild
      if (menuElement) {
        menuElement.addEventListener('blur', (event) => {
          event.preventDefault()
          event.stopImmediatePropagation()
        })
      }
    },
    // Add this too for smooth transitions:
    moveTransition: 'transform 0.15s ease-out',
  }}
  // ...
```

## Testing Checklist

After implementing changes, test:

- [ ] Drag handle appears on hover over blocks
- [ ] Drag handle disappears on scroll, keyboard input, mouse leave
- [ ] Drag and drop reorders blocks correctly
- [ ] List items maintain their type (OL vs UL) when dragged
- [ ] Drag handle cursor changes (grab → grabbing)
- [ ] Focus is maintained when clicking floating menu
- [ ] Focus is maintained when clicking bubble menu
- [ ] Slash commands still work
- [ ] No cursor jumping or selection issues
- [ ] Performance is smooth with large documents (50+ blocks)
- [ ] Works in modal dialogs if applicable
- [ ] Responsive behavior (handle hidden on mobile)

## Additional Resources

- **GlobalDragHandle Package:** [npm](https://www.npmjs.com/package/tiptap-extension-global-drag-handle)
- **Novel Repository:** https://github.com/steven-tey/novel
- **Tiptap Documentation:** https://tiptap.dev/
- **Novel's Prosemirror CSS:** `/Users/pierre/Sites/notifuse3/code/novel-study/apps/web/styles/prosemirror.css`

## Conclusion

The main learnings from studying Novel are:

1. **Don't reinvent the wheel:** Use `tiptap-extension-global-drag-handle` instead of custom implementation
2. **Single element is better:** One repositioned handle performs better than multiple decorations
3. **Focus preservation is critical:** Explicitly call `view.focus()` and prevent blur events
4. **Fixed positioning works better:** Use `position: fixed` for drag handles
5. **Simple state management:** Jotai atoms are sufficient for editor state
6. **Slash commands > Floating menu:** For block insertion, slash commands are more discoverable

Implementing Priority 1 (GlobalDragHandle) and Priority 2 (Focus handling) will likely resolve most of the current issues with the blog editor's Notion-like features.

---

**Document Created:** November 12, 2025
**Novel Version Studied:** 1.0.0 (GlobalDragHandle 0.1.16)
**Study Location:** `/Users/pierre/Sites/notifuse3/code/novel-study`
