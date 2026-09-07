import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { App, ConfigProvider } from "antd";
import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { AuditLogDetailsDrawer } from "./AuditLogDetailsDrawer";
import type { AuditLog } from "../../../services/api/audit_log";

i18n.loadAndActivate({ locale: "en", messages: {} });

const entry: AuditLog = {
  id: "log-1",
  occurred_at: "2026-09-05T10:00:00Z",
  workspace_id: "ws1",
  action: "workspaces.setUserPermissions",
  category: "members",
  outcome: "success",
  status_code: 200,
  actor_type: "user",
  actor_email: "ann@example.com",
  actor_role: "owner",
  auth_method: "session",
  target_type: "user",
  target_id: "u2",
  ip_address: "203.0.113.9",
  request_id: "req-1",
  changes: {
    name: { old: "Old name", new: "New name" },
    api_key: { old: "[redacted]", new: "[redacted]" },
  },
  metadata: { scope: "restricted" },
};

const renderDrawer = (log: AuditLog | null = entry) =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <AuditLogDetailsDrawer
            log={log}
            scope="workspace"
            timezone="UTC"
            onClose={vi.fn()}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>,
  );

describe("AuditLogDetailsDrawer", () => {
  it("renders the actor, the target, old → new changes and the redaction marker", () => {
    renderDrawer();
    expect(
      screen.getByText("workspaces.setUserPermissions"),
    ).toBeInTheDocument();
    expect(screen.getByText("ann@example.com")).toBeInTheDocument();
    expect(screen.getByText("203.0.113.9")).toBeInTheDocument();
    expect(screen.getByText("u2")).toBeInTheDocument();
    const changes = screen.getByTestId("audit-changes");
    expect(changes.textContent).toContain("name:");
    expect(changes.textContent).toContain("Old name");
    expect(changes.textContent).toContain("New name");
    expect(changes.textContent).toContain("[redacted]");
    expect(screen.getByText("restricted")).toBeInTheDocument();
  });

  it("copies a link carrying the entry id", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    window.history.replaceState(
      null,
      "",
      "/console/workspace/ws1/settings/audit-logs",
    );

    renderDrawer();
    fireEvent.click(screen.getByTestId("audit-copy-link"));
    await waitFor(() => expect(writeText).toHaveBeenCalled());
    expect(writeText.mock.calls[0][0]).toContain("id=log-1");
  });

  it("renders nothing without an entry", () => {
    renderDrawer(null);
    expect(screen.queryByTestId("audit-changes")).not.toBeInTheDocument();
  });
});
