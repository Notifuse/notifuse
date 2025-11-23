import type { Editor } from '@tiptap/react'
import type React from 'react'

/**
 * Type of action in the editor
 */
export type ActionType = 'block-op' | 'transform' | 'mark' | 'alignment' | 'custom'

/**
 * Definition of a single editor action
 */
export interface ActionDefinition {
  /** Unique identifier for the action */
  id: string

  /** Type/category of action */
  type: ActionType

  /** Display label for the action */
  label: string

  /** Icon component (from lucide-react or @ant-design/icons) */
  icon: React.ComponentType<{ className?: string; style?: React.CSSProperties }>

  /** Keyboard shortcut (e.g., "mod+d") */
  shortcut?: string

  /** Check if action is available in current editor state */
  checkAvailability: (editor: Editor | null) => boolean

  /** Check if action is currently active (optional) */
  checkActive?: (editor: Editor | null) => boolean

  /** Execute the action (can be sync or async) */
  execute: (editor: Editor | null, context?: any) => boolean | Promise<boolean>

  /** Group name for organizing in menus */
  group?: string

  /** Whether to hide when action is unavailable */
  hideWhenUnavailable?: boolean
}

/**
 * Central registry for all editor actions
 * Provides a single source of truth for action definitions
 */
export class ActionRegistry {
  private actions: Map<string, ActionDefinition> = new Map()

  /**
   * Register a single action definition
   */
  register(definition: ActionDefinition): void {
    if (this.actions.has(definition.id)) {
      console.warn(`Action with id "${definition.id}" is already registered. Overwriting.`)
    }
    this.actions.set(definition.id, definition)
  }

  /**
   * Register multiple action definitions at once
   */
  registerMany(definitions: ActionDefinition[]): void {
    definitions.forEach((def) => this.register(def))
  }

  /**
   * Get an action by its ID
   */
  get(id: string): ActionDefinition | undefined {
    return this.actions.get(id)
  }

  /**
   * Get all actions of a specific type
   */
  getByType(type: ActionType): ActionDefinition[] {
    return Array.from(this.actions.values()).filter((action) => action.type === type)
  }

  /**
   * Get all actions in a specific group
   */
  getByGroup(group: string): ActionDefinition[] {
    return Array.from(this.actions.values()).filter((action) => action.group === group)
  }

  /**
   * Get all registered actions
   */
  getAll(): ActionDefinition[] {
    return Array.from(this.actions.values())
  }

  /**
   * Check if an action is registered
   */
  has(id: string): boolean {
    return this.actions.has(id)
  }

  /**
   * Clear all registered actions (useful for testing)
   */
  clear(): void {
    this.actions.clear()
  }

  /**
   * Get the count of registered actions
   */
  get size(): number {
    return this.actions.size
  }
}

/**
 * Singleton instance of the action registry
 */
export const notifuseActionRegistry = new ActionRegistry()
