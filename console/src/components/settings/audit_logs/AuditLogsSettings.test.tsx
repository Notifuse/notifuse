import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { App, ConfigProvider } from "antd";
import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import type { ReactNode } from "react";
import type { Workspace } from "../../../services/api/workspace";

const licence = vi.hoisted(() => ({ licensed: true }));

vi.mock("../../../hooks/useLicense", () => ({
  useLicense: () => ({
    has: () => licence.licensed,
    licensed: licence.licensed,
    expiresAt: null,
    entitlements: null,
    canManageLicense: false,
    refresh: vi.fn(),
    adopt: vi.fn(),
  }),
}));
vi.mock("@tanstack/react-router", () => ({
  useBlocker: () => ({ status: "idle", proceed: undefined, reset: undefined }),
  useNavigate: () => vi.fn(),
  Link: ({
    children,
    search,
  }: {
    children?: ReactNode;
    search?: { tab?: string };
  }) => (
    <a data-testid="audit-log-link" data-tab={search?.tab}>
      {children}
    </a>
  ),
}));
vi.mock("../../../services/api/client", () => ({
  api: { get: vi.fn(), post: vi.fn(), download: vi.fn() },
}));

import { AuditLogsSettings } from "./AuditLogsSettings";

i18n.loadAndActivate({ locale: "en", messages: {} });

const workspace = {
  id: "ws1",
  name: "My WS",
  settings: { timezone: "UTC" },
} as unknown as Workspace;

const renderSettings = (isOwner: boolean) =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <AuditLogsSettings
            workspace={workspace}
            isOwner={isOwner}
            onWorkspaceUpdate={vi.fn()}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>,
  );

describe("AuditLogsSettings", () => {
  beforeEach(() => {
    licence.licensed = true;
  });

  it("shows the licence notice when audit logs are not licensed", () => {
    licence.licensed = false;
    renderSettings(true);
    expect(
      screen.getByText(/Audit logs require a Notifuse Enterprise licence/),
    ).toBeInTheDocument();
  });

  it("hides the notice when licensed", () => {
    renderSettings(true);
    expect(screen.queryByText(/require a Notifuse/)).not.toBeInTheDocument();
  });

  it("links to the Logs page audit tab", () => {
    renderSettings(true);
    expect(screen.getByTestId("audit-log-link")).toHaveAttribute(
      "data-tab",
      "audit",
    );
    expect(screen.getByText("Open the audit log")).toBeInTheDocument();
  });

  it("shows the retention card to owners only", () => {
    renderSettings(false);
    expect(
      screen.queryByTestId("audit-retention-card"),
    ).not.toBeInTheDocument();
  });

  it("shows the retention card to owners", () => {
    renderSettings(true);
    expect(screen.getByTestId("audit-retention-card")).toBeInTheDocument();
  });
});
