import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("./client", () => ({
  api: {
    get: vi.fn().mockResolvedValue({ logs: [], has_more: false }),
    post: vi.fn().mockResolvedValue({ status: "success" }),
    download: vi.fn().mockResolvedValue(new Blob(["id,action\n"])),
  },
}));

import { api } from "./client";
import { auditLogApi, buildAuditLogQuery } from "./audit_log";

describe("buildAuditLogQuery", () => {
  it("sends only the filters that are set, lists comma-joined", () => {
    const query = buildAuditLogQuery({
      scope: "workspace",
      workspace_id: "ws1",
      actions: ["templates.update", "lists.create"],
      categories: [],
      actor_email: "",
      outcome: "denied",
      from: "2026-01-01T00:00:00.000Z",
      limit: 20,
    });
    expect(query.get("scope")).toBe("workspace");
    expect(query.get("workspace_id")).toBe("ws1");
    expect(query.get("actions")).toBe("templates.update,lists.create");
    expect(query.get("outcome")).toBe("denied");
    expect(query.get("from")).toBe("2026-01-01T00:00:00.000Z");
    expect(query.get("limit")).toBe("20");
    expect(query.has("categories")).toBe(false);
    expect(query.has("actor_email")).toBe(false);
    expect(query.has("cursor")).toBe(false);
  });
});

describe("auditLogApi", () => {
  beforeEach(() => {
    vi.mocked(api.get).mockClear();
    vi.mocked(api.post).mockClear();
    vi.mocked(api.download).mockClear();
  });

  it("lists with the query string", async () => {
    await auditLogApi.list({ workspace_id: "ws1", cursor: "abc" });
    expect(api.get).toHaveBeenCalledWith(
      "/api/auditLogs.list?workspace_id=ws1&cursor=abc",
    );
  });

  it("gets one entry in its scope", async () => {
    vi.mocked(api.get).mockResolvedValueOnce({ log: { id: "a" } });
    await auditLogApi.get("a", "deployment");
    expect(api.get).toHaveBeenCalledWith(
      "/api/auditLogs.get?scope=deployment&id=a",
    );
  });

  it("exports through the download path, without a cursor, with the format", async () => {
    const blob = await auditLogApi.export(
      { workspace_id: "ws1", cursor: "stale", actions: ["x"] },
      "ndjson",
    );
    expect(blob).toBeInstanceOf(Blob);
    expect(api.download).toHaveBeenCalledWith("/api/auditLogs.export", {
      workspace_id: "ws1",
      cursor: undefined,
      actions: ["x"],
      format: "ndjson",
    });
  });

  it("saves the workspace retention", async () => {
    await auditLogApi.setWorkspaceSettings("ws1", { retention_days: 90 });
    expect(api.post).toHaveBeenCalledWith(
      "/api/workspaces.setAuditLogSettings",
      {
        workspace_id: "ws1",
        settings: { retention_days: 90 },
      },
    );
  });
});
