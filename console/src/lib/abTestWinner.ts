import type { Broadcast } from '../services/api/broadcast'

// canSelectWinner reports whether the owner can pick an A/B test's winner from the
// variations table. A completed test offers it whether or not the winner is chosen
// automatically: the automatic choice skips variations with no tracked send for its
// metric and leaves the choice to the owner when none has any, and the server
// accepts a manual choice for any broadcast in test_completed.
export function canSelectWinner(broadcast: Pick<Broadcast, 'status'>): boolean {
  return broadcast.status === 'test_completed'
}
