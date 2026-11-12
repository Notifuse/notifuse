import type { ComposeValues } from './composeValues'

/** How a card changes the composer's values: from the latest ones, so two quick changes both land. */
export type ComposeUpdate = (update: (values: ComposeValues) => ComposeValues) => void
