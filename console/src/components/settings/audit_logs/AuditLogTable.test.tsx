import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { App, ConfigProvider } from "antd";
import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { AuditLog } from "../../../services/api/audit_log";

// services/api/client imports the router, which imports every page; stubbing it keeps that
// graph out of this suite (see WebAnalyticsSettings.test.tsx).
vi.mock("../../../services/api/client", () => ({
  api: { get: vi.fn(), post: vi.fn(), download: vi.fn() },
}));
vi.mock("../../../services/api/audit_log", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../../services/api/audit_log")>();
  return {
    ...actual,
    auditLogApi: {
      list: vi.fn(),
      get: vi.fn(),
      actions: vi.fn().mockResolvedValue({
        actions: [
          { action: "templates.update", category: "templates" },
          { action: "workspaces.inviteMember", category: "members" },
        ],
        categories: ["members", "templates"],
      }),
      export: vi.fn().mockResolvedValue(new Blob(["id,action\n"])),
      setWorkspaceSettings: vi.fn(),
    },
  };
});
vi.mock("../../../lib/download", () => ({ downloadBlob: vi.fn() }));

import { auditLogApi } from "../../../services/api/audit_log";
import { AuditLogTable, formatAuditDate, toListParams } from "./AuditLogTable";
import { EMPTY_AUDIT_FILTERS, type AuditFilters } from "./AuditFilterBuilder";
import dayjs from "../../../lib/dayjs";

i18n.loadAndActivate({ locale: "en", messages: {} });

const row = (id: string, action: string): AuditLog => ({
  id,
  occurred_at: "2026-09-05T10:00:00Z",
  workspace_id: "ws1",
  action,
  category: action.startsWith("templates") ? "templates" : "members",
  outcome: "success",
  status_code: 200,
  actor_type: "user",
  actor_email: `${id}@example.com`,
  target_type: "template",
  target_id: `t-${id}`,
  ip_address: "203.0.113.9",
});

const renderTable = (
  scope: "workspace" | "deployment" = "workspace",
  filters: AuditFilters = EMPTY_AUDIT_FILTERS,
) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const tree = (current: AuditFilters) => (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <AuditLogTable
              scope={scope}
              workspaceId={scope === "workspace" ? "ws1" : undefined}
              timezone="UTC"
              filters={current}
            />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  );
  const utils = render(tree(filters));
  return {
    ...utils,
    rerenderWith: (next: AuditFilters) => utils.rerender(tree(next)),
  };
};

describe("AuditLogTable", () => {
  beforeEach(() => {
    vi.mocked(auditLogApi.list).mockReset();
    vi.mocked(auditLogApi.get).mockReset();
    vi.mocked(auditLogApi.export).mockClear();
    window.history.replaceState(
      null,
      "",
      "/console/workspace/ws1/settings/audit-logs",
    );
  });

  it("renders the first page and appends the next one on Load More", async () => {
    vi.mocked(auditLogApi.list)
      .mockResolvedValueOnce({
        logs: [row("a", "templates.update")],
        next_cursor: "c2",
        has_more: true,
      })
      .mockResolvedValueOnce({
        logs: [row("b", "workspaces.inviteMember")],
        has_more: false,
      });

    renderTable();
    expect(await screen.findByText("a@example.com")).toBeInTheDocument();
    expect(auditLogApi.list).toHaveBeenLastCalledWith(
      expect.objectContaining({
        scope: "workspace",
        workspace_id: "ws1",
        cursor: undefined,
        limit: 50,
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: /load more/i }));
    expect(await screen.findByText("b@example.com")).toBeInTheDocument();
    expect(screen.getByText("a@example.com")).toBeInTheDocument();
    expect(auditLogApi.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ cursor: "c2" }),
    );
    expect(
      screen.queryByRole("button", { name: /load more/i }),
    ).not.toBeInTheDocument();
  });

  it("starts a new first page when the filters change", async () => {
    vi.mocked(auditLogApi.list)
      .mockResolvedValueOnce({
        logs: [row("a", "templates.update")],
        next_cursor: "c2",
        has_more: true,
      })
      .mockResolvedValueOnce({
        logs: [row("b", "workspaces.inviteMember")],
        has_more: false,
      })
      .mockResolvedValueOnce({
        logs: [row("c", "lists.create")],
        has_more: false,
      });
    const { rerenderWith } = renderTable();
    await screen.findByText("a@example.com");
    fireEvent.click(screen.getByRole("button", { name: /load more/i }));
    await screen.findByText("b@example.com");

    rerenderWith({ ...EMPTY_AUDIT_FILTERS, actor_email: "ann@" });
    await screen.findByText("c@example.com");
    expect(auditLogApi.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ actor_email: "ann@", cursor: undefined }),
    );
    expect(screen.queryByText("a@example.com")).not.toBeInTheDocument();
    expect(screen.queryByText("b@example.com")).not.toBeInTheDocument();
  });

  it("opens the drawer for a clicked row", async () => {
    vi.mocked(auditLogApi.list).mockResolvedValue({
      logs: [row("a", "templates.update")],
      has_more: false,
    });
    renderTable();
    fireEvent.click(await screen.findByText("a@example.com"));
    expect(await screen.findByText("Raw entry")).toBeInTheDocument();
  });

  it("opens the entry named by ?id= on mount", async () => {
    window.history.replaceState(
      null,
      "",
      "/console/workspace/ws1/settings/audit-logs?id=deep",
    );
    vi.mocked(auditLogApi.list).mockResolvedValue({
      logs: [],
      has_more: false,
    });
    vi.mocked(auditLogApi.get).mockResolvedValue({
      log: row("deep", "workspaces.inviteMember"),
    });
    renderTable();
    expect(await screen.findByText("Raw entry")).toBeInTheDocument();
    expect(auditLogApi.get).toHaveBeenCalledWith("deep", "workspace", "ws1");
  });

  it("shows the workspace column only in the deployment scope", async () => {
    vi.mocked(auditLogApi.list).mockResolvedValue({
      logs: [row("a", "templates.update")],
      has_more: false,
    });
    renderTable("deployment");
    await screen.findByText("a@example.com");
    expect(screen.getAllByText("Workspace").length).toBeGreaterThan(0);
    expect(auditLogApi.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ scope: "deployment", workspace_id: undefined }),
    );
  });

  it("shows the empty state", async () => {
    vi.mocked(auditLogApi.list).mockResolvedValue({
      logs: [],
      has_more: false,
    });
    renderTable();
    expect(
      await screen.findByText("No audit entries match"),
    ).toBeInTheDocument();
  });
});

describe("formatAuditDate", () => {
  it("renders in the workspace timezone, which is what the label claims", () => {
    expect(formatAuditDate("2026-09-05T00:00:00Z", "Asia/Tokyo")).toContain(
      "9:00 AM",
    );
    expect(
      formatAuditDate("2026-09-05T00:00:00Z", "America/New_York"),
    ).toContain("8:00 PM");
  });
});

describe("toListParams", () => {
  it("bounds a picked day in the workspace timezone", () => {
    const day = dayjs("2026-09-05");
    const params = toListParams(
      {
        actions: [],
        outcome: undefined,
        actor_email: "",
        target_id: "",
        ip_address: "",
        range: [day, day],
      },
      "workspace",
      "ws1",
      "Asia/Tokyo",
    );
    // Midnight in Tokyo is 15:00 UTC the day before.
    expect(params.from).toBe("2026-09-04T15:00:00.000Z");
    expect(params.to).toBe("2026-09-05T14:59:59.999Z");
  });

  it("drops empty filters and turns the range into day bounds", () => {
    const params = toListParams(
      {
        actions: [],
        outcome: undefined,
        actor_email: "",
        target_id: "t1",
        ip_address: "",
        range: null,
      },
      "workspace",
      "ws1",
    );
    expect(params).toEqual({
      scope: "workspace",
      workspace_id: "ws1",
      actions: undefined,
      outcome: undefined,
      actor_email: undefined,
      target_id: "t1",
      ip_address: undefined,
      from: undefined,
      to: undefined,
    });
  });
});
