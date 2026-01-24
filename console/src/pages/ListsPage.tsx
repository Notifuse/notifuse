import { useQuery } from '@tanstack/react-query'
import {
  Card,
  Row,
  Col,
  Tag,
  Typography,
  Space,
  Tooltip,
  Descriptions,
  Button,
  Divider,
  Modal,
  Input,
  message
} from 'antd'
import { useParams } from '@tanstack/react-router'
import { listsApi } from '../services/api/list'
import { templatesApi } from '../services/api/template'
import type { List, TemplateReference, Workspace } from '../services/api/types'
import { CreateListDrawer } from '../components/lists/ListDrawer'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPenToSquare, faTrashCan } from '@fortawesome/free-regular-svg-icons'
import { faRefresh } from '@fortawesome/free-solid-svg-icons'
import { Check, X } from 'lucide-react'
import TemplatePreviewDrawer from '../components/templates/TemplatePreviewDrawer'
import { CreateTemplateDrawer } from '../components/templates/CreateTemplateDrawer'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ImportContactsToListButton } from '../components/lists/ImportContactsToListButton'
import { ListStats } from '../components/lists/ListStats'
import { useLingui } from '@lingui/react/macro'

const { Title, Paragraph, Text } = Typography

// Component to fetch template data and render the preview popover
const TemplatePreviewButton = ({
  templateRef,
  workspace
}: {
  templateRef: TemplateReference
  workspace: Workspace
}) => {
  const { t } = useLingui()
  const { data, isLoading } = useQuery({
    queryKey: ['template', workspace.id, templateRef.id, templateRef.version],
    queryFn: async () => {
      const response = await templatesApi.get({
        workspace_id: workspace.id,
        id: templateRef.id,
        version: templateRef.version
      })
      return response.template
    },
    enabled: !!templateRef && !!workspace.id,
    // No need to refetch often - template won't change
    staleTime: 1000 * 60 * 5 // 5 minutes
  })

  if (isLoading || !data) {
    return (
      <Button type="link" size="small" loading={isLoading}>
        {t`preview`}
      </Button>
    )
  }

  return (
    <Space>
      <TemplatePreviewDrawer record={data} workspace={workspace}>
        <Button type="link" size="small">
          {t`preview`}
        </Button>
      </TemplatePreviewDrawer>
      {workspace && (
        <CreateTemplateDrawer
          template={data}
          workspace={workspace}
          buttonContent={t`edit`}
          buttonProps={{ type: 'link', size: 'small' }}
        />
      )}
    </Space>
  )
}

export function ListsPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/lists' })
  const [deleteModalVisible, setDeleteModalVisible] = useState(false)
  const [listToDelete, setListToDelete] = useState<List | null>(null)
  const [confirmationInput, setConfirmationInput] = useState('')
  const [isDeleting, setIsDeleting] = useState(false)
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const workspace = workspaces.find((w) => w.id === workspaceId)

  const { data, isLoading } = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => {
      return listsApi.list({ workspace_id: workspaceId })
    }
  })

  const handleDelete = async () => {
    if (!listToDelete) return

    setIsDeleting(true)
    try {
      await listsApi.delete({
        workspace_id: workspaceId,
        id: listToDelete.id
      })

      message.success(t`List "${listToDelete.name}" deleted successfully`)
      queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
      setDeleteModalVisible(false)
      setListToDelete(null)
      setConfirmationInput('')
    } catch (error) {
      message.error(t`Failed to delete list`)
      console.error(error)
    } finally {
      setIsDeleting(false)
    }
  }

  const openDeleteModal = (list: List) => {
    setListToDelete(list)
    setDeleteModalVisible(true)
  }

  const closeDeleteModal = () => {
    setDeleteModalVisible(false)
    setListToDelete(null)
    setConfirmationInput('')
  }

  const handleRefresh = () => {
    queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
    message.success(t`Lists refreshed`)
  }

  const hasLists = !isLoading && data?.lists && data.lists.length > 0

  if (!workspace) {
    return <div>{t`Loading...`}</div>
  }

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <div className="text-2xl font-medium">{t`Lists`}</div>
        {(isLoading || hasLists) && (
          <Space>
            <Tooltip title={t`Refresh`}>
              <Button
                type="text"
                size="small"
                icon={<FontAwesomeIcon icon={faRefresh} />}
                onClick={handleRefresh}
                className="opacity-70 hover:opacity-100"
              />
            </Tooltip>
            <Tooltip
              title={
                !permissions?.lists?.write ? t`You don't have write permission for lists` : undefined
              }
            >
              <div>
                <CreateListDrawer
                  workspaceId={workspaceId}
                  buttonProps={{
                    disabled: !permissions?.lists?.write
                  }}
                />
              </div>
            </Tooltip>
          </Space>
        )}
      </div>

      {isLoading ? (
        <Row gutter={[16, 16]}>
          {[1, 2, 3].map((key) => (
            <Col xs={24} sm={12} lg={8} key={key}>
              <Card loading variant="outlined" />
            </Col>
          ))}
        </Row>
      ) : hasLists ? (
        <Space direction="vertical" size="large">
          {data.lists.map((list: List) => (
            <Card
              title={
                <div className="flex items-center justify-between">
                  <Text strong>{list.name}</Text>
                </div>
              }
              extra={
                <Space>
                  <Tooltip
                    title={
                      !permissions?.lists?.write
                        ? t`You don't have write permission for lists`
                        : t`Delete List`
                    }
                  >
                    <Button
                      type="text"
                      size="small"
                      onClick={() => openDeleteModal(list)}
                      disabled={!permissions?.lists?.write}
                    >
                      <FontAwesomeIcon icon={faTrashCan} style={{ opacity: 0.7 }} />
                    </Button>
                  </Tooltip>
                  <Tooltip
                    title={
                      !permissions?.lists?.write
                        ? t`You don't have write permission for lists`
                        : t`Edit List`
                    }
                  >
                    <div>
                      <CreateListDrawer
                        workspaceId={workspaceId}
                        list={list}
                        buttonProps={{
                          type: 'text',
                          size: 'small',
                          buttonContent: (
                            <FontAwesomeIcon icon={faPenToSquare} style={{ opacity: 0.7 }} />
                          ),
                          disabled: !permissions?.lists?.write
                        }}
                      />
                    </div>
                  </Tooltip>
                  <Tooltip
                    title={
                      !permissions?.lists?.write
                        ? t`You don't have write permission for lists`
                        : undefined
                    }
                  >
                    <div>
                      <ImportContactsToListButton
                        list={list}
                        workspaceId={workspaceId}
                        lists={data.lists}
                        disabled={!permissions?.lists?.write}
                      />
                    </div>
                  </Tooltip>
                </Space>
              }
              key={list.id}
            >
              <ListStats workspaceId={workspaceId} listId={list.id} />

              <Divider />

              <Descriptions size="small" column={2}>
                <Descriptions.Item label={t`ID`}>{list.id}</Descriptions.Item>

                <Descriptions.Item label={t`Description`}>{list.description}</Descriptions.Item>
                <Descriptions.Item label={t`Visibility`}>
                  {list.is_public ? (
                    <Tag bordered={false} color="green">
                      {t`Public`}
                    </Tag>
                  ) : (
                    <Tag bordered={false} color="volcano">
                      {t`Private`}
                    </Tag>
                  )}
                </Descriptions.Item>

                {/* Double Opt-in Template */}
                <Descriptions.Item label={t`Double Opt-in Template`}>
                  {list.double_optin_template ? (
                    <Space>
                      <Check size={16} className="text-green-500 mt-1" />
                      <TemplatePreviewButton
                        templateRef={list.double_optin_template}
                        workspace={workspace}
                      />
                    </Space>
                  ) : (
                    <X size={16} className="text-slate-500 mt-1" />
                  )}
                </Descriptions.Item>
              </Descriptions>
            </Card>
          ))}
        </Space>
      ) : (
        <div className="text-center py-12">
          <Title level={4} type="secondary">
            {t`No lists found`}
          </Title>
          <Paragraph type="secondary">{t`Create your first list to get started`}</Paragraph>
          <div className="mt-4">
            <CreateListDrawer workspaceId={workspaceId} buttonProps={{ size: 'large' }} />
          </div>
        </div>
      )}

      <Modal
        title={t`Delete List`}
        open={deleteModalVisible}
        onCancel={closeDeleteModal}
        footer={[
          <Button key="cancel" onClick={closeDeleteModal}>
            {t`Cancel`}
          </Button>,
          <Button
            key="delete"
            type="primary"
            danger
            loading={isDeleting}
            disabled={confirmationInput !== (listToDelete?.id || '')}
            onClick={handleDelete}
          >
            {t`Delete`}
          </Button>
        ]}
      >
        {listToDelete && (
          <>
            <p>{t`Are you sure you want to delete the list "${listToDelete.name}"?`}</p>
            <p>
              {t`This action cannot be undone. To confirm, please enter the list ID:`}{' '}
              <Text code>{listToDelete.id}</Text>
            </p>
            <Input
              placeholder={t`Enter list ID to confirm`}
              value={confirmationInput}
              onChange={(e) => setConfirmationInput(e.target.value)}
              status={confirmationInput && confirmationInput !== listToDelete.id ? 'error' : ''}
            />
            {confirmationInput && confirmationInput !== listToDelete.id && (
              <p className="text-red-500 mt-2">{t`ID doesn't match`}</p>
            )}
          </>
        )}
      </Modal>
    </div>
  )
}
