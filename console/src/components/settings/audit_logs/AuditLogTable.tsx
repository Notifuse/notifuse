import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  Empty,
  Space,
  Spin,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from "antd";
import { useLingui } from "@lingui/react/macro";
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useState,
} from "react";
import type { Dayjs } from "dayjs";
import dayjs from "../../../lib/dayjs";
import {
  auditLogApi,
  type AuditExportFormat,
  type AuditLog,
  type AuditLogListParams,
  type AuditOutcome,
  type AuditScope,
} from "../../../services/api/audit_log";
import { downloadBlob } from "../../../lib/download";
import { AuditLogDetailsDrawer } from "./AuditLogDetailsDrawer";
import type { AuditFilters } from "./AuditFilterBuilder";
import { categoryColor, outcomeColor, splitAction } from "./actionLabels";

const { Text } = Typography;

const PAGE_SIZE = 50;

interface AuditLogTableProps {
  scope: AuditScope;
  workspaceId?: string;
  timezone?: string;
  // Owned by the tab, which renders the filter buttons beside its scope
  // control; a change resets the cursor and the accumulated pages here.
  filters: AuditFilters;
}

// What the page's tab bar drives: a refresh and an export of the current
// filter. The table owns the filter; the buttons live with the tabs.
export interface AuditLogTableHandle {
  refresh: () => void;
  exportAs: (format: AuditExportFormat) => Promise<void>;
}

// A picked day is a day in the workspace's timezone, not the viewer's: the
// entries are labelled in that zone, so the bounds must agree with the labels.
function dayBound(
  day: Dayjs,
  edge: "start" | "end",
  timezone?: string,
): string {
  const date = day.format("YYYY-MM-DD");
  const inZone = timezone ? dayjs.tz(date, timezone) : dayjs(date);
  return (
    edge === "start" ? inZone.startOf("day") : inZone.endOf("day")
  ).toISOString();
}

export function toListParams(
  filters: AuditFilters,
  scope: AuditScope,
  workspaceId?: string,
  timezone?: string,
): AuditLogListParams {
  return {
    scope,
    workspace_id: workspaceId,
    actions: filters.actions.length ? filters.actions : undefined,
    outcome: filters.outcome,
    actor_email: filters.actor_email || undefined,
    target_id: filters.target_id || undefined,
    ip_address: filters.ip_address || undefined,
    from: filters.range
      ? dayBound(filters.range[0], "start", timezone)
      : undefined,
    to: filters.range ? dayBound(filters.range[1], "end", timezone) : undefined,
  };
}

// The workspace's own clock, when there is one, so the label "in Asia/Tokyo"
// is true of the time it sits next to.
export function formatAuditDate(value: string, timezone?: string): string {
  return timezone
    ? dayjs(value).tz(timezone).format("lll")
    : dayjs(value).format("lll");
}

export const AuditLogTable = forwardRef<
  AuditLogTableHandle,
  AuditLogTableProps
>(function AuditLogTable({ scope, workspaceId, timezone, filters }, ref) {
  const { t } = useLingui();
  const queryClient = useQueryClient();

  // The cursor belongs to the filter it was paged under. Deriving it here,
  // rather than resetting it in an effect, means a filter change never sends
  // one request with the old cursor before the reset lands.
  const [paging, setPaging] = useState<{
    filters: AuditFilters;
    cursor?: string;
  }>({ filters });
  const cursor = paging.filters === filters ? paging.cursor : undefined;
  const setCursor = (next?: string) => setPaging({ filters, cursor: next });
  const [rows, setRows] = useState<AuditLog[]>([]);
  const [selected, setSelected] = useState<AuditLog | null>(null);

  const params = useMemo(
    () => toListParams(filters, scope, workspaceId, timezone),
    [filters, scope, workspaceId, timezone],
  );

  const { data, dataUpdatedAt, isLoading, isFetching, error } = useQuery({
    queryKey: ["audit-logs", scope, workspaceId, params, cursor],
    queryFn: () => auditLogApi.list({ ...params, cursor, limit: PAGE_SIZE }),
    staleTime: 5000,
    refetchOnWindowFocus: false,
  });

  // Pages accumulate; a fresh first page replaces them. Keyed on dataUpdatedAt
  // as well as data: a refetch that returns identical rows keeps the same
  // data reference (structural sharing), and the table must still repaint.
  useEffect(() => {
    if (!data) return;
    setRows((previous) => (cursor ? [...previous, ...data.logs] : data.logs));
  }, [data, dataUpdatedAt, cursor]);

  // ?id=… deep link (the "Copy link" button in the drawer): open that entry.
  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get("id");
    if (!id) return;
    auditLogApi
      .get(id, scope, workspaceId)
      .then((result) => setSelected(result.log))
      .catch(() => message.error(t`This audit entry could not be found`));
    // eslint-disable-next-line react-hooks/exhaustive-deps -- once, on mount
  }, []);

  // A new filter starts from an empty table; the first page fills it.
  useEffect(() => {
    setRows([]);
  }, [filters]);

  // The rows stay on screen until the fresh first page replaces them.
  const refresh = () => {
    setCursor(undefined);
    queryClient.invalidateQueries({
      queryKey: ["audit-logs", scope, workspaceId],
    });
  };

  const exportAs = async (format: AuditExportFormat) => {
    const blob = await auditLogApi.export(params, format);
    const stamp = dayjs().format("YYYYMMDD");
    downloadBlob(blob, `audit-logs-${workspaceId ?? scope}-${stamp}.${format}`);
  };

  useImperativeHandle(ref, () => ({ refresh, exportAs }));

  const loadMore = () => {
    if (data?.next_cursor) setCursor(data.next_cursor);
  };

  const formatDate = (value: string) =>
    timezone
      ? t`${formatAuditDate(value, timezone)} in ${timezone}`
      : formatAuditDate(value);

  const columns = [
    {
      title: t`When`,
      dataIndex: "occurred_at",
      key: "occurred_at",
      width: 140,
      render: (value: string) => (
        <Tooltip title={formatDate(value)}>{dayjs(value).fromNow()}</Tooltip>
      ),
    },
    {
      title: t`Actor`,
      key: "actor",
      render: (_: unknown, record: AuditLog) => (
        <div className="leading-tight">
          <div>
            {record.actor_email ?? (
              <Text type="secondary">{record.actor_type}</Text>
            )}
          </div>
          <div className="text-xs">
            <Text type="secondary">{record.actor_type}</Text>
            {record.actor_role ? (
              <Tag className="ml-1">{record.actor_role}</Tag>
            ) : null}
          </div>
        </div>
      ),
    },
    {
      title: t`Action`,
      dataIndex: "action",
      key: "action",
      render: (action: string, record: AuditLog) => {
        const { resource, verb } = splitAction(action);
        return (
          <Space size={4}>
            <Tag color={categoryColor(record.category)}>{record.category}</Tag>
            <code className="text-xs">
              {resource}
              {verb ? <span className="font-semibold">.{verb}</span> : null}
            </code>
          </Space>
        );
      },
    },
    {
      title: t`Target`,
      key: "target",
      render: (_: unknown, record: AuditLog) =>
        record.target_type || record.target_id ? (
          <div className="leading-tight">
            <div>{record.target_name ?? record.target_id}</div>
            <div className="text-xs">
              <Text type="secondary">
                {record.target_type}
                {record.target_name && record.target_id
                  ? ` · ${record.target_id}`
                  : ""}
              </Text>
            </div>
          </div>
        ) : (
          <Text type="secondary">—</Text>
        ),
    },
    ...(scope === "deployment"
      ? [
          {
            title: t`Workspace`,
            dataIndex: "workspace_id",
            key: "workspace_id",
            render: (value?: string) =>
              value ?? <Text type="secondary">{t`Deployment`}</Text>,
          },
        ]
      : []),
    {
      title: t`Outcome`,
      dataIndex: "outcome",
      key: "outcome",
      width: 100,
      render: (outcome: AuditOutcome) => (
        <Tag color={outcomeColor(outcome)}>{outcome}</Tag>
      ),
    },
    {
      title: t`IP`,
      dataIndex: "ip_address",
      key: "ip_address",
      width: 130,
      render: (value?: string) => (
        <Text className="text-xs">{value ?? "—"}</Text>
      ),
    },
  ];

  return (
    <div className="space-y-4" data-testid={`audit-log-table-${scope}`}>
      {error ? (
        <Text type="danger">{(error as Error).message}</Text>
      ) : isLoading && rows.length === 0 ? (
        <div className="py-12 text-center">
          <Spin size="large" />
        </div>
      ) : rows.length === 0 ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={t`No audit entries match`}
        />
      ) : (
        <>
          <Table
            dataSource={rows}
            columns={columns}
            rowKey="id"
            pagination={false}
            size="small"
            className="border border-gray-300 rounded"
            scroll={{ x: "max-content" }}
            onRow={(record) => ({
              onClick: () => setSelected(record),
              style: { cursor: "pointer" },
            })}
          />
          {data?.has_more && (
            <div className="text-center">
              <Button size="small" onClick={loadMore} loading={isFetching}>
                {t`Load More`}
              </Button>
            </div>
          )}
        </>
      )}

      <AuditLogDetailsDrawer
        log={selected}
        scope={scope}
        timezone={timezone}
        onClose={() => setSelected(null)}
      />
    </div>
  );
});
