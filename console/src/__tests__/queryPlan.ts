import type { QueryClient } from '@tanstack/react-query'

/** The key of each query a render made, and the refetch interval its observer asked for. */
export function queryPlan(client: QueryClient): { key: readonly unknown[]; refetchInterval: unknown }[] {
  return client
    .getQueryCache()
    .getAll()
    .map((query) => ({
      key: query.queryKey,
      refetchInterval: query.observers[0]?.options.refetchInterval
    }))
}
