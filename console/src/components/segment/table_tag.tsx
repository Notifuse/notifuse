import { Tag } from 'antd'

export interface TableTagProps {
  table: string
}
const TableTag = (props: TableTagProps) => {
  // magenta red volcano orange gold lime green cyan blue geekblue purple
  const table = props.table.toLowerCase()
  let color = 'geekblue'

  if (table === 'contact') color = 'lime'
  if (table === 'contact_list') color = 'magenta'
  if (table === 'contact_timeline') color = 'cyan'

  return (
    <Tag style={{ margin: 0 }} bordered={false} color={color}>
      {props.table}
    </Tag>
  )
}

export default TableTag
