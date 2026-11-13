/** The columns of the Messages tab a member may hide, and their default. */
export const MESSAGE_COLUMN_DEFAULTS = {
  recipient: true,
  error: true,
  id: false,
  external_id: false,
  metadata: false,
  template: false,
  list: false,
  created: false
}
export type MessageColumnKey = keyof typeof MESSAGE_COLUMN_DEFAULTS
export type MessageColumns = Record<MessageColumnKey, boolean>

export const MESSAGE_COLUMNS_KEY = 'logs_message_columns'
/** The key of the old message history tab, still written by the contact sheet. */
export const LEGACY_COLUMNS_KEY = 'message_columns_visibility'

const LEGACY_NAMES: Record<string, MessageColumnKey> = {
  id: 'id',
  external_id: 'external_id',
  metadata: 'metadata',
  contact_email: 'recipient',
  template_id: 'template',
  list_id: 'list',
  status_info: 'error',
  created_at: 'created'
}

// What the old tab showed by default. It stored its whole state, defaults included, at the first
// change: only a value that differs from these is a member's choice.
const LEGACY_DEFAULTS: Record<string, boolean> = {
  id: true,
  external_id: false,
  metadata: true,
  contact_email: true,
  template_id: true,
  list_id: true,
  status_info: true,
  created_at: true
}

function read(key: string): Record<string, unknown> | undefined {
  try {
    const raw = localStorage.getItem(key)
    if (raw === null) return undefined
    const parsed: unknown = JSON.parse(raw)
    return parsed && typeof parsed === 'object' ? (parsed as Record<string, unknown>) : {}
  } catch {
    return {}
  }
}

function write(columns: MessageColumns) {
  try {
    localStorage.setItem(MESSAGE_COLUMNS_KEY, JSON.stringify(columns))
  } catch {
    // Storage full or refused: the choice lasts for this page only.
  }
}

/**
 * The columns shown. Without a choice of its own, the tab takes the old tab's once, for the
 * columns that still exist and where the member changed the old default, and stores the result
 * under its key at once.
 */
export function loadMessageColumns(): MessageColumns {
  const columns: MessageColumns = { ...MESSAGE_COLUMN_DEFAULTS }
  const own = read(MESSAGE_COLUMNS_KEY)
  if (own) {
    for (const key of Object.keys(columns) as MessageColumnKey[]) {
      if (typeof own[key] === 'boolean') columns[key] = own[key]
    }
    return columns
  }
  const legacy = read(LEGACY_COLUMNS_KEY) ?? {}
  for (const [oldKey, key] of Object.entries(LEGACY_NAMES)) {
    const value = legacy[oldKey]
    if (typeof value === 'boolean' && value !== LEGACY_DEFAULTS[oldKey]) columns[key] = value
  }
  write(columns)
  return columns
}
