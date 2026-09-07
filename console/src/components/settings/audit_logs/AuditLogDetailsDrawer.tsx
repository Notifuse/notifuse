import {
  Button,
  Descriptions,
  Drawer,
  Tag,
  Tooltip,
  Typography,
  message,
} from "antd";
import { CopyOutlined } from "@ant-design/icons";
import { useLingui } from "@lingui/react/macro";
import { formatAuditDate } from "./AuditLogTable";
import type { AuditLog, AuditScope } from "../../../services/api/audit_log";
import { categoryColor, outcomeColor } from "./actionLabels";

const { Text } = Typography;

interface AuditLogDetailsDrawerProps {
  log: AuditLog | null;
  scope: AuditScope;
  timezone?: string;
  onClose: () => void;
}

const REDACTED = "[redacted]";

function renderValue(value: unknown): React.ReactNode {
  if (value === null || value === undefined) {
    return (
      <Text type="secondary" italic>
        null
      </Text>
    );
  }
  if (value === REDACTED) {
    return <Tag>{REDACTED}</Tag>;
  }
  if (typeof value === "object") {
    return (
      <Tooltip
        title={
          <pre className="m-0 text-xs">{JSON.stringify(value, null, 2)}</pre>
        }
      >
        <Tag className="cursor-help">JSON</Tag>
      </Tooltip>
    );
  }
  return <Text>{String(value)}</Text>;
}

export function AuditLogDetailsDrawer({
  log,
  scope,
  timezone,
  onClose,
}: AuditLogDetailsDrawerProps) {
  const { t } = useLingui();

  const copyLink = async () => {
    if (!log) return;
    const url = new URL(window.location.href);
    url.searchParams.set("id", log.id);
    try {
      await navigator.clipboard.writeText(url.toString());
      message.success(t`Link copied`);
    } catch {
      message.error(t`Could not copy the link`);
    }
  };

  const changes = Object.entries(log?.changes ?? {});
  const metadata = Object.entries(log?.metadata ?? {});

  return (
    <Drawer
      title={log ? log.action : t`Audit entry`}
      open={Boolean(log)}
      onClose={onClose}
      size={640}
      extra={
        <Button
          size="small"
          icon={<CopyOutlined />}
          onClick={copyLink}
          data-testid="audit-copy-link"
        >
          {t`Copy link`}
        </Button>
      }
    >
      {log && (
        <div className="space-y-6">
          <Descriptions size="small" column={1} bordered>
            <Descriptions.Item label={t`When`}>
              {formatAuditDate(log.occurred_at, timezone)}
              {timezone ? ` (${timezone})` : ""}
            </Descriptions.Item>
            <Descriptions.Item label={t`Outcome`}>
              <Tag color={outcomeColor(log.outcome)}>{log.outcome}</Tag>
              {log.status_code ? (
                <Text type="secondary">HTTP {log.status_code}</Text>
              ) : null}
            </Descriptions.Item>
            <Descriptions.Item label={t`Category`}>
              <Tag color={categoryColor(log.category)}>{log.category}</Tag>
            </Descriptions.Item>
            {scope === "deployment" && (
              <Descriptions.Item label={t`Workspace`}>
                {log.workspace_id ?? (
                  <Text type="secondary">{t`Deployment`}</Text>
                )}
              </Descriptions.Item>
            )}
          </Descriptions>

          <Descriptions size="small" column={1} bordered title={t`Actor`}>
            <Descriptions.Item label={t`Type`}>
              <Tag>{log.actor_type}</Tag>
              {log.actor_role ? <Tag color="blue">{log.actor_role}</Tag> : null}
            </Descriptions.Item>
            <Descriptions.Item label={t`Email`}>
              {log.actor_email ?? "—"}
            </Descriptions.Item>
            <Descriptions.Item label={t`Name`}>
              {log.actor_name ?? "—"}
            </Descriptions.Item>
            <Descriptions.Item label={t`Signed in with`}>
              {log.auth_method ?? "—"}
            </Descriptions.Item>
            <Descriptions.Item label={t`IP address`}>
              {log.ip_address ?? "—"}
            </Descriptions.Item>
            <Descriptions.Item label={t`User agent`}>
              <Text className="text-xs break-all">{log.user_agent ?? "—"}</Text>
            </Descriptions.Item>
            <Descriptions.Item label={t`Request ID`}>
              <Text code className="text-xs">
                {log.request_id ?? "—"}
              </Text>
            </Descriptions.Item>
          </Descriptions>

          {(log.target_type || log.target_id || log.target_name) && (
            <Descriptions size="small" column={1} bordered title={t`Target`}>
              <Descriptions.Item label={t`Type`}>
                {log.target_type ?? "—"}
              </Descriptions.Item>
              <Descriptions.Item label={t`ID`}>
                <Text code className="text-xs">
                  {log.target_id ?? "—"}
                </Text>
              </Descriptions.Item>
              <Descriptions.Item label={t`Name`}>
                {log.target_name ?? "—"}
              </Descriptions.Item>
            </Descriptions>
          )}

          {changes.length > 0 && (
            <div>
              <div className="font-medium mb-2">{t`Changes`}</div>
              <div className="space-y-1" data-testid="audit-changes">
                {changes.map(([key, change]) => (
                  <div key={key} className="text-sm">
                    <Text type="secondary" className="font-mono text-xs">
                      {key}:
                    </Text>{" "}
                    {renderValue(change.old)}
                    <Text type="secondary"> → </Text>
                    {renderValue(change.new)}
                  </div>
                ))}
              </div>
            </div>
          )}

          {metadata.length > 0 && (
            <div>
              <div className="font-medium mb-2">{t`Details`}</div>
              <div className="space-y-1">
                {metadata.map(([key, value]) => (
                  <div key={key} className="text-sm">
                    <Text type="secondary" className="font-mono text-xs">
                      {key}:
                    </Text>{" "}
                    {renderValue(value)}
                  </div>
                ))}
              </div>
            </div>
          )}

          <div>
            <div className="font-medium mb-2">{t`Raw entry`}</div>
            <pre className="p-2 bg-gray-100 rounded text-xs overflow-auto max-h-80">
              {JSON.stringify(log, null, 2)}
            </pre>
          </div>
        </div>
      )}
    </Drawer>
  );
}
