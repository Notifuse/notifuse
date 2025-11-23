/**
 * Action Specs Registry Initialization
 *
 * This file automatically registers all action definitions with the
 * ActionRegistry when imported. Import this file in your editor
 * setup to make all actions available throughout the application.
 */

import { notifuseActionRegistry } from '../ActionRegistry'

// Import all action specification modules
import { blockOperationSpecs } from './block-ops'
import { nodeTransformSpecs } from './node-transforms'
import { textMarkSpecs } from './text-marks'
import { textAlignmentSpecs } from './text-alignment'
import { linkColorSpecs } from './link-color-actions'

/**
 * Register all action definitions with the registry
 */
notifuseActionRegistry.registerMany([
  ...blockOperationSpecs,
  ...nodeTransformSpecs,
  ...textMarkSpecs,
  ...textAlignmentSpecs,
  ...linkColorSpecs
])

/**
 * Re-export all action specs and individual actions for direct access
 */
export * from './block-ops'
export * from './node-transforms'
export * from './text-marks'
export * from './text-alignment'
export * from './link-color-actions'

/**
 * Export the registry instance for direct access
 */
export { notifuseActionRegistry } from '../ActionRegistry'

/**
 * Export consumer hooks
 */
export { useAction } from '../useAction'
export { useActions, useActionsArray } from '../useActions'

/**
 * Export types
 */
export type { ActionDefinition, ActionType } from '../ActionRegistry'
export type { ActionState, UseActionConfig } from '../useAction'
export type { BatchActionState, UseActionsConfig } from '../useActions'
