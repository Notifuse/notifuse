import { Handle, Position, useConnection } from '@xyflow/react'
import { colors } from '../../../theme/tokens'

/** Handle colours, from the theme tokens. */
export const HANDLE_COLORS = {
  default: colors.primary[500],
  orphan: colors.status.warning,
  connecting: colors.status.success,
  yes: colors.status.success,
  no: colors.status.danger,
  muted: colors.gray[400],
  warning: colors.status.warning
} as const

export type HandleTone = keyof typeof HANDLE_COLORS

/** The input of a step: it grows while a connection is being drawn. */
export function TargetHandle({ isOrphan }: { isOrphan?: boolean }) {
  const connection = useConnection()
  const size = connection.inProgress ? 16 : 10
  const background = connection.inProgress ? HANDLE_COLORS.connecting : isOrphan ? HANDLE_COLORS.orphan : HANDLE_COLORS.default
  return (
    <Handle
      type="target"
      position={Position.Top}
      style={{ background, width: size, height: size, transition: 'all 0.15s ease' }}
    />
  )
}

/** An output of a step; `left` places one of several along the bottom edge. */
export function SourceHandle({ id, left, tone, isOrphan }: { id?: string; left?: string; tone?: HandleTone; isOrphan?: boolean }) {
  const background = HANDLE_COLORS[tone ?? (isOrphan ? 'orphan' : 'default')]
  return (
    <Handle
      type="source"
      position={Position.Bottom}
      id={id}
      style={{ background, width: 10, height: 10, ...(left ? { left } : {}) }}
    />
  )
}
