import { useQuery } from '@tanstack/react-query'
import { Row, Col, Statistic, Space, Spin } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCircleCheck, faFaceFrown, faHourglass } from '@fortawesome/free-regular-svg-icons'
import { faBan, faTriangleExclamation } from '@fortawesome/free-solid-svg-icons'
import { Link, useParams } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { listsApi } from '../../services/api/list'

interface ListStatsProps {
  workspaceId: string
  listId: string
}

export function ListStats({ workspaceId, listId }: ListStatsProps) {
  const { t } = useLingui()
  const { workspaceId: paramWorkspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  // Use workspaceId from params if available, otherwise use the prop
  const currentWorkspaceId = paramWorkspaceId || workspaceId

  const { data, isLoading } = useQuery({
    queryKey: ['list-stats', workspaceId, listId],
    queryFn: async () => {
      return listsApi.stats({
        workspace_id: workspaceId,
        list_id: listId
      })
    },
    // Refetch every minute to keep stats up to date
    refetchInterval: 60000
  })

  const stats = data?.stats || {
    total_active: 0,
    total_pending: 0,
    total_unsubscribed: 0,
    total_bounced: 0,
    total_complained: 0
  }

  // Formatter function for statistics that handles loading state
  const formatStat = (value: number | string) => {
    if (isLoading) {
      return <Spin size="small" />
    }
    return value
  }

  return (
    <Row gutter={[16, 16]} wrap={false}>
      <Col flex="1">
        <Statistic
          title={
            <Link
              to="/console/workspace/$workspaceId/contacts"
              params={{ workspaceId: currentWorkspaceId }}
              search={{ list_id: listId, contact_list_status: 'active' }}
              className="text-inherit hover:text-primary transition-colors"
            >
              <Space>
                <FontAwesomeIcon
                  icon={faCircleCheck}
                  className="text-green-500"
                  style={{ opacity: 0.7 }}
                />{' '}
                {t`Active`}
              </Space>
            </Link>
          }
          value={stats.total_active}
          valueStyle={{ fontSize: '16px' }}
          formatter={formatStat}
        />
      </Col>
      <Col flex="1">
        <Statistic
          title={
            <Link
              to="/console/workspace/$workspaceId/contacts"
              params={{ workspaceId: currentWorkspaceId }}
              search={{ list_id: listId, contact_list_status: 'pending' }}
              className="text-inherit hover:text-primary transition-colors"
            >
              <Space>
                <FontAwesomeIcon
                  icon={faHourglass}
                  className="text-blue-500"
                  style={{ opacity: 0.7 }}
                />{' '}
                {t`Pending`}
              </Space>
            </Link>
          }
          value={stats.total_pending}
          valueStyle={{ fontSize: '16px' }}
          formatter={formatStat}
        />
      </Col>
      <Col flex="1">
        <Statistic
          title={
            <Link
              to="/console/workspace/$workspaceId/contacts"
              params={{ workspaceId: currentWorkspaceId }}
              search={{ list_id: listId, contact_list_status: 'unsubscribed' }}
              className="text-inherit hover:text-primary transition-colors"
            >
              <Space>
                <FontAwesomeIcon icon={faBan} className="text-gray-500" style={{ opacity: 0.7 }} />{' '}
                {t`Unsub`}
              </Space>
            </Link>
          }
          value={stats.total_unsubscribed}
          valueStyle={{ fontSize: '16px' }}
          formatter={formatStat}
        />
      </Col>
      <Col flex="1">
        <Statistic
          title={
            <Link
              to="/console/workspace/$workspaceId/contacts"
              params={{ workspaceId: currentWorkspaceId }}
              search={{ list_id: listId, contact_list_status: 'bounced' }}
              className="text-inherit hover:text-primary transition-colors"
            >
              <Space>
                <FontAwesomeIcon
                  icon={faTriangleExclamation}
                  className="text-yellow-500"
                  style={{ opacity: 0.7 }}
                />{' '}
                {t`Bounced`}
              </Space>
            </Link>
          }
          value={stats.total_bounced}
          valueStyle={{ fontSize: '16px' }}
          formatter={formatStat}
        />
      </Col>
      <Col flex="1">
        <Statistic
          title={
            <Link
              to="/console/workspace/$workspaceId/contacts"
              params={{ workspaceId: currentWorkspaceId }}
              search={{ list_id: listId, contact_list_status: 'complained' }}
              className="text-inherit hover:text-primary transition-colors"
            >
              <Space>
                <FontAwesomeIcon
                  icon={faFaceFrown}
                  className="text-red-500"
                  style={{ opacity: 0.7 }}
                />{' '}
                {t`Complaints`}
              </Space>
            </Link>
          }
          value={stats.total_complained}
          valueStyle={{ fontSize: '16px' }}
          formatter={formatStat}
        />
      </Col>
    </Row>
  )
}
