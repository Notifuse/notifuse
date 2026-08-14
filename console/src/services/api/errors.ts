/**
 * API error types.
 *
 * These live apart from client.ts because that module imports the router, and the router
 * builds routes at module scope. A component that only wants to inspect an error's status
 * would otherwise drag the whole router graph into its test environment, where a shallow
 * router mock cannot satisfy it.
 */
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public data?: unknown
  ) {
    super(message)
    this.name = 'ApiError'
  }
}
