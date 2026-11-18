import { Alert, Button, Drawer, Form, Input, Select, Space, App } from 'antd'
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { debounce } from 'lodash'
import { useParams } from '@tanstack/react-router'
import { useAuth } from '../../contexts/AuthContext'
import { TreeNodeInput, HasLeaf } from './input'
import { useQuery } from '@tanstack/react-query'
import { listsApi } from '../../services/api/list'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPlus } from '@fortawesome/free-solid-svg-icons'
import {
  Segment,
  createSegment,
  updateSegment,
  previewSegment,
  getSegment,
  CreateSegmentRequest,
  UpdateSegmentRequest,
  PreviewSegmentRequest,
  PreviewSegmentResponse,
  TreeNode
} from '../../services/api/segment'
import { TIMEZONE_OPTIONS } from '../../lib/timezones'
import { TableSchemas } from './table_schemas'
import { treeHasRelativeDates } from './relative_dates'
import { isTreeQueryable } from './tree_completeness'
import { useLingui } from '@lingui/react/macro'
import { SegmentCountCard } from './SegmentCountCard'
import { segmentDotColor } from './segmentColor'

const PREVIEW_LIMIT = 100
// Long enough that a burst of typing inside a condition costs a single count query.
const PREVIEW_DEBOUNCE_MS = 600

const ButtonUpsertSegment = (props: {
  segment?: Segment
  btnType?: 'primary' | 'default' | 'dashed' | 'link' | 'text' | undefined
  btnSize?: 'small' | 'middle' | 'large' | undefined
  totalContacts?: number
  onSuccess?: () => void
  children?: React.ReactNode
  // Without Templates read the email conditions name their template by ID, locked.
  canReadTemplates: boolean
}) => {
  const { t } = useLingui()
  const [drawserVisible, setDrawserVisible] = useState(false)

  // but the drawer in a separate component to make sure the
  // form is reset when the drawer is closed
  return (
    <>
      {props.children ? (
        <span onClick={() => setDrawserVisible(!drawserVisible)}>{props.children}</span>
      ) : (
        <Button
          type={props.btnType || 'primary'}
          size={props.btnSize || 'small'}
          ghost
          icon={!props.segment ? <FontAwesomeIcon icon={faPlus} /> : undefined}
          onClick={() => setDrawserVisible(!drawserVisible)}
        >
          {props.segment ? t`Edit segment` : t`Segment`}
        </Button>
      )}
      {drawserVisible && (
        <DrawerSegment
          segment={props.segment}
          totalContacts={props.totalContacts}
          setDrawserVisible={setDrawserVisible}
          onSuccess={props.onSuccess}
          canReadTemplates={props.canReadTemplates}
        />
      )}
    </>
  )
}

// Exported for the Contacts rail, which opens it from its own "+" button.
export const DrawerSegment = (props: {
  segment?: Segment
  /** The conditions a new segment starts with (the Contacts filters); ignored when editing. */
  initialTree?: TreeNode
  totalContacts?: number
  setDrawserVisible: (visible: boolean) => void
  onSuccess?: () => void
  canReadTemplates: boolean
}) => {
  const { t } = useLingui()
  // The App's message, which follows the theme (the static one warned it had no context).
  const { message } = App.useApp()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId' })
  const { workspaces } = useAuth()
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [loadingPreview, setLoadingPreview] = useState(false)
  const [previewResponse, setPreviewResponse] = useState<PreviewSegmentResponse | undefined>()
  const [previewedHash, setPreviewedHash] = useState<string | undefined>() // request behind previewResponse
  const [previewError, setPreviewError] = useState<string | undefined>()
  // Set while a condition is open in its form, so the count can follow it before it is confirmed
  const [draftTree, setDraftTree] = useState<TreeNode | undefined>()
  const [idValidation, setIdValidation] = useState<{
    status: '' | 'validating' | 'error' | 'success'
    message: string
  }>({ status: '', message: '' })

  // Find the current workspace
  const workspace = useMemo(() => {
    if (workspaceId && workspaces.length > 0) {
      return workspaces.find((w) => w.id === workspaceId) || null
    }
    return null
  }, [workspaceId, workspaces])

  // Fetch lists for the current workspace
  const { data: listsData } = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: !!workspaceId
  })

  const lists = listsData?.lists || []

  // Generate segment ID from name (same logic as in onFinish)
  const generateSegmentId = (name: string): string => {
    return name
      .toLowerCase()
      .replace(/[\s-]+/g, '_')
      .replace(/[^a-z0-9_]/g, '')
      .replace(/^_+|_+$/g, '')
      .replace(/_+/g, '_')
  }

  // Debounced function to check if segment ID exists
  const checkIdExists = useMemo(
    () =>
      debounce(async (name: string) => {
        // Skip validation in edit mode or if no workspace
        if (!name || !workspaceId || props.segment) {
          setIdValidation({ status: '', message: '' })
          return
        }

        const id = generateSegmentId(name)
        if (!id) {
          setIdValidation({ status: '', message: '' })
          return
        }

        setIdValidation({ status: 'validating', message: '' })

        try {
          await getSegment({ workspace_id: workspaceId, id })
          // Segment exists (active or deleted) - show error
          setIdValidation({
            status: 'error',
            message: t`A segment with ID "${id}" already exists`
          })
        } catch {
          // Segment not found - ID is available
          setIdValidation({ status: 'success', message: '' })
        }
      }, 500),
    [workspaceId, props.segment, t]
  )

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      checkIdExists.cancel()
    }
  }, [checkIdExists])

  // The committed tree, overridden by the condition currently being edited. A condition only
  // reaches the tree on Confirm, so without the draft the count would ignore what is on screen.
  const watchedTree = Form.useWatch<TreeNode | undefined>('tree', form)
  const effectiveTree = draftTree ?? watchedTree ?? props.segment?.tree

  // Only trees the backend can compile are worth a request; a half-filled condition keeps the
  // last count on screen instead of replacing it with an error.
  const previewHash = useMemo(() => {
    if (!workspaceId || !effectiveTree || !isTreeQueryable(effectiveTree)) return undefined

    const requestData: PreviewSegmentRequest = {
      workspace_id: workspaceId,
      tree: effectiveTree,
      limit: PREVIEW_LIMIT
    }
    return JSON.stringify(requestData)
  }, [workspaceId, effectiveTree])

  const requestSeqRef = useRef(0)
  // The request queued in the debounce or already in flight — what an answer is on its way for,
  // as opposed to previewedHash, which is the answer currently on screen.
  const pendingHashRef = useRef<string | undefined>(undefined)
  const hasRequestedRef = useRef(false)

  const runPreview = useCallback(
    async (tree: TreeNode, hash: string) => {
      if (!workspaceId) return

      const seq = ++requestSeqRef.current
      hasRequestedRef.current = true
      setLoadingPreview(true)

      try {
        const res = await previewSegment({
          workspace_id: workspaceId,
          tree: tree,
          limit: PREVIEW_LIMIT
        })
        // A newer edit was requested in the meantime, that answer is the one that counts
        if (seq !== requestSeqRef.current) return

        setPreviewResponse(res)
        setPreviewedHash(hash)
        setPreviewError(undefined)
      } catch (error) {
        if (seq !== requestSeqRef.current) return

        // No toast here: this runs on every edit, so the failure is reported on the circle itself
        console.error('Preview error:', error)
        setPreviewError(error instanceof Error ? error.message : t`Failed to preview segment`)
      } finally {
        if (seq === requestSeqRef.current) {
          // Only release the marker this run owns; a newer edit may already be queued behind it
          if (pendingHashRef.current === hash) pendingHashRef.current = undefined
          setLoadingPreview(false)
        }
      }
    },
    [workspaceId, t]
  )

  // Held in a ref so the debounce survives re-renders: rebuilding it would cancel the call
  // pending for the edit in progress, and typing re-renders constantly.
  const runPreviewRef = useRef(runPreview)
  runPreviewRef.current = runPreview

  const debouncedPreview = useMemo(
    () =>
      debounce(
        (tree: TreeNode, hash: string) => runPreviewRef.current(tree, hash),
        PREVIEW_DEBOUNCE_MS
      ),
    []
  )

  useEffect(() => () => debouncedPreview.cancel(), [debouncedPreview])

  // Refresh whenever the tree — committed or in progress — settles on something not counted yet
  useEffect(() => {
    if (!previewHash || !effectiveTree) return

    if (previewHash === previewedHash) {
      // The form is back on the count already displayed. Drop what is queued or in flight for the
      // states in between: their answers would replace a count we know matches the form.
      debouncedPreview.cancel()
      requestSeqRef.current++
      pendingHashRef.current = undefined
      setLoadingPreview(false)
      // Any error describes one of those abandoned states, not the count now on screen
      setPreviewError(undefined)
      return
    }

    if (previewHash === pendingHashRef.current) return

    pendingHashRef.current = previewHash
    setPreviewError(undefined)

    // The first count, i.e. opening an existing segment, has nothing to debounce against
    if (!hasRequestedRef.current) {
      runPreviewRef.current(effectiveTree, previewHash)
      return
    }

    debouncedPreview(effectiveTree, previewHash)
    // runPreview is deliberately absent: it changes identity on every render (its `t` dependency
    // does), which would re-run this on every render for no reason.
  }, [previewHash, previewedHash, effectiveTree, debouncedPreview])

  const previewNow = () => {
    if (!previewHash || !effectiveTree) return

    debouncedPreview.cancel()
    pendingHashRef.current = previewHash
    runPreview(effectiveTree, previewHash)
  }

  const initialValues = Object.assign(
    {
      color: 'blue',
      timezone: workspace?.settings.timezone || 'UTC',
      tree: props.initialTree ?? {
        kind: 'branch',
        branch: {
          operator: 'and',
          leaves: []
        }
      }
    },
    props.segment
  )

  const onFinish = async (values: { name: string; color: string; tree: TreeNode; timezone: string }) => {
    if (loading || !workspaceId) return

    // Block submission if ID validation failed (only for create mode)
    if (!props.segment && idValidation.status === 'error') {
      message.error(t`Please choose a different segment name`)
      return
    }

    setLoading(true)

    try {
      if (props.segment) {
        // Update existing segment
        const requestData: UpdateSegmentRequest = {
          workspace_id: workspaceId,
          id: props.segment.id,
          name: values.name,
          color: values.color,
          tree: values.tree,
          timezone: values.timezone
        }
        await updateSegment(requestData)
        message.success(t`The segment has been updated!`)
      } else {
        // Create new segment
        // Generate snake_case ID: lowercase, replace spaces/hyphens with underscores, remove invalid chars
        const generatedId = values.name
          .toLowerCase()
          .replace(/[\s-]+/g, '_') // Replace spaces and hyphens with underscores
          .replace(/[^a-z0-9_]/g, '') // Remove any character that's not a-z, 0-9, or underscore
          .replace(/^_+|_+$/g, '') // Remove leading/trailing underscores
          .replace(/_+/g, '_') // Replace multiple consecutive underscores with single underscore

        const requestData: CreateSegmentRequest = {
          workspace_id: workspaceId,
          id: generatedId,
          name: values.name,
          color: values.color,
          tree: values.tree,
          timezone: values.timezone
        }
        await createSegment(requestData)
        message.success(t`The segment has been created!`)
      }

      form.resetFields()
      setLoading(false)
      props.setDrawserVisible(false)

      // Call onSuccess callback to refresh segments list in parent
      if (props.onSuccess) {
        props.onSuccess()
      }
    } catch (error) {
      console.error('Segment operation error:', error)
      message.error(props.segment ? t`Failed to update segment` : t`Failed to create segment`)
      setLoading(false)
    }
  }
  const hasPreview = previewResponse !== undefined
  // The count on screen no longer answers what the form says: either a request is on its way, or
  // the condition being edited is not complete enough to ask for one yet.
  const isPreviewStale = hasPreview && previewedHash !== previewHash
  // A count is on its way: either the request is out, or it is waiting out the debounce. Both are
  // the same wait as far as the user is concerned, so both get the spinner.
  const isPreviewRefreshing =
    loadingPreview || (!!previewHash && previewHash !== previewedHash && !previewError)
  // Stale with nothing on its way: the form as it stands cannot be counted, so the number is
  // frozen until the condition is finished. A refresh merely waiting its turn is not this.
  const isPreviewBlocked =
    isPreviewStale &&
    !previewHash &&
    !loadingPreview &&
    !previewError &&
    !!effectiveTree &&
    HasLeaf(effectiveTree)

  // Use the table schemas for segmentation
  const schemas = useMemo(() => {
    return {
      contacts: TableSchemas.contacts,
      contact_lists: TableSchemas.contact_lists,
      contact_timeline: TableSchemas.contact_timeline,
      custom_events_goals: TableSchemas.custom_events_goals
    }
  }, [])

  return (
    <Drawer
      title={props.segment ? t`Update segment` : t`New segment`}
      open={true}
      size={'90%'}
      onClose={() => props.setDrawserVisible(false)}
      styles={{ body: { paddingBottom: 80 } }}
      extra={
        <Space>
          <Button loading={loading} onClick={() => props.setDrawserVisible(false)}>
            {t`Cancel`}
          </Button>
          <Button
            loading={loading}
            onClick={() => {
              form.submit()
            }}
            type="primary"
          >
            {t`Confirm`}
          </Button>
        </Space>
      }
    >
      <Form form={form} initialValues={initialValues} layout="vertical" name="groupForm" onFinish={onFinish}>
        <div className="mx-auto max-w-[1100px]">
          <div className="grid grid-cols-[minmax(0,1fr)_260px] items-start gap-6">
            <div>
              <Form.Item
                label={t`Name`}
                name="name"
                rules={[{ required: true, type: 'string', message: t`Please enter a name` }]}
                validateStatus={
                  idValidation.status === 'validating'
                    ? 'validating'
                    : idValidation.status === 'error'
                      ? 'error'
                      : idValidation.status === 'success'
                        ? 'success'
                        : undefined
                }
                help={idValidation.status === 'error' ? idValidation.message : undefined}
                hasFeedback={!props.segment && idValidation.status !== ''}
                style={{ marginBottom: 8 }}
              >
                <Input placeholder={t`e.g. Big spenders`} onChange={(e) => checkIdExists(e.target.value)} />
              </Form.Item>
              <div className="mb-4 flex items-center gap-2">
                <span className="text-xs text-ink-3">{t`Color`}</span>
                <Form.Item noStyle name="color">
                  <ColorSwatches />
                </Form.Item>
              </div>

              <Form.Item
                name="timezone"
                label={t`Timezone used for dates`}
                rules={[{ required: true, type: 'string' }]}
                style={{ marginBottom: 12 }}
              >
                <Select
                  style={{ maxWidth: 320 }}
                  placeholder={t`Select a time zone`}
                  allowClear={false}
                  showSearch={true}
                  filterOption={(input: string, option) => {
                    if (!input || !option) return true
                    const label = option.label || option.value || ''
                    return String(label).toLowerCase().includes(input.toLowerCase())
                  }}
                  optionFilterProp="label"
                  options={TIMEZONE_OPTIONS}
                />
              </Form.Item>

              {/* Alert for segments with relative date filters */}
              <Form.Item noStyle dependencies={['tree', 'timezone']}>
                {() => {
                  const values = form.getFieldsValue()
                  const hasRelativeDates = treeHasRelativeDates(effectiveTree)
                  const timezone = values.timezone || workspace?.settings.timezone || 'UTC'

                  if (!hasRelativeDates) return null
                  return (
                    <Alert
                      type="info"
                      showIcon
                      title={t`This segment uses relative date filters and will be automatically recomputed daily at 5:00 AM (${timezone})`}
                    />
                  )
                }}
              </Form.Item>
            </div>

            <SegmentCountCard
              response={previewResponse}
              totalContacts={props.totalContacts}
              refreshing={isPreviewRefreshing}
              stale={isPreviewStale}
              blocked={isPreviewBlocked}
              error={previewError}
              canCount={!!previewHash}
              onCount={previewNow}
            />
          </div>

          <div className="mt-5 mb-3 border-t border-line-soft pt-4 text-overline font-semibold tracking-wide text-ink-3 uppercase">
            {t`Conditions`}
          </div>

          <Form.Item
            name="tree"
            noStyle
            rules={[
              {
                required: true,
                validator: (_rule, value) =>
                  HasLeaf(value) ? Promise.resolve(undefined) : Promise.reject(new Error(t`A tree is required`))
              }
            ]}
          >
            <TreeNodeInput
              schemas={schemas}
              lists={lists}
              workspaceId={workspaceId}
              canReadTemplates={props.canReadTemplates}
              customFieldLabels={workspace?.settings?.custom_field_labels}
              onDraftTreeChange={setDraftTree}
            />
          </Form.Item>
        </div>
      </Form>
    </Drawer>
  )
}

const SEGMENT_COLORS = [
  'magenta',
  'red',
  'volcano',
  'orange',
  'gold',
  'lime',
  'green',
  'cyan',
  'blue',
  'geekblue',
  'purple',
  'grey'
]

/** The colour of a segment, as a row of dots. Form.Item hands it value and onChange. */
function ColorSwatches({ value, onChange }: { value?: string; onChange?: (color: string) => void }) {
  const { t } = useLingui()
  return (
    <div role="radiogroup" aria-label={t`Color`} className="flex flex-wrap gap-1.5">
      {SEGMENT_COLORS.map((color) => {
        const selected = value === color
        return (
          <button
            key={color}
            type="button"
            role="radio"
            aria-checked={selected}
            aria-label={color}
            title={color}
            onClick={() => onChange?.(color)}
            className={`size-4.5 cursor-pointer rounded-full border-2 border-surface ${
              selected ? 'ring-2 ring-primary-500' : 'ring-1 ring-line'
            }`}
            style={{ backgroundColor: segmentDotColor(color) }}
          />
        )
      })}
    </div>
  )
}

export default ButtonUpsertSegment
