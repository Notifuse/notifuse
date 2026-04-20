import { Modal, Progress, Space, Typography, Tag, Button, Alert, List as AntList, Spin } from 'antd'
import { PauseCircleOutlined, PlayCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { useLingui, Plural } from '@lingui/react/macro'
import type { BulkActionProgress } from '../../hooks/useBulkContactAction'

const { Text } = Typography

export interface BulkActionProgressModalProps {
  open: boolean
  title: string
  progress: BulkActionProgress | null
  onPause: () => void
  onResume: () => void
  onCancel: () => void
  onClose: () => void
}

export function BulkActionProgressModal({
  open,
  title,
  progress,
  onPause,
  onResume,
  onCancel,
  onClose
}: BulkActionProgressModalProps) {
  const { t } = useLingui()

  if (!progress) return null

  const isRunning = progress.running
  const isPaused = progress.paused
  const isDone = !progress.running
  const isIterative = progress.mode === 'iterative'
  const percent = progress.total > 0 ? Math.round((progress.processed / progress.total) * 100) : 0

  return (
    <Modal
      title={title}
      open={open}
      onCancel={isDone ? onClose : undefined}
      closable={isDone}
      maskClosable={false}
      keyboard={false}
      width={720}
      footer={
        <Space>
          {isRunning && isIterative && !isPaused && (
            <Button icon={<PauseCircleOutlined />} onClick={onPause}>
              {t`Pause`}
            </Button>
          )}
          {isRunning && isIterative && isPaused && (
            <Button icon={<PlayCircleOutlined />} onClick={onResume}>
              {t`Resume`}
            </Button>
          )}
          {isRunning && isIterative && (
            <Button danger icon={<CloseCircleOutlined />} onClick={onCancel}>
              {t`Cancel`}
            </Button>
          )}
          {isDone && (
            <Button type="primary" onClick={onClose}>
              {t`Close`}
            </Button>
          )}
        </Space>
      }
    >
      <div className="flex flex-col gap-4 py-4">
        <div className="flex flex-col gap-2">
          <div className="flex justify-between items-center">
            <Text strong>
              {isRunning ? t`Processing contacts...` : t`Operation complete`}
            </Text>
            <Text type="secondary">
              {isRunning && !isIterative ? (
                <Plural value={progress.total} one="# contact" other="# contacts" />
              ) : (
                `${progress.processed} / ${progress.total}`
              )}
            </Text>
          </div>
          {isRunning && !isIterative ? (
            <div className="flex items-center gap-2 py-2">
              <Spin />
              <Text type="secondary">{t`Submitting batch request to the server...`}</Text>
            </div>
          ) : (
            <Progress
              percent={percent}
              status={isRunning ? 'active' : progress.failed > 0 ? 'exception' : 'success'}
            />
          )}
          <div className="flex justify-between">
            <Text type="success">
              {t`Successful`}: {progress.succeeded}
            </Text>
            {progress.skipped > 0 && (
              <Text type="warning">
                {t`Skipped`}: {progress.skipped}
              </Text>
            )}
            <Text type="danger">
              {t`Failed`}: {progress.failed}
            </Text>
          </div>
        </div>

        {isPaused && (
          <Alert
            type="warning"
            message={t`Processing paused`}
            description={t`Click Resume to continue processing.`}
          />
        )}
        {progress.cancelled && (
          <Alert
            type="warning"
            message={t`Operation cancelled`}
            description={t`Processing was stopped before completion.`}
          />
        )}

        {progress.results.length > 0 && (
          <div className="flex flex-col gap-2">
            <Text strong>{t`Results:`}</Text>
            <div className="max-h-60 overflow-y-auto">
              <AntList
                size="small"
                dataSource={progress.results}
                renderItem={(item) => (
                  <AntList.Item>
                    <div className="flex justify-between items-center w-full">
                      <Text className="text-sm">{item.email}</Text>
                      <div>
                        {item.skipped ? (
                          <Tag color="warning" title={item.error}>
                            {t`Skipped`}
                          </Tag>
                        ) : item.success ? (
                          <Tag color="success">{t`Success`}</Tag>
                        ) : (
                          <Tag color="error" title={item.error}>
                            {t`Failed`}
                          </Tag>
                        )}
                      </div>
                    </div>
                  </AntList.Item>
                )}
              />
            </div>
          </div>
        )}
      </div>
    </Modal>
  )
}
