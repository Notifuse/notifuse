import { useLingui } from '@lingui/react/macro'
import type { SaveBlocker } from './segmentDrawerState'

/** Why the segment cannot be saved yet, worded for the summary and the Save button's tooltip. */
export function useSaveBlockerText(id: string): Record<SaveBlocker, string> {
  const { t } = useLingui()
  return {
    condition_open: t`A condition is open: confirm or cancel it.`,
    no_condition: t`Add at least one condition.`,
    empty_group: t`A group is empty: add a condition to it or remove it.`,
    name_missing: t`Give the segment a name.`,
    id_empty: t`The name needs at least one Latin letter or digit, to make the segment's ID.`,
    id_checking: t`Checking that the ID ${id} is free…`,
    id_taken: t`A segment already has the ID ${id}: change the name.`
  }
}
