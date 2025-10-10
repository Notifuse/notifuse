import { Workspace } from '../services/api/types'

interface CustomFieldLabelResult {
  displayLabel: string
  technicalName: string
  showTooltip: boolean
}

/**
 * Hook to get display label for a custom field with its technical name
 * Falls back to default label if no custom label is set
 */
export function useCustomFieldLabel(
  fieldKey: string,
  workspace: Workspace | null | undefined
): CustomFieldLabelResult {
  // Default labels for custom fields
  const getDefaultLabel = (key: string): string => {
    // Extract the type and number from the field key
    // e.g., "custom_string_1" => "Custom String 1"
    const parts = key.split('_')
    if (parts.length >= 3 && parts[0] === 'custom') {
      const type = parts[1].charAt(0).toUpperCase() + parts[1].slice(1)
      const number = parts[2]
      return `Custom ${type} ${number}`
    }
    return key
  }

  const defaultLabel = getDefaultLabel(fieldKey)
  const customLabel = workspace?.settings?.custom_field_labels?.[fieldKey]

  return {
    displayLabel: customLabel || defaultLabel,
    technicalName: fieldKey,
    showTooltip: !!customLabel // Only show tooltip if there's a custom label
  }
}

/**
 * Get the display label for a custom field (without the full result object)
 */
export function getCustomFieldLabel(
  fieldKey: string,
  workspace: Workspace | null | undefined
): string {
  const result = useCustomFieldLabel(fieldKey, workspace)
  return result.displayLabel
}
