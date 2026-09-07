import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { App, ConfigProvider } from "antd";
import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { AuditLogRetentionCard } from "./AuditLogRetentionCard";
import type { Workspace } from "../../../services/api/workspace";

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
      ...actual.auditLogApi,
      setWorkspaceSettings: vi.fn().mockResolvedValue(undefined),
    },
  };
});
vi.mock("@tanstack/react-router", () => ({
  useBlocker: () => ({ status: "idle", proceed: undefined, reset: undefined }),
}));

import { auditLogApi } from "../../../services/api/audit_log";

i18n.loadAndActivate({ locale: "en", messages: {} });

const makeWorkspace = (retention?: number): Workspace =>
  ({
    id: "ws1",
    name: "My WS",
    settings: {
      timezone: "UTC",
      audit_logs:
        retention === undefined ? undefined : { retention_days: retention },
    },
  }) as unknown as Workspace;

const renderCard = (canManage: boolean, workspace = makeWorkspace()) => {
  const onWorkspaceUpdate = vi.fn();
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <AuditLogRetentionCard
            workspace={workspace}
            canManage={canManage}
            onWorkspaceUpdate={onWorkspaceUpdate}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>,
  );
  return onWorkspaceUpdate;
};

describe("AuditLogRetentionCard", () => {
  beforeEach(() => vi.mocked(auditLogApi.setWorkspaceSettings).mockClear());

  it("shows the deployment default as a placeholder when the workspace has no setting", () => {
    renderCard(true);
    const input = screen.getByRole("spinbutton") as HTMLInputElement;
    expect(input.value).toBe("");
    expect(input.placeholder).toBe("365");
  });

  it("shows the stored retention", () => {
    renderCard(true, makeWorkspace(90));
    expect((screen.getByRole("spinbutton") as HTMLInputElement).value).toBe(
      "90",
    );
  });

  it("saves a new retention and reports the updated workspace", async () => {
    const onWorkspaceUpdate = renderCard(true);
    const input = screen.getByRole("spinbutton");
    fireEvent.change(input, { target: { value: "90" } });
    fireEvent.click(await screen.findByRole("button", { name: /save/i }));

    await waitFor(() =>
      expect(auditLogApi.setWorkspaceSettings).toHaveBeenCalledWith("ws1", {
        retention_days: 90,
      }),
    );
    expect(onWorkspaceUpdate).toHaveBeenCalled();
    expect(onWorkspaceUpdate.mock.calls[0][0].settings.audit_logs).toEqual({
      retention_days: 90,
    });
  });

  it("refuses a retention below the minimum", async () => {
    renderCard(true);
    fireEvent.change(screen.getByRole("spinbutton"), {
      target: { value: "7" },
    });
    fireEvent.click(await screen.findByRole("button", { name: /save/i }));
    expect(await screen.findByText(/between 30 and 3650/)).toBeInTheDocument();
    expect(auditLogApi.setWorkspaceSettings).not.toHaveBeenCalled();
  });

  it("is read-only without the right to manage", () => {
    renderCard(false);
    expect(screen.getByRole("spinbutton")).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: /save/i }),
    ).not.toBeInTheDocument();
  });
});
