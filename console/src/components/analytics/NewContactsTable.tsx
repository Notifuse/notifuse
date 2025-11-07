import React, { useState, useEffect } from 'react'
import { Card, Button, Table, Tag, Space, Tooltip } from 'antd'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faBan, faTriangleExclamation } from '@fortawesome/free-solid-svg-icons'
import { faHourglass, faFaceFrown, faCircleCheck } from '@fortawesome/free-regular-svg-icons'
import { contactsApi, Contact, ListContactsRequest } from '../../services/api/contacts'
import { listsApi } from '../../services/api/list'
import { Workspace } from '../../services/api/types'
import dayjs from '../../lib/dayjs'

interface NewContactsTableProps {
  workspace: Workspace
}

export const NewContactsTable: React.FC<NewContactsTableProps> = ({ workspace }) => {
  const navigate = useNavigate()
  const [contacts, setContacts] = useState<Contact[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Fetch lists for the current workspace
  const { data: listsData } = useQuery({
    queryKey: ['lists', workspace.id],
    queryFn: () => listsApi.list({ workspace_id: workspace.id })
  })

  const buildParams = (): ListContactsRequest => ({
    workspace_id: workspace.id,
    limit: 5,
    with_contact_lists: true
  })

  const fetchData = async () => {
    try {
      setLoading(true)
      setError(null)

      const params = buildParams()
      const response = await contactsApi.list(params)
      setContacts(response.contacts)
    } catch (err) {
      console.error('Failed to fetch new contacts data:', err)
      setError(err instanceof Error ? err.message : 'Failed to fetch new contacts data')
    } finally {
      setLoading(false)
    }
  }

  const handleViewMore = () => {
    navigate({
      to: '/console/workspace/$workspaceId/contacts',
      params: { workspaceId: workspace.id }
    })
  }

  useEffect(() => {
    fetchData()
  }, [workspace.id])

  const columns = [
    {
      title: 'Email',
      dataIndex: 'email',
      key: 'email',
      render: (email: string) => <span className="text-sm">{email}</span>
    },
    {
      title: 'Lists',
      key: 'lists',
      render: (record: Contact) => (
        <Space direction="vertical" size={2}>
          {record.contact_lists.map(
            (list: { list_id: string; status?: string; created_at?: string }) => {
              let color = 'blue'
              let icon = null
              let statusText = ''

              // Match status to color and icon
              switch (list.status) {
                case 'active':
                  color = 'green'
                  icon = <FontAwesomeIcon icon={faCircleCheck} style={{ marginRight: '4px' }} />
                  statusText = 'Active subscriber'
                  break
                case 'pending':
                  color = 'blue'
                  icon = <FontAwesomeIcon icon={faHourglass} style={{ marginRight: '4px' }} />
                  statusText = 'Pending confirmation'
                  break
                case 'unsubscribed':
                  color = 'gray'
                  icon = <FontAwesomeIcon icon={faBan} style={{ marginRight: '4px' }} />
                  statusText = 'Unsubscribed from list'
                  break
                case 'bounced':
                  color = 'orange'
                  icon = (
                    <FontAwesomeIcon icon={faTriangleExclamation} style={{ marginRight: '4px' }} />
                  )
                  statusText = 'Email bounced'
                  break
                case 'complained':
                  color = 'red'
                  icon = <FontAwesomeIcon icon={faFaceFrown} style={{ marginRight: '4px' }} />
                  statusText = 'Marked as spam'
                  break
                default:
                  color = 'blue'
                  statusText = 'Status unknown'
                  break
              }

              // Find list name from listsData
              const listData = listsData?.lists?.find((l) => l.id === list.list_id)
              const listName = listData?.name || list.list_id

              // Format creation date if available using workspace timezone
              const creationDate = list.created_at
                ? dayjs(list.created_at).tz(workspace.settings.timezone).format('LL - HH:mm')
                : 'Unknown date'

              const tooltipTitle = (
                <>
                  <div>
                    <strong>{statusText}</strong>
                  </div>
                  <div>Subscribed on: {creationDate}</div>
                  <div>
                    <small>Timezone: {workspace.settings.timezone}</small>
                  </div>
                </>
              )

              return (
                <Tooltip key={list.list_id} title={tooltipTitle}>
                  <Tag
                    bordered={false}
                    color={color}
                    style={{ marginBottom: '2px' }}
                    className="text-xs"
                  >
                    {icon}
                    {listName}
                  </Tag>
                </Tooltip>
              )
            }
          )}
        </Space>
      )
    },
    {
      title: 'Name',
      key: 'name',
      render: (record: Contact) => {
        const name = [record.first_name, record.last_name].filter(Boolean).join(' ')
        return <span className="text-sm">{name || '-'}</span>
      }
    },
    {
      title: 'Language',
      dataIndex: 'language',
      key: 'language',
      render: (language: string) => <span className="text-sm">{language || '-'}</span>
    },
    {
      title: 'Timezone',
      dataIndex: 'timezone',
      key: 'timezone',
      render: (timezone: string) => <span className="text-sm">{timezone || '-'}</span>
    },
    {
      title: 'Country',
      dataIndex: 'country',
      key: 'country',
      render: (country: string) => <span className="text-sm">{country || '-'}</span>
    },
    {
      title: 'Since',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => (
        <span
          className="text-xs text-gray-500"
          title={dayjs(date).tz(workspace.settings.timezone).format('lll')}
        >
          {dayjs(date).fromNow()}
        </span>
      )
    }
  ]

  const cardExtra = (
    <Button type="link" size="small" onClick={handleViewMore}>
      View more
    </Button>
  )

  return (
    <Card title="Recent New Contacts" extra={cardExtra}>
      {error ? (
        <div className="text-red-500 p-4">
          <p>Error: {error}</p>
        </div>
      ) : (
        <Table
          dataSource={contacts}
          columns={columns}
          rowKey="email"
          pagination={false}
          loading={loading}
          size="small"
          showHeader={true}
        />
      )}
    </Card>
  )
}
