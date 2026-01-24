import { useState } from 'react'
import { Card, Typography, Space, Button, Form, message } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { TreeNodeInput, SegmentSchemas } from '../components/segment/input'
import { TreeNode, List } from '../services/api/segment'
import { TableSchemas } from '../components/segment/table_schemas'

const { Title, Paragraph } = Typography

// Use actual database table schemas
const segmentSchemas: SegmentSchemas = {
  contacts: TableSchemas.contacts,
  contact_lists: TableSchemas.contact_lists,
  contact_timeline: TableSchemas.contact_timeline
}

// Mock lists for demonstration - in production, these would be fetched from API
const mockLists: List[] = [
  { id: 'list_newsletter', name: 'Newsletter Subscribers' },
  { id: 'list_customers', name: 'Customers' },
  { id: 'list_leads', name: 'Leads' },
  { id: 'list_vip', name: 'VIP Members' }
]

const initialTree: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: []
  }
}

export function DebugSegmentPage() {
  const { t } = useLingui()
  const [form] = Form.useForm()
  const [tree, setTree] = useState<TreeNode>(initialTree)

  const handleSubmit = () => {
    console.log('Segment tree:', JSON.stringify(tree, null, 2))
    message.success(t`Segment tree logged to console!`)
  }

  const handleReset = () => {
    setTree(initialTree)
    message.info(t`Segment tree reset`)
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1400px', margin: '0 auto' }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <div>
          <Title level={2}>{t`Debug Segment Builder`}</Title>
          <Paragraph>
            {t`Build complex contact segments using AND/OR logic with filters across multiple data tables. Start by adding conditions for contacts, contact lists, or contact timeline events.`}
          </Paragraph>
        </div>

        <Card>
          <Form form={form} layout="vertical">
            <Form.Item
              label={
                <Space>
                  <span style={{ fontSize: '16px', fontWeight: 600 }}>{t`Segment Conditions`}</span>
                  <Paragraph type="secondary" style={{ margin: 0 }}>
                    {t`Add conditions to define your contact segment`}
                  </Paragraph>
                </Space>
              }
            >
              <TreeNodeInput
                value={tree}
                onChange={setTree}
                schemas={segmentSchemas}
                lists={mockLists}
              />
            </Form.Item>

            <Form.Item>
              <Space>
                <Button type="primary" onClick={handleSubmit}>
                  {t`Save Segment (Console Log)`}
                </Button>
                <Button onClick={handleReset}>{t`Reset`}</Button>
              </Space>
            </Form.Item>
          </Form>
        </Card>

        <Card title={t`Current Segment Tree JSON`} size="small">
          <pre
            style={{
              background: '#f5f5f5',
              padding: '16px',
              borderRadius: '4px',
              overflow: 'auto',
              maxHeight: '400px'
            }}
          >
            {JSON.stringify(tree, null, 2)}
          </pre>
        </Card>
      </Space>
    </div>
  )
}
