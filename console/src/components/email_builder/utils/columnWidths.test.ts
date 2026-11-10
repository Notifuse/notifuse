import { describe, it, expect } from "vitest";
import type { EmailBlock } from "../types";
import { rebalanceAfterAdd, rebalanceAfterRemove } from "./columnWidths";

interface Node {
  id: string;
  type: string;
  attributes?: Record<string, unknown>;
  children?: Node[];
}

const col = (id: string, width?: string): Node => ({
  id,
  type: "mj-column",
  attributes: width === undefined ? {} : { width },
  children: [],
});
const container = (type: string, children: Node[]) =>
  ({ id: "X", type, attributes: {}, children }) as unknown as EmailBlock;
const widths = (block: EmailBlock) =>
  (block.children ?? []).map(
    (child) => (child.attributes as Record<string, unknown>)?.width,
  );

describe("rebalanceAfterAdd", () => {
  it("gives the new column its share and keeps the ratio of the others", () => {
    const section = container("mj-section", [
      col("a", "30%"),
      col("b", "70%"),
      col("new", "100%"),
    ]);
    rebalanceAfterAdd(section, "new");
    expect(widths(section)).toEqual(["20%", "46.67%", "33.33%"]);
  });

  it("lets the last column in order take the rounding remainder", () => {
    const section = container("mj-section", [
      col("a", "50%"),
      col("b", "50%"),
      col("new"),
    ]);
    rebalanceAfterAdd(section, "new");
    expect(widths(section)).toEqual(["33.33%", "33.33%", "33.34%"]);
  });

  it("equalises when the other widths are not percentages adding up to 100", () => {
    for (const others of [
      ["200px", "400px"],
      ["auto", "50%"],
      [undefined, "50%"],
      ["40%", "40%"],
    ]) {
      const section = container("mj-section", [
        col("a", others[0]),
        col("b", others[1]),
        col("new"),
      ]);
      rebalanceAfterAdd(section, "new");
      expect(widths(section)).toEqual(["33.33%", "33.33%", "33.34%"]);
    }
  });

  it("leaves groups, raw HTML and Liquid alone and does not count them", () => {
    const section = container("mj-section", [
      { id: "g", type: "mj-group", attributes: {}, children: [] },
      col("a", "100%"),
      { id: "r", type: "mj-raw", attributes: {} },
      col("new"),
    ]);
    rebalanceAfterAdd(section, "new");
    expect(widths(section)).toEqual([undefined, "50%", undefined, "50%"]);
  });

  it("rebalances the columns of a group the same way", () => {
    const group = container("mj-group", [
      col("a", "30%"),
      col("b", "70%"),
      col("new"),
    ]);
    rebalanceAfterAdd(group, "new");
    expect(widths(group)).toEqual(["20%", "46.67%", "33.33%"]);
  });

  it("gives a lone column 100%", () => {
    const section = container("mj-section", [col("new", "30%")]);
    rebalanceAfterAdd(section, "new");
    expect(widths(section)).toEqual(["100%"]);
  });

  it("does nothing outside a section or a group", () => {
    const column = container("mj-column", [col("new", "30%")]);
    rebalanceAfterAdd(column, "new");
    expect(widths(column)).toEqual(["30%"]);
  });
});

describe("rebalanceAfterRemove", () => {
  it("scales the remaining columns to 100% keeping their ratio", () => {
    const section = container("mj-section", [col("a", "20%"), col("b", "30%")]);
    rebalanceAfterRemove(section);
    expect(widths(section)).toEqual(["40%", "60%"]);
  });

  it("equalises three columns with pixel widths, the last taking the remainder", () => {
    const section = container("mj-section", [
      col("a", "100px"),
      col("b", "200px"),
      col("c", "300px"),
    ]);
    rebalanceAfterRemove(section);
    expect(widths(section)).toEqual(["33.33%", "33.33%", "33.34%"]);
  });

  it("equalises zero widths instead of writing NaN", () => {
    const section = container("mj-section", [col("a", "0%"), col("b", "0%")]);
    rebalanceAfterRemove(section);
    expect(widths(section)).toEqual(["50%", "50%"]);
  });

  it("gives the last remaining column 100% and leaves an empty section unchanged", () => {
    const section = container("mj-section", [col("a", "30%")]);
    rebalanceAfterRemove(section);
    expect(widths(section)).toEqual(["100%"]);

    const empty = container("mj-section", []);
    rebalanceAfterRemove(empty);
    expect(empty.children).toEqual([]);
  });
});
