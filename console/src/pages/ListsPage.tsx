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
import { Check, X, Globe } from 'lucide-react'
import TemplatePreviewDrawer from '../components/templates/TemplatePreviewDrawer'
import { CreateTemplateDrawer } from '../components/templates/CreateTemplateDrawer'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ImportContactsToListButton } from '../components/lists/ImportContactsToListButton'
import { ListStats } from '../components/lists/ListStats'

const { Title, Paragraph, Text, Link } = Typography

// Component to fetch template data and render the preview popover
const TemplatePreviewButton = ({
  templateRef,
  workspace
}: {
  templateRef: TemplateReference
  workspace: Workspace
}) => {
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
        preview
      </Button>
    )
  }

  return (
    <Space>
      <TemplatePreviewDrawer record={data} workspace={workspace}>
        <Button type="link" size="small">
          preview
        </Button>
      </TemplatePreviewDrawer>
      {workspace && (
        <CreateTemplateDrawer
          template={data}
          workspace={workspace}
          buttonContent="edit"
          buttonProps={{ type: 'link', size: 'small' }}
        />
      )}
    </Space>
  )
}

export function ListsPage() {
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

      message.success(`List "${listToDelete.name}" deleted successfully`)
      queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
      setDeleteModalVisible(false)
      setListToDelete(null)
      setConfirmationInput('')
    } catch (error) {
      message.error('Failed to delete list')
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
    message.success('Lists refreshed')
  }

  const hasLists = !isLoading && data?.lists && data.lists.length > 0

  if (!workspace) {
    return <div>Loading...</div>
  }

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <div className="text-2xl font-medium">Lists</div>
        {(isLoading || hasLists) && (
          <Space>
            <Tooltip title="Refresh">
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
                !permissions?.lists?.write ? "You don't have write permission for lists" : undefined
              }
            >
              <div>
                <CreateListDrawer
                  workspaceId={workspaceId}
                  workspace={workspace}
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
                        ? "You don't have write permission for lists"
                        : 'Delete List'
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
                        ? "You don't have write permission for lists"
                        : 'Edit List'
                    }
                  >
                    <div>
                      <CreateListDrawer
                        workspaceId={workspaceId}
                        workspace={workspace}
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
                        ? "You don't have write permission for lists"
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
                <Descriptions.Item label="ID">{list.id}</Descriptions.Item>

                <Descriptions.Item label="Description">{list.description}</Descriptions.Item>
                <Descriptions.Item label="Visibility">
                  {list.is_public ? (
                    <Tag bordered={false} color="green">
                      Public
                    </Tag>
                  ) : (
                    <Tag bordered={false} color="volcano">
                      Private
                    </Tag>
                  )}
                </Descriptions.Item>

                {/* Double Opt-in Template */}
                <Descriptions.Item label="Double Opt-in Template">
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

                {/* Welcome Template */}
                <Descriptions.Item label="Welcome Template">
                  {list.welcome_template ? (
                    <Space>
                      <Check size={16} className="text-green-500 mt-1" />
                      <TemplatePreviewButton
                        templateRef={list.welcome_template}
                        workspace={workspace}
                      />
                    </Space>
                  ) : (
                    <X size={16} className="text-slate-500 mt-1" />
                  )}
                </Descriptions.Item>

                {/* Unsubscribe Template */}
                <Descriptions.Item label="Unsubscribe Template">
                  {list.unsubscribe_template ? (
                    <Space>
                      <Check size={16} className="text-green-500 mt-1" />
                      <TemplatePreviewButton
                        templateRef={list.unsubscribe_template}
                        workspace={workspace}
                      />
                    </Space>
                  ) : (
                    <X size={16} className="text-slate-500 mt-1" />
                  )}
                </Descriptions.Item>

                {/* Web Publication Settings (with per-setting Description) */}
                <Descriptions.Item label="Web Publication">
                  {list.web_publication_enabled ? (
                    <Space direction="vertical" size="small" className="w-full">
                      {/* Enabled status */}
                      <div className="flex items-center gap-2">
                        <Tag bordered={false} color="green">
                          Enabled
                        </Tag>
                      </div>
                    </Space>
                  ) : (
                    <div className="flex items-center gap-2">
                      <Tag bordered={false} color="volcano">
                        Disabled
                      </Tag>
                    </div>
                  )}
                </Descriptions.Item>

                {list.web_publication_enabled && (
                  <>
                    <Descriptions.Item label="Public URL" span={1}>
                      {list.web_publication_settings?.slug &&
                      workspace?.settings?.custom_endpoint_url ? (
                        <div className="flex items-center gap-2">
                          <Link
                            href={`${workspace.settings.custom_endpoint_url}/${list.web_publication_settings.slug}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-sm"
                          >
                            {workspace.settings.custom_endpoint_url}/
                            {list.web_publication_settings.slug}
                          </Link>
                        </div>
                      ) : (
                        <Text type="secondary">Not set</Text>
                      )}
                    </Descriptions.Item>

                    {/* SEO Title */}
                    <Descriptions.Item label="SEO Title">
                      {list.web_publication_settings?.meta_title ? (
                        <span>{list.web_publication_settings.meta_title}</span>
                      ) : (
                        <Text type="secondary">Not set</Text>
                      )}
                    </Descriptions.Item>

                    {/* SEO Description */}
                    <Descriptions.Item label="SEO Description">
                      {list.web_publication_settings?.meta_description ? (
                        <span>{list.web_publication_settings.meta_description}</span>
                      ) : (
                        <Text type="secondary">Not set</Text>
                      )}
                    </Descriptions.Item>

                    {/* SEO Keywords */}
                    <Descriptions.Item label="SEO Keywords">
                      {list.web_publication_settings?.keywords &&
                      list.web_publication_settings.keywords.length > 0 ? (
                        <Space size={4} wrap>
                          {list.web_publication_settings.keywords.map((keyword, idx) => (
                            <Tag key={idx} bordered={false} className="text-xs">
                              {keyword}
                            </Tag>
                          ))}
                        </Space>
                      ) : (
                        <Text type="secondary">None</Text>
                      )}
                    </Descriptions.Item>
                  </>
                )}
              </Descriptions>

              {/* Open Graph Preview - Separate section */}
              {list.web_publication_enabled && list.web_publication_settings && (
                <Descriptions size="small" layout="vertical" column={1} style={{ marginTop: 8 }}>
                  <Descriptions.Item label="Open Graph Preview">
                    <div
                      className="border border-gray-200 rounded-lg overflow-hidden bg-white flex"
                      style={{ width: 350 }}
                    >
                      {/* OG Image - Square on the left */}
                      {list.web_publication_settings.og_image ? (
                        <div className="w-24 h-24 flex-shrink-0 bg-gray-100 overflow-hidden">
                          <img
                            src={list.web_publication_settings.og_image}
                            alt={list.web_publication_settings.og_title || list.name}
                            className="w-full h-full object-cover"
                          />
                        </div>
                      ) : (
                        <div className="w-24 h-24 flex-shrink-0 bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center">
                          <Globe size={24} className="text-blue-300" />
                        </div>
                      )}

                      {/* OG Content - Text on the right */}
                      <div className="flex-1 p-3 flex flex-col justify-center min-w-0">
                        {workspace?.settings?.custom_endpoint_url && (
                          <div className="text-xs text-gray-500 mb-1 truncate">
                            {workspace.settings.custom_endpoint_url.replace(/^https?:\/\//, '')}
                          </div>
                        )}
                        <div className="text-sm font-semibold text-gray-900 mb-1 line-clamp-2">
                          {list.web_publication_settings.og_title ||
                            list.web_publication_settings.meta_title ||
                            list.name}
                        </div>
                        <div className="text-xs text-gray-600 line-clamp-2">
                          {list.web_publication_settings.og_description ||
                            list.web_publication_settings.meta_description ||
                            list.description ||
                            'Subscribe to receive updates from this list.'}
                        </div>
                      </div>
                    </div>
                  </Descriptions.Item>
                </Descriptions>
              )}
            </Card>
          ))}
        </Space>
      ) : (
        <div className="text-center py-12">
          <Title level={4} type="secondary">
            No lists found
          </Title>
          <Paragraph type="secondary">Create your first list to get started</Paragraph>
          <div className="mt-4">
            <CreateListDrawer
              workspaceId={workspaceId}
              workspace={workspace}
              buttonProps={{ size: 'large' }}
            />
          </div>
        </div>
      )}

      <Modal
        title="Delete List"
        open={deleteModalVisible}
        onCancel={closeDeleteModal}
        footer={[
          <Button key="cancel" onClick={closeDeleteModal}>
            Cancel
          </Button>,
          <Button
            key="delete"
            type="primary"
            danger
            loading={isDeleting}
            disabled={confirmationInput !== (listToDelete?.id || '')}
            onClick={handleDelete}
          >
            Delete
          </Button>
        ]}
      >
        {listToDelete && (
          <>
            <p>Are you sure you want to delete the list "{listToDelete.name}"?</p>
            <p>
              This action cannot be undone. To confirm, please enter the list ID:{' '}
              <Text code>{listToDelete.id}</Text>
            </p>
            <Input
              placeholder="Enter list ID to confirm"
              value={confirmationInput}
              onChange={(e) => setConfirmationInput(e.target.value)}
              status={confirmationInput && confirmationInput !== listToDelete.id ? 'error' : ''}
            />
            {confirmationInput && confirmationInput !== listToDelete.id && (
              <p className="text-red-500 mt-2">ID doesn't match</p>
            )}
          </>
        )}
      </Modal>
    </div>
  )
}
