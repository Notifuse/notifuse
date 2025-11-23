import { Extension } from '@tiptap/core'

/**
 * Interface representing the state of editor UI controls and configuration
 */
export interface EditorControls {
  isDragging: boolean
  dragHandleLocked: boolean
  activeMenuId: string | null
  disableH1: boolean
}

/**
 * Default initial state for editor controls
 */
export const INITIAL_EDITOR_CONTROLS: EditorControls = {
  isDragging: false,
  dragHandleLocked: false,
  activeMenuId: null,
  disableH1: false
}

/**
 * Extend Tiptap's core types to include our custom commands and storage
 */
declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    notifuseEditorControls: {
      setDragging: (value: boolean) => ReturnType
      setHandleLock: (value: boolean) => ReturnType
      setActiveMenu: (id: string | null) => ReturnType
      resetControls: () => ReturnType
    }
  }

  interface Storage {
    notifuseEditorControls: EditorControls
  }
}

/**
 * Options for ControlsExtension
 */
export interface ControlsExtensionOptions {
  disableH1?: boolean
}

/**
 * ControlsExtension - Manages UI control state for the Notifuse editor
 *
 * This extension provides a centralized way to manage editor UI state separate
 * from document state, including drag operations, menu visibility, and control locks.
 */
export const ControlsExtension = Extension.create<ControlsExtensionOptions>({
  name: 'notifuseEditorControls',

  addOptions() {
    return {
      disableH1: false
    }
  },

  addStorage() {
    return {
      notifuseEditorControls: { ...INITIAL_EDITOR_CONTROLS }
    }
  },

  addCommands() {
    return {
      setDragging: (value: boolean) => () => {
        this.storage.notifuseEditorControls.isDragging = value
        return true
      },

      setHandleLock: (value: boolean) => () => {
        this.storage.notifuseEditorControls.dragHandleLocked = value
        return true
      },

      setActiveMenu: (id: string | null) => () => {
        this.storage.notifuseEditorControls.activeMenuId = id
        return true
      },

      resetControls: () => () => {
        this.storage.notifuseEditorControls = { ...INITIAL_EDITOR_CONTROLS }
        return true
      }
    }
  },

  onCreate() {
    // Initialize storage on extension creation with options
    this.storage.notifuseEditorControls = {
      ...INITIAL_EDITOR_CONTROLS,
      disableH1: this.options.disableH1 ?? false
    }
  }
})
