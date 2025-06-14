import { useState } from 'react'
import { SelectedBlockButtonsProp } from '../Editor'
import { Tooltip, Popconfirm, Modal, Form, Button, Spin, Input, Select, App } from 'antd'
import { DragOutlined, DeleteOutlined, CopyOutlined, SaveOutlined } from '@ant-design/icons'
import { EmailTemplateBlock } from '../types'
import uuid from 'short-uuid'

const SelectedBlockButtons = (props: SelectedBlockButtonsProp) => {
  const [saveVisible, setSaveVisible] = useState(false)
  const [loading, setLoading] = useState(false)
  const { message } = App.useApp()

  const [form] = Form.useForm()

  const toggleModal = () => {
    // reset on hide
    if (saveVisible === true) {
      form.resetFields()
    }
    setSaveVisible(!saveVisible)
  }

  const onSubmit = () => {
    form
      .validateFields()
      .then((values: any) => {
        setLoading(true)

        const clonedBlocks = [...(props.existingBlocks || [])]

        if (values.operation === 'create') {
          clonedBlocks.push({
            id: uuid.generate(),
            name: values.name,
            content: JSON.stringify(props.block)
          })
        } else {
          const block = clonedBlocks.find((x: EmailTemplateBlock) => x.id === values.id)
          if (block) {
            block.name = values.name
            block.content = JSON.stringify(props.block)
          }
        }

        props
          .onExistingBlocksUpdate(clonedBlocks)
          .then(() => {
            if (values.operation === 'create') {
              message.success('The block has been saved!')
            } else {
              message.success('The block has been updated!')
            }

            setLoading(false)
            setSaveVisible(false)
            form.resetFields()
          })
          .catch((e: any) => {
            console.error(e)
            message.error(e.message)
            setLoading(false)
          })
      })
      .catch((e: any) => {
        console.error(e)
      })
  }

  // console.log('props', props)
  const isDraggable = props.blockDefinitions[props.block.kind].isDraggable
  const isDeletable = props.blockDefinitions[props.block.kind].isDeletable

  return (
    <div className="xpeditor-selected-block-buttons">
      {isDraggable === true && (
        <Tooltip placement="left" title="Move">
          <div className="xpeditor-selected-block-button">
            <DragOutlined style={{ verticalAlign: 'middle', cursor: 'grab' }} />
          </div>
        </Tooltip>
      )}

      {isDraggable === true && (
        <Tooltip placement="left" title="Clone">
          <div
            className="xpeditor-selected-block-button"
            onClick={props.cloneBlock.bind(null, props.block)}
          >
            <CopyOutlined style={{ verticalAlign: 'middle' }} />
          </div>
        </Tooltip>
      )}

      {isDraggable === true && (
        <Tooltip placement="left" title="Save">
          <div className="xpeditor-selected-block-button" onClick={toggleModal}>
            <SaveOutlined style={{ verticalAlign: 'middle' }} />
          </div>
        </Tooltip>
      )}

      {isDeletable === true && (
        <Tooltip placement="left" title="Delete">
          <Popconfirm
            title="Are you sure to delete this element?"
            onConfirm={props.deleteBlock.bind(null, props.block)}
            okText="Yes"
            cancelText="No"
          >
            <div className="xpeditor-selected-block-button">
              <DeleteOutlined style={{ verticalAlign: 'middle' }} />
            </div>
          </Popconfirm>
        </Tooltip>
      )}

      {saveVisible && (
        <Modal
          title="Save block"
          wrapClassName="vertical-center-modal"
          open={true}
          onCancel={toggleModal}
          footer={[
            <Button key="back" ghost loading={loading} onClick={toggleModal}>
              Cancel
            </Button>,
            <Button key="submit" type="primary" loading={loading} onClick={onSubmit}>
              Confirm
            </Button>
          ]}
        >
          <Spin tip="Loading..." spinning={loading}>
            <Form form={form} initialValues={{ operation: 'create' }} layout="vertical">
              <Form.Item
                name="operation"
                label="Operation"
                rules={[{ required: true, message: 'This field is required!' }]}
              >
                <Select
                  options={[
                    { label: 'Save as new block', value: 'create' },
                    { label: 'Update existing block', value: 'update' }
                  ]}
                />
              </Form.Item>

              <Form.Item noStyle shouldUpdate={true}>
                {({ getFieldValue }: any) => {
                  const blocks = props.existingBlocks || []
                  return (
                    <>
                      {getFieldValue('operation') === 'update' && (
                        <Form.Item
                          name="id"
                          label="Block"
                          rules={[{ required: true, message: 'This field is required!' }]}
                        >
                          <Select
                            onChange={(val: any) => {
                              form.setFieldsValue({
                                name: blocks.find((x: EmailTemplateBlock) => x.id === val)?.name
                              })
                            }}
                            options={blocks.map((b: EmailTemplateBlock) => {
                              return {
                                label: b.name,
                                value: b.id
                              }
                            })}
                          />
                        </Form.Item>
                      )}

                      {(getFieldValue('operation') === 'create' || getFieldValue('id')) && (
                        <Form.Item
                          name="name"
                          label="Name"
                          rules={[{ required: true, message: 'This field is required!' }]}
                        >
                          <Input />
                        </Form.Item>
                      )}
                    </>
                  )
                }}
              </Form.Item>
            </Form>
          </Spin>
        </Modal>
      )}

      {/* <span style={{fontSize: '10px'}}>kind: {props.block.kind}, {isDraggable && <>draggable into: {props.blockDefinitions[props.block.kind].draggableIntoGroup}</>}</span> */}
    </div>
  )
}

export default SelectedBlockButtons
