import type { Page } from "@playwright/test";
import { expect } from "./auth";
import { openTemplateEditor } from "./template-editor";
import { mockTemplates } from "./mock-data";

// Drag-and-drop in the template editor's structure tree, with real mouse gestures

export interface TreeBlock {
  id: string;
  type: string;
  attributes?: Record<string, unknown>;
  content?: string;
  children?: TreeBlock[];
}

export const text = (id: string, label: string): TreeBlock => ({
  id,
  type: "mj-text",
  attributes: {},
  content: `<p>${label}</p>`,
});
export const column = (
  id: string,
  children: TreeBlock[],
  width?: string,
): TreeBlock => ({
  id,
  type: "mj-column",
  attributes: width ? { width } : {},
  children,
});
export const section = (id: string, children: TreeBlock[]): TreeBlock => ({
  id,
  type: "mj-section",
  attributes: { backgroundColor: "#ffffff" },
  children,
});
export const block = (
  id: string,
  type: string,
  children?: TreeBlock[],
): TreeBlock => ({
  id,
  type,
  attributes: {},
  ...(children ? { children } : {}),
});

// body > S1[C1[T1,BT1,IM1], C2[T2,SO[SE1,SE2]]], W[SW[CW[TW]]], S2[G[G1[TG1],G2[TG2]]], S3[C3[D,SP,T3,T4]], R, L
export const auditBody = (): TreeBlock[] => [
  section("S1", [
    column(
      "C1",
      [
        text("T1", "Alpha"),
        { id: "BT1", type: "mj-button", attributes: {}, content: "Shop" },
        {
          id: "IM1",
          type: "mj-image",
          attributes: { src: "https://placehold.co/300x100" },
        },
      ],
      "50%",
    ),
    column(
      "C2",
      [
        text("T2", "Bravo"),
        block("SO", "mj-social", [
          {
            id: "SE1",
            type: "mj-social-element",
            attributes: { name: "facebook", href: "#" },
            content: "Facebook",
          },
          {
            id: "SE2",
            type: "mj-social-element",
            attributes: { name: "twitter", href: "#" },
            content: "Twitter",
          },
        ]),
      ],
      "50%",
    ),
  ]),
  block("W", "mj-wrapper", [
    section("SW", [column("CW", [text("TW", "Wrapped")])]),
  ]),
  section("S2", [
    block("G", "mj-group", [
      column("G1", [text("TG1", "Left")], "50%"),
      column("G2", [text("TG2", "Right")], "50%"),
    ]),
  ]),
  section("S3", [
    column("C3", [
      block("D", "mj-divider"),
      block("SP", "mj-spacer"),
      text("T3", "Charlie"),
      text("T4", "Delta"),
    ]),
  ]),
  { id: "R", type: "mj-raw", attributes: {}, content: "<div>raw</div>" },
  {
    id: "L",
    type: "mj-liquid",
    attributes: {},
    content: "{{ contact.email }}",
  },
];

const ROW = '.ant-tree-treenode:not([aria-hidden="true"])';

export const shape = (b: TreeBlock): string =>
  b.children && b.children.length
    ? `${b.id}[${b.children.map(shape).join(",")}]`
    : b.id;

export const find = (b: TreeBlock, id: string): TreeBlock | undefined =>
  b.id === id ? b : b.children?.map((child) => find(child, id)).find(Boolean);

export const openTreeEditor = async (page: Page, body: TreeBlock[]) => {
  const root: TreeBlock = {
    id: "ROOT",
    type: "mjml",
    children: [block("HEAD", "mj-head", []), block("B", "mj-body", body)],
  };
  const template = {
    ...mockTemplates[0],
    email: { ...mockTemplates[0].email, visual_editor_tree: root },
  };
  const saves: TreeBlock[] = [];
  await page.route(
    "https://localapi.notifuse.com:4000/api/templates.update*",
    async (route) => {
      saves.push(route.request().postDataJSON().email.visual_editor_tree);
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ template }),
      });
    },
  );
  await page.setViewportSize({ width: 1280, height: 1300 });
  const drawer = await openTemplateEditor(page, template);
  const tree = drawer.locator(".ant-tree").nth(1);

  // Rows in display order: the editor opens with every container that has children expanded
  const visible = (
    expandedOnly = (b: TreeBlock) => !!b.children?.length,
  ): string[] => {
    const ids: string[] = [];
    const walk = (b: TreeBlock) => {
      ids.push(b.id);
      if (expandedOnly(b)) b.children?.forEach(walk);
    };
    walk(root.children![1]);
    return ids;
  };
  const allExpanded = visible();
  let order = allExpanded;
  await expect(tree.locator(ROW)).toHaveCount(order.length);

  const row = (id: string) => tree.locator(ROW).nth(order.indexOf(id));

  const collapse = async (id: string) => {
    await row(id).locator(".ant-tree-switcher").click();
    order = visible((b) => !!b.children?.length && b.id !== id);
    await expect(tree.locator(ROW)).toHaveCount(order.length);
  };

  // Grab `from` by its label and hover `to` at a height (0 top, 1 bottom) and a sideways offset from its label
  const hover = async (from: string, to: string, yFrac: number, dx = 0) => {
    const src = row(from).locator(".ant-tree-node-content-wrapper");
    const dst = row(to).locator(".ant-tree-node-content-wrapper");
    const s = (await src.boundingBox())!;
    await page.mouse.move(s.x + 20, s.y + s.height / 2);
    await page.mouse.down();
    await page.mouse.move(s.x + 25, s.y + s.height / 2 + 3, { steps: 3 });
    const d = (await dst.boundingBox())!;
    await page.mouse.move(d.x + 20 + dx, d.y + d.height * yFrac, { steps: 8 });
    await page.waitForTimeout(150);
    await page.mouse.move(d.x + 21 + dx, d.y + d.height * yFrac, { steps: 2 });
    await page.waitForTimeout(150);
  };
  const lineShown = async () =>
    (await tree.locator(".ant-tree-drop-indicator").count()) > 0;
  const release = async () => {
    await page.mouse.up();
    await page.waitForTimeout(300);
  };
  const drag = async (from: string, to: string, yFrac: number, dx = 0) => {
    await hover(from, to, yFrac, dx);
    await release();
  };

  const saved = async () => {
    const count = saves.length;
    await drawer.getByRole("button", { name: "Save", exact: true }).click();
    await expect.poll(() => saves.length).toBe(count + 1);
    return find(saves[saves.length - 1], "B")!;
  };

  return {
    drawer,
    tree,
    rows: tree.locator(ROW),
    allExpanded: allExpanded.length,
    row,
    collapse,
    hover,
    lineShown,
    release,
    drag,
    saved,
    original: shape(root.children![1]),
  };
};
