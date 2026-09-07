import { useQuery } from "@tanstack/react-query";
import { Button, DatePicker, Input, Popover, Select, Space } from "antd";
import { useLingui } from "@lingui/react/macro";
import { useMemo, useState } from "react";
import type { Dayjs } from "dayjs";
import {
  auditLogApi,
  type AuditOutcome,
} from "../../../services/api/audit_log";

const { RangePicker } = DatePicker;

// The filter state the user edits; scope and workspace come from the page.
export interface AuditFilters {
  actions: string[];
  outcome?: AuditOutcome;
  actor_email: string;
  target_id: string;
  ip_address: string;
  range: [Dayjs, Dayjs] | null;
}

export const EMPTY_AUDIT_FILTERS: AuditFilters = {
  actions: [],
  outcome: undefined,
  actor_email: "",
  target_id: "",
  ip_address: "",
  range: null,
};

export type AuditFilterField = keyof AuditFilters;

const FIELDS: AuditFilterField[] = [
  "actions",
  "outcome",
  "actor_email",
  "target_id",
  "ip_address",
  "range",
];

interface AuditFilterBuilderProps {
  filters: AuditFilters;
  onChange: (next: Partial<AuditFilters>) => void;
}

export function hasAuditFilterValue(
  key: AuditFilterField,
  source: AuditFilters,
): boolean {
  switch (key) {
    case "actions":
      return source.actions.length > 0;
    case "outcome":
      return Boolean(source.outcome);
    case "range":
      return source.range !== null;
    default:
      return source[key] !== "";
  }
}

// One small button per filter, each opening a popover with its input and
// Apply/Clear — the same chrome as the message and webhook tabs beside it. A
// set filter turns its button primary and shows "label: value".
export function AuditFilterBuilder({
  filters,
  onChange,
}: AuditFilterBuilderProps) {
  const { t } = useLingui();
  const [open, setOpen] = useState<Partial<Record<AuditFilterField, boolean>>>(
    {},
  );
  const [draft, setDraft] = useState<AuditFilters>(filters);

  const { data: catalogue } = useQuery({
    queryKey: ["audit-log-actions"],
    queryFn: () => auditLogApi.actions(),
    staleTime: Infinity,
  });

  const actionOptions = useMemo(() => {
    const byCategory = new Map<string, { label: string; value: string }[]>();
    for (const entry of catalogue?.actions ?? []) {
      const options = byCategory.get(entry.category) ?? [];
      options.push({ label: entry.action, value: entry.action });
      byCategory.set(entry.category, options);
    }
    return (catalogue?.categories ?? Array.from(byCategory.keys()))
      .filter((category) => byCategory.has(category))
      .map((category) => ({
        label: category,
        title: category,
        options: byCategory.get(category) ?? [],
      }));
  }, [catalogue]);

  const labels: Record<AuditFilterField, string> = {
    actions: t`Action`,
    outcome: t`Outcome`,
    actor_email: t`Actor email`,
    target_id: t`Target ID`,
    ip_address: t`IP address`,
    range: t`Date range`,
  };

  const outcomeLabels: Record<AuditOutcome, string> = {
    success: t`Success`,
    failure: t`Failure`,
    denied: t`Denied`,
  };

  const describe = (key: AuditFilterField): string => {
    switch (key) {
      case "actions":
        return filters.actions.length > 2
          ? t`${filters.actions.length} actions`
          : filters.actions.join(", ");
      case "outcome":
        return filters.outcome ? outcomeLabels[filters.outcome] : "";
      case "range":
        return filters.range
          ? `${filters.range[0].format("ll")} → ${filters.range[1].format("ll")}`
          : "";
      default:
        return filters[key];
    }
  };

  const setOpenFor = (key: AuditFilterField, visible: boolean) => {
    // The draft starts from what is applied, so reopening a filter shows its
    // current value rather than the last thing typed.
    if (visible) setDraft(filters);
    setOpen((previous) => ({ ...previous, [key]: visible }));
  };

  const apply = (key: AuditFilterField) => {
    onChange({ [key]: draft[key] } as Partial<AuditFilters>);
    setOpenFor(key, false);
  };

  const clear = (key: AuditFilterField) => {
    onChange({ [key]: EMPTY_AUDIT_FILTERS[key] } as Partial<AuditFilters>);
    setDraft((previous) => ({ ...previous, [key]: EMPTY_AUDIT_FILTERS[key] }));
    setOpenFor(key, false);
  };

  const clearAll = () => {
    onChange({ ...EMPTY_AUDIT_FILTERS });
    setDraft(EMPTY_AUDIT_FILTERS);
  };

  const control = (key: AuditFilterField) => {
    switch (key) {
      case "actions":
        return (
          <Select
            mode="multiple"
            allowClear
            showSearch
            style={{ width: "100%", marginBottom: 8 }}
            placeholder={t`Select ${labels[key]}`}
            value={draft.actions}
            options={actionOptions}
            onChange={(actions) => setDraft({ ...draft, actions })}
            maxTagCount="responsive"
          />
        );
      case "outcome":
        return (
          <Select
            allowClear
            style={{ width: "100%", marginBottom: 8 }}
            placeholder={t`Select ${labels[key]}`}
            value={draft.outcome}
            options={(["success", "failure", "denied"] as AuditOutcome[]).map(
              (value) => ({
                value,
                label: outcomeLabels[value],
              }),
            )}
            onChange={(outcome) => setDraft({ ...draft, outcome })}
          />
        );
      case "range":
        return (
          <RangePicker
            allowClear
            style={{ width: "100%", marginBottom: 8 }}
            value={draft.range}
            onChange={(range) =>
              setDraft({
                ...draft,
                range:
                  range && range[0] && range[1] ? [range[0], range[1]] : null,
              })
            }
          />
        );
      default:
        return (
          <Input
            placeholder={t`Enter ${labels[key]}`}
            value={draft[key]}
            onChange={(event) =>
              setDraft({ ...draft, [key]: event.target.value })
            }
            onPressEnter={() => apply(key)}
            style={{ marginBottom: 8 }}
          />
        );
    }
  };

  const anyActive = FIELDS.some((key) => hasAuditFilterValue(key, filters));

  return (
    <Space wrap data-testid="audit-filter-builder">
      {FIELDS.map((key) => {
        const isActive = hasAuditFilterValue(key, filters);
        return (
          <Popover
            key={key}
            trigger="click"
            placement="bottom"
            open={open[key]}
            onOpenChange={(visible) => setOpenFor(key, visible)}
            content={
              <div
                style={{
                  width: key === "actions" || key === "range" ? 300 : 200,
                }}
              >
                {control(key)}
                <div className="flex gap-2">
                  <Button
                    type="primary"
                    size="small"
                    style={{ flex: 1 }}
                    disabled={!hasAuditFilterValue(key, draft)}
                    onClick={() => apply(key)}
                  >
                    {t`Apply`}
                  </Button>
                  {isActive && (
                    <Button danger size="small" onClick={() => clear(key)}>
                      {t`Clear`}
                    </Button>
                  )}
                </div>
              </div>
            }
          >
            <Button
              type={isActive ? "primary" : "default"}
              size="small"
              data-testid={`audit-filter-${key}`}
            >
              {isActive ? `${labels[key]}: ${describe(key)}` : labels[key]}
            </Button>
          </Popover>
        );
      })}

      {anyActive && (
        <Button size="small" onClick={clearAll}>
          {t`Clear All`}
        </Button>
      )}
    </Space>
  );
}
