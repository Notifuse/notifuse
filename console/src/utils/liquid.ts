/**
 * Whether a value holds a Liquid output (`{{ var }}`). The editor's link fields
 * accept such a value in place of a URL, since it has no address until the
 * email is sent.
 */
export function hasLiquidOutput(value: string): boolean {
  return /\{\{[^}]+\}\}/.test(value)
}

/** Whether a value holds Liquid: an output (`{{ var }}`) or a tag (`{% if x %}`). */
export function isLiquidExpression(value: string): boolean {
  return hasLiquidOutput(value) || /\{%.*?%\}/.test(value)
}
