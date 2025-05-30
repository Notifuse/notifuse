import { useState, useRef } from 'react'
import { BlockDefinitionInterface, BlockInterface, BlockRenderSettingsProps } from '../../Block'
import { BlockEditorRendererProps } from '../../BlockEditorRenderer'
import {
  Popover,
  Button,
  Form,
  InputNumber,
  Divider,
  Radio,
  Input,
  Switch,
  Modal,
  Space
} from 'antd'
import BorderInputs from '../Widgets/BorderInputs'
import PaddingInputs from '../Widgets/PaddingInputs'
import { AlignLeft, AlignCenter, AlignRight, Image as ImageIcon } from 'lucide-react'
import { MobileWidth } from '../Layout'
import { FileManager } from '../../../file_manager/fileManager'
import { ItemFilter, StorageObject } from '../../../file_manager/interfaces'
import { FileManagerSettings } from '../../../../services/api/types'
import { useEditorContext } from '../../Editor'

interface ImageProps {
  block: BlockInterface
  updateTree: any
}

const AlternativeText = (props: ImageProps) => {
  const altInputRef = useRef<any>(null)
  const [alt, setAlt] = useState(props.block.data.image.alt)
  const [altModalVisible, setAltModalVisible] = useState(false)

  return (
    <Form.Item
      label="Alternative text"
      className="xpeditor-form-item-align-right"
      labelCol={{ span: 10 }}
      wrapperCol={{ span: 14 }}
    >
      <Popover
        content={
          <>
            <Input
              style={{ width: '100%' }}
              onChange={(e) => setAlt(e.target.value)}
              value={alt}
              size="small"
              ref={altInputRef}
            />
            <Button
              style={{ marginTop: '12px' }}
              type="primary"
              size="small"
              block
              onClick={() => {
                props.block.data.image.alt = alt
                props.updateTree(props.block.path, props.block)
                setAltModalVisible(false)
              }}
              disabled={props.block.data.image.alt === alt}
            >
              Save changes
            </Button>
          </>
        }
        title="Alternative text"
        trigger="click"
        open={altModalVisible}
        onOpenChange={(visible) => {
          setAltModalVisible(visible)
          setTimeout(() => {
            if (visible)
              altInputRef.current!.focus({
                cursor: 'start'
              })
          }, 10)
        }}
      >
        {props.block.data.image.alt === '' && (
          <Button type="primary" size="small" block>
            Set value
          </Button>
        )}
        {props.block.data.image.alt !== '' && (
          <>
            {props.block.data.image.alt} &nbsp;&nbsp;
            <span className="xpeditor-ui-link">update</span>
          </>
        )}
      </Popover>
    </Form.Item>
  )
}

const ClickURL = (props: ImageProps) => {
  const hrefInputRef = useRef<any>(null)
  const [href, setHref] = useState(props.block.data.image.href)
  const [disableTracking, setDisableTracking] = useState(props.block.data.image.disable_tracking)
  const [hrefModalVisible, setHrefModalVisible] = useState(false)
  return (
    <>
      <Form.Item
        label="Click URL"
        className="xpeditor-form-item-align-right"
        labelCol={{ span: 10 }}
        wrapperCol={{ span: 14 }}
      >
        <Popover
          content={
            <>
              <Input
                style={{ width: '100%' }}
                onChange={(e) => setHref(e.target.value)}
                value={href}
                size="small"
                ref={hrefInputRef}
                placeholder="https://www..."
              />
              <Button
                style={{ marginTop: '12px' }}
                type="primary"
                size="small"
                block
                onClick={() => {
                  props.block.data.image.href = href
                  props.updateTree(props.block.path, props.block)
                  setHrefModalVisible(false)
                }}
                disabled={props.block.data.image.href === href}
              >
                Save changes
              </Button>
            </>
          }
          title="Click URL"
          trigger="click"
          open={hrefModalVisible}
          onOpenChange={(visible) => {
            setHrefModalVisible(visible)
            setTimeout(() => {
              if (visible)
                hrefInputRef.current!.focus({
                  cursor: 'start'
                })
            }, 10)
          }}
        >
          {!props.block.data.image.href && (
            <Button type="primary" size="small" block>
              Set value
            </Button>
          )}
          {props.block.data.image.href && (
            <>
              {props.block.data.image.href} &nbsp;&nbsp;
              <span className="xpeditor-ui-link">update</span>
            </>
          )}
        </Popover>
      </Form.Item>
      {/* disable tracking switch */}
      <Form.Item
        valuePropName="checked"
        label="Disable URL tracking"
        labelAlign="left"
        className="xpeditor-form-item-align-right"
        labelCol={{ span: 10 }}
        wrapperCol={{ span: 14 }}
      >
        <Switch
          onChange={(value) => {
            props.block.data.image.disable_tracking = value
            props.updateTree(props.block.path, props.block)
            setDisableTracking(value)
          }}
          checked={disableTracking}
          size="small"
        />
      </Form.Item>
    </>
  )
}

// the UploadButton useState cant reside directly in RenderSettings()
// because it's not a proper React functional component
interface UploadButtonProps {
  block: BlockInterface
  updateTree: (path: string, data: any) => void
  settings?: FileManagerSettings
  onUpdateSettings: (settings: FileManagerSettings) => Promise<void>
}

const UploadButton = (props: UploadButtonProps) => {
  const [fileManagerVisible, setFileManagerVisible] = useState(false)
  const [selectedImageURL, setSelectedImageURL] = useState<string | undefined>(
    props.block.data.image.src
  )

  const filters: ItemFilter[] = []

  const handleURLChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSelectedImageURL(e.target.value)
  }

  const applyImageURL = () => {
    if (selectedImageURL && selectedImageURL !== props.block.data.image.src) {
      props.block.data.image.src = selectedImageURL
      props.updateTree(props.block.path, props.block)
    }
  }

  return (
    <>
      {fileManagerVisible && (
        <Modal
          open={true}
          title="Select or upload"
          width={1100}
          styles={{
            body: {
              background: '#F3F6FC'
            }
          }}
          onOk={() => {
            if (selectedImageURL && selectedImageURL !== props.block.data.image.src) {
              props.block.data.image.src = selectedImageURL
              setSelectedImageURL(undefined)
              setFileManagerVisible(false)
              props.updateTree(props.block.path, props.block)
            }
          }}
          onCancel={() => setFileManagerVisible(false)}
          destroyOnClose={true}
          okText="Use image"
          okButtonProps={{
            disabled: !selectedImageURL
          }}
        >
          <div style={{ height: 500 }}>
            <FileManager
              settings={props.settings}
              onUpdateSettings={props.onUpdateSettings}
              itemFilters={filters}
              onError={() => {
                // console.error(error)
                // message.error(error)
              }}
              onSelect={(items: StorageObject[]) => {
                if (items && items.length) {
                  setSelectedImageURL(items[0].file_info.url)
                } else {
                  setSelectedImageURL(undefined)
                }
              }}
              height={500}
              acceptFileType="images/*"
              acceptItem={(item: StorageObject) => {
                if (item.is_folder) return false
                return item.file_info.content_type.includes('image')
              }}
              multiple={false}
              withSelection={true}
            />
          </div>
        </Modal>
      )}
      <Input
        value={selectedImageURL}
        onChange={handleURLChange}
        onPressEnter={applyImageURL}
        onBlur={applyImageURL}
        placeholder="Enter image URL"
        style={{ marginBottom: 8 }}
      />
      <Button type="primary" size="small" block onClick={() => setFileManagerVisible(true)}>
        Select or upload
      </Button>
    </>
  )
}

const ImageBlockDefinition: BlockDefinitionInterface = {
  name: 'Image',
  kind: 'image',
  containsDraggables: false,
  isDraggable: true,
  draggableIntoGroup: 'column',
  isDeletable: true,
  defaultData: {
    wrapper: {
      align: 'center',
      paddingControl: 'separate', // all, separate
      paddingTop: '20px',
      paddingBottom: '20px'
    },
    image: {
      borderControl: 'all', // all, separate
      borderColor: '#000000',
      borderWidth: '2px',
      borderStyle: 'none',
      fullWidthOnMobile: false,
      src: 'https://images.unsplash.com/photo-1432889490240-84df33d47091?ixid=MnwxMjA3fDB8MHxzZWFyY2h8MTZ8fHRyb3BpY2FsfGVufDB8fDB8fA%3D%3D&ixlib=rb-1.2.1&auto=format&fit=crop&w=500&q=60',
      alt: '',
      href: '',
      width: '100%',
      height: 'auto'
    }
  },
  menuSettings: {},

  RenderSettings: (props: BlockRenderSettingsProps) => {
    // console.log('img block is', props.block)
    const editorCtx = useEditorContext()

    return (
      <div className="xpeditor-padding-h-l">
        <Form.Item
          label="Image"
          labelAlign="left"
          className="xpeditor-form-item-align-right"
          labelCol={{ span: 10 }}
          wrapperCol={{ span: 14 }}
        >
          <UploadButton
            block={props.block}
            updateTree={props.updateTree}
            settings={editorCtx.fileManagerSettings}
            onUpdateSettings={editorCtx.onUpdateFileManagerSettings}
          />
        </Form.Item>

        <AlternativeText block={props.block} updateTree={props.updateTree} />

        <ClickURL block={props.block} updateTree={props.updateTree} />

        <Form.Item
          label="Align"
          labelAlign="left"
          className="xpeditor-form-item-align-right"
          labelCol={{ span: 10 }}
          wrapperCol={{ span: 14 }}
        >
          <Radio.Group
            style={{ width: '100%' }}
            onChange={(e) => {
              props.block.data.wrapper.align = e.target.value
              props.updateTree(props.block.path, props.block)
            }}
            value={props.block.data.wrapper.align}
            optionType="button"
            size="small"
          >
            <Radio.Button style={{ width: '33.33%', textAlign: 'center' }} value="left">
              <AlignLeft size={16} style={{ display: 'inline-block' }} />
            </Radio.Button>
            <Radio.Button style={{ width: '33.33%', textAlign: 'center' }} value="center">
              <AlignCenter size={16} style={{ display: 'inline-block' }} />
            </Radio.Button>
            <Radio.Button style={{ width: '33.33%', textAlign: 'center' }} value="right">
              <AlignRight size={16} style={{ display: 'inline-block' }} />
            </Radio.Button>
          </Radio.Group>
        </Form.Item>

        <Divider />

        <Form.Item
          valuePropName="checked"
          label="Full width on mobile"
          labelAlign="left"
          className="xpeditor-form-item-align-right"
          labelCol={{ span: 10 }}
          wrapperCol={{ span: 14 }}
        >
          <Switch
            onChange={(value) => {
              props.block.data.image.fullWidthOnMobile = value
              props.updateTree(props.block.path, props.block)
            }}
            checked={props.block.data.image.fullWidthOnMobile || false}
            size="small"
          />
        </Form.Item>

        <Form.Item
          label="Width"
          labelAlign="left"
          className="xpeditor-form-item-align-right"
          labelCol={{ span: 10 }}
          wrapperCol={{ span: 14 }}
        >
          <Radio.Group
            style={{ width: '100%' }}
            value={props.block.data.image.width}
            optionType="button"
            size="small"
            onChange={(e) => {
              props.block.data.image.width = e.target.value
              props.updateTree(props.block.path, props.block)
            }}
          >
            <Radio.Button value="100%" style={{ width: '40%', textAlign: 'center' }}>
              100%
            </Radio.Button>
            <label
              style={{
                display: 'inline-block',
                height: '24px',
                lineHeight: '22px',
                width: '20%',
                textAlign: 'center'
              }}
            >
              or
            </label>
            <Radio.Button
              style={{ width: '40%' }}
              value={
                props.block.data.image.width !== '100%' ? props.block.data.image.width : '200px'
              }
            >
              <InputNumber
                style={{ width: '100%' }}
                variant="borderless"
                value={parseInt(props.block.data.image.width || '100px')}
                onChange={(value) => {
                  props.block.data.image.width = value + 'px'
                  props.updateTree(props.block.path, props.block)
                }}
                onClick={() => {
                  // switch focus to px
                  if (props.block.data.image.width === '100%') {
                    props.block.data.image.width = '100px'
                    props.updateTree(props.block.path, props.block)
                  }
                }}
                defaultValue={parseInt(props.block.data.image.width)}
                size="small"
                step={1}
                min={0}
                parser={(value: string | undefined) => {
                  return value ? parseInt(value.replace('px', '')) : 0
                }}
                formatter={(value) => value + 'px'}
              />
            </Radio.Button>
          </Radio.Group>
        </Form.Item>

        <Form.Item
          label="Height"
          labelAlign="left"
          className="xpeditor-form-item-align-right"
          labelCol={{ span: 10 }}
          wrapperCol={{ span: 14 }}
        >
          <Radio.Group
            style={{ width: '100%' }}
            value={props.block.data.image.height}
            optionType="button"
            size="small"
            onChange={(e) => {
              props.block.data.image.height = e.target.value
              props.updateTree(props.block.path, props.block)
            }}
          >
            <Radio.Button value="auto" style={{ width: '40%', textAlign: 'center' }}>
              auto
            </Radio.Button>
            <label
              style={{
                display: 'inline-block',
                height: '24px',
                lineHeight: '22px',
                width: '20%',
                textAlign: 'center'
              }}
            >
              or
            </label>
            <Radio.Button
              style={{ width: '40%' }}
              value={
                props.block.data.image.height !== 'auto' ? props.block.data.image.height : '100px'
              }
            >
              <InputNumber
                style={{ height: '100%' }}
                variant="borderless"
                value={parseInt(props.block.data.image.height || '100px')}
                onChange={(value) => {
                  props.block.data.image.height = value + 'px'
                  props.updateTree(props.block.path, props.block)
                }}
                onClick={() => {
                  // switch focus to px
                  if (props.block.data.image.height === 'auto') {
                    props.block.data.image.height = '100px'
                    props.updateTree(props.block.path, props.block)
                  }
                }}
                defaultValue={parseInt(props.block.data.image.height || '100px')}
                size="small"
                step={1}
                min={0}
                parser={(value: string | undefined) => {
                  if (value === undefined) return 0
                  return parseInt(value.replace('px', ''))
                }}
                formatter={(value) => value + 'px'}
              />
            </Radio.Button>
          </Radio.Group>
        </Form.Item>

        <Divider />

        <Form.Item
          label="Border control"
          labelAlign="left"
          className="xpeditor-form-item-align-right"
          labelCol={{ span: 10 }}
          wrapperCol={{ span: 14 }}
        >
          <Radio.Group
            style={{ width: '100%' }}
            onChange={(e) => {
              props.block.data.image.borderControl = e.target.value
              props.updateTree(props.block.path, props.block)
            }}
            value={props.block.data.image.borderControl}
            optionType="button"
            size="small"
            // buttonStyle="solid"
          >
            <Radio.Button style={{ width: '50%', textAlign: 'center' }} value="all">
              All
            </Radio.Button>
            <Radio.Button style={{ width: '50%', textAlign: 'center' }} value="separate">
              Separate
            </Radio.Button>
          </Radio.Group>
        </Form.Item>

        <Form.Item
          label="Border radius"
          labelAlign="left"
          className="xpeditor-form-item-align-right"
          labelCol={{ span: 10 }}
          wrapperCol={{ span: 14 }}
        >
          <InputNumber
            style={{ width: '100%' }}
            value={parseInt(props.block.data.image.borderRadius || '0px')}
            onChange={(value) => {
              props.block.data.image.borderRadius = value + 'px'
              props.updateTree(props.block.path, props.block)
            }}
            defaultValue={props.block.data.image.borderRadius}
            size="small"
            step={1}
            min={0}
            parser={(value: string | undefined) => {
              // if (['▭'].indexOf(value)) {
              //     value = value.substring(1)
              // }
              if (!value) {
                return 0
              }
              return parseInt(value.replace('px', ''))
            }}
            // formatter={value => '▭  ' + value + 'px'}
            formatter={(value) => value + 'px'}
          />
        </Form.Item>

        {props.block.data.image.borderControl === 'all' && (
          <>
            <Form.Item
              label="Borders"
              labelAlign="left"
              className="xpeditor-form-item-align-right"
              labelCol={{ span: 10 }}
              wrapperCol={{ span: 14 }}
            >
              <BorderInputs
                styles={props.block.data.image}
                propertyPrefix="border"
                onChange={(updatedStyles: any) => {
                  props.block.data.image = updatedStyles
                  props.updateTree(props.block.path, props.block)
                }}
              />
            </Form.Item>
          </>
        )}

        {props.block.data.image.borderControl === 'separate' && (
          <>
            <Form.Item
              label="Border top"
              labelAlign="left"
              className="xpeditor-form-item-align-right"
              labelCol={{ span: 10 }}
              wrapperCol={{ span: 14 }}
            >
              <BorderInputs
                styles={props.block.data.image}
                propertyPrefix="borderTop"
                onChange={(updatedStyles: any) => {
                  props.block.data.image = updatedStyles
                  props.updateTree(props.block.path, props.block)
                }}
              />
            </Form.Item>
            <Form.Item
              label="Border right"
              labelAlign="left"
              className="xpeditor-form-item-align-right"
              labelCol={{ span: 10 }}
              wrapperCol={{ span: 14 }}
            >
              <BorderInputs
                styles={props.block.data.image}
                propertyPrefix="borderRight"
                onChange={(updatedStyles: any) => {
                  props.block.data.image = updatedStyles
                  props.updateTree(props.block.path, props.block)
                }}
              />
            </Form.Item>
            <Form.Item
              label="Border bottom"
              labelAlign="left"
              className="xpeditor-form-item-align-right"
              labelCol={{ span: 10 }}
              wrapperCol={{ span: 14 }}
            >
              <BorderInputs
                styles={props.block.data.image}
                propertyPrefix="borderBottom"
                onChange={(updatedStyles: any) => {
                  props.block.data.image = updatedStyles
                  props.updateTree(props.block.path, props.block)
                }}
              />
            </Form.Item>
            <Form.Item
              label="Border left"
              labelAlign="left"
              className="xpeditor-form-item-align-right"
              labelCol={{ span: 10 }}
              wrapperCol={{ span: 14 }}
            >
              <BorderInputs
                styles={props.block.data.image}
                propertyPrefix="borderLeft"
                onChange={(updatedStyles: any) => {
                  props.block.data.image = updatedStyles
                  props.updateTree(props.block.path, props.block)
                }}
              />
            </Form.Item>
          </>
        )}

        <Divider />

        <Form.Item
          label="Padding control"
          labelAlign="left"
          className="xpeditor-form-item-align-right"
          labelCol={{ span: 10 }}
          wrapperCol={{ span: 14 }}
        >
          <Radio.Group
            style={{ width: '100%' }}
            onChange={(e) => {
              props.block.data.wrapper.paddingControl = e.target.value
              props.updateTree(props.block.path, props.block)
            }}
            value={props.block.data.wrapper.paddingControl}
            optionType="button"
            size="small"
            // buttonStyle="solid"
          >
            <Radio.Button style={{ width: '50%', textAlign: 'center' }} value="all">
              All
            </Radio.Button>
            <Radio.Button style={{ width: '50%', textAlign: 'center' }} value="separate">
              Separate
            </Radio.Button>
          </Radio.Group>
        </Form.Item>

        {props.block.data.wrapper.paddingControl === 'all' && (
          <>
            <Form.Item
              label="Paddings"
              labelAlign="left"
              className="xpeditor-form-item-align-right"
              labelCol={{ span: 10 }}
              wrapperCol={{ span: 14 }}
            >
              <InputNumber
                style={{ width: '100%' }}
                value={parseInt(props.block.data.wrapper.padding || '0px')}
                onChange={(value) => {
                  props.block.data.wrapper.padding = value + 'px'
                  props.updateTree(props.block.path, props.block)
                }}
                size="small"
                step={1}
                min={0}
                parser={(value: string | undefined) => {
                  // if (['▭'].indexOf(value)) {
                  //     value = value.substring(1)
                  // }
                  if (!value) {
                    return 0
                  }
                  return parseInt(value.replace('px', ''))
                }}
                // formatter={value => '▭  ' + value + 'px'}
                formatter={(value) => value + 'px'}
              />
            </Form.Item>
          </>
        )}

        {props.block.data.wrapper.paddingControl === 'separate' && (
          <>
            <Form.Item
              label="Paddings"
              labelAlign="left"
              className="xpeditor-form-item-align-right"
              labelCol={{ span: 10 }}
              wrapperCol={{ span: 14 }}
            >
              <PaddingInputs
                styles={props.block.data.wrapper}
                onChange={(updatedStyles: any) => {
                  props.block.data.wrapper = updatedStyles
                  props.updateTree(props.block.path, props.block)
                }}
              />
            </Form.Item>
          </>
        )}
      </div>
    )
  },

  renderEditor: (props: BlockEditorRendererProps) => {
    const wrapperStyles: any = {}
    const imageStyles: any = {
      width: props.block.data.image.width,
      height: props.block.data.image.height,
      display: 'inline-block'
    }

    wrapperStyles.textAlign = props.block.data.wrapper.align

    if (props.block.data.image.borderControl === 'all') {
      if (
        props.block.data.image.borderStyle !== 'none' &&
        props.block.data.image.borderWidth &&
        props.block.data.image.borderColor
      ) {
        imageStyles.border =
          props.block.data.image.borderWidth +
          ' ' +
          props.block.data.image.borderStyle +
          ' ' +
          props.block.data.image.borderColor
      }
    }

    if (props.block.data.image.width !== '100%') {
      imageStyles.width = props.block.data.image.width
    }

    if (props.block.data.image.height !== 'auto') {
      imageStyles.height = props.block.data.image.height
    }

    if (props.block.data.image.fullWidthOnMobile === true && props.deviceWidth <= MobileWidth) {
      imageStyles.width = '100%'
      imageStyles.height = 'auto'
    }

    if (props.block.data.image.borderRadius && props.block.data.image.borderRadius !== '0px') {
      imageStyles.borderRadius = props.block.data.image.borderRadius
    }

    if (props.block.data.image.borderControl === 'separate') {
      if (
        props.block.data.image.borderTopStyle !== 'none' &&
        props.block.data.image.borderTopWidth &&
        props.block.data.image.borderTopColor
      ) {
        imageStyles.borderTop =
          props.block.data.image.borderTopWidth +
          ' ' +
          props.block.data.image.borderTopStyle +
          ' ' +
          props.block.data.image.borderTopColor
      }

      if (
        props.block.data.image.borderRightStyle !== 'none' &&
        props.block.data.image.borderRightWidth &&
        props.block.data.image.borderRightColor
      ) {
        imageStyles.borderRight =
          props.block.data.image.borderRightWidth +
          ' ' +
          props.block.data.image.borderRightStyle +
          ' ' +
          props.block.data.image.borderRightColor
      }

      if (
        props.block.data.image.borderBottomStyle !== 'none' &&
        props.block.data.image.borderBottomWidth &&
        props.block.data.image.borderBottomColor
      ) {
        imageStyles.borderBottom =
          props.block.data.image.borderBottomWidth +
          ' ' +
          props.block.data.image.borderBottomStyle +
          ' ' +
          props.block.data.image.borderBottomColor
      }

      if (
        props.block.data.image.borderLeftStyle !== 'none' &&
        props.block.data.image.borderLeftWidth &&
        props.block.data.image.borderLeftColor
      ) {
        imageStyles.borderLeft =
          props.block.data.image.borderLeftWidth +
          ' ' +
          props.block.data.image.borderLeftStyle +
          ' ' +
          props.block.data.image.borderLeftColor
      }
    }

    if (props.block.data.wrapper.paddingControl === 'all') {
      if (props.block.data.wrapper.padding && props.block.data.wrapper.padding !== '0px') {
        wrapperStyles.padding = props.block.data.wrapper.padding
      }
    }

    if (props.block.data.wrapper.paddingControl === 'separate') {
      if (props.block.data.wrapper.paddingTop && props.block.data.wrapper.paddingTop !== '0px') {
        wrapperStyles.paddingTop = props.block.data.wrapper.paddingTop
      }
      if (
        props.block.data.wrapper.paddingRight &&
        props.block.data.wrapper.paddingRight !== '0px'
      ) {
        wrapperStyles.paddingRight = props.block.data.wrapper.paddingRight
      }
      if (
        props.block.data.wrapper.paddingBottom &&
        props.block.data.wrapper.paddingBottom !== '0px'
      ) {
        wrapperStyles.paddingBottom = props.block.data.wrapper.paddingBottom
      }
      if (props.block.data.wrapper.paddingLeft && props.block.data.wrapper.paddingLeft !== '0px') {
        wrapperStyles.paddingLeft = props.block.data.wrapper.paddingLeft
      }
    }

    return (
      <div style={wrapperStyles}>
        <img
          style={imageStyles}
          src={props.block.data.image.src}
          alt={props.block.data.image.alt}
        />
      </div>
    )
  },

  renderMenu: (_blockDefinition: BlockDefinitionInterface) => {
    return (
      <div className="xpeditor-ui-block">
        <Space size="middle">
          <ImageIcon size={16} style={{ marginTop: '5px' }} />
          Image
        </Space>
      </div>
    )
  }
}

export default ImageBlockDefinition
