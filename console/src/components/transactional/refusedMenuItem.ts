/**
 * The props of a menu entry a rule refuses. Not antd's `disabled`, which takes the entry out of
 * the keyboard's path: the entry stays reachable, drawn with antd's own disabled class, and its
 * label (MenuEntryLabel) says why. The menu's click handler ignores it.
 */
export const refusedMenuItem = (reason: string | undefined) =>
  reason !== undefined ? { className: 'ant-dropdown-menu-item-disabled' } : {}
