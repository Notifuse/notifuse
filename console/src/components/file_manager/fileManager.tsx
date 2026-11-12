import { FileExplorer, type FileExplorerMode } from './FileExplorer'
import type { FileManagerProps } from './interfaces'

/** The file manager, kept under its name for its callers: the explorer, in page mode by default. */
export const FileManager = ({ mode = 'page', ...props }: FileManagerProps & { mode?: FileExplorerMode }) => (
  <FileExplorer mode={mode} {...props} />
)
