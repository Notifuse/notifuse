export interface FileInfo {
  size: number
  size_human: string
  content_type: string // guessed from the file extension
  url: string
}

export interface StorageObject {
  key: string
  name: string
  is_folder: boolean
  path: string
  last_modified: Date
  file_info: FileInfo
  // A folder that exists only as the prefix of other keys, without a "folder/" marker; its date
  // is that of its newest object.
  implicit?: boolean
}

export interface FileManagerProps {
  currentPath?: string
  itemFilters?: ItemFilter[]
  onError: (error: Error) => void
  onSelect: (items: StorageObject[]) => void
  height: number
  acceptFileType: string
  acceptItem: (item: StorageObject) => boolean
  withSelection?: boolean
  multiple?: boolean
  settings?: FileManagerSettings
  onUpdateSettings: (settings: FileManagerSettings) => Promise<void>
  settingsInfo?: React.ReactNode
  readOnly?: boolean
  // Controlled path mode for router sync
  controlledPath?: string
  onPathChange?: (path: string) => void
  // The workspace whose bucket is listed: part of the cached listing's key.
  workspaceId: string
  // Controlled chosen file (the page keeps it in the URL); internal state otherwise.
  controlledFile?: string
  onFileChange?: (key: string | undefined) => void
  // Picker: where the chosen file goes ("the Image block"), shown in the footer.
  targetLabel?: string
  // Picker: the footer's Cancel.
  onCancel?: () => void
}

export interface ItemFilter {
  key: string // item key
  value: string | number | boolean
  operator: string // contains equals greaterThan lessThan
}

export interface FileManagerSettings {
  provider?: string
  endpoint: string
  access_key: string
  bucket: string
  region?: string
  secret_key?: string
  encrypted_secret_key?: string
  cdn_endpoint?: string
  force_path_style?: boolean
}
