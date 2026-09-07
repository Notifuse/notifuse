import { Button, Typography } from "antd";
import { Link } from "@tanstack/react-router";
import { useLingui } from "@lingui/react/macro";
import type { Workspace } from "../../../services/api/workspace";
import { LicenceGateNotice } from "../../license/LicenceGateNotice";
import { SettingsSectionHeader } from "../SettingsSectionHeader";
import { AuditLogRetentionCard } from "./AuditLogRetentionCard";

const { Text } = Typography;

interface AuditLogsSettingsProps {
  workspace: Workspace | null;
  isOwner: boolean;
  onWorkspaceUpdate: (workspace: Workspace) => void;
}

// Settings → Audit logs holds what an owner configures: the retention, and
// the licence notice when nothing is being recorded. The log itself is read
// under Logs → Audit logs.
export function AuditLogsSettings({
  workspace,
  isOwner,
  onWorkspaceUpdate,
}: AuditLogsSettingsProps) {
  const { t } = useLingui();
  if (!workspace) return null;

  return (
    <div>
      <SettingsSectionHeader
        title={t`Audit logs`}
        description={t`Who did what in this workspace: members, permissions, API keys, integrations, settings and content changes, with the address it came from and whether it was allowed.`}
      />
      <LicenceGateNotice feature="audit_logs" workspaceId={workspace.id} />
      <div className="mb-6 flex items-center gap-3">
        <Text type="secondary">{t`Browse, filter and export the log under Logs → Audit logs.`}</Text>
        <Link
          to="/console/workspace/$workspaceId/logs"
          params={{ workspaceId: workspace.id }}
          search={{ tab: "audit" }}
        >
          <Button size="small">{t`Open the audit log`}</Button>
        </Link>
      </div>
      {isOwner && (
        <AuditLogRetentionCard
          workspace={workspace}
          canManage={isOwner}
          onWorkspaceUpdate={onWorkspaceUpdate}
        />
      )}
    </div>
  );
}
