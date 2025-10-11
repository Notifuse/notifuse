import { Workspace } from '../services/api/types'

interface CustomFieldLabelResult {
  displayLabel: string
  technicalName: string
  showTooltip: boolean
}

/**
 * Get the default label for a custom field key
 * e.g., "custom_string_1" => "Custom String 1"
 */
function getDefaultLabel(key: string): string {
  const parts = key.split('_')
  if (parts.length >= 3 && parts[0] === 'custom') {
    const type = parts[1].charAt(0).toUpperCase() + parts[1].slice(1)
    const number = parts[2]
    return `Custom ${type} ${number}`
  }
  return key
}

/**
 * Pure utility function to compute custom field label result
 * Does not use React hooks - can be called from anywhere
 */
function computeCustomFieldLabel(
  fieldKey: string,
  workspace: Workspace | null | undefined
): CustomFieldLabelResult {
  const defaultLabel = getDefaultLabel(fieldKey)
  const customLabel = workspace?.settings?.custom_field_labels?.[fieldKey]

  return {
    displayLabel: customLabel || defaultLabel,
    technicalName: fieldKey,
    showTooltip: !!customLabel // Only show tooltip if there's a custom label
  }
}

/**
 * Hook to get display label for a custom field with its technical name
 * Falls back to default label if no custom label is set
 */
export function useCustomFieldLabel(
  fieldKey: string,
  workspace: Workspace | null | undefined
): CustomFieldLabelResult {
  return computeCustomFieldLabel(fieldKey, workspace)
}

/**
 * Get the display label for a custom field (without the full result object)
 * Pure function - does not use React hooks
 */
export function getCustomFieldLabel(
  fieldKey: string,
  workspace: Workspace | null | undefined
): string {
  const result = computeCustomFieldLabel(fieldKey, workspace)
  return result.displayLabel
}
