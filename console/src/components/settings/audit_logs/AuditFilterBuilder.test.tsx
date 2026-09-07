import { describe, it, expect, vi } from "vitest";
import {
  render,
  screen,
  fireEvent,
  waitFor,
  within,
} from "@testing-library/react";
import { App, ConfigProvider } from "antd";
import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import dayjs from "../../../lib/dayjs";

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
      actions: vi.fn().mockResolvedValue({
        actions: [{ action: "templates.update", category: "templates" }],
        categories: ["templates"],
      }),
    },
  };
});

import {
  AuditFilterBuilder,
  EMPTY_AUDIT_FILTERS,
  hasAuditFilterValue,
} from "./AuditFilterBuilder";

i18n.loadAndActivate({ locale: "en", messages: {} });

// antd keeps a closed popover in the DOM; only the open one holds the live Clear.
const openPopover = () =>
  document.querySelector(".ant-popover:not(.ant-popover-hidden)");

const renderBuilder = (filters = EMPTY_AUDIT_FILTERS) => {
  const onChange = vi.fn();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <AuditFilterBuilder filters={filters} onChange={onChange} />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>,
  );
  return onChange;
};

describe("AuditFilterBuilder", () => {
  it("renders one plain button per filter, like the other log tabs", () => {
    renderBuilder();
    for (const label of [
      "Action",
      "Outcome",
      "Actor email",
      "Target ID",
      "IP address",
      "Date range",
    ]) {
      expect(screen.getByRole("button", { name: label })).toBeInTheDocument();
    }
    expect(
      screen.queryByRole("button", { name: /clear all/i }),
    ).not.toBeInTheDocument();
  });

  it("opens the popover, applies the value on Enter and reports it", async () => {
    const onChange = renderBuilder();
    fireEvent.click(screen.getByRole("button", { name: "Target ID" }));
    const input = await screen.findByPlaceholderText("Enter Target ID");
    fireEvent.change(input, { target: { value: "t-1" } });
    fireEvent.keyDown(input, { key: "Enter", code: "Enter" });
    expect(onChange).toHaveBeenCalledWith({ target_id: "t-1" });
  });

  it('shows an active filter as "label: value", offers Clear, and Clear All', async () => {
    const onChange = renderBuilder({
      ...EMPTY_AUDIT_FILTERS,
      outcome: "denied",
      range: [dayjs("2026-09-01"), dayjs("2026-09-05")],
    });
    const outcome = screen.getByTestId("audit-filter-outcome");
    expect(outcome.textContent).toBe("Outcome: Denied");
    expect(screen.getByTestId("audit-filter-range").textContent).toContain(
      "Sep 1, 2026",
    );

    fireEvent.click(outcome);
    await waitFor(() => expect(openPopover()).not.toBeNull());
    // The Select's own clear icon is also a button named "Clear"; the filter's is the antd Button.
    const clear = within(openPopover() as HTMLElement)
      .getAllByRole("button", { name: "Clear" })
      .find((el) => el.classList.contains("ant-btn"));
    fireEvent.click(clear as HTMLElement);
    expect(onChange).toHaveBeenCalledWith({ outcome: undefined });

    fireEvent.click(screen.getByRole("button", { name: /clear all/i }));
    expect(onChange).toHaveBeenLastCalledWith({ ...EMPTY_AUDIT_FILTERS });
  });

  it("does not apply an empty value", async () => {
    const onChange = renderBuilder();
    fireEvent.click(screen.getByRole("button", { name: "IP address" }));
    const apply = await screen.findByRole("button", { name: /apply/i });
    expect(apply).toBeDisabled();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("lists the catalogue in the action picker", async () => {
    renderBuilder();
    fireEvent.click(screen.getByRole("button", { name: "Action" }));
    await waitFor(() =>
      expect(screen.getByText("Select Action")).toBeInTheDocument(),
    );
  });
});

describe("hasAuditFilterValue", () => {
  it("knows an empty filter from a set one", () => {
    expect(hasAuditFilterValue("actions", EMPTY_AUDIT_FILTERS)).toBe(false);
    expect(
      hasAuditFilterValue("actions", {
        ...EMPTY_AUDIT_FILTERS,
        actions: ["x"],
      }),
    ).toBe(true);
    expect(
      hasAuditFilterValue("actor_email", {
        ...EMPTY_AUDIT_FILTERS,
        actor_email: "a",
      }),
    ).toBe(true);
    expect(hasAuditFilterValue("range", EMPTY_AUDIT_FILTERS)).toBe(false);
  });
});
