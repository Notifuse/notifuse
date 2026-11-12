import type { Automation, JourneyCounts } from '../../services/api/automation'

/** Journeys that ended: completed, exited or failed. The rates of the list are over them. */
export const finishedOf = (c: JourneyCounts) => c.completed + c.exited + c.failed

const rate = (part: number, whole: number) => (whole > 0 ? part / whole : null)

export const completedRate = (c: JourneyCounts) => rate(c.completed, finishedOf(c))
export const exitedRate = (c: JourneyCounts) => rate(c.exited, finishedOf(c))

/** A draft no contact ever entered: its journey cells read "—" rather than 0. */
export const neverRan = (automation: Pick<Automation, 'status'>, counts: JourneyCounts | undefined) =>
  automation.status === 'draft' && (counts?.total ?? 0) === 0

export const hasEmailStep = (automation: Pick<Automation, 'nodes'>) =>
  (automation.nodes ?? []).some((node) => node.type === 'email')
