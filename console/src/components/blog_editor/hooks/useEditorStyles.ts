import { useMemo } from 'react'
import type { EditorStyleConfig } from '../types/EditorStyleConfig'
import { validateStyleConfig } from '../utils/validateStyleConfig'
import { generateEditorCSSVariables } from '../utils/styleUtils'

/**
 * Hook to validate and convert EditorStyleConfig to CSS variables
 * Returns inline styles object ready to apply to the editor wrapper
 *
 * @param config - Editor style configuration
 * @returns CSS properties object with CSS custom properties
 * @throws StyleConfigValidationError if config is invalid
 */
export function useEditorStyles(config: EditorStyleConfig): React.CSSProperties {
  return useMemo(() => {
    // Validate configuration
    const validatedConfig = validateStyleConfig(config)

    // Generate CSS variables
    return generateEditorCSSVariables(validatedConfig)
  }, [config])
}
