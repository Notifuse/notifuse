import type { EmailBlock } from "../types";
import { EmailBlockClass } from "../EmailBlockClass";

// rc-tree's relative drop position: before the row (rc-tree only offers it on the first root row), first child of
// the row, or after the row and its visible subtree
export type DropSide = -1 | 0 | 1;

// Where a dragged block goes: index is its position in parent.children once the block has been taken out
export interface DropTarget {
  parentId: string;
  index: number;
}

interface Located {
  block: EmailBlock;
  parent: EmailBlock | null;
}

const locate = (tree: EmailBlock, id: string): Located | null => {
  if (tree.id === id) return { block: tree, parent: null };
  const walk = (parent: EmailBlock): Located | null => {
    for (const child of parent.children ?? []) {
      if (child.id === id) return { block: child, parent };
      const found = walk(child);
      if (found) return found;
    }
    return null;
  };
  return walk(tree);
};

const contains = (ancestor: EmailBlock, id: string): boolean =>
  ancestor.id === id ||
  (ancestor.children ?? []).some((child) => contains(child, id));

/**
 * Resolves a drop proposed by rc-tree into the place the dragged block will take, or null when there is none
 * (the line is then not shown). The same answer drives the line and the move, so a line always means a move.
 *
 * Candidates are tried in order and the first valid one decides; when it is the block's current place, the
 * result is null rather than a later candidate.
 */
export function resolveDropTarget(
  tree: EmailBlock,
  dragId: string,
  dropId: string,
  side: DropSide,
  expandedKeys: readonly string[],
): DropTarget | null {
  const drag = locate(tree, dragId);
  const drop = locate(tree, dropId);
  if (!drag || !drop || !drag.parent) return null;

  const parentOf = (block: EmailBlock) =>
    locate(tree, block.id)?.parent ?? null;
  const isOpen = (block: EmailBlock) =>
    !!block.children?.length && expandedKeys.includes(block.id);
  const isContainer = (block: EmailBlock) =>
    EmailBlockClass.from(block).canHaveChildren();
  const valid = (parent: EmailBlock | null): parent is EmailBlock =>
    !!parent &&
    !contains(drag.block, parent.id) &&
    EmailBlockClass.from(parent).canAcceptChild(drag.block.type);

  // Turns a slot counted in the parent's current children into the final index, or null for no move
  const settle = (parent: EmailBlock, slot: number): DropTarget | null => {
    const children = parent.children ?? [];
    const current = children.findIndex((child) => child.id === dragId);
    const index = current !== -1 && current < slot ? slot - 1 : slot;
    if (current !== -1 && current === index) return null;
    return { parentId: parent.id, index };
  };
  const indexIn = (parent: EmailBlock, block: EmailBlock) =>
    (parent.children ?? []).findIndex((child) => child.id === block.id);

  const candidates: Array<() => DropTarget | null | undefined> = [];
  const tryAt = (parent: EmailBlock | null, slot: () => number) => {
    candidates.push(() => (valid(parent) ? settle(parent, slot()) : undefined));
  };

  if (side === 0) {
    tryAt(drop.block, () => 0);
  } else if (side === -1) {
    tryAt(drop.parent, () => indexIn(drop.parent!, drop.block));
    tryAt(drop.block, () => 0);
  } else {
    // 1. An empty container takes the block rather than the slot after it
    if (isContainer(drop.block) && !drop.block.children?.length)
      tryAt(drop.block, () => 0);
    // 2. The level rc-tree proposes
    tryAt(drop.parent, () => indexIn(drop.parent!, drop.block) + 1);
    // 3. Down the chain of open last children, shallowest first: the gap is after their visible content
    for (
      let node: EmailBlock | undefined = drop.block;
      node && isContainer(node);
    ) {
      const current: EmailBlock = node;
      tryAt(current, () => current.children?.length ?? 0);
      node = isOpen(current)
        ? current.children![current.children!.length - 1]
        : undefined;
    }
    // 4. Up the chain of last children: the gap after a last child is also the gap after its parent
    for (let node: EmailBlock = drop.block, parent = drop.parent; parent;) {
      const grandParent = parentOf(parent);
      const siblings = parent.children ?? [];
      if (!grandParent || siblings[siblings.length - 1]?.id !== node.id) break;
      const lifted: EmailBlock = parent;
      tryAt(grandParent, () => indexIn(grandParent, lifted) + 1);
      node = parent;
      parent = grandParent;
    }
  }

  for (const candidate of candidates) {
    const result = candidate();
    if (result !== undefined) return result;
  }
  return null;
}
