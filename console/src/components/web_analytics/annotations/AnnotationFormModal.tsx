import { useEffect } from 'react'
import { DatePicker, Form, Input, Modal, Select, TimePicker } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { Dayjs } from 'dayjs'
// The plugin-extended instance: the bare package has no dayjs.tz.
import dayjs from '../../../lib/dayjs'
import { TIMEZONE_OPTIONS } from '../../../lib/timezones'
import { Annotation } from '../../../services/api/annotation'

export const ANNOTATION_DEFAULT_COLOR = '#3b82f6'

const PRESET_COLORS = [
  '#22c55e', // green — launches and other good news
  '#ef4444', // red — incidents
  '#f59e0b', // amber — warnings
  ANNOTATION_DEFAULT_COLOR, // blue — informational
  // purple — campaigns. Kept in step with domain.AnnotationBroadcastColor: it is
  // the colour every broadcast annotation is written with, so leaving it out of
  // the row meant opening one for editing showed no swatch selected and the
  // first click on any other colour lost it for good.
  '#7763f1'
]

const MAX_TITLE_LENGTH = 100
const MAX_DESCRIPTION_LENGTH = 500

/** What the modal hands back; the caller adds the workspace and the id. */
export interface AnnotationDraft {
  annotated_at: string
  timezone: string
  title: string
  description?: string
  color: string
}

interface AnnotationFormValues {
  date: Dayjs
  time: Dayjs
  timezone: string
  title: string
  description?: string
  color: string
}

/**
 * A Form-controlled swatch row: antd injects value/onChange, which keeps the
 * colour out of component state and out of the prefill effect.
 */
function ColorSwatches(props: { value?: string; onChange?: (color: string) => void }) {
  return (
    <div className="flex gap-2">
      {PRESET_COLORS.map((preset) => (
        <button
          key={preset}
          type="button"
          aria-label={preset}
          onClick={() => props.onChange?.(preset)}
          className={`h-6 w-6 shrink-0 cursor-pointer rounded-full transition-all ${
            props.value === preset
              ? 'outline outline-1 outline-offset-2 outline-[var(--ant-color-primary)]'
              : 'hover:scale-105'
          }`}
          style={{ backgroundColor: preset }}
        />
      ))}
    </div>
  )
}

export interface AnnotationFormModalProps {
  open: boolean
  /** Absent means "create". */
  annotation?: Annotation
  /** Timezone the form starts on, i.e. the one the charts are drawn in. */
  defaultTimezone: string
  saving?: boolean
  onClose: () => void
  onSubmit: (draft: AnnotationDraft) => void
}

export function AnnotationFormModal(props: AnnotationFormModalProps) {
  const { t } = useLingui()
  const { open, annotation, defaultTimezone, saving, onClose, onSubmit } = props
  const [form] = Form.useForm<AnnotationFormValues>()

  useEffect(() => {
    if (!open) return

    if (annotation) {
      // The stored timezone is what the operator typed in, so replaying the
      // instant through it reopens the form showing the same wall clock —
      // a 9am Tokyo annotation stays 9am for a reader sitting in Paris.
      const wallClock = dayjs(annotation.annotated_at).tz(annotation.timezone || defaultTimezone)
      form.setFieldsValue({
        date: wallClock,
        time: wallClock,
        timezone: annotation.timezone || defaultTimezone,
        title: annotation.title,
        description: annotation.description,
        color: annotation.color || ANNOTATION_DEFAULT_COLOR
      })
      return
    }

    form.resetFields()
    form.setFieldsValue({
      date: dayjs().tz(defaultTimezone),
      time: dayjs('12:00', 'HH:mm'),
      timezone: defaultTimezone,
      color: ANNOTATION_DEFAULT_COLOR
    })
  }, [open, annotation, defaultTimezone, form])

  const handleOk = () => {
    form.validateFields().then((values) => {
      // The two pickers hold a wall clock and nothing more: the instant only
      // exists once the selected timezone is applied to them.
      const wallClock = `${values.date.format('YYYY-MM-DD')} ${values.time.format('HH:mm')}`
      onSubmit({
        annotated_at: dayjs.tz(wallClock, values.timezone).toISOString(),
        timezone: values.timezone,
        title: values.title,
        description: values.description || undefined,
        color: values.color || ANNOTATION_DEFAULT_COLOR
      })
    })
  }

  return (
    <Modal
      title={annotation ? t`Edit annotation` : t`Add annotation`}
      open={open}
      onCancel={onClose}
      onOk={handleOk}
      confirmLoading={saving}
      okText={annotation ? t`Save` : t`Add`}
      // Wide enough that date, time and the five swatches share one line: at the
      // default 520 the colour row wrapped under the pickers.
      width={620}
    >
      <Form form={form} layout="vertical" className="mt-4">
        <div className="flex flex-wrap items-end gap-4">
          <Form.Item
            name="date"
            label={t`Date`}
            rules={[{ required: true, message: t`Date is required` }]}
            className="min-w-[160px]"
          >
            <DatePicker className="w-full" />
          </Form.Item>

          <Form.Item
            name="time"
            label={t`Time`}
            rules={[{ required: true, message: t`Time is required` }]}
          >
            <TimePicker format="HH:mm" />
          </Form.Item>

          {/* shrink-0 so the swatches keep their size and stay beside the
              pickers; below md the row still breaks and this goes full width. */}
          <Form.Item name="color" label={t`Color`} className="w-full shrink-0 md:w-auto">
            <ColorSwatches />
          </Form.Item>
        </div>

        <Form.Item
          name="timezone"
          label={t`Timezone`}
          rules={[{ required: true, message: t`Timezone is required` }]}
        >
          <Select
            showSearch
            optionFilterProp="label"
            placeholder={t`Select timezone`}
            options={TIMEZONE_OPTIONS}
          />
        </Form.Item>

        <Form.Item
          name="title"
          label={t`Title`}
          rules={[
            { required: true, message: t`Title is required` },
            { max: MAX_TITLE_LENGTH, message: t`Title must be ${MAX_TITLE_LENGTH} characters or less` }
          ]}
        >
          <Input placeholder={t`e.g. Product launch`} maxLength={MAX_TITLE_LENGTH} />
        </Form.Item>

        <Form.Item
          name="description"
          label={t`Description`}
          rules={[
            { max: MAX_DESCRIPTION_LENGTH, message: t`Description must be ${MAX_DESCRIPTION_LENGTH} characters or less` }
          ]}
        >
          <Input.TextArea
            placeholder={t`Additional context about this event`}
            maxLength={MAX_DESCRIPTION_LENGTH}
            rows={3}
          />
        </Form.Item>
      </Form>
    </Modal>
  )
}
