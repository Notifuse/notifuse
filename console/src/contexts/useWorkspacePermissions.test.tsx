import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import {
  AuthContext,
  useWorkspacePermissions,
  type User,
  type WorkspacePermissions
} from './AuthContext'
import { workspaceService } from '../services/api/workspace'
import { createEmptyPermissions, createFullPermissions } from '../services/api/permissions'
import type { GetWorkspaceMembersResponse, UserPermissions } from '../services/api/types'

vi.mock('../services/api/workspace', () => ({
  workspaceService: { getMembers: vi.fn() }
}))

const getMembers = vi.mocked(workspaceService.getMembers)

const member: User = { id: 'u1', email: 'member@example.com' }
const other: User = { id: 'u2', email: 'other@example.com' }
const root: User = { id: 'u0', email: 'root@example.com' }

// Permissions that tell the workspaces apart: in A the member reads broadcasts, in B templates.
const permsA = {
  ...createEmptyPermissions(),
  broadcasts: { read: true, write: false }
} as UserPermissions
const permsB = {
  ...createEmptyPermissions(),
  templates: { read: true, write: true }
} as UserPermissions

const membersOf = (
  ...entries: { userId?: string; permissions: Partial<UserPermissions>; role?: string }[]
) =>
  ({
    members: entries.map(({ userId = member.id, permissions, role = 'member' }) => ({
      user_id: userId,
      role,
      permissions
    }))
  }) as unknown as GetWorkspaceMembersResponse

// A getMembers answer the test releases when it wants, to order the answers.
const deferred = () => {
  let resolve!: (value: GetWorkspaceMembersResponse) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<GetWorkspaceMembersResponse>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

// Each workspace answers its promises in order; a request the test did not expect fails it.
const answering = (answers: Record<string, Promise<GetWorkspaceMembersResponse>[]>) =>
  getMembers.mockImplementation((id) => {
    const next = answers[id]?.shift()
    if (!next) throw new Error(`unexpected getMembers(${id})`)
    return next
  })

// renderHook's wrapper gets no props: it reads the signed-in user from here, and rerender()
// renders it again.
let currentUser: User | null = member

const newClient = () => new QueryClient({ defaultOptions: { queries: { retry: false } } })

// A second call with the same client mounts another caller over the same query cache, as a page
// mounting under the layout.
const renderPermissions = (
  initialWorkspaceId: string,
  initialUser: User | null = member,
  client = newClient()
) => {
  currentUser = initialUser
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <AuthContext.Provider
        value={{
          user: currentUser,
          workspaces: [],
          isAuthenticated: !!currentUser,
          signin: vi.fn(),
          signout: vi.fn(),
          loading: false,
          refreshWorkspaces: vi.fn(),
          licenseEntitlements: null
        }}
      >
        {children}
      </AuthContext.Provider>
    </QueryClientProvider>
  )
  // Every answer the hook rendered, in order: a value shown for a single render counts too.
  const renders: WorkspacePermissions[] = []
  const view = renderHook(
    ({ workspaceId }: { workspaceId: string }) => {
      const answer = useWorkspacePermissions(workspaceId)
      renders.push(answer)
      return answer
    },
    { initialProps: { workspaceId: initialWorkspaceId }, wrapper: Wrapper }
  )
  const switchTo = (workspaceId: string, user: User | null = currentUser) => {
    currentUser = user
    view.rerender({ workspaceId })
  }
  return { result: view.result, switchTo, renders }
}

// Query results reach the component on a later tick: give any pending answer time to land.
const settle = () => act(() => new Promise((resolve) => setTimeout(resolve, 20)))

const loadingAnswer: WorkspacePermissions = { permissions: null, role: undefined, loading: true }
const emptyAnswer: WorkspacePermissions = { permissions: null, role: undefined, loading: false }

describe('useWorkspacePermissions', () => {
  beforeEach(() => {
    getMembers.mockReset()
    window.ROOT_EMAIL = root.email
  })
  afterEach(() => {
    window.ROOT_EMAIL = ''
  })

  it("never renders the previous workspace's permissions after a switch", async () => {
    const a = deferred()
    const b = deferred()
    answering({ A: [a.promise], B: [b.promise] })
    const { result, switchTo, renders } = renderPermissions('A')

    expect(result.current).toEqual(loadingAnswer)
    a.resolve(membersOf({ permissions: permsA }))
    await settle()
    expect(result.current).toEqual({ permissions: permsA, role: 'member', loading: false })

    const switchedAt = renders.length
    switchTo('B')
    b.resolve(membersOf({ permissions: permsB, role: 'owner' }))
    await settle()

    expect(result.current).toEqual({ permissions: permsB, role: 'owner', loading: false })
    // Each render after the switch is either loading or B's answer, never A's.
    const bAnswer = { permissions: permsB, role: 'owner', loading: false }
    const afterSwitch = renders.slice(switchedAt)
    expect(afterSwitch[0]).toEqual(loadingAnswer)
    for (const answer of afterSwitch) {
      expect([loadingAnswer, bAnswer]).toContainEqual(answer)
    }
  })

  it("ignores the previous workspace's answer when it arrives last", async () => {
    const a = deferred()
    const b = deferred()
    answering({ A: [a.promise], B: [b.promise] })
    const { result, switchTo } = renderPermissions('A')

    switchTo('B')
    b.resolve(membersOf({ permissions: permsB }))
    await settle()
    a.resolve(membersOf({ permissions: permsA }))
    await settle()

    expect(result.current).toEqual({ permissions: permsB, role: 'member', loading: false })
  })

  it("ignores the previous workspace's failure when it arrives last", async () => {
    const a = deferred()
    const b = deferred()
    answering({ A: [a.promise], B: [b.promise] })
    const { result, switchTo } = renderPermissions('A')

    switchTo('B')
    b.resolve(membersOf({ permissions: permsB }))
    await settle()
    a.reject(new Error('timeout'))
    await settle()

    expect(result.current).toEqual({ permissions: permsB, role: 'member', loading: false })
  })

  it("answers with A's own list when coming back to A before it answers", async () => {
    const a = deferred()
    const b = deferred()
    answering({ A: [a.promise], B: [b.promise] })
    const { result, switchTo } = renderPermissions('A')

    switchTo('B')
    switchTo('A')
    b.resolve(membersOf({ permissions: permsB }))
    await settle()
    expect(result.current).toEqual(loadingAnswer)

    a.resolve(membersOf({ permissions: permsA }))
    await settle()
    expect(result.current).toEqual({ permissions: permsA, role: 'member', loading: false })
  })

  it("waits for the new user's own list when the user changes", async () => {
    // Without Workspace read a member is sent only their own entry: another user's list says
    // nothing about the next user.
    const second = deferred()
    answering({
      A: [Promise.resolve(membersOf({ permissions: permsA })), second.promise]
    })
    const { result, switchTo } = renderPermissions('A')
    await waitFor(() => expect(result.current.permissions).toEqual(permsA))

    switchTo('A', other)
    expect(result.current).toEqual(loadingAnswer)

    second.resolve(membersOf({ userId: other.id, permissions: permsB, role: 'owner' }))
    await settle()
    expect(result.current).toEqual({ permissions: permsB, role: 'owner', loading: false })
  })

  it('waits for its own answer when mounted over a cached list, and both callers follow it', async () => {
    const fresh = deferred()
    answering({ A: [Promise.resolve(membersOf({ permissions: permsA })), fresh.promise] })
    const client = newClient()
    const first = renderPermissions('A', member, client)
    await waitFor(() => expect(first.result.current.permissions).toEqual(permsA))

    // The list may predate a change of permissions: the new caller does not use it.
    const second = renderPermissions('A', member, client)
    expect(getMembers).toHaveBeenCalledTimes(2)
    expect(second.result.current).toEqual(loadingAnswer)

    fresh.resolve(membersOf({ permissions: permsB }))
    await settle()
    expect(second.result.current).toEqual({ permissions: permsB, role: 'member', loading: false })
    expect(first.result.current).toEqual({ permissions: permsB, role: 'member', loading: false })
  })

  it('keeps the list it has when a refetch fails', async () => {
    const fails = deferred()
    answering({ A: [Promise.resolve(membersOf({ permissions: permsA })), fails.promise] })
    const client = newClient()
    const first = renderPermissions('A', member, client)
    await waitFor(() => expect(first.result.current.permissions).toEqual(permsA))

    // A second caller's fetch fails: the failure is the second caller's own answer, and the
    // last list is the best either of them has.
    const second = renderPermissions('A', member, client)
    expect(second.result.current).toEqual(loadingAnswer)
    fails.reject(new Error('boom'))
    await settle()

    const served = { permissions: permsA, role: 'member', loading: false }
    expect(first.result.current).toEqual(served)
    expect(second.result.current).toEqual(served)
  })

  it('gives a root user every permission on the first render, without a request', () => {
    const { renders } = renderPermissions('A', root)

    expect(renders[0]).toEqual({
      permissions: createFullPermissions(),
      role: 'root',
      loading: false
    })
    expect(getMembers).not.toHaveBeenCalled()
  })

  it('gives no permissions to a non-member', async () => {
    getMembers.mockResolvedValue(membersOf({ userId: 'someone-else', permissions: permsA }))
    const { result } = renderPermissions('A')

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current).toEqual({
      permissions: createEmptyPermissions(),
      role: undefined,
      loading: false
    })
  })

  it('gives no permissions when the lookup fails', async () => {
    getMembers.mockRejectedValue(new Error('boom'))
    const { result } = renderPermissions('A')

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current).toEqual({
      permissions: createEmptyPermissions(),
      role: undefined,
      loading: false
    })
  })

  it('denies the resources a partial permission map does not mention', async () => {
    getMembers.mockResolvedValue(
      membersOf({ permissions: { broadcasts: { read: true, write: false } } })
    )
    const { result } = renderPermissions('A')

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.permissions).toEqual(permsA)
  })

  it('keeps the same answer object while nothing changes', async () => {
    getMembers.mockResolvedValue(membersOf({ permissions: permsA }))
    const { result, switchTo } = renderPermissions('A')
    await waitFor(() => expect(result.current.loading).toBe(false))
    const first = result.current

    switchTo('A')

    expect(result.current).toBe(first)
  })

  it('answers nothing, without a request, once the workspace or the user is gone', async () => {
    getMembers.mockResolvedValue(membersOf({ permissions: permsA }))
    const { result, switchTo } = renderPermissions('A')
    await waitFor(() => expect(result.current.loading).toBe(false))
    getMembers.mockClear()

    switchTo('')
    expect(result.current).toEqual(emptyAnswer)

    switchTo('A', null)
    expect(result.current).toEqual(emptyAnswer)
    expect(getMembers).not.toHaveBeenCalled()
  })

  it('answers nothing, without a request, with no user', () => {
    const { result } = renderPermissions('A', null)

    expect(result.current).toEqual(emptyAnswer)
    expect(getMembers).not.toHaveBeenCalled()
  })
})
