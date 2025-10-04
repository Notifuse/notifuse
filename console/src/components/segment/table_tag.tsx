import { Tag } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faMousePointer } from '@fortawesome/free-solid-svg-icons'
import { faUser, faFolderOpen } from '@fortawesome/free-regular-svg-icons'

export interface TableTagProps {
  table: string
}
const TableTag = (props: TableTagProps) => {
  // magenta red volcano orange gold lime green cyan blue geekblue purple
  const table = props.table.toLowerCase()
  let color = 'geekblue'
  let label = props.table
  let icon = null

  if (table === 'contacts') {
    color = 'lime'
    label = 'Contact property'
    icon = faUser
  }
  if (table === 'contact_lists') {
    color = 'magenta'
    label = 'List subscription'
    icon = faFolderOpen
  }
  if (table === 'contact_timeline') {
    color = 'cyan'
    label = 'Activity'
    icon = faMousePointer
  }

  return (
    <Tag style={{ margin: 0 }} bordered={false} color={color}>
      {icon && <FontAwesomeIcon icon={icon} style={{ width: 18, marginRight: 8 }} />}
      {label}
    </Tag>
  )
}

export default TableTag
