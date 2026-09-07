import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { App, ConfigProvider } from "antd";
import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { forwardRef, useImperativeHandle } from "react";

const state = vi.hoisted(() => ({
  navigate: vi.fn(),
  access: { canRead: true, isRoot: false, loading: false },
  refresh: vi.fn(),
  exportAs: vi.fn().mockResolvedValue(undefined),
}));

// services/api/client imports the router, which imports every page; stub it so the
// real router never loads under the mock below.
vi.mock("../services/api/client", () => ({
  api: { get: vi.fn(), post: vi.fn(), download: vi.fn() },
}));
vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => state.navigate,
  useParams: () => ({ workspaceId: "ws1" }),
  useSearch: () => ({}),
}));
vi.mock("../contexts/AuthContext", () => ({
  useAuth: () => ({
    user: { id: "u1", email: "ann@example.com" },
    workspaces: [{ id: "ws1", settings: { timezone: "UTC" } }],
  }),
}));
vi.mock("../hooks/useAuditLogAccess", () => ({
  useAuditLogAccess: () => state.access,
}));
vi.mock("../components/messages/MessageHistoryTab", () => ({
  MessageHistoryTab: () => <div>messages-tab</div>,
}));
vi.mock("../components/webhooks/InboundWebhookEventsTab", () => ({
  InboundWebhookEventsTab: () => <div>inbound-tab</div>,
}));
vi.mock("../components/webhooks/OutgoingWebhooksTab", () => ({
  OutgoingWebhooksTab: () => <div>outgoing-tab</div>,
}));
vi.mock("../components/logs/AuditLogsTab", () => ({
  AuditLogsTab: forwardRef(function FakeAuditLogsTab(_props, ref) {
    useImperativeHandle(ref, () => ({
      refresh: state.refresh,
      exportAs: state.exportAs,
    }));
    return <div>audit-tab</div>;
  }),
}));

import { LogsPage } from "./LogsPage";

i18n.loadAndActivate({ locale: "en", messages: {} });

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <LogsPage />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>,
  );

describe("LogsPage audit tab", () => {
  beforeEach(() => {
    state.access = { canRead: true, isRoot: false, loading: false };
    state.navigate.mockClear();
    state.refresh.mockClear();
    state.exportAs.mockClear();
  });

  it("offers the Audit tab only to those who may read the log", () => {
    renderPage();
    expect(screen.getByText("Audit logs")).toBeInTheDocument();
  });

  it("hides the Audit tab without the grant", () => {
    state.access = { canRead: false, isRoot: false, loading: false };
    renderPage();
    expect(screen.queryByText("Audit logs")).not.toBeInTheDocument();
  });

  it("shows Refresh and Export in the tab bar only on the Audit tab, records the tab in the URL, and drives the tab", async () => {
    renderPage();
    expect(
      screen.queryByRole("button", { name: /refresh/i }),
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByText("Audit logs"));
    expect(await screen.findByText("audit-tab")).toBeInTheDocument();
    expect(state.navigate).toHaveBeenCalledWith(
      expect.objectContaining({ replace: true }),
    );
    const navigation = state.navigate.mock.calls[0][0] as {
      search: (previous: Record<string, unknown>) => Record<string, unknown>;
    };
    expect(navigation.search({ is_failed: "true" })).toEqual({
      is_failed: "true",
      tab: "audit",
    });

    fireEvent.click(await screen.findByRole("button", { name: /refresh/i }));
    expect(state.refresh).toHaveBeenCalled();

    fireEvent.mouseEnter(screen.getByTestId("audit-export"));
    fireEvent.click(await screen.findByText("NDJSON"));
    await waitFor(() => expect(state.exportAs).toHaveBeenCalledWith("ndjson"));

    fireEvent.click(screen.getByText("Message History"));
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: /refresh/i }),
      ).not.toBeInTheDocument(),
    );
  });
});
