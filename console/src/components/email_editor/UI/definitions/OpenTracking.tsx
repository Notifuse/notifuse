import { Alert, Space } from 'antd'
import { BlockDefinitionInterface, BlockRenderSettingsProps } from '../../Block'
import { BlockEditorRendererProps } from '../../BlockEditorRenderer'
import { Eye } from 'lucide-react'

const OpenTrackingBlockDefinition: BlockDefinitionInterface = {
  name: 'Open tracking',
  kind: 'openTracking',
  containsDraggables: false,
  isDraggable: true,
  draggableIntoGroup: 'column',
  isDeletable: true,
  defaultData: {},
  menuSettings: {},

  RenderSettings: (props: BlockRenderSettingsProps) => {
    return (
      <div className="xpeditor-padding-h-l">
        <Alert
          type="info"
          showIcon
          message="An invisible tracking pixel will be added to the email. When the email is opened, the pixel will be loaded and the open event will be recorded."
        />
      </div>
    )
  },

  renderEditor: (_props: BlockEditorRendererProps) => {
    return (
      <div
        style={{
          width: '100%',
          backgroundColor: '#f7f7f7',
          padding: '12px',
          border: '1px solid #e8e8e8',
          borderRadius: '4px'
        }}
      >
        OPEN TRACKING PIXEL
      </div>
    )
  },

  renderMenu: (_blockDefinition: BlockDefinitionInterface) => {
    return (
      <div className="xpeditor-ui-block">
        <Space size="middle">
          <Eye size={16} style={{ marginTop: '5px' }} />
          Open tracking
        </Space>
      </div>
    )
  }
}

export default OpenTrackingBlockDefinition
