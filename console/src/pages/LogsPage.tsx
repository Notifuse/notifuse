import { Tabs } from "antd";
import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useLingui } from "@lingui/react/macro";
import { useRef, useState } from "react";
import { MessageHistoryTab } from "../components/messages/MessageHistoryTab";
import { InboundWebhookEventsTab } from "../components/webhooks/InboundWebhookEventsTab";
import { OutgoingWebhooksTab } from "../components/webhooks/OutgoingWebhooksTab";
import { AuditLogsTab } from "../components/logs/AuditLogsTab";
import { AuditLogActions } from "../components/logs/AuditLogActions";
import { PageHeader } from "../components/layout";
import type { AuditLogTableHandle } from "../components/settings/audit_logs/AuditLogTable";
import { useAuditLogAccess } from "../hooks/useAuditLogAccess";
import { useAuth } from "../contexts/AuthContext";
import { LOGS_TAB } from "../components/logs/logsUrl";

export function LogsPage() {
  const { workspaceId } = useParams({ strict: false });
  const search = useSearch({ strict: false }) as { tab?: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { t } = useLingui();
  const { workspaces } = useAuth();
  const [activeTab, setActiveTab] = useState<string>(search.tab || LOGS_TAB.messages);
  const auditTable = useRef<AuditLogTableHandle>(null);
  const auditAccess = useAuditLogAccess(workspaceId);

  if (!workspaceId) {
    return <div>{t`Loading...`}</div>;
  }

  const timezone = workspaces.find((workspace) => workspace.id === workspaceId)
    ?.settings.timezone;

  const handleRefreshInboundWebhookEvents = () => {
    queryClient.invalidateQueries({
      queryKey: ["inbound-webhook-events", workspaceId],
    });
  };

  // The tab lands in the URL so a copied link opens the same tab; the audit
  // drawer's "Copy link" relies on it. The other params stay as they are.
  const handleTabChange = (key: string) => {
    setActiveTab(key);
    navigate({
      to: "/console/workspace/$workspaceId/logs",
      params: { workspaceId },
      search: (previous) => ({ ...previous, tab: key }),
      replace: true,
    });
  };

  const items = [
    {
      key: LOGS_TAB.messages,
      label: t`Message History`,
      children: <MessageHistoryTab workspaceId={workspaceId} />,
    },
    {
      key: LOGS_TAB.incomingWebhooks,
      label: t`Incoming Webhooks`,
      children: (
        <InboundWebhookEventsTab
          workspaceId={workspaceId}
          onRefresh={handleRefreshInboundWebhookEvents}
        />
      ),
    },
    {
      key: LOGS_TAB.outgoingWebhooks,
      label: t`Outgoing Webhooks`,
      children: <OutgoingWebhooksTab workspaceId={workspaceId} />,
    },
    ...(auditAccess.canRead
      ? [
          {
            key: "audit",
            label: t`Audit logs`,
            children: (
              <AuditLogsTab
                ref={auditTable}
                workspaceId={workspaceId}
                timezone={timezone}
                isRoot={auditAccess.isRoot}
              />
            ),
          },
        ]
      : []),
  ];

  return (
    <div className="flex flex-col gap-3">
      <PageHeader
        title={t`Logs`}
        description={t`Monitor message delivery status, webhook events and the audit trail`}
      />

      <Tabs
        activeKey={activeTab}
        onChange={handleTabChange}
        items={items}
        tabBarExtraContent={
          activeTab === "audit" && auditAccess.canRead ? (
            <AuditLogActions
              onRefresh={() => auditTable.current?.refresh()}
              onExport={async (format) => {
                await auditTable.current?.exportAs(format);
              }}
            />
          ) : undefined
        }
      />
    </div>
  );
}
