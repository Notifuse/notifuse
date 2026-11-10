import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, fireEvent, createEvent, screen } from "@testing-library/react";
import { TreePanel } from "./TreePanel";
import type { EmailBlock } from "../types";

interface Node {
  id: string;
  type: string;
  attributes: Record<string, unknown>;
  content?: string;
  children?: Node[];
}

const n = (id: string, type: string, children?: Node[]): Node => ({
  id,
  type,
  attributes: {},
  children,
});
const text = (id: string): Node => ({
  id,
  type: "mj-text",
  attributes: {},
  content: `<p>${id}</p>`,
});

// Body: S1[C2[T2, SO[SE1, SE2]]], S3[C3[...c3]], W[SW[CW[TW]]]
const build = (c3: Node[]): EmailBlock =>
  n("ROOT", "mjml", [
    n("HEAD", "mj-head", []),
    n("B", "mj-body", [
      n("S1", "mj-section", [
        n("C2", "mj-column", [
          text("T2"),
          n("SO", "mj-social", [
            n("SE1", "mj-social-element", []),
            n("SE2", "mj-social-element", []),
          ]),
        ]),
      ]),
      n("S3", "mj-section", [n("C3", "mj-column", c3)]),
      n("W", "mj-wrapper", [
        n("SW", "mj-section", [n("CW", "mj-column", [text("TW")])]),
      ]),
    ]),
  ]) as unknown as EmailBlock;

const c3 = () => [
  n("D", "mj-divider"),
  n("SP", "mj-spacer"),
  text("T3"),
  text("T4"),
];
const ALL = ["B", "S1", "C2", "SO", "S3", "C3", "W", "SW", "CW"];

// Rows of the body tree in display order: a pre-order walk of the visible blocks
const visibleOrder = (
  tree: EmailBlock,
  expanded: string[],
  hidden: string[] = [],
): string[] => {
  const ids: string[] = [];
  const walk = (b: EmailBlock) => {
    ids.push(b.id);
    if (expanded.includes(b.id))
      b.children?.filter((c) => !hidden.includes(c.type)).forEach(walk);
  };
  walk(tree.children!.find((c) => c.type === "mj-body")!);
  return ids;
};

const setup = (
  tree: EmailBlock,
  expanded: string[],
  hiddenBlocks?: string[],
) => {
  const onMoveBlock = vi.fn();
  const onTreeExpand = vi.fn();
  const utils = render(
    <TreePanel
      emailTree={tree}
      selectedBlockId={null}
      onSelectBlock={vi.fn()}
      onAddBlock={vi.fn()}
      onAddSavedBlock={vi.fn()}
      onDeleteBlock={vi.fn()}
      onCloneBlock={vi.fn()}
      onMoveBlock={onMoveBlock}
      expandedKeys={expanded}
      onTreeExpand={onTreeExpand}
      hiddenBlocks={hiddenBlocks}
    />,
  );
  const trees = utils.container.querySelectorAll(".ant-tree");
  const bodyTree = trees[trees.length - 1] as HTMLElement;
  const order = visibleOrder(tree, expanded, hiddenBlocks);
  const row = (id: string) => {
    const rows = Array.from(
      bodyTree.querySelectorAll('[role="treeitem"]'),
    ) as HTMLElement[];
    expect(rows).toHaveLength(order.length);
    return rows[order.indexOf(id)];
  };
  return { row, onMoveBlock, onTreeExpand, bodyTree };
};

// jsdom has no DragEvent: coordinates are set on the event by hand, and the target row gets a box
const fire = (
  el: HTMLElement,
  type: "dragStart" | "dragEnter" | "dragOver" | "drop",
  init: Record<string, number>,
) => {
  const event = createEvent[type](el, init);
  for (const [key, value] of Object.entries(init)) {
    if ((event as unknown as Record<string, unknown>)[key] === undefined) {
      Object.defineProperty(event, key, { value });
    }
  }
  return fireEvent(el, event);
};
const box = (el: HTMLElement) => {
  el.getBoundingClientRect = () =>
    ({
      top: 100,
      height: 28,
      left: 0,
      right: 300,
      bottom: 128,
      width: 300,
      x: 0,
      y: 100,
      toJSON: () => ({}),
    }) as DOMRect;
};
// Grab a row and hover the lower half of another, without moving sideways
const dragOverLowerHalf = (from: HTMLElement, to: HTMLElement) => {
  box(to);
  fire(from, "dragStart", { clientX: 200, clientY: 50 });
  fire(to, "dragEnter", { clientX: 200, clientY: 120 });
  fire(to, "dragOver", { clientX: 200, clientY: 120 });
};
const drop = (to: HTMLElement) =>
  fire(to, "drop", { clientX: 200, clientY: 120 });

// rc-tree reads the indent width from offsetWidth, which jsdom reports as 0
const offsetWidth = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  "offsetWidth",
);

describe("TreePanel drag and drop", () => {
  beforeEach(() => {
    Object.defineProperty(HTMLElement.prototype, "offsetWidth", {
      configurable: true,
      get: () => 24,
    });
    vi.spyOn(console, "warn").mockImplementation(() => {});
  });
  afterEach(() => {
    if (offsetWidth)
      Object.defineProperty(HTMLElement.prototype, "offsetWidth", offsetWidth);
    vi.restoreAllMocks();
  });

  it("moves a block down within its column to the slot the line shows", () => {
    const { row, onMoveBlock } = setup(build(c3()), ALL);
    dragOverLowerHalf(row("D"), row("T3"));
    drop(row("T3"));
    expect(onMoveBlock).toHaveBeenCalledWith("D", "C3", 2);
  });

  it("drops a text into a collapsed column and expands it", () => {
    const expanded = ALL.filter((key) => key !== "C3");
    const { row, onMoveBlock, onTreeExpand } = setup(build(c3()), expanded);
    dragOverLowerHalf(row("T2"), row("C3"));
    drop(row("C3"));
    expect(onMoveBlock).toHaveBeenCalledWith("T2", "C3", 4);
    expect(onTreeExpand).toHaveBeenCalledWith(
      expect.arrayContaining([...expanded, "C3"]),
    );
  });

  it("shows no line where the block cannot go, and does not move it", () => {
    const { row, onMoveBlock, bodyTree } = setup(build(c3()), ALL);
    dragOverLowerHalf(row("T2"), row("SE1"));
    expect(bodyTree.querySelector(".ant-tree-drop-indicator")).toBeNull();
    drop(row("SE1"));
    expect(onMoveBlock).not.toHaveBeenCalled();
  });

  it("counts children the tree hides when computing the index", () => {
    const tree = build([
      { id: "RH", type: "mj-raw", attributes: {}, content: "x" },
      ...c3(),
    ]);
    const { row, onMoveBlock } = setup(tree, ALL, ["mj-raw"]);
    dragOverLowerHalf(row("T4"), row("D"));
    drop(row("D"));
    expect(onMoveBlock).toHaveBeenCalledWith("T4", "C3", 2);
  });

  it("gives the wrapper a drag handle and keeps the body still", () => {
    const { row } = setup(build(c3()), ALL);
    expect(
      row("W")
        .querySelector("[data-draggable]")
        ?.getAttribute("data-draggable"),
    ).toBe("true");
    expect(
      row("B")
        .querySelector("[data-draggable]")
        ?.getAttribute("data-draggable"),
    ).toBe("false");
    expect(
      screen.getAllByLabelText("Drag to move this block in the tree").length,
    ).toBeGreaterThan(0);
    expect(
      row("W").querySelector(
        '[aria-label="Drag to move this block in the tree"]',
      ),
    ).not.toBeNull();
  });

  it("cancels a drag started on the body row, not on a section row", () => {
    const { row } = setup(build(c3()), ALL);
    expect(fire(row("B"), "dragStart", { clientX: 200, clientY: 50 })).toBe(
      false,
    );
    expect(fire(row("S1"), "dragStart", { clientX: 200, clientY: 50 })).toBe(
      true,
    );
  });
});
