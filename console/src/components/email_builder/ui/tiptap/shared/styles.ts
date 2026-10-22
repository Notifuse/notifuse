// Shared CSS styles for Tiptap editors
export const injectTiptapStyles = () => {
  const styleId = 'tiptap-shared-styles'

  // Looked up in the head: content shown on the page may carry any id
  if (!document.head.querySelector(`style#${styleId}`)) {
    const style = document.createElement('style')
    style.id = styleId
    style.textContent = `
      /* Base ProseMirror styles */
      .ProseMirror {
        border: none !important;
        outline: none !important;
        box-shadow: none !important;
        background-color: transparent !important;
      }
      
      .ProseMirror:focus {
        border: none !important;
        outline: none !important;
        box-shadow: none !important;
      }
      
      .ProseMirror-focused {
        border: none !important;
        outline: none !important;
        box-shadow: none !important;
      }
      
      /* Inline mode specific styles */
      .ProseMirror[data-inline-mode="true"] {
        display: inline !important;
      }
      
      .ProseMirror span[data-inline-doc] {
        display: inline !important;
        margin: 0 !important;
        padding: 0 !important;
      }
      
      /* Emoji Popover Styles */
      .tiptap-emoji-popover .ant-popover-content {
        z-index: 99999 !important;
      }
      
      .tiptap-emoji-popover .ant-popover-inner {
        padding: 8px !important;
        border-radius: 8px !important;
        box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15) !important;
      }
      
      .tiptap-emoji-popover .ant-popover-arrow {
        display: none !important;
      }
      
      /* Ensure emoji picker is fully visible */
      .tiptap-emoji-popover em-emoji-picker {
        background: white !important;
        border-radius: 8px !important;
      }
      
      /* Link popover styles */
      .tiptap-link-popover .ant-popover-content {
        z-index: 99999 !important;
        position: fixed !important;
      }
      
      .tiptap-link-popover .ant-popover-inner {
        padding: 8px !important;
        border-radius: 8px !important;
        box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15) !important;
      }
    `
    document.head.appendChild(style)
  }
}

// Default toolbar style configuration
export const defaultToolbarStyle = {
  fontSize: '16px',
  lineHeight: '16px',
  top: -40,
  width: 'max-content',
  minWidth: 'fit-content',
  maxWidth: 'none',
  left: '50%',
  transform: 'translateX(-50%)'
}

// Default toolbar classes
export const defaultToolbarClasses =
  'flex items-center px-2 py-1 bg-black/80 backdrop-blur-md rounded-md gap-1 flex-wrap absolute z-10'

// Toolbar button classes
export const getToolbarButtonClasses = (isActive: boolean, disabled: boolean) => `
  flex items-center justify-center text-white
  transition-colors duration-200 rounded-xs
  ${isActive ? ' text-white border-1 border-primary' : 'bg-transparent hover:bg-white/10'}
  ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}
`

// Toolbar separator classes
export const toolbarSeparatorClasses = 'w-px h-6 bg-white/20 mx-1.5 flex-shrink-0'
