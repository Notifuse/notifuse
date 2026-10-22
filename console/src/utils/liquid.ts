/** Whether a value holds Liquid: an output (`{{ var }}`) or a tag (`{% if x %}`). */
export function isLiquidExpression(value: string): boolean {
  return /\{\{[^}]+\}\}|\{%.*?%\}/.test(value)
}
