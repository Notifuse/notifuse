import { useQuery } from "@tanstack/react-query";
import { useAuth } from "../contexts/AuthContext";
import { isRootUser } from "../services/api/auth";
import { workspaceService } from "../services/api/workspace";

export interface AuditLogAccess {
  // Owners always, root users always, members only with the opt-in audit_logs
  // read grant — the same rule the server enforces on auditLogs.list.
  canRead: boolean;
  isRoot: boolean;
  loading: boolean;
}

// Answers "may this user open the audit log of this workspace" from the
// membership the server returns, so a member without the grant never sees a
// tab that would answer 403.
export function useAuditLogAccess(workspaceId?: string): AuditLogAccess {
  const { user } = useAuth();
  const isRoot = isRootUser(user?.email);

  const { data, isLoading } = useQuery({
    queryKey: ["workspace-members", workspaceId],
    queryFn: () => workspaceService.getMembers(workspaceId as string),
    enabled: Boolean(workspaceId) && Boolean(user) && !isRoot,
    staleTime: 60_000,
  });

  if (isRoot) return { canRead: true, isRoot: true, loading: false };
  if (!user || !workspaceId)
    return { canRead: false, isRoot: false, loading: false };

  const member = data?.members.find(
    (candidate) => candidate.user_id === user.id,
  );
  const canRead =
    member?.role === "owner" || member?.permissions?.audit_logs?.read === true;
  return { canRead, isRoot: false, loading: isLoading };
}
