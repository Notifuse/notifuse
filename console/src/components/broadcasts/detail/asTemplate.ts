import type { BroadcastVariation } from '../../../services/api/broadcast'
import type { Template } from '../../../services/api/template'

/** The template a broadcast joins to a variant, as the template screens type it. */
export const asTemplate = (variation: BroadcastVariation) =>
  variation.template as unknown as Template | undefined
