import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

const auth = vi.hoisted(() => ({
  user: { id: "u1", email: "ann@example.com" } as {
    id: string;
    email: string;
  } | null,
}));
vi.mock("../contexts/AuthContext", () => ({
  useAuth: () => ({ user: auth.user, workspaces: [] }),
}));
vi.mock("../services/api/auth", () => ({
  isRootUser: (email?: string) => email === "root@example.com",
}));
vi.mock("../services/api/workspace", () => ({
  workspaceService: { getMembers: vi.fn() },
}));

import { workspaceService } from "../services/api/workspace";
import { useAuditLogAccess } from "./useAuditLogAccess";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
  >
    {children}
  </QueryClientProvider>
);

const members = (member: Record<string, unknown>) => ({
  members: [{ user_id: "u1", role: "member", ...member }],
});

describe("useAuditLogAccess", () => {
  beforeEach(() => {
    auth.user = { id: "u1", email: "ann@example.com" };
    vi.mocked(workspaceService.getMembers).mockReset();
  });

  it("grants an owner", async () => {
    vi.mocked(workspaceService.getMembers).mockResolvedValue(
      members({ role: "owner" }) as never,
    );
    const { result } = renderHook(() => useAuditLogAccess("ws1"), { wrapper });
    await waitFor(() => expect(result.current.canRead).toBe(true));
    expect(result.current.isRoot).toBe(false);
  });

  it("denies a full-access member without the opt-in grant", async () => {
    vi.mocked(workspaceService.getMembers).mockResolvedValue(
      members({
        permissions: { contacts: { read: true, write: true } },
      }) as never,
    );
    const { result } = renderHook(() => useAuditLogAccess("ws1"), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.canRead).toBe(false);
  });

  it("grants a member holding audit_logs read", async () => {
    vi.mocked(workspaceService.getMembers).mockResolvedValue(
      members({
        permissions: { audit_logs: { read: true, write: false } },
      }) as never,
    );
    const { result } = renderHook(() => useAuditLogAccess("ws1"), { wrapper });
    await waitFor(() => expect(result.current.canRead).toBe(true));
  });

  it("grants a root user without asking for the membership", () => {
    auth.user = { id: "r1", email: "root@example.com" };
    const { result } = renderHook(() => useAuditLogAccess("ws1"), { wrapper });
    expect(result.current).toEqual({
      canRead: true,
      isRoot: true,
      loading: false,
    });
    expect(workspaceService.getMembers).not.toHaveBeenCalled();
  });

  it("denies when signed out or without a workspace", () => {
    auth.user = null;
    const { result } = renderHook(() => useAuditLogAccess("ws1"), { wrapper });
    expect(result.current.canRead).toBe(false);
  });
});
