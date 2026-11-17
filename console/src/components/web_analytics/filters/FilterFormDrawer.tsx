import { useMemo, useState } from 'react'
import { App, Button, ConfigProvider, Form, Input, InputNumber, Select, Switch, Tooltip } from 'antd'
import { Tag } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { WebFilter, WebFilterCondition, WebFilterOperation } from '../../../services/api/web_analytics'
import { FullDrawer, Panel, Pill } from '../../layout'
import { SUGGESTED_TAGS } from '../lib/filterCatalog'
import { draftOf, ruleDraftProblems, sameRuleDraft, type FilterDraft } from '../lib/ruleDraft'
import { ConditionsBuilder } from './ConditionsBuilder'
import { OperationsBuilder } from './OperationsBuilder'
import { QuickTestCard, RuleChecks, RuleOrderCard, WhenItAppliesCard } from './RuleEditorCards'
import { TestFilterPanel } from './TestFilterPanel'
import { useWebAnalyticsWriteReason } from './useWebAnalyticsWriteReason'

export type { FilterDraft } from '../lib/ruleDraft'

// The panels space their fields themselves (gap-3): a form item drops the bottom margin antd gives
// stacked labelled fields, and keeps its validation message under the field.
const PANEL_FORM_THEME = { components: { Form: { itemMarginBottom: 0 } } }

export interface FilterFormDrawerProps {
  open: boolean
  /** Absent when creating. */
  filter?: WebFilter
  /** Every saved rule, for the rule order and the priority check. */
  rules: WebFilter[]
  workspaceId: string
  existingTags: string[]
  customDimensionLabels?: Record<string, string>
  saving?: boolean
  /** Without the web analytics write permission: the rule is shown, not changed. */
  readOnly?: boolean
  /** The permissions are still loading: saving waits for them, without saying read only. */
  permissionsLoading?: boolean
  onClose: () => void
  onSubmit: (draft: FilterDraft) => void
}

/** Creates or edits an attribution rule in a full-width drawer. */
export function FilterFormDrawer(props: FilterFormDrawerProps) {
  if (!props.open) return null
  // A fresh form for each rule, so nothing typed for one shows on the next.
  return <RuleEditor key={props.filter?.id ?? 'new'} {...props} />
}

function RuleEditor(props: FilterFormDrawerProps) {
  const { t } = useLingui()
  const { modal } = App.useApp()
  const [form] = Form.useForm<FilterDraft>()
  const {
    filter,
    existingTags,
    rules,
    workspaceId,
    customDimensionLabels,
    saving = false,
    readOnly = false,
    permissionsLoading = false
  } = props
  const editing = Boolean(filter)

  const [baseline] = useState<FilterDraft>(() => draftOf(filter))
  // The watch answers {} until the form is connected, so it is laid over the values the form
  // started from.
  const watched = Form.useWatch([], { form, preserve: true }) as Partial<FilterDraft> | undefined
  const draft: FilterDraft = { ...baseline, ...watched }
  const dirty = !readOnly && !sameRuleDraft(draft, baseline)

  // The Quick test's visit, by field: kept when a condition changes field, and handed to the tester.
  const [quickValues, setQuickValues] = useState<Record<string, string>>({})
  const [testerOpen, setTesterOpen] = useState(false)

  const tagOptions = useMemo(() => {
    const all = new Set([...existingTags, ...SUGGESTED_TAGS])
    return Array.from(all).map((tag) => ({ value: tag, label: tag }))
  }, [existingTags])

  const conditionMessages = { missing_value: t`All conditions must have a value` }
  const operationMessages = {
    none: t`At least one operation is required`,
    missing_value: t`Every "set" operation needs a value`
  }

  const handleSubmit = async () => {
    if (readOnly || permissionsLoading) return
    try {
      const values = await form.validateFields()
      props.onSubmit({
        name: values.name.trim(),
        priority: values.priority,
        tags: values.tags ?? [],
        enabled: values.enabled,
        conditions: values.conditions ?? [],
        operations: values.operations ?? []
      })
    } catch {
      // validateFields already surfaces the field-level errors
    }
  }

  const requestClose = () => {
    // A save under way finishes first: the tab closes the editor once it lands.
    if (saving) return
    if (!dirty) {
      props.onClose()
      return
    }
    modal.confirm({
      title: t`Discard your changes?`,
      content: editing ? t`The changes you made to this rule will be lost.` : t`This rule is not created yet.`,
      okText: t`Discard`,
      okButtonProps: { danger: true },
      cancelText: t`Keep editing`,
      onOk: props.onClose
    })
  }

  const noWrite = useWebAnalyticsWriteReason()
  const priority = draft.priority ?? filter?.priority
  const barTitle = (
    <>
      <span className="truncate text-sm font-semibold text-ink">
        {editing ? draft.name?.trim() || filter?.name : t`New rule`}
      </span>
      {editing && priority !== undefined && priority !== null ? (
        <span className="hidden shrink-0 text-xs text-ink-3 tabular-nums sm:inline">{t`Priority ${priority}`}</span>
      ) : null}
      {!editing ? <Pill tone="primary">{t`Not created`}</Pill> : null}
      {readOnly ? (
        <Pill tone="neutral" dot={false}>
          {t`Read only`}
        </Pill>
      ) : dirty ? (
        <Pill tone="warning">{t`Unsaved changes`}</Pill>
      ) : null}
    </>
  )

  const topActions = readOnly ? (
    <Tooltip title={noWrite}>
      {/* A disabled button gets no pointer events: the span carries the tooltip. */}
      <span>
        <Button type="primary" disabled>
          {t`Save changes`}
        </Button>
      </span>
    </Tooltip>
  ) : editing ? (
    <>
      {/* resetFields restores the values the form started from and clears the errors. */}
      {dirty ? <Button onClick={() => form.resetFields()}>{t`Discard`}</Button> : null}
      <Button type="primary" disabled={!dirty || permissionsLoading} loading={saving} onClick={handleSubmit}>
        {t`Save changes`}
      </Button>
    </>
  ) : (
    <Button type="primary" disabled={permissionsLoading} loading={saving} onClick={handleSubmit}>
      {t`Create rule`}
    </Button>
  )

  const conditions = (draft.conditions ?? []) as WebFilterCondition[]
  const operations = (draft.operations ?? []) as WebFilterOperation[]

  return (
    <>
      <FullDrawer
        open
        onClose={requestClose}
        backLabel={t`Attribution rules`}
        // The rule being edited is not in the address, so a copied link would not reopen it.
        copyLink={false}
        // Selects and pop-confirms use Escape themselves, and closing by accident would lose work.
        keyboard={false}
        ariaLabel={editing ? t`Edit rule` : t`New rule`}
        barTitle={barTitle}
        topActions={topActions}
      >
        <div className="flex flex-col gap-3 min-[1200px]:flex-row min-[1200px]:items-start">
          <div className="flex min-w-0 flex-1 flex-col gap-3">
            <ConfigProvider theme={PANEL_FORM_THEME}>
              <Form
                form={form}
                layout="vertical"
                initialValues={baseline}
                disabled={readOnly}
                // The conditions and operations are checked on submit only (a row is added empty), but
                // once their error shows, each edit checks them again so a fixed list clears it.
                onValuesChange={(changed: Partial<FilterDraft>) => {
                  for (const name of ['conditions', 'operations'] as const) {
                    if (name in changed && form.getFieldError(name).length > 0) {
                      form.validateFields([name]).catch(() => undefined)
                    }
                  }
                }}
              >
                <div className="flex flex-col gap-3">
                  <Panel title={t`Details`} icon={<Tag size={14} aria-hidden="true" />}>
                    <div className="flex flex-col gap-3">
                      <div className="flex flex-col gap-3 md:flex-row md:items-start">
                        <div className="min-w-0 md:flex-1">
                          <Form.Item
                            name="name"
                            label={t`Name`}
                            rules={[{ required: true, whitespace: true, message: t`Name is required` }]}
                          >
                            <Input placeholder={t`e.g. Set channel for Google Ads`} />
                          </Form.Item>
                        </div>
                        <div className="md:w-[200px]">
                          <Form.Item
                            name="priority"
                            label={t`Priority`}
                            rules={[{ required: true, message: t`Priority is required` }]}
                          >
                            <InputNumber min={0} max={1000} style={{ width: '100%' }} />
                          </Form.Item>
                        </div>
                        <Form.Item name="enabled" label={t`Enabled`} valuePropName="checked">
                          <Switch />
                        </Form.Item>
                      </div>
                      <Form.Item name="tags" label={t`Tags`}>
                        <Select mode="tags" options={tagOptions} placeholder={t`Add tags...`} tokenSeparators={[',']} />
                      </Form.Item>
                      <p className="m-0 text-xs text-ink-3">
                        {t`Higher priority rules are evaluated first (0-1000). On the same dimension, the higher priority wins; at equal priority, the rule lower in the list wins.`}
                      </p>
                    </div>
                  </Panel>

                  <div className="grid grid-cols-1 gap-3 min-[1200px]:grid-cols-2 min-[1200px]:items-start">
                    <Panel title={t`Conditions`} extra={<span className="text-xs text-ink-3">{t`All must match`}</span>}>
                      <Form.Item
                        name="conditions"
                        validateTrigger={[]}
                        // No status for the whole list, which would outline every select in it: the
                        // message shows under the list, and each row outlines its own empty value.
                        validateStatus=""
                        rules={[
                          {
                            validator: (_, value: WebFilterCondition[]) => {
                              const problem = ruleDraftProblems({ conditions: value, operations: [] }).conditions
                              return problem ? Promise.reject(new Error(conditionMessages[problem])) : Promise.resolve()
                            }
                          }
                        ]}
                      >
                        <ConditionsBuilder />
                      </Form.Item>
                    </Panel>

                    <Panel
                      title={t`Operations`}
                      extra={<span className="text-xs text-ink-3">{t`Run when the rule matches`}</span>}
                    >
                      <Form.Item
                        name="operations"
                        validateTrigger={[]}
                        validateStatus=""
                        rules={[
                          {
                            validator: (_, value: WebFilterOperation[]) => {
                              const problem = ruleDraftProblems({ conditions: [], operations: value }).operations
                              return problem ? Promise.reject(new Error(operationMessages[problem])) : Promise.resolve()
                            }
                          }
                        ]}
                      >
                        <OperationsBuilder customDimensionLabels={customDimensionLabels} />
                      </Form.Item>
                    </Panel>
                  </div>
                </div>
              </Form>
            </ConfigProvider>

            <RuleOrderCard
              rules={rules}
              draft={draft}
              editingId={filter?.id}
              customDimensionLabels={customDimensionLabels}
            />
          </div>

          {/* Outside the form: the Quick test stays usable when the rule is read only. */}
          <aside className="flex shrink-0 flex-col gap-3 min-[1200px]:sticky min-[1200px]:top-16 min-[1200px]:w-aside">
            <RuleChecks
              rules={rules}
              draft={draft}
              editingId={filter?.id}
              customDimensionLabels={customDimensionLabels}
              messages={{ ...conditionMessages, ...operationMessages }}
              onShowField={(path) => form.scrollToField(path, { block: 'center', behavior: 'smooth' })}
            />
            <QuickTestCard
              conditions={conditions}
              operations={operations}
              values={quickValues}
              onChange={setQuickValues}
              customDimensionLabels={customDimensionLabels}
              onOpenTester={() => setTesterOpen(true)}
            />
            <WhenItAppliesCard workspaceId={workspaceId} />
          </aside>
        </div>
      </FullDrawer>

      {/* A sibling, not a child: its keystrokes never reach the editor's drawer. */}
      <TestFilterPanel
        open={testerOpen}
        onClose={() => setTesterOpen(false)}
        mode="single"
        ruleName={draft.name}
        backLabel={editing ? t`Edit rule` : t`New rule`}
        conditions={conditions}
        operations={operations}
        customDimensionLabels={customDimensionLabels}
        initialValues={quickValues}
      />
    </>
  )
}
