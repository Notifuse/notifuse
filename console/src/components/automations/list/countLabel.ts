import { plural } from '@lingui/core/macro'

/** "{n} automations", for the list's footer. */
export const automationsCountLabel = (n: number) => plural(n, { one: '# automation', other: '# automations' })
