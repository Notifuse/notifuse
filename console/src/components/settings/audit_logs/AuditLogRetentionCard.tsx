import { Form, InputNumber, Typography, message } from "antd";
import { useLingui } from "@lingui/react/macro";
import { useEffect, useState } from "react";
import type { Workspace } from "../../../services/api/workspace";
import {
  auditLogApi,
  AUDIT_LOG_DEFAULT_RETENTION_DAYS,
  AUDIT_LOG_MAX_RETENTION_DAYS,
  AUDIT_LOG_MIN_RETENTION_DAYS,
} from "../../../services/api/audit_log";
import { SettingsSaveBar } from "../SettingsSaveBar";

const { Text } = Typography;

interface RetentionFormValues {
  retention_days: number | null;
}

interface AuditLogRetentionCardProps {
  workspace: Workspace;
  canManage: boolean;
  onWorkspaceUpdate: (workspace: Workspace) => void;
}

function toFormValues(workspace: Workspace): RetentionFormValues {
  const stored = workspace.settings.audit_logs?.retention_days;
  return { retention_days: stored === undefined ? null : stored };
}

// The workspace's retention. Empty means the deployment default (365 days
// unless a root user changed it); 0 keeps every entry forever.
export function AuditLogRetentionCard({
  workspace,
  canManage,
  onWorkspaceUpdate,
}: AuditLogRetentionCardProps) {
  const { t } = useLingui();
  const [form] = Form.useForm<RetentionFormValues>();
  const [saving, setSaving] = useState(false);
  const [formTouched, setFormTouched] = useState(false);

  useEffect(() => {
    form.setFieldsValue(toFormValues(workspace));
    setFormTouched(false);
  }, [workspace, form]);

  const handleDiscard = () => {
    form.setFieldsValue(toFormValues(workspace));
    setFormTouched(false);
  };

  const handleSave = async (values: RetentionFormValues) => {
    setSaving(true);
    try {
      const retention =
        values.retention_days === null || values.retention_days === undefined
          ? undefined
          : values.retention_days;
      await auditLogApi.setWorkspaceSettings(workspace.id, {
        retention_days: retention,
      });
      onWorkspaceUpdate({
        ...workspace,
        settings: {
          ...workspace.settings,
          audit_logs: { retention_days: retention },
        },
      });
      setFormTouched(false);
      message.success(t`Audit log retention saved`);
    } catch (err) {
      message.error(
        err instanceof Error ? err.message : t`Failed to save the retention`,
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <div
      className="border border-gray-200 rounded p-4 bg-white"
      data-testid="audit-retention-card"
    >
      <div className="font-medium mb-1">{t`Retention`}</div>
      <Text type="secondary" className="block mb-3">
        {t`Entries older than this are deleted once a day. Leave it empty for the deployment default (${AUDIT_LOG_DEFAULT_RETENTION_DAYS} days unless a root user changed it); 0 keeps every entry forever.`}
      </Text>
      <Form<RetentionFormValues>
        form={form}
        layout="vertical"
        disabled={!canManage}
        onValuesChange={() => setFormTouched(true)}
        onFinish={handleSave}
      >
        <Form.Item
          name="retention_days"
          label={t`Keep entries for (days)`}
          rules={[
            {
              validator: (_, value: number | null | undefined) => {
                if (value === null || value === undefined || value === 0)
                  return Promise.resolve();
                if (
                  value < AUDIT_LOG_MIN_RETENTION_DAYS ||
                  value > AUDIT_LOG_MAX_RETENTION_DAYS
                ) {
                  return Promise.reject(
                    new Error(
                      t`Use 0 to keep forever, or a value between ${AUDIT_LOG_MIN_RETENTION_DAYS} and ${AUDIT_LOG_MAX_RETENTION_DAYS}`,
                    ),
                  );
                }
                return Promise.resolve();
              },
            },
          ]}
        >
          <InputNumber
            min={0}
            max={AUDIT_LOG_MAX_RETENTION_DAYS}
            className="w-full"
            placeholder={String(AUDIT_LOG_DEFAULT_RETENTION_DAYS)}
          />
        </Form.Item>
        {canManage && (
          <SettingsSaveBar
            dirty={formTouched}
            saving={saving}
            onSave={() => form.submit()}
            onDiscard={handleDiscard}
            leaveWarning={t`You have unsaved retention changes. Leave without saving?`}
          />
        )}
      </Form>
    </div>
  );
}
